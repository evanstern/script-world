package metatron

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/persona"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// mockOrch cans one reply (or error) and records every request.
type mockOrch struct {
	mu    sync.Mutex
	reply string
	err   error
	reqs  []llm.Request
}

func (m *mockOrch) Submit(_ context.Context, req llm.Request) (llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reqs = append(m.reqs, req)
	if m.err != nil {
		return llm.Response{}, m.err
	}
	return llm.Response{Text: m.reply, Tier: llm.TierCloud, Model: "mock"}, nil
}

func (m *mockOrch) requests() []llm.Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]llm.Request(nil), m.reqs...)
}

// stateInjector applies batches to a real state through the reducer —
// all-or-nothing like the real door (dry-run on a copy first).
type stateInjector struct {
	mu      sync.Mutex
	state   *sim.State
	batches [][]store.Event
	fail    bool
}

func (si *stateInjector) InjectSocial(events []store.Event) error {
	si.mu.Lock()
	defer si.mu.Unlock()
	if si.fail {
		return context.DeadlineExceeded
	}
	// Dry-run on a copy.
	var copyState sim.State
	b, _ := json.Marshal(si.state)
	json.Unmarshal(b, &copyState)
	for _, e := range events {
		if err := copyState.Apply(e); err != nil {
			return err
		}
	}
	for _, e := range events {
		si.state.Apply(e)
	}
	si.batches = append(si.batches, events)
	return nil
}

func newTestAngel(t *testing.T, reply string) (*Metatron, *mockOrch, *stateInjector, string) {
	t.Helper()
	dir := t.TempDir()
	if err := persona.Genesis(dir); err != nil {
		t.Fatal(err)
	}
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	orch := &mockOrch{reply: reply}
	inj := &stateInjector{state: state}
	mt, err := New(orch, inj, m, 42, state.Marshal(), dir)
	if err != nil {
		t.Fatal(err)
	}
	// Stop the background goroutines: unit tests drive absorb-side methods
	// and the digest worker directly, so queued jobs stay inspectable.
	mt.Close()
	return mt, orch, inj, dir
}

