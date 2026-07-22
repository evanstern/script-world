package sim

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

func socialEvent(t *testing.T, tick int64, typ string, payload any) store.Event {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return store.Event{Tick: tick, Type: typ, Payload: b}
}

// TestEdgeRules is AC#1's edge half: deterministic rules move directed
// edges, clamped, replayable.
func TestEdgeRules(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	if err := s.Apply(socialEvent(t, 100, "social.relation_changed",
		RelationChangedPayload{A: 0, B: 1, TrustDelta: 30, AffectionDelta: 20, Reason: "test"})); err != nil {
		t.Fatal(err)
	}
	r := s.RelationBetween(0, 1)
	if r.Trust != 30 || r.Affection != 20 {
		t.Errorf("edge = %+v", r)
	}
	if back := s.RelationBetween(1, 0); back.Trust != 0 {
		t.Error("edges must be directed")
	}

	// Clamping.
	for i := 0; i < 100; i++ {
		s.Apply(socialEvent(t, 100, "social.relation_changed",
			RelationChangedPayload{A: 0, B: 1, TrustDelta: 500, AffectionDelta: -500}))
	}
	r = s.RelationBetween(0, 1)
	if r.Trust != relMax || r.Affection != relMin {
		t.Errorf("edges must clamp: %+v", r)
	}

	// Self/invalid pairs rejected.
	if err := s.Apply(socialEvent(t, 100, "social.relation_changed",
		RelationChangedPayload{A: 2, B: 2, TrustDelta: 1})); err == nil {
		t.Error("self-edge must be rejected")
	}
}

// TestLedgerLifecycle is AC#2 / SC-002: give→debt→repay(kept) and
// give→lapse(broken) with reputation moving.
func TestLedgerLifecycle(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.Agents[0].Inv.FoodRaw = 3

	// A(0) gives to B(1): debt opens (B owes A).
	if err := s.Apply(socialEvent(t, 1000, "social.gave", GavePayload{From: 0, To: 1, Kind: "food"})); err != nil {
		t.Fatal(err)
	}
	if len(s.Debts) != 1 || s.Debts[0].Debtor != 1 || s.Debts[0].Creditor != 0 || s.Debts[0].Status != "open" {
		t.Fatalf("debt after give: %+v", s.Debts)
	}
	if s.Agents[0].Inv.FoodRaw != 2 || s.Agents[1].Inv.FoodRaw != 1 {
		t.Errorf("food transfer wrong: %d %d", s.Agents[0].Inv.FoodRaw, s.Agents[1].Inv.FoodRaw)
	}
	if s.Debts[0].Due != 1000+debtDueTicks {
		t.Errorf("due = %d", s.Debts[0].Due)
	}

	// B gives back: settles kept.
	s.Agents[1].Inv.FoodRaw = 2
	if err := s.Apply(socialEvent(t, 2000, "social.gave", GavePayload{From: 1, To: 0, Kind: "food"})); err != nil {
		t.Fatal(err)
	}
	if s.Debts[0].Status != "kept" || len(s.Debts) != 1 {
		t.Fatalf("repayment should settle, not open a new debt: %+v", s.Debts)
	}
	if Reputation(s, 1) != 600 {
		t.Errorf("kept debt should raise reputation: %d", Reputation(s, 1))
	}

	// Another give, left to lapse.
	s.Agents[0].LastGive = 0
	if err := s.Apply(socialEvent(t, 3000, "social.gave", GavePayload{From: 0, To: 1, Kind: "food"})); err != nil {
		t.Fatal(err)
	}
	brokenID := s.Debts[1].ID
	if err := s.Apply(socialEvent(t, 3000+debtDueTicks+3600, "social.promise_broken",
		PromiseBrokenPayload{ID: brokenID})); err != nil {
		t.Fatal(err)
	}
	if s.Debts[1].Status != "broken" {
		t.Fatalf("debt should be broken: %+v", s.Debts[1])
	}
	if rep := Reputation(s, 1); rep != 400 { // 500 +100 kept −200 broken
		t.Errorf("reputation after kept+broken = %d, want 400", rep)
	}
}

