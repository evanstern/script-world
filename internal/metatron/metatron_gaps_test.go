package metatron

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/persona"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/toolloop"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// This file closes the coverage gaps inventoried in research.md R1: the
// charge-mirror/regeneration seam, true concurrent turn serialization,
// Observe backpressure, the absorb-mirror pipeline, and the soul/transcript
// tail windows. It deliberately does NOT re-test the instruction surface,
// charter fallbacks, digest, nudge/miracle landing, or charge decrement/
// zero-refusal — metatron_test.go already covers those (R1 anti-duplication).

// newLiveTestAngel is newTestAngel (T001) EXCEPT it does not Close() the
// angel after New — the absorb (run()) and digest goroutines stay alive, so
// Observe-driven tests can watch the replica/mirror pipeline actually run.
// t.Cleanup(mt.Close) still tears the goroutines down at test end.
func newLiveTestAngel(t *testing.T, reply string) (*Metatron, *mockOrch, *stateInjector, string) {
	t.Helper()
	dir := t.TempDir()
	if err := persona.Genesis(dir); err != nil {
		t.Fatal(err)
	}
	m := worldmap.Generate(42, 64, 64)
	state := sim.NewState(42, m)
	orch := &mockOrch{reply: reply}
	inj := &stateInjector{state: state}
	mt, err := New(orch, inj, &loopControlStub{}, m, 42, state.Marshal(), dir, testLoopRounds, testTurnTokens)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mt.Close)
	// The mock is not a *llm.Orchestrator, so New wired no runLoop; install the
	// default converse loop exactly like newTestAngel.
	mt.runLoop = converseLoop(mt)
	return mt, orch, inj, dir
}

// waitFor polls cond on a short channel-paced tick until it is true or the
// deadline elapses, at which point it fails the test — a bounded,
// channel-gated substitute for a sleep-as-the-only-gate poll (the TASK-69
// flake lesson: deterministic-under-race, fails in seconds, never hangs).
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	if cond() {
		return
	}
	for {
		select {
		case <-tick.C:
			if cond() {
				return
			}
		case <-deadline.C:
			t.Fatalf("condition not met within %s", timeout)
		}
	}
}

// TestChargeMirrorAccrualAndCap (US1, spec AC US1-2/US1-3, FR-002): delivered
// metatron.charge_regenerated events accrue the replica/mirror by +1 each,
// through Observe -> run() -> replica.Apply -> mirrorState -> Status(), and
// NEVER exceed sim.MetatronChargeCap even when more than enough events
// arrive; a subsequent metatron.nudged event then decrements the mirror.
func TestChargeMirrorAccrualAndCap(t *testing.T) {
	mt, _, _, _ := newLiveTestAngel(t, "")

	// Genesis starts at MetatronGenesisCharges (1); deliver cap+2 regeneration
	// events one batch at a time so each is individually mirrored.
	for i := 0; i < sim.MetatronChargeCap+2; i++ {
		mt.Observe([]store.Event{{Tick: int64(i + 1), Type: "metatron.charge_regenerated"}})
	}
	waitFor(t, 2*time.Second, func() bool {
		return mt.Status().Charges == sim.MetatronChargeCap
	})
	if got := mt.Status().Charges; got != sim.MetatronChargeCap {
		t.Fatalf("charges = %d after %d regen events, want the cap %d",
			got, sim.MetatronChargeCap+2, sim.MetatronChargeCap)
	}

	// A valid nudge decrements the mirror through the same absorb pipeline.
	payload, err := json.Marshal(sim.MetatronNudgedPayload{Form: "dream", Targets: []int{0}, Text: "a whisper"})
	if err != nil {
		t.Fatal(err)
	}
	mt.Observe([]store.Event{{Tick: int64(sim.MetatronChargeCap + 3), Type: "metatron.nudged", Payload: payload}})
	waitFor(t, 2*time.Second, func() bool {
		return mt.Status().Charges == sim.MetatronChargeCap-1
	})
}

