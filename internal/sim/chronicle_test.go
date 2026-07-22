package sim

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

func chronicleEvent(t *testing.T, tick int64, p ChronicleEntryPayload) store.Event {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return store.Event{Tick: tick, Type: "chronicle.entry", Payload: b}
}

// TestChronicleRing: entries append in order with full payload fidelity and
// the ring stays bounded — the snapshot-carried catch-up working set.
func TestChronicleRing(t *testing.T) {
	m := worldmap.Generate(7, 32, 32)
	s := NewState(7, m)

	e := chronicleEvent(t, 1000, ChronicleEntryPayload{
		Day: 1, FromTick: 0, ToTick: 960,
		Text:   "Ash built the first fire while Sage watched the treeline.",
		Thread: "cold-start", Agents: []int{0, 7},
	})
	if err := s.Apply(e); err != nil {
		t.Fatal(err)
	}
	if len(s.Chronicle) != 1 {
		t.Fatalf("ring size = %d, want 1", len(s.Chronicle))
	}
	c := s.Chronicle[0]
	if c.Tick != 1000 || c.Day != 1 || c.FromTick != 0 || c.ToTick != 960 ||
		c.Thread != "cold-start" || len(c.Agents) != 2 {
		t.Errorf("entry mangled: %+v", c)
	}
	if !c.Mentions(0) || !c.Mentions(7) || c.Mentions(3) {
		t.Error("Mentions wrong")
	}

	for i := 0; i < chronicleCap+10; i++ {
		s.Apply(chronicleEvent(t, int64(2000+i), ChronicleEntryPayload{
			Day: 2, Text: fmt.Sprintf("entry %d", i), Thread: "filler",
		}))
	}
	if len(s.Chronicle) != chronicleCap {
		t.Errorf("ring size = %d, want cap %d", len(s.Chronicle), chronicleCap)
	}
	if s.Chronicle[len(s.Chronicle)-1].Text != fmt.Sprintf("entry %d", chronicleCap+9) {
		t.Error("ring did not keep newest entries")
	}
}

// TestChronicleSurvivesMarshal: the ring rides State.Marshal — that is what
// hands an attaching client its catch-up history.
func TestChronicleSurvivesMarshal(t *testing.T) {
	m := worldmap.Generate(7, 32, 32)
	s := NewState(7, m)
	s.Apply(chronicleEvent(t, 500, ChronicleEntryPayload{
		Day: 1, Text: "The village woke cold.", Thread: "cold-start", Agents: []int{2},
	}))

	restored := NewState(7, m)
	if err := json.Unmarshal(s.Marshal(), restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Chronicle) != 1 || restored.Chronicle[0].Text != "The village woke cold." {
		t.Errorf("chronicle lost in marshal round-trip: %+v", restored.Chronicle)
	}
}