// TestExecutorGiveAndDueCheck: the deterministic acts fire end-to-end in
// the tick loop — give to a starving neighbor, break overdue debts.
func TestExecutorGiveAndDueCheck(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	// Place 0 next to 1; 1 starving; 0 stocked. Freeze both with sleep? No —
	// leave them idle; the social slot runs at tick%60==30.
	s.Agents[0].X, s.Agents[0].Y = 10, 10
	s.Agents[1].X, s.Agents[1].Y = 10, 11
	s.Agents[0].Inv.FoodRaw = 3
	s.Agents[1].Needs.Food = 100
	// Park everyone else far away and asleep so nothing interferes.
	for i := 2; i < len(s.Agents); i++ {
		s.Agents[i].Asleep = true
		s.Agents[i].X, s.Agents[i].Y = 60, 60
	}

	log := driveTicks(t, s, m, 120, nil)
	var gave bool
	for _, e := range log {
		if e.Type == "social.gave" {
			var p GavePayload
			json.Unmarshal(e.Payload, &p)
			if p.From == 0 && p.To == 1 {
				gave = true
			}
		}
	}
	if !gave {
		t.Fatal("executor never gave food to a starving neighbor")
	}
	if len(s.Debts) == 0 || s.Debts[0].Status != "open" {
		t.Fatalf("give should open a debt: %+v", s.Debts)
	}
	// Receiver's trust in the giver moved.
	if r := s.RelationBetween(1, 0); r.Trust < giveTrustToGiver {
		t.Errorf("receiver trust = %d", r.Trust)
	}

	// Jump the clock to just before due (a live 2-day drive would let the
	// debtor legitimately repay — good behavior, wrong for this assertion),
	// pauper the debtor, separate the pair, and cross one hourly boundary.
	s.Agents[1].Inv.FoodRaw = 0
	s.Agents[1].X, s.Agents[1].Y = 50, 50
	s.Tick = s.Debts[0].Due - 50
	log = driveTicks(t, s, m, s.Debts[0].Due+3700, nil)
	var broken, memory bool
	for _, e := range log {
		if e.Type == "social.promise_broken" {
			broken = true
		}
		if e.Type == "agent.memory_added" && strings.Contains(string(e.Payload), "never repaid") {
			memory = true
		}
	}
	if !broken {
		t.Fatal("overdue debt never broke")
	}
	if !memory {
		t.Error("creditor should remember the betrayal (gossip seed)")
	}
	if rep := Reputation(s, 1); rep >= 500 {
		t.Errorf("broken promise should drop reputation: %d", rep)
	}
}

// TestRumorProvenanceChain is AC#1's rumor half / SC-003: A→B→C with
// provenance links, decay, and recorded (mutable) text.
func TestRumorProvenanceChain(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	// Birth: A(0) tells B(1) — id assigned by reducer.
	if err := s.Apply(socialEvent(t, 100, "social.rumor_told", RumorToldPayload{
		From: 0, To: 1, RumorID: 0, Subject: 3, Tone: -80,
		Text: "Watched Rowan steal from the stores.", Confidence: 80,
	})); err != nil {
		t.Fatal(err)
	}
	if len(s.Rumors) != 1 || s.Rumors[0].Subject != 3 {
		t.Fatalf("registry: %+v", s.Rumors)
	}
	id := s.Rumors[0].ID

	// A holds the original (provenance −1); B heard it from A.
	if !knows(&s.Agents[0], id) || s.Agents[0].Known[0].From != -1 {
		t.Errorf("originator provenance: %+v", s.Agents[0].Known)
	}
	if s.Agents[1].Known[0].From != 0 || s.Agents[1].Known[0].Confidence != 80 {
		t.Errorf("B's variant: %+v", s.Agents[1].Known[0])
	}
	// Hearing shifted B's affection toward the subject.
	if r := s.RelationBetween(1, 3); r.Affection != -80/4 {
		t.Errorf("tone shift = %d", r.Affection)
	}

	// Retell: B tells C(2) with mutated text and decayed confidence.
	tell, ok := TellableFor(s, 1, 2)
	if !ok || tell.RumorID != id {
		t.Fatalf("B should have a tellable: %+v %v", tell, ok)
	}
	if tell.Confidence != 80*rumorDecayNum/rumorDecayDen {
		t.Errorf("decay: %d", tell.Confidence)
	}
	if err := s.Apply(socialEvent(t, 200, "social.rumor_told", RumorToldPayload{
		From: 1, To: 2, RumorID: id, Subject: 3, Tone: -80,
		Text:       "They say Rowan robbed the stores blind.", // mutated
		Confidence: tell.Confidence,
	})); err != nil {
		t.Fatal(err)
	}
	c := s.Agents[2].Known[0]
	if c.From != 1 || c.Text != "They say Rowan robbed the stores blind." || c.Confidence != 64 {
		t.Errorf("C's variant: %+v", c)
	}
	// Provenance chain: C←B←A←origin.
	if s.Agents[2].Known[0].From != 1 || s.Agents[1].Known[0].From != 0 || s.Agents[0].Known[0].From != -1 {
		t.Error("provenance chain broken")
	}

	// Nobody tells a rumor to its own subject.
	if _, ok := TellableFor(s, 1, 3); ok {
		t.Error("must not tell Rowan the rumor about Rowan")
	}
}

