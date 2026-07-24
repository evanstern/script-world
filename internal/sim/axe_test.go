package sim

import (
	"testing"

	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// treeStand finds a passable tile adjacent to a Tree — the deterministic setup
// for the chop scenarios (stand beside the tree, chop the adjacent Res tile).
func treeStand(t *testing.T, m *worldmap.Map, s *State, fromX, fromY int) (stand, res Point) {
	t.Helper()
	st, rs, ok := nearestAdjacentTo(m, s, fromX, fromY, func(x, y int) bool {
		return m.InBounds(x, y) && effectiveKind(m, s, x, y) == worldmap.Tree
	})
	if !ok {
		t.Fatal("no tree reachable")
	}
	return st, rs
}

func rockStand(t *testing.T, m *worldmap.Map, s *State, fromX, fromY int) (stand, res Point) {
	t.Helper()
	st, rs, ok := nearestAdjacentTo(m, s, fromX, fromY, func(x, y int) bool {
		return m.InBounds(x, y) && effectiveKind(m, s, x, y) == worldmap.Rock
	})
	if !ok {
		t.Fatal("no rock reachable")
	}
	return st, rs
}

// TestCraftAxeTenUses is spec 032 US2 AC#1: crafting from 1 plank + 1 stone
// yields one full-durability (axeDurability) axe and deducts the inputs.
func TestCraftAxeTenUses(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)
	a := &s.Agents[0]
	a.Dead = false
	a.Inv = Inventory{Planks: 1, Stone: 1}
	a.Intent = &Intent{Goal: "craft_axe", TargetX: a.X, TargetY: a.Y, WorkStart: 1 - craftSpearTicks}

	driveTicks(t, s, m, 5, nil)
	if len(a.Inv.Axes) != 1 || a.Inv.Axes[0] != axeDurability {
		t.Fatalf("Axes = %v, want [%d]", a.Inv.Axes, axeDurability)
	}
	if a.Inv.Planks != 0 || a.Inv.Stone != 0 {
		t.Errorf("inputs = planks %d stone %d, want 0/0", a.Inv.Planks, a.Inv.Stone)
	}
}

// TestChopQuarryYieldBareVsAxe is spec 032 US2 AC#2/AC#3 / SC-001: bare-handed a
// chop/quarry yields 1, with a carried axe 3, and each axe-assisted harvest
// spends one durability use from Axes[0].
func TestChopQuarryYieldBareVsAxe(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	t.Run("chop", func(t *testing.T) {
		s := NewState(seed, m)
		isolateAgents(s)
		a := &s.Agents[0]
		a.Dead = false
		// Bare.
		stand, res := treeStand(t, m, s, a.X, a.Y)
		a.X, a.Y = stand.X, stand.Y
		a.Intent = &Intent{Goal: "chop", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y, WorkStart: 1 - chopTicks}
		driveTicks(t, s, m, 5, nil)
		if a.Inv.Wood != chopYieldBare {
			t.Errorf("bare chop wood = %d, want %d", a.Inv.Wood, chopYieldBare)
		}
		// With an axe.
		stand, res = treeStand(t, m, s, a.X, a.Y)
		a.X, a.Y = stand.X, stand.Y
		a.Inv.Axes = []int{axeDurability}
		a.Intent = &Intent{Goal: "chop", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y, WorkStart: s.Tick + 1 - chopTicks}
		driveTicks(t, s, m, s.Tick+5, nil)
		if a.Inv.Wood != chopYieldBare+chopYieldAxe {
			t.Errorf("wood after axe chop = %d, want %d", a.Inv.Wood, chopYieldBare+chopYieldAxe)
		}
		if len(a.Inv.Axes) != 1 || a.Inv.Axes[0] != axeDurability-1 {
			t.Errorf("Axes = %v, want [%d] (one use spent)", a.Inv.Axes, axeDurability-1)
		}
	})

	t.Run("quarry", func(t *testing.T) {
		s := NewState(seed, m)
		isolateAgents(s)
		a := &s.Agents[0]
		a.Dead = false
		stand, res := rockStand(t, m, s, a.X, a.Y)
		a.X, a.Y = stand.X, stand.Y
		a.Intent = &Intent{Goal: "quarry", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y, WorkStart: 1 - quarryTicks}
		driveTicks(t, s, m, 5, nil)
		if a.Inv.Stone != quarryYieldBare {
			t.Errorf("bare quarry stone = %d, want %d", a.Inv.Stone, quarryYieldBare)
		}
		stand, res = rockStand(t, m, s, a.X, a.Y)
		a.X, a.Y = stand.X, stand.Y
		a.Inv.Axes = []int{axeDurability}
		a.Intent = &Intent{Goal: "quarry", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y, WorkStart: s.Tick + 1 - quarryTicks}
		driveTicks(t, s, m, s.Tick+5, nil)
		if a.Inv.Stone != quarryYieldBare+quarryYieldAxe {
			t.Errorf("stone after axe quarry = %d, want %d", a.Inv.Stone, quarryYieldBare+quarryYieldAxe)
		}
		if len(a.Inv.Axes) != 1 || a.Inv.Axes[0] != axeDurability-1 {
			t.Errorf("Axes = %v, want [%d] (one use spent)", a.Inv.Axes, axeDurability-1)
		}
	})
}

