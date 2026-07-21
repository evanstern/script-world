package sim

import (
	"encoding/json"
	"testing"

	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

func nudgeEvent(t *testing.T, tick int64, p MetatronNudgedPayload) store.Event {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return store.Event{Tick: tick, Type: "metatron.nudged", Payload: b}
}

var regenEvent = store.Event{Type: "metatron.charge_regenerated", Payload: []byte("{}")}

// TestChargeInvariants: genesis 1; regen caps at 3; spends floor via
// validation (a spend at 0 is a reducer error, not a clamp).
func TestChargeInvariants(t *testing.T) {
	m := worldmap.Generate(7, 32, 32)
	s := NewState(7, m)
	if s.MetatronCharges != MetatronGenesisCharges {
		t.Fatalf("genesis charges = %d, want %d", s.MetatronCharges, MetatronGenesisCharges)
	}
	for i := 0; i < 10; i++ {
		if err := s.Apply(regenEvent); err != nil {
			t.Fatal(err)
		}
		if s.MetatronCharges > MetatronChargeCap {
			t.Fatalf("charges %d exceeded cap after regen storm", s.MetatronCharges)
		}
	}
	if s.MetatronCharges != MetatronChargeCap {
		t.Fatalf("charges = %d, want cap %d", s.MetatronCharges, MetatronChargeCap)
	}
	for i := 0; i < MetatronChargeCap; i++ {
		if err := s.Apply(nudgeEvent(t, 100, MetatronNudgedPayload{
			Form: "dream", Targets: []int{0}, Text: "a quiet dream"})); err != nil {
			t.Fatal(err)
		}
	}
	if s.MetatronCharges != 0 {
		t.Fatalf("charges = %d after spending the bank, want 0", s.MetatronCharges)
	}
	if err := s.Apply(nudgeEvent(t, 101, MetatronNudgedPayload{
		Form: "dream", Targets: []int{0}, Text: "one too many"})); err == nil {
		t.Fatal("spend at 0 charges must be a reducer error (the dry-run gate)")
	}
}

// TestNudgeValidation: the reducer rejects malformed nudges so the
// InjectSocial dry-run refuses them at the door.
func TestNudgeValidation(t *testing.T) {
	m := worldmap.Generate(7, 32, 32)
	long := make([]byte, NudgeTextMax+1)
	for i := range long {
		long[i] = 'x'
	}
	cases := []struct {
		name string
		p    MetatronNudgedPayload
		dead int // -1 = none
	}{
		{"unknown form", MetatronNudgedPayload{Form: "whisper", Targets: []int{0}, Text: "t"}, -1},
		{"dream multi-target", MetatronNudgedPayload{Form: "dream", Targets: []int{0, 1}, Text: "t"}, -1},
		{"omen no targets", MetatronNudgedPayload{Form: "omen", Targets: nil, Text: "t"}, -1},
		{"unknown target", MetatronNudgedPayload{Form: "dream", Targets: []int{99}, Text: "t"}, -1},
		{"dead target", MetatronNudgedPayload{Form: "dream", Targets: []int{2}, Text: "t"}, 2},
		{"empty text", MetatronNudgedPayload{Form: "dream", Targets: []int{0}, Text: ""}, -1},
		{"over-cap text", MetatronNudgedPayload{Form: "dream", Targets: []int{0}, Text: string(long)}, -1},
	}
	for _, c := range cases {
		s := NewState(7, m)
		if c.dead >= 0 {
			s.Agents[c.dead].Dead = true
		}
		if err := s.Apply(nudgeEvent(t, 50, c.p)); err == nil {
			t.Errorf("%s: expected reducer rejection", c.name)
		}
		if s.MetatronCharges != MetatronGenesisCharges {
			t.Errorf("%s: rejected nudge changed charges", c.name)
		}
	}
	// The valid shapes pass.
	s := NewState(7, m)
	if err := s.Apply(nudgeEvent(t, 50, MetatronNudgedPayload{
		Form: "omen", Targets: []int{0, 1, 2, 3, 4, 5, 6, 7}, Text: "the sky split"})); err != nil {
		t.Fatalf("valid omen rejected: %v", err)
	}
}

// TestRegenBoundaries: the executor emits regeneration exactly at absolute
// 6-game-hour tick boundaries, only below cap.
func TestRegenBoundaries(t *testing.T) {
	m := worldmap.Generate(7, 32, 32)
	s := NewState(7, m)
	s.MetatronCharges = 1

	count := func(tick int64) int {
		n := 0
		for _, e := range stepEvents(s, m, tick) {
			if e.Type == "metatron.charge_regenerated" {
				n++
			}
		}
		return n
	}
	if got := count(chargeRegenTicks); got != 1 {
		t.Errorf("boundary tick emitted %d regens, want 1", got)
	}
	if got := count(chargeRegenTicks + 1); got != 0 {
		t.Errorf("off-boundary tick emitted %d regens, want 0", got)
	}
	s.MetatronCharges = MetatronChargeCap
	if got := count(2 * chargeRegenTicks); got != 0 {
		t.Errorf("at-cap boundary emitted %d regens, want 0", got)
	}
}

// TestOldSnapshotGainsGenesisCharge: pre-TASK-12 snapshots have no charges
// field; unmarshal into genesis state keeps the default (documented upgrade).
// A modern snapshot with a spent-to-zero bank must round-trip as 0.
func TestOldSnapshotGainsGenesisCharge(t *testing.T) {
	m := worldmap.Generate(7, 32, 32)
	// Pre-TASK-12 shape: strip the field entirely.
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(NewState(7, m).Marshal(), &asMap); err != nil {
		t.Fatal(err)
	}
	delete(asMap, "metatron_charges")
	oldBytes, _ := json.Marshal(asMap)
	restored := NewState(7, m)
	if err := json.Unmarshal(oldBytes, restored); err != nil {
		t.Fatal(err)
	}
	if restored.MetatronCharges != MetatronGenesisCharges {
		t.Fatalf("restored charges = %d, want genesis %d", restored.MetatronCharges, MetatronGenesisCharges)
	}
	// Modern shape: zero survives the round trip.
	spent := NewState(7, m)
	spent.MetatronCharges = 0
	rt := NewState(7, m)
	if err := json.Unmarshal(spent.Marshal(), rt); err != nil {
		t.Fatal(err)
	}
	if rt.MetatronCharges != 0 {
		t.Fatalf("spent bank resurrected as %d, want 0", rt.MetatronCharges)
	}
}

// TestChargesReplayIdentically: a state rebuilt by replaying recorded
// regen/spend events matches the live sequence exactly.
func TestChargesReplayIdentically(t *testing.T) {
	m := worldmap.Generate(7, 32, 32)
	live := NewState(7, m)
	events := []store.Event{
		regenEvent, regenEvent, // 1 -> 3
		nudgeEvent(t, 10, MetatronNudgedPayload{Form: "dream", Targets: []int{3}, Text: "d1"}),   // 2
		nudgeEvent(t, 20, MetatronNudgedPayload{Form: "omen", Targets: []int{0, 1}, Text: "o1"}), // 1
		regenEvent, // 2
	}
	for _, e := range events {
		if err := live.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	replayed := NewState(7, m)
	for _, e := range events {
		if err := replayed.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	if live.MetatronCharges != 2 || replayed.MetatronCharges != live.MetatronCharges {
		t.Fatalf("live %d vs replayed %d (want 2)", live.MetatronCharges, replayed.MetatronCharges)
	}
	if live.Hash() != replayed.Hash() {
		t.Fatal("state hashes diverge under replay")
	}
}