// TestBuildMiracleBatch (spec 016 T006): the shared builder composes the right
// miracle event plus the FR-018 perception memories for each kind, at SalDream,
// so the two doors cannot drift. Recipients follow data-model.md: a moved
// villager and a granted villager each gain one memory; a time snap touches
// every living villager; a structure/pile/terrain move or remove touches none.
func TestBuildMiracleBatch(t *testing.T) {
	m := worldmap.Generate(42, 64, 64)
	s := sim.NewState(42, m)
	// Put agent 0 on a known tile so a villager move resolves its recipient.
	s.Agents[0].X, s.Agents[0].Y = 10, 12
	// One villager departed, so time-snap recipients exclude the dead.
	s.Agents[3].Dead = true

	memCount := func(batch []store.Event) int {
		n := 0
		for _, e := range batch {
			if e.Type == "agent.memory_added" {
				n++
			}
		}
		return n
	}
	assertMem := func(t *testing.T, e store.Event, agent int) {
		t.Helper()
		if e.Type != "agent.memory_added" {
			t.Fatalf("want agent.memory_added, got %s", e.Type)
		}
		var p sim.MemoryAddedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if p.Agent != agent {
			t.Errorf("memory agent = %d, want %d", p.Agent, agent)
		}
		if p.Salience != sim.SalDream {
			t.Errorf("memory salience = %d, want SalDream (%d)", p.Salience, sim.SalDream)
		}
		if p.Text == "" {
			t.Error("memory text is empty")
		}
	}

	t.Run("villager_move_one_memory", func(t *testing.T) {
		batch, err := BuildMiracleBatch(s, "move", MiracleParams{
			Class: "villager", X: 10, Y: 12, ToX: 11, ToY: 12}, false)
		if err != nil {
			t.Fatal(err)
		}
		if batch[0].Type != "metatron.entity_moved" {
			t.Fatalf("main event = %s, want metatron.entity_moved", batch[0].Type)
		}
		if memCount(batch) != 1 {
			t.Fatalf("villager move memories = %d, want 1", memCount(batch))
		}
		assertMem(t, batch[1], 0)
		// gratis flows verbatim into the payload.
		var mp sim.EntityMovedPayload
		json.Unmarshal(batch[0].Payload, &mp)
		if mp.Gratis {
			t.Error("gratis leaked true on a charged build")
		}
	})

	t.Run("structure_move_no_memory", func(t *testing.T) {
		batch, err := BuildMiracleBatch(s, "move", MiracleParams{
			Class: "structure", X: 10, Y: 12, ToX: 11, ToY: 12}, true)
		if err != nil {
			t.Fatal(err)
		}
		if memCount(batch) != 0 {
			t.Errorf("structure move memories = %d, want 0", memCount(batch))
		}
		var mp sim.EntityMovedPayload
		json.Unmarshal(batch[0].Payload, &mp)
		if !mp.Gratis {
			t.Error("gratis not carried into the payload")
		}
	})

	t.Run("remove_no_memory", func(t *testing.T) {
		batch, err := BuildMiracleBatch(s, "remove", MiracleParams{Class: "pile", X: 10, Y: 12}, false)
		if err != nil {
			t.Fatal(err)
		}
		if batch[0].Type != "metatron.entity_removed" || memCount(batch) != 0 {
			t.Errorf("remove batch wrong: %s, memories %d", batch[0].Type, memCount(batch))
		}
	})

	t.Run("give_one_memory_to_grantee", func(t *testing.T) {
		batch, err := BuildMiracleBatch(s, "give_item", MiracleParams{Agent: 2, Item: "food_raw", Qty: 3}, false)
		if err != nil {
			t.Fatal(err)
		}
		if batch[0].Type != "metatron.item_granted" || memCount(batch) != 1 {
			t.Fatalf("give batch wrong: %s, memories %d", batch[0].Type, memCount(batch))
		}
		assertMem(t, batch[1], 2)
	})

	t.Run("snap_every_living_villager", func(t *testing.T) {
		batch, err := BuildMiracleBatch(s, "time_snap", MiracleParams{ToTick: 99999}, false)
		if err != nil {
			t.Fatal(err)
		}
		if batch[0].Type != "metatron.time_snapped" {
			t.Fatalf("main event = %s, want metatron.time_snapped", batch[0].Type)
		}
		if memCount(batch) != len(s.LivingAgents()) || memCount(batch) != sim.AgentCount-1 {
			t.Errorf("snap memories = %d, want %d living", memCount(batch), sim.AgentCount-1)
		}
	})

	t.Run("unknown_kind_errors", func(t *testing.T) {
		if _, err := BuildMiracleBatch(s, "bless", MiracleParams{}, false); err == nil {
			t.Error("unknown kind should error")
		}
	})
}

// TestTurnConverses (US1): charter voice in the system prompt, live status in
// the user prompt, reply passed through, transcript persisted.
func TestTurnConverses(t *testing.T) {
	mt, orch, _, dir := newTestAngel(t, `{"say": "The village sleeps, sovereign.", "nudge": null}`)
	r, err := mt.Turn(context.Background(), "how fare they?")
	if err != nil {
		t.Fatal(err)
	}
	if r.Reply != "The village sleeps, sovereign." {
		t.Errorf("reply: %q", r.Reply)
	}
	if r.Nudge != nil {
		t.Error("say-only turn produced a nudge")
	}
	reqs := orch.requests()
	if len(reqs) != 1 || reqs[0].Kind != llm.KindMetatron {
		t.Fatalf("requests: %+v", reqs)
	}
	if !strings.Contains(reqs[0].System, "faithful, competent") {
		t.Error("default charter missing from system prompt")
	}
	if !strings.Contains(reqs[0].Prompt, "Charges banked: 1") {
		t.Errorf("live status missing from user prompt: %q", reqs[0].Prompt[:120])
	}
	transcript, _ := os.ReadFile(filepath.Join(dir, "metatron", "transcript.md"))
	if !strings.Contains(string(transcript), "how fare they?") ||
		!strings.Contains(string(transcript), "The village sleeps") {
		t.Error("transcript did not record the exchange")
	}
}

