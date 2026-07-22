package sim

import (
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

// Theft (spec 013 US4, T031): a non-owner chest withdrawal is never blocked and
// never free. The transfer always lands, and in the SAME batch it drags the full
// social mark through the existing machinery — the taking record, a reason-tagged
// owner→taker trust drop, the owner's any-distance gossip-seed memory, and a
// witness memory for each in-range neighbor (SC-003). An owner fetching from their
// own chest triggers none of it (US4-AS4). A dead owner keeps the record, the
// relation delta, and the witnesses but forms no memory. The owner's subject-tagged
// memory is a live rumor seed. A scripted theft run replays byte-identically.

// theftSetup builds a state with agent 0 = chest owner, agent 1 = taker standing
// on the chest tile (stocked with wood), and the rest positioned/killed by the
// caller. Returns the chest tile.
func theftSetup(t *testing.T, seed uint64) (*State, int, int) {
	t.Helper()
	m := testMap(seed)
	s := NewState(seed, m)
	// The chest sits on the taker's tile so no walking is scheduled.
	cx, cy := s.Agents[1].X, s.Agents[1].Y
	s.Structures = append(s.Structures, Structure{
		Kind: "chest", X: cx, Y: cy, Owner: 0, Store: &Inventory{Wood: 10},
	})
	// The taker withdraws; healthy needs mean no reflex hijacks the intent.
	s.Agents[1].Inv = Inventory{}
	s.Agents[1].Intent = &Intent{Goal: "withdraw", TargetX: cx, TargetY: cy, Kind: "wood", Qty: 3}
	return s, cx, cy
}

// memAboutTaker counts an agent's memories tagged with the theft's fingerprint
// (subject = taker, the theft salience and tone) — the exact shape theftCompanions
// stamps, so unrelated memories never false-positive.
func memAboutTaker(a *Agent, taker int) int {
	c := 0
	for _, mm := range a.Memories {
		if mm.Subject == taker && mm.Salience == salTaking && mm.Tone == theftMemoryTone {
			c++
		}
	}
	return c
}

func idxOfType(log []store.Event, typ string) int {
	for i, e := range log {
		if e.Type == typ {
			return i
		}
	}
	return -1
}

// TestNonOwnerWithdrawalTheftBatch is SC-003 / US4-AS1-3: one non-owner withdrawal
// produces, in one batch and in contract order, the chest_taken record + the
// reason-tagged relation delta + the owner memory + in-range witness memories —
// and the transfer itself is never blocked.
func TestNonOwnerWithdrawalTheftBatch(t *testing.T) {
	const seed = 42
	s, cx, cy := theftSetup(t, seed)
	m := testMap(seed)

	// 0 owner (in range, to prove it is NOT also counted as a witness),
	// 2 bystander witness (in range), 3 far villager (out of range).
	s.Agents[0].X, s.Agents[0].Y = cx+2, cy
	s.Agents[2].X, s.Agents[2].Y = cx+1, cy
	far := cx + 30
	if cx >= 32 {
		far = cx - 30
	}
	s.Agents[3].X, s.Agents[3].Y = far, cy
	for i := 4; i < len(s.Agents); i++ {
		s.Agents[i].Dead = true
	}

	log := driveTicks(t, s, m, 2, nil)

	// The transfer landed — never blocked (FR-012).
	if s.Agents[1].Inv.Wood != 3 {
		t.Fatalf("taker Wood = %d, want 3 (the transfer must never be blocked)", s.Agents[1].Inv.Wood)
	}
	if ch := s.chestAt(cx, cy); ch == nil || ch.Store.Wood != 7 {
		t.Fatalf("chest Wood = %v, want 7 (3 taken)", ch)
	}

	// The record.
	var takenN int
	for _, e := range log {
		if e.Type == "social.chest_taken" {
			var p ChestTakenPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Owner != 0 || p.Taker != 1 || p.X != cx || p.Y != cy {
				t.Errorf("chest_taken payload = %+v, want owner 0 taker 1 at (%d,%d)", p, cx, cy)
			}
			takenN++
		}
	}
	if takenN != 1 {
		t.Fatalf("social.chest_taken count = %d, want exactly 1", takenN)
	}

	// The reason-tagged relation delta (US4-AS3).
	var relN int
	for _, e := range log {
		if e.Type == "social.relation_changed" {
			var p RelationChangedPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.A != 0 || p.B != 1 || p.Reason != "theft" {
				continue
			}
			relN++
			if p.TrustDelta != theftTrustDelta || p.AffectionDelta != theftAffectionDelta {
				t.Errorf("theft relation deltas = %d/%d, want %d/%d",
					p.TrustDelta, p.AffectionDelta, theftTrustDelta, theftAffectionDelta)
			}
		}
	}
	if relN != 1 {
		t.Fatalf("theft social.relation_changed count = %d, want exactly 1", relN)
	}
	if got := s.RelationBetween(0, 1).Trust; got != theftTrustDelta {
		t.Errorf("owner→taker trust = %d, want %d (dropped once through the edge)", got, theftTrustDelta)
	}

	// Memories: owner suffers exactly one (any distance), the bystander witnesses
	// one, the far villager none, and the owner is NOT double-counted as a witness.
	if got := memAboutTaker(&s.Agents[0], 1); got != 1 {
		t.Errorf("owner theft memories = %d, want exactly 1 (suffered, not also a witness)", got)
	}
	if got := memAboutTaker(&s.Agents[2], 1); got != 1 {
		t.Errorf("in-range bystander theft memories = %d, want 1 (witness)", got)
	}
	if got := memAboutTaker(&s.Agents[3], 1); got != 0 {
		t.Errorf("out-of-range villager theft memories = %d, want 0", got)
	}

	// Contract order within the batch: record, then relation, then memories.
	iTaken := idxOfType(log, "social.chest_taken")
	iRel := idxOfType(log, "social.relation_changed")
	iMem := idxOfType(log, "agent.memory_added")
	if !(iTaken >= 0 && iRel > iTaken && iMem > iRel) {
		t.Errorf("batch order = chest_taken@%d relation@%d memory@%d, want strictly increasing", iTaken, iRel, iMem)
	}
}

// TestOwnerWithdrawalNoSocialEvents is US4-AS4: fetching from your own chest is
// just fetching — agent.withdrew alone, none of the social machinery.
func TestOwnerWithdrawalNoSocialEvents(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s := NewState(seed, m)
	cx, cy := s.Agents[0].X, s.Agents[0].Y
	s.Structures = append(s.Structures, Structure{
		Kind: "chest", X: cx, Y: cy, Owner: 0, Store: &Inventory{Wood: 10},
	})
	s.Agents[0].Inv = Inventory{}
	s.Agents[0].Intent = &Intent{Goal: "withdraw", TargetX: cx, TargetY: cy, Kind: "wood", Qty: 3}
	for i := 1; i < len(s.Agents); i++ {
		s.Agents[i].Dead = true // quiet everyone else
	}

	log := driveTicks(t, s, m, 2, nil)

	if s.Agents[0].Inv.Wood != 3 {
		t.Fatalf("owner Wood = %d, want 3 (own withdrawal still works)", s.Agents[0].Inv.Wood)
	}
	for _, e := range log {
		switch e.Type {
		case "social.chest_taken", "social.relation_changed":
			t.Errorf("own-chest withdrawal emitted %s — must be silent (US4-AS4)", e.Type)
		}
	}
	if got := memAboutTaker(&s.Agents[0], 0); got != 0 {
		t.Errorf("own-chest withdrawal left %d theft memories, want 0", got)
	}
}

// TestDeadOwnerTheftRule is the "chest owner dies" edge case: taking from a dead
// owner's chest still records the happening, the relation delta, and witness
// memories — only the owner memory is skipped (the dead don't remember; the
// village does).
func TestDeadOwnerTheftRule(t *testing.T) {
	const seed = 99
	s, cx, cy := theftSetup(t, seed)
	m := testMap(seed)

	s.Agents[0].Dead = true                 // the owner is gone
	s.Agents[2].X, s.Agents[2].Y = cx+1, cy // a living witness
	for i := 3; i < len(s.Agents); i++ {
		s.Agents[i].Dead = true
	}

	log := driveTicks(t, s, m, 2, nil)

	if s.Agents[1].Inv.Wood != 3 {
		t.Fatalf("taker Wood = %d, want 3 (a dead owner never blocks the take)", s.Agents[1].Inv.Wood)
	}
	if idxOfType(log, "social.chest_taken") < 0 {
		t.Error("dead-owner take produced no chest_taken record")
	}
	var sawRel bool
	for _, e := range log {
		if e.Type == "social.relation_changed" {
			var p RelationChangedPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.A == 0 && p.B == 1 && p.Reason == "theft" {
				sawRel = true
			}
		}
	}
	if !sawRel {
		t.Error("dead-owner take produced no owner→taker theft relation delta")
	}
	if got := memAboutTaker(&s.Agents[0], 1); got != 0 {
		t.Errorf("dead owner formed %d theft memories, want 0 (the dead don't remember)", got)
	}
	if got := memAboutTaker(&s.Agents[2], 1); got != 1 {
		t.Errorf("living witness theft memories = %d, want 1 (the village remembers)", got)
	}
}

// TestOutOfRangeOwnerStillRemembers is US4-AS2's any-distance rule: a living owner
// far from their chest still gains the memory, even though no witness that far
// would.
func TestOutOfRangeOwnerStillRemembers(t *testing.T) {
	const seed = 123
	s, cx, cy := theftSetup(t, seed)
	m := testMap(seed)

	// Put the owner well outside witnessRadius of the chest.
	ox := cx + 25
	if cx >= 32 {
		ox = cx - 25
	}
	s.Agents[0].X, s.Agents[0].Y = ox, cy
	if abs(ox-cx) <= witnessRadius {
		t.Fatalf("owner placement (%d) not out of witness range of %d", ox, cx)
	}
	for i := 2; i < len(s.Agents); i++ {
		s.Agents[i].Dead = true
	}

	driveTicks(t, s, m, 2, nil)

	if got := memAboutTaker(&s.Agents[0], 1); got != 1 {
		t.Errorf("far-away owner theft memories = %d, want 1 (any distance, US4-AS2)", got)
	}
}

// TestTheftMemoryIsGossipSeed is the rumor-birth half of US4-AS2/FR-012: the
// owner's subject-tagged theft memory is a valid gossip seed for the existing
// rumor machinery — TellableFor births a rumor about the taker from it.
func TestTheftMemoryIsGossipSeed(t *testing.T) {
	const seed = 321
	s, cx, cy := theftSetup(t, seed)
	m := testMap(seed)

	s.Agents[0].X, s.Agents[0].Y = cx+2, cy
	for i := 2; i < len(s.Agents); i++ {
		s.Agents[i].Dead = true
	}
	// Revive a distinct third party (not owner, not taker) as the listener.
	const listener = 4
	s.Agents[listener].Dead = false

	driveTicks(t, s, m, 2, nil)

	if memAboutTaker(&s.Agents[0], 1) != 1 {
		t.Fatalf("precondition: owner lacks the theft memory to gossip from")
	}
	tell, ok := TellableFor(s, 0, listener)
	if !ok {
		t.Fatal("owner's theft memory did not qualify as a gossip seed (TellableFor found nothing)")
	}
	if tell.RumorID != 0 {
		t.Errorf("Tellable.RumorID = %d, want 0 (a birth from the memory)", tell.RumorID)
	}
	if tell.Subject != 1 {
		t.Errorf("Tellable.Subject = %d, want 1 (the taker)", tell.Subject)
	}
	if tell.Tone >= 0 {
		t.Errorf("Tellable.Tone = %d, want negative (a bad rumor about the taker)", tell.Tone)
	}
}

// TestReplayByteIdentityTheft is SC-005 over US4: a scripted non-owner withdrawal
// (with its full social batch) replays from genesis to a byte-identical state.
func TestReplayByteIdentityTheft(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	genesis := func() *State {
		s := NewState(seed, m)
		cx, cy := s.Agents[1].X, s.Agents[1].Y
		s.Structures = append(s.Structures, Structure{
			Kind: "chest", X: cx, Y: cy, Owner: 0, Store: &Inventory{Wood: 10},
		})
		s.Agents[0].X, s.Agents[0].Y = cx+2, cy // owner in range
		s.Agents[2].X, s.Agents[2].Y = cx+1, cy // a witness
		for i := 3; i < len(s.Agents); i++ {
			s.Agents[i].Dead = true
		}
		s.Agents[1].Inv = Inventory{}
		return s
	}

	live := genesis()
	cx, cy := live.Agents[1].X, live.Agents[1].Y
	pl := func(v any) []byte { return mustPayload(v) }
	commands := map[int64][]store.Event{
		30: {{Tick: 30, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
			Agent: 1, Goal: "withdraw", TargetX: cx, TargetY: cy, Kind: "wood", Qty: 3, Source: "planner"})}},
	}

	const ticks = 120
	log := driveTicks(t, live, m, ticks, commands)

	var sawTaken, sawWithdrew bool
	for _, e := range log {
		switch e.Type {
		case "social.chest_taken":
			sawTaken = true
		case "agent.withdrew":
			sawWithdrew = true
		}
	}
	if !sawWithdrew || !sawTaken {
		t.Fatalf("scripted theft run missing events (withdrew %v, chest_taken %v)", sawWithdrew, sawTaken)
	}

	replay := genesis()
	for _, e := range log {
		if err := replay.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replay.Tick = e.Tick
	}
	driveTicks(t, replay, m, ticks, nil)
	if live.Hash() != replay.Hash() {
		t.Fatalf("theft replay diverged:\nlive:     %s\nreplayed: %s", string(live.Marshal()), string(replay.Marshal()))
	}
}