// TestTurnBusyConcurrent (US1, spec AC US1-4, FR-006): two REAL goroutines
// contend on the turnBusy CAS. The scripted loop parks turn A inside the
// loop; turn B, launched only after A is provably inside (a loop-entered
// signal, never a sleep), fails fast with ErrTurnBusy; releasing A lets it
// complete normally with its reply intact. Meaningful under -race — upgrades
// the manual-flag TestTurnSingleFlight, which stays untouched.
func TestTurnBusyConcurrent(t *testing.T) {
	mt, _, _, _ := newTestAngel(t, "released")
	entered := make(chan struct{})
	release := make(chan struct{})
	mt.runLoop = func(ctx context.Context, j toolloop.Job) (toolloop.Result, error) {
		close(entered)
		<-release
		return toolloop.Result{Final: "released", Term: toolloop.TermModelDone}, nil
	}

	type outcome struct {
		res TurnResult
		err error
	}
	doneA := make(chan outcome, 1)
	go func() {
		r, err := mt.Turn(context.Background(), "first")
		doneA <- outcome{r, err}
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("turn A never entered the loop")
	}

	if _, err := mt.Turn(context.Background(), "second"); err != ErrTurnBusy {
		t.Fatalf("concurrent turn B = %v, want ErrTurnBusy", err)
	}

	close(release)
	select {
	case out := <-doneA:
		if out.err != nil {
			t.Fatalf("turn A failed after release: %v", out.err)
		}
		if out.res.Reply != "released" {
			t.Errorf("turn A reply = %q, want %q", out.res.Reply, "released")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("turn A never completed after release")
	}
}

// TestObserveNeverBlocks (US1, data-model.md §1 notify backpressure): with the
// standard closed-goroutine angel (absorb not draining, per T001), sending
// well over the events channel's capacity through Observe returns promptly
// every single call — the non-blocking send drops on a full channel rather
// than wedging the caller.
func TestObserveNeverBlocks(t *testing.T) {
	mt, _, _, _ := newTestAngel(t, "")
	done := make(chan struct{})
	go func() {
		for i := 0; i < 300; i++ {
			mt.Observe([]store.Event{{Tick: int64(i)}})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Observe wedged instead of dropping on a full/undrained channel")
	}
}

// TestAbsorbRefreshesMirrors (US1, data-model.md §1 absorb mirrors): an
// observed agent.died event refreshes the alive mirror end-to-end through
// run()/mirrorState, so the very next Turn's user prompt lists the villager
// under "Departed:" and no longer counts them alive; a flood of chronicle
// entries mirrors down to the last 8 in the story tail.
func TestAbsorbRefreshesMirrors(t *testing.T) {
	mt, orch, _, _ := newLiveTestAngel(t, "The village endures.")

	died, err := json.Marshal(sim.DiedPayload{Agent: 0, Cause: "starvation"})
	if err != nil {
		t.Fatal(err)
	}
	mt.Observe([]store.Event{{Tick: 500, Type: "agent.died", Payload: died}})
	waitFor(t, 2*time.Second, func() bool {
		mt.stateMu.Lock()
		defer mt.stateMu.Unlock()
		return !mt.alive[0]
	})

	if _, err := mt.Turn(context.Background(), "how do they fare?"); err != nil {
		t.Fatal(err)
	}
	reqs := orch.requests()
	prompt := reqs[len(reqs)-1].Prompt
	wantDeparted := "Departed: " + sim.AgentNames[0]
	if !strings.Contains(prompt, wantDeparted) {
		t.Errorf("user prompt missing %q: %q", wantDeparted, prompt)
	}

	// Flood more than 8 chronicle-bearing events; the mirrored story tail
	// caps at the last 8 (mirrorState).
	var batch []store.Event
	for i := 0; i < 12; i++ {
		entry, err := json.Marshal(sim.ChronicleEntryPayload{
			Day: 1, Text: fmt.Sprintf("entry %d", i), Thread: "test",
		})
		if err != nil {
			t.Fatal(err)
		}
		batch = append(batch, store.Event{Tick: int64(600 + i), Type: "chronicle.entry", Payload: entry})
	}
	mt.Observe(batch)
	waitFor(t, 2*time.Second, func() bool {
		mt.stateMu.Lock()
		defer mt.stateMu.Unlock()
		return len(mt.story) == 8
	})
	mt.stateMu.Lock()
	last := mt.story[len(mt.story)-1]
	mt.stateMu.Unlock()
	if !strings.Contains(last, "entry 11") {
		t.Errorf("story tail's last entry = %q, want the most recent (entry 11)", last)
	}
}

// TestTailOfFile (US2, spec AC US2-5, data-model.md §1 tail windows): the
// low-level trailing-bytes reader — a file longer than n yields exactly the
// trailing n bytes, a file shorter than n yields the whole file, a missing
// file yields "", and an empty file yields "".
func TestTailOfFile(t *testing.T) {
	t.Run("longer than n returns the trailing n bytes", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "f.txt")
		if err := os.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := tailOfFile(path, 4); got != "6789" {
			t.Errorf("tail = %q, want %q", got, "6789")
		}
	})
	t.Run("shorter than n returns the whole file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "f.txt")
		if err := os.WriteFile(path, []byte("short"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := tailOfFile(path, 100); got != "short" {
			t.Errorf("tail = %q, want %q", got, "short")
		}
	})
	t.Run("missing file returns empty", func(t *testing.T) {
		got := tailOfFile(filepath.Join(t.TempDir(), "does-not-exist.txt"), 10)
		if got != "" {
			t.Errorf("tail of missing file = %q, want empty", got)
		}
	})
	t.Run("empty file returns empty", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.txt")
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := tailOfFile(path, 10); got != "" {
			t.Errorf("tail of empty file = %q, want empty", got)
		}
	})
}