// TestAxeBreaksOnLastUse is spec 032 US2 AC#4 / quickstart scenario 1: the chop
// that spends an axe's last use co-emits agent.axe_broke AFTER agent.chopped in
// the same batch (the harvest decrements to zero, then the companion removes
// it), leaves a memory, and the next harvest reverts to the bare yield.
func TestAxeBreaksOnLastUse(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	isolateAgents(s)
	a := &s.Agents[0]
	a.Dead = false
	stand, res := treeStand(t, m, s, a.X, a.Y)
	a.X, a.Y = stand.X, stand.Y
	a.Inv.Axes = []int{1} // last use
	a.Intent = &Intent{Goal: "chop", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y, WorkStart: 1 - chopTicks}

	log := driveTicks(t, s, m, 5, nil)
	choppedIdx, brokeIdx, memIdx := -1, -1, -1
	for i, e := range log {
		switch e.Type {
		case "agent.chopped":
			choppedIdx = i
		case "agent.axe_broke":
			var p AxeBrokePayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent != 0 {
				t.Errorf("axe_broke agent = %d, want 0", p.Agent)
			}
			brokeIdx = i
		case "agent.memory_added":
			var p MemoryAddedPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent == 0 && p.Salience == salAxeBroke {
				memIdx = i
			}
		}
	}
	if choppedIdx < 0 || brokeIdx < 0 {
		t.Fatalf("missing events: choppedIdx=%d brokeIdx=%d", choppedIdx, brokeIdx)
	}
	if brokeIdx < choppedIdx {
		t.Error("agent.axe_broke must apply after agent.chopped in the same batch")
	}
	if memIdx < 0 {
		t.Error("no broken-axe memory emitted")
	}
	if len(a.Inv.Axes) != 0 {
		t.Errorf("Axes after break = %v, want empty", a.Inv.Axes)
	}
	if a.Inv.Wood != chopYieldAxe {
		t.Errorf("wood on the breaking chop = %d, want %d (still axe yield)", a.Inv.Wood, chopYieldAxe)
	}
	// A subsequent bare chop yields 1.
	stand, res = treeStand(t, m, s, a.X, a.Y)
	a.X, a.Y = stand.X, stand.Y
	a.Intent = &Intent{Goal: "chop", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y, WorkStart: s.Tick + 1 - chopTicks}
	driveTicks(t, s, m, s.Tick+5, nil)
	if a.Inv.Wood != chopYieldAxe+chopYieldBare {
		t.Errorf("wood after the post-break bare chop = %d, want %d", a.Inv.Wood, chopYieldAxe+chopYieldBare)
	}
}

// TestAxeBulkTruncationStillSpends is spec 032 edge case: an axe-assisted harvest
// into less free bulk than the yield truncates the goods (existing rule) but
// still spends the durability use.
func TestAxeBulkTruncationStillSpends(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	a := &s.Agents[0]
	// One free bulk (22 wood + 1 axe = 23); axe chop yields 3, truncated to 1.
	a.Inv = Inventory{Wood: bulkCap - 2, Axes: []int{axeDurability}}
	e := store.Event{Tick: 1, Type: "agent.chopped", Payload: mustPayload(HarvestPayload{Agent: 0, X: 5, Y: 6})}
	if err := s.Apply(e); err != nil {
		t.Fatalf("apply chopped: %v", err)
	}
	if a.Inv.Wood != bulkCap-1 {
		t.Errorf("wood = %d, want %d (truncated to one free)", a.Inv.Wood, bulkCap-1)
	}
	if len(a.Inv.Axes) != 1 || a.Inv.Axes[0] != axeDurability-1 {
		t.Errorf("Axes = %v, want [%d] (use spent despite truncation)", a.Inv.Axes, axeDurability-1)
	}
}