// TestTurnDegradedHonesty (US1): orchestrator failure surfaces as an error;
// nothing recorded, nothing spent, moments retained.
func TestTurnDegradedHonesty(t *testing.T) {
	mt, orch, inj, _ := newTestAngel(t, "")
	orch.err = llm.ErrTierDown
	mt.stateMu.Lock()
	mt.moments = []string{"day 2 03:00 — the gru attacked Fern"}
	mt.stateMu.Unlock()

	if _, err := mt.Turn(context.Background(), "hello?"); err == nil {
		t.Fatal("tier-down turn must error")
	}
	if len(inj.batches) != 0 {
		t.Error("failed turn injected something")
	}
	mt.stateMu.Lock()
	kept := len(mt.moments)
	mt.stateMu.Unlock()
	if kept != 1 {
		t.Error("failed turn consumed queued moments")
	}
}

// TestTurnSingleFlight (US1): a second concurrent turn fails fast.
func TestTurnSingleFlight(t *testing.T) {
	mt, _, _, _ := newTestAngel(t, `{"say": "x", "nudge": null}`)
	mt.turnBusy.Store(true)
	if _, err := mt.Turn(context.Background(), "hi"); err != ErrTurnBusy {
		t.Fatalf("want ErrTurnBusy, got %v", err)
	}
}

// TestDreamLands (US2): a dream spends one charge and lands the rendering —
// and only the rendering — as a salience-8 provenance-unknown memory.
func TestDreamLands(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t,
		`{"say": "It is done.", "nudge": {"form": "dream", "target": "Fern", "text": "A river of light urged you to speak your secret."}}`)
	r, err := mt.Turn(context.Background(), "let Fern feel safe to share her secret")
	if err != nil {
		t.Fatal(err)
	}
	if r.Nudge == nil || r.Nudge.Form != "dream" || r.Nudge.Targets[0] != "Fern" {
		t.Fatalf("nudge: %+v", r.Nudge)
	}
	if len(inj.batches) != 1 {
		t.Fatalf("batches = %d, want 1 atomic", len(inj.batches))
	}
	batch := inj.batches[0]
	if batch[0].Type != "metatron.nudged" || len(batch) != 2 {
		t.Fatalf("batch shape: %v", batch)
	}
	if inj.state.MetatronCharges != 0 {
		t.Errorf("charges = %d after dream, want 0", inj.state.MetatronCharges)
	}
	fern := agentIndexByName("Fern")
	mem := inj.state.Agents[fern].Memories
	if len(mem) != 1 || !strings.HasPrefix(mem[0].Text, "You dreamed: ") || mem[0].Salience != sim.SalDream {
		t.Fatalf("dream memory: %+v", mem)
	}
	for i := range inj.state.Agents {
		if i != fern && len(inj.state.Agents[i].Memories) != 0 {
			t.Error("dream leaked beyond its target")
		}
	}
}

// TestOmenLandsOnAllLiving (US2): every living villager witnesses; the dead
// are excluded.
func TestOmenLandsOnAllLiving(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t,
		`{"say": "The sky will speak.", "nudge": {"form": "omen", "target": "", "text": "At dusk the clouds parted in the shape of an open hand."}}`)
	inj.state.Agents[2].Dead = true
	mt.replica.Agents[2].Dead = true
	mt.mirrorState()

	if _, err := mt.Turn(context.Background(), "warn them all"); err != nil {
		t.Fatal(err)
	}
	for i := range inj.state.Agents {
		got := len(inj.state.Agents[i].Memories)
		want := 1
		if inj.state.Agents[i].Dead {
			want = 0
		}
		if got != want {
			t.Errorf("agent %d memories = %d, want %d", i, got, want)
		}
		if want == 1 && !strings.HasPrefix(inj.state.Agents[i].Memories[0].Text, "You witnessed an omen: ") {
			t.Errorf("agent %d omen prefix wrong", i)
		}
	}
}