// TestSecretsSeededAndGated: genesis secrets exist, stay private, and are
// excluded from ordinary tellables.
func TestSecretsSeededAndGated(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	for i := range s.Agents {
		if err := s.Apply(socialEvent(t, 0, "social.secret_seeded",
			SecretSeededPayload{Agent: i, Text: "a dark secret", Tone: -70})); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.Rumors) != agentCount {
		t.Fatalf("secrets in registry: %d", len(s.Rumors))
	}
	for i := range s.Agents {
		if _, _, ok := SecretOf(s, i); !ok {
			t.Errorf("agent %d has no live secret", i)
		}
		// Secrets never appear as ordinary tellables.
		if tell, ok := TellableFor(s, i, (i+1)%agentCount); ok && tell.Text == "a dark secret" {
			t.Error("secret leaked through TellableFor")
		}
	}
	// Once a second holder knows it, it stops being a live secret.
	id := s.Rumors[0].ID
	s.Apply(socialEvent(t, 100, "social.rumor_told", RumorToldPayload{
		From: 0, To: 1, RumorID: id, Subject: 0, Tone: -70, Text: "a dark secret", Confidence: 80, Secret: true,
	}))
	if _, _, ok := SecretOf(s, 0); ok {
		t.Error("shared secret should no longer be private")
	}
}

// TestSocialDeterminismAndReplay re-proves SC-005 with a social timeline.
func TestSocialDeterminismAndReplay(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	timeline := map[int64][]store.Event{
		0: {socialEvent(t, 0, "social.secret_seeded", SecretSeededPayload{Agent: 0, Text: "s", Tone: -70})},
		500: {
			socialEvent(t, 500, "social.conversation", ConversationPayload{Conv: 500, A: 0, B: 1, Gist: "weather", Turns: 6}),
			socialEvent(t, 500, "social.relation_changed", RelationChangedPayload{A: 0, B: 1, TrustDelta: 24, AffectionDelta: 50, Reason: "conversation"}),
			socialEvent(t, 500, "social.rumor_told", RumorToldPayload{From: 0, To: 1, RumorID: 0, Subject: 2, Tone: 30, Text: "Cedar builds well.", Confidence: 40}),
		},
	}
	a, b := NewState(seed, m), NewState(seed, m)
	logA := driveTicks(t, a, m, 8000, timeline)
	driveTicks(t, b, m, 8000, timeline)
	if a.Hash() != b.Hash() {
		t.Fatal("social timeline broke determinism")
	}

	// Replay from the log alone.
	replayed := NewState(seed, m)
	for _, e := range logA {
		if err := replayed.Apply(e); err != nil {
			t.Fatalf("replay %s: %v", e.Type, err)
		}
		replayed.Tick = e.Tick
	}
	driveTicks(t, replayed, m, 8000, nil)
	if a.Hash() != replayed.Hash() {
		t.Fatal("social state not reproducible from the log")
	}
}