// TestSoulTailWindow (US2, spec AC US2-5, FR-006): soulTail() is exactly the
// 4000-byte (soulTailBytes) trailing window of soul.md, carrying a tail
// marker and excluding a head marker written far enough back to fall outside
// the window; a Turn's user prompt carries that windowed tail, not the full
// soul.
func TestSoulTailWindow(t *testing.T) {
	mt, orch, _, dir := newTestAngel(t, "noted.")
	const headMarker = "HEAD-MARKER-SENTINEL"
	const tailMarker = "TAIL-MARKER-SENTINEL"
	filler := strings.Repeat("x", soulTailBytes*2)
	content := headMarker + filler + tailMarker
	if err := os.WriteFile(filepath.Join(dir, "metatron", "soul.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got := mt.soulTail()
	if len(got) != soulTailBytes {
		t.Fatalf("soulTail length = %d, want %d (soulTailBytes)", len(got), soulTailBytes)
	}
	if !strings.Contains(got, tailMarker) {
		t.Error("soulTail is missing the tail marker")
	}
	if strings.Contains(got, headMarker) {
		t.Error("soulTail leaked the head marker outside the window")
	}

	if _, err := mt.Turn(context.Background(), "what have you seen?"); err != nil {
		t.Fatal(err)
	}
	reqs := orch.requests()
	prompt := reqs[len(reqs)-1].Prompt
	if !strings.Contains(prompt, tailMarker) {
		t.Error("user prompt missing the windowed soul tail")
	}
	if strings.Contains(prompt, headMarker) {
		t.Error("user prompt carried the full soul instead of the windowed tail")
	}
}

// TestTranscriptTailTurns (US2, spec AC US2-5, FR-006): transcriptTail()
// keeps exactly the last transcriptTailTurns (6) whole turns, oldest first /
// newest last, dropping earlier ones — within the 3000-byte read so the
// turn-trim rule itself, not the byte truncation, is what's under test.
func TestTranscriptTailTurns(t *testing.T) {
	mt, _, _, _ := newTestAngel(t, "")
	const totalTurns = 8 // > transcriptTailTurns (6)
	for i := 1; i <= totalTurns; i++ {
		mt.recordTurn(int64(i), turnOrigin{jobPrefix: "turn", seed: fmt.Sprintf("question %d", i)}, TurnResult{Reply: fmt.Sprintf("answer %d", i)})
	}

	got := mt.transcriptTail()
	for i := 1; i <= totalTurns-transcriptTailTurns; i++ {
		if strings.Contains(got, fmt.Sprintf("question %d\n", i)) {
			t.Errorf("transcriptTail retained turn %d, which is outside the last %d", i, transcriptTailTurns)
		}
	}
	var firstIdx, lastIdx = -1, -1
	for i := totalTurns - transcriptTailTurns + 1; i <= totalTurns; i++ {
		want := fmt.Sprintf("question %d", i)
		idx := strings.Index(got, want)
		if idx < 0 {
			t.Fatalf("transcriptTail missing turn %d: %q", i, got)
		}
		if firstIdx < 0 {
			firstIdx = idx
		}
		lastIdx = idx
	}
	if firstIdx > lastIdx {
		t.Errorf("turns not ordered oldest-first/newest-last: first at %d, last at %d", firstIdx, lastIdx)
	}
}