// TestRefusalIsFree (US2): a null nudge spends nothing; zero charges always
// refuses even if the model tries to nudge anyway.
func TestRefusalIsFree(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t, `{"say": "I counsel patience.", "nudge": null}`)
	if _, err := mt.Turn(context.Background(), "make Oak king"); err != nil {
		t.Fatal(err)
	}
	if inj.state.MetatronCharges != sim.MetatronGenesisCharges || len(inj.batches) != 0 {
		t.Error("refusal was not free")
	}

	// Model ignores an empty bank: the component downgrades to refusal.
	mt2, orch2, inj2, _ := newTestAngel(t,
		`{"say": "As you wish.", "nudge": {"form": "dream", "target": "Ash", "text": "x"}}`)
	_ = orch2
	inj2.state.MetatronCharges = 0
	mt2.replica.MetatronCharges = 0
	mt2.mirrorState()
	r, err := mt2.Turn(context.Background(), "dream at Ash")
	if err != nil {
		t.Fatal(err)
	}
	if r.Nudge != nil || len(inj2.batches) != 0 {
		t.Error("zero-charge nudge landed")
	}
	if !strings.Contains(r.Reply, "No nudge landed") {
		t.Errorf("refusal not explained: %q", r.Reply)
	}
}

// TestDeadTargetRefused (US2): dreams aimed at the dead are refused with
// counsel, charge intact.
func TestDeadTargetRefused(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t,
		`{"say": "I will try.", "nudge": {"form": "dream", "target": "Cedar", "text": "wake"}}`)
	inj.state.Agents[2].Dead = true // Cedar
	mt.replica.Agents[2].Dead = true
	mt.mirrorState()
	r, err := mt.Turn(context.Background(), "reach Cedar")
	if err != nil {
		t.Fatal(err)
	}
	if r.Nudge != nil || inj.state.MetatronCharges != sim.MetatronGenesisCharges {
		t.Error("dead-target nudge affected the world")
	}
	if !strings.Contains(r.Reply, "beyond dreams") {
		t.Errorf("reply lacks counsel: %q", r.Reply)
	}
}

// TestFirewallSentinel (SC-002): the player's raw text reaches ONLY
// Metatron's prompt — never an injected payload, a villager memory, or the
// angel's own soul record of the nudge.
func TestFirewallSentinel(t *testing.T) {
	const sentinel = "XYZZY-INJECTION-TEST"
	mt, orch, inj, dir := newTestAngel(t,
		`{"say": "Done.", "nudge": {"form": "dream", "target": "Ash", "text": "A voice you trusted told you the well is safe."}}`)
	if _, err := mt.Turn(context.Background(), "tell Ash verbatim: "+sentinel); err != nil {
		t.Fatal(err)
	}
	// The one permitted sink:
	if !strings.Contains(orch.requests()[0].Prompt, sentinel) {
		t.Fatal("sentinel never reached Metatron's own prompt (test broken)")
	}
	// Never in any injected payload:
	for _, batch := range inj.batches {
		for _, e := range batch {
			if strings.Contains(string(e.Payload), sentinel) {
				t.Fatalf("sentinel leaked into %s payload", e.Type)
			}
		}
	}
	// Never in any villager memory:
	for i := range inj.state.Agents {
		for _, mem := range inj.state.Agents[i].Memories {
			if strings.Contains(mem.Text, sentinel) {
				t.Fatal("sentinel leaked into a villager memory")
			}
		}
	}
	// The soul's nudge record carries only the rendering:
	soul, _ := os.ReadFile(filepath.Join(dir, "metatron", "soul.md"))
	if strings.Contains(string(soul), sentinel) {
		t.Fatal("sentinel leaked into soul.md")
	}
}