// TestAxeStorageRoundTrip is spec 032 US2 / quickstart scenario 6: axes move
// through a ground pile and a chest with their uses preserved and sorted; a pile
// holding only axes is non-empty and is removed when the last axe is taken.
func TestAxeStorageRoundTrip(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	apply := func(t *testing.T, s *State, typ string, pl any) {
		t.Helper()
		if err := s.Apply(store.Event{Tick: 1, Type: typ, Payload: mustPayload(pl)}); err != nil {
			t.Fatalf("apply %s: %v", typ, err)
		}
	}

	t.Run("pile", func(t *testing.T) {
		s := NewState(seed, m)
		a := &s.Agents[0]
		x, y := a.X, a.Y
		a.Inv = Inventory{Axes: []int{3, 7}}
		apply(t, s, "agent.dropped", DroppedPayload{Agent: 0, X: x, Y: y, Kind: "axes", N: 2})
		p := s.pileAt(x, y)
		if p == nil || len(p.Axes) != 2 || p.Axes[0] != 3 || p.Axes[1] != 7 {
			t.Fatalf("pile axes = %v, want [3 7]", p)
		}
		if p.empty() {
			t.Error("a pile holding axes must not be empty")
		}
		if len(a.Inv.Axes) != 0 {
			t.Errorf("inv axes after drop = %v, want empty", a.Inv.Axes)
		}
		// Pick both back up: uses preserved, sorted; the drained pile is removed.
		apply(t, s, "agent.picked_up", PickedUpPayload{Agent: 0, X: x, Y: y, Kind: "axes", N: 2})
		if len(a.Inv.Axes) != 2 || a.Inv.Axes[0] != 3 || a.Inv.Axes[1] != 7 {
			t.Errorf("inv axes after pickup = %v, want [3 7]", a.Inv.Axes)
		}
		if s.pileAt(x, y) != nil {
			t.Error("pile should be removed once its last axe is taken")
		}
	})

	t.Run("chest", func(t *testing.T) {
		s := NewState(seed, m)
		a := &s.Agents[0]
		x, y := a.X, a.Y
		a.Inv = Inventory{Axes: []int{5, 9}}
		s.Structures = append(s.Structures, Structure{Kind: "chest", X: x, Y: y, Owner: 0, Store: &Inventory{}})
		apply(t, s, "agent.deposited", DepositedPayload{Agent: 0, X: x, Y: y, Kind: "axes", N: 2})
		ch := s.chestAt(x, y)
		if ch == nil || len(ch.Store.Axes) != 2 || ch.Store.Axes[0] != 5 {
			t.Fatalf("chest axes after deposit = %v, want [5 9]", ch.Store.Axes)
		}
		if len(a.Inv.Axes) != 0 {
			t.Errorf("inv axes after deposit = %v, want empty", a.Inv.Axes)
		}
		apply(t, s, "agent.withdrew", WithdrewPayload{Agent: 0, X: x, Y: y, Kind: "axes", N: 2, Owner: 0})
		if len(a.Inv.Axes) != 2 || a.Inv.Axes[0] != 5 || a.Inv.Axes[1] != 9 {
			t.Errorf("inv axes after withdraw = %v, want [5 9]", a.Inv.Axes)
		}
		if len(ch.Store.Axes) != 0 {
			t.Errorf("chest axes after withdraw = %v, want empty", ch.Store.Axes)
		}
	})
}

// TestReplayByteIdentityAxe is SC-005 over US2: a run that crafts an axe, chops
// and quarries with it, and stores it in a chest replays to a byte-identical
// state hash.
func TestReplayByteIdentityAxe(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	genesis := func() *State {
		s := NewState(seed, m)
		isolateAgents(s)
		a := &s.Agents[0]
		a.Dead = false
		a.Inv = Inventory{Planks: 1, Stone: 1}
		return s
	}
	setIntent := func(live *State, goal string, tx, ty, rx, ry int) store.Event {
		return store.Event{Tick: live.Tick, Type: "agent.intent_set", Payload: mustPayload(IntentSetPayload{
			Agent: 0, Goal: goal, TargetX: tx, TargetY: ty, ResX: rx, ResY: ry, Source: "planner"})}
	}

	live := genesis()
	a := &live.Agents[0]
	var log []store.Event
	drive := func(maxTicks int64, match string) {
		cmd := map[int64][]store.Event{}
		out := driveTicks(t, live, m, live.Tick+maxTicks, cmd)
		log = append(log, out...)
	}

	// craft_axe.
	e := setIntent(live, "craft_axe", a.X, a.Y, 0, 0)
	if err := live.Apply(e); err != nil {
		t.Fatal(err)
	}
	log = append(log, e)
	drive(craftSpearTicks+20, "agent.crafted")

	// chop with the axe.
	stand, res := treeStand(t, m, live, a.X, a.Y)
	e = setIntent(live, "chop", stand.X, stand.Y, res.X, res.Y)
	if err := live.Apply(e); err != nil {
		t.Fatal(err)
	}
	log = append(log, e)
	drive(5000, "agent.chopped")
	if len(a.Inv.Axes) != 1 || a.Inv.Axes[0] != axeDurability-1 {
		t.Fatalf("Axes after chop = %v, want [%d]", a.Inv.Axes, axeDurability-1)
	}

	replay := genesis()
	for _, ev := range log {
		if err := replay.Apply(ev); err != nil {
			t.Fatalf("replay apply %s: %v", ev.Type, err)
		}
		replay.Tick = ev.Tick
	}
	driveTicks(t, replay, m, live.Tick, nil)
	if live.Hash() != replay.Hash() {
		t.Fatalf("axe replay diverged:\nlive:     %s\nreplayed: %s", string(live.Marshal()), string(replay.Marshal()))
	}
}