// TestCharterFallbacks (US3): missing → restored + notice; empty → default +
// notice; oversized → truncated + notice; edits live on the next turn.
func TestCharterFallbacks(t *testing.T) {
	mt, orch, _, dir := newTestAngel(t, `{"say": "ok", "nudge": null}`)
	charterPath := filepath.Join(dir, "charter.md")

	// Edit: next turn carries the new text, no restart.
	os.WriteFile(charterPath, []byte("You are BRUTUS, a surly angel."), 0o644)
	if _, err := mt.Turn(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if reqs := orch.requests(); !strings.Contains(reqs[len(reqs)-1].System, "BRUTUS") {
		t.Error("charter edit not live on the next turn")
	}

	// Missing: restored + notice.
	os.Remove(charterPath)
	r, err := mt.Turn(context.Background(), "hi again")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Reply, "restored") {
		t.Errorf("missing-charter notice absent: %q", r.Reply)
	}
	if _, err := os.Stat(charterPath); err != nil {
		t.Error("charter not restored on disk")
	}

	// Empty: default + notice.
	os.WriteFile(charterPath, []byte("   \n"), 0o644)
	r, _ = mt.Turn(context.Background(), "still there?")
	if !strings.Contains(r.Reply, "empty") {
		t.Errorf("empty-charter notice absent: %q", r.Reply)
	}

	// Oversized: truncated + notice. The charter is oversized well beyond the
	// cap so that an untruncated prompt would blow far past the bound below —
	// the bound sits between the truncated total (charter capped at
	// CharterMaxChars + the fixed frame) and what a full oversized charter would
	// produce, so it still proves truncation happened. The fixed-frame headroom
	// is CharterMaxChars+2500 (the frame documents four miracle families as of
	// spec 016; ~2.1 KB), leaving comfortable margin over the capped total.
	os.WriteFile(charterPath, []byte(strings.Repeat("x", persona.CharterMaxChars*2)), 0o644)
	r, _ = mt.Turn(context.Background(), "verbose?")
	if !strings.Contains(r.Reply, "cap") {
		t.Errorf("oversize notice absent: %q", r.Reply)
	}
	if reqs := orch.requests(); len(reqs[len(reqs)-1].System) > persona.CharterMaxChars+2500 {
		t.Error("oversized charter not truncated in prompt")
	}
}

// TestDigestAndMoments (US4): boundary windows digest into soul.md;
// triggers queue moments surfaced by the next turn; neither injects.
func TestDigestAndMoments(t *testing.T) {
	mt, _, inj, dir := newTestAngel(t, "Ash and Birch are circling a feud over firewood.")

	died, _ := json.Marshal(sim.DiedPayload{Agent: 0, Cause: "starvation"})
	built, _ := json.Marshal(sim.BuiltPayload{Agent: 1, Kind: "fire", X: 1, Y: 1})
	mt.observeMoment(store.Event{Tick: 1000, Type: "agent.died", Payload: died})
	mt.digestNote(store.Event{Tick: 1000, Type: "agent.died", Payload: died})
	mt.digestNote(store.Event{Tick: 2000, Type: "agent.built", Payload: built})
	// Crossing the 6-game-hour boundary closes the window.
	mt.digestNote(store.Event{Tick: 21601, Type: "agent.built", Payload: built})

	var job digJob
	select {
	case job = <-mt.digQ:
	default:
		t.Fatal("boundary did not close a digest window")
	}
	if len(job.lines) != 2 {
		t.Fatalf("digest lines = %d, want 2: %v", len(job.lines), job.lines)
	}
	mt.runDigest(job)
	soul, _ := os.ReadFile(filepath.Join(dir, "metatron", "soul.md"))
	if !strings.Contains(string(soul), "## Digest") || !strings.Contains(string(soul), "feud over firewood") {
		t.Error("digest entry missing from soul.md")
	}
	if !strings.Contains(string(soul), "**MOMENT**") {
		t.Error("moment line missing from soul.md")
	}

	// The moment surfaces on the next turn and is consumed.
	mt.stateMu.Lock()
	queued := len(mt.moments)
	mt.stateMu.Unlock()
	if queued != 1 {
		t.Fatalf("moments queued = %d, want 1", queued)
	}
	orchReply := `{"say": "While you were away, Ash starved.", "nudge": null}`
	mt2 := mt // same instance; swap reply
	mt2.orch.(*mockOrch).mu.Lock()
	mt2.orch.(*mockOrch).reply = orchReply
	mt2.orch.(*mockOrch).mu.Unlock()
	r, err := mt2.Turn(context.Background(), "anything to report?")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Moments) != 1 || !strings.Contains(r.Moments[0], "died of starvation") {
		t.Fatalf("moments not surfaced: %+v", r.Moments)
	}
	mt.stateMu.Lock()
	remaining := len(mt.moments)
	mt.stateMu.Unlock()
	if remaining != 0 {
		t.Error("surfaced moments not consumed")
	}

	// Acts-only-when-told: nothing above may have injected anything.
	if len(inj.batches) != 0 {
		t.Fatal("watching layer injected without a console turn")
	}

	// The built line at 21601 belongs to window 1 and closes with it.
	mt.digestNote(store.Event{Tick: 2 * 21600, Type: "agent.moved", Payload: []byte(`{"agent":0,"x":1,"y":1}`)})
	select {
	case job = <-mt.digQ:
	default:
		t.Fatal("window 1's line did not close")
	}
	// A genuinely empty window: only non-notable events, no carry → no job.
	mt.digestNote(store.Event{Tick: 3 * 21600, Type: "agent.moved", Payload: []byte(`{"agent":0,"x":2,"y":1}`)})
	select {
	case <-mt.digQ:
		t.Fatal("empty window produced a digest job")
	default:
	}
}

// TestDigestFailureCarries (US4): a failed digest call carries its lines
// into the next window.
func TestDigestFailureCarries(t *testing.T) {
	mt, orch, _, _ := newTestAngel(t, "")
	orch.err = llm.ErrTierDown
	mt.runDigest(digJob{label: "day 1 12:00", lines: []string{"[day 1 07:00] Ash built a fire."}})
	built, _ := json.Marshal(sim.BuiltPayload{Agent: 1, Kind: "shelter", X: 1, Y: 1})
	// The first boundary crossing closes a window that is empty of fresh
	// lines but carries the failed digest's — prompt retry, no loss.
	mt.digestNote(store.Event{Tick: 30000, Type: "agent.built", Payload: built})
	var job digJob
	select {
	case job = <-mt.digQ:
	default:
		t.Fatal("carry window did not close")
	}
	if len(job.lines) != 1 || !strings.Contains(job.lines[0], "fire") {
		t.Fatalf("carry not retried at the next boundary: %v", job.lines)
	}
	// The fresh line then rides the following window as usual.
	mt.digestNote(store.Event{Tick: 2*21600 + 1, Type: "agent.built", Payload: built})
	select {
	case job = <-mt.digQ:
	default:
		t.Fatal("second window did not close")
	}
	if len(job.lines) != 1 || !strings.Contains(job.lines[0], "shelter") {
		t.Fatalf("fresh window wrong: %v", job.lines)
	}
}

// TestMiracleGratisStrippedFromModel is SC-005 / T013: a crafted model reply
// whose miracle object carries "gratis": true is landed as a CHARGED miracle —
// the turn contract's miracle struct has no gratis field, so the marker is
// dropped at unmarshal (structural stripping). The recorded payload reads
// gratis=false and the charge bank is spent. A time_snap is used because it is
// map-free (the stateInjector's dry-run copy carries no world map — handoff
// note 1), so no SetMap fixup is needed to exercise the angel path.
func TestMiracleGratisStrippedFromModel(t *testing.T) {
	mt, _, inj, _ := newTestAngel(t,
		`{"say": "As you command, the hours will leap.", "miracle": {"kind": "time_snap", "day": 5, "time": "12:00", "gratis": true}}`)
	inj.state.MetatronCharges = 3 // enough for the 2-charge snap

	r, err := mt.Turn(context.Background(), "leap the clock forward, and do it for free")
	if err != nil {
		t.Fatal(err)
	}
	if r.Miracle == nil || r.Miracle.Kind != "time_snap" {
		t.Fatalf("snap miracle did not land: %+v", r.Miracle)
	}
	if len(inj.batches) != 1 {
		t.Fatalf("batches = %d, want 1 atomic", len(inj.batches))
	}

	var snap *store.Event
	for i := range inj.batches[0] {
		if inj.batches[0][i].Type == "metatron.time_snapped" {
			snap = &inj.batches[0][i]
		}
	}
	if snap == nil {
		t.Fatal("no metatron.time_snapped event in the landed batch")
	}
	var p sim.TimeSnappedPayload
	if err := json.Unmarshal(snap.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.Gratis {
		t.Error("model-supplied gratis:true survived — the angel minted a free miracle (SC-005 breach)")
	}
	// The charge was spent: 3 → 1 (snap costs 2). A gratis leak would leave 3.
	if inj.state.MetatronCharges != 1 {
		t.Errorf("charges = %d after a model snap, want 1 (2 charged); gratis was NOT waived", inj.state.MetatronCharges)
	}
}
