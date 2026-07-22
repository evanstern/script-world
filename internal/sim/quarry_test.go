package sim

import (
	"encoding/json"
	"testing"

	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

// findRockOutcropWithTwoStands scans the map for a Rock tile with at least
// two distinct orthogonally-adjacent passable neighbors — a deterministic
// scenario setup for the contested-quarry test (two agents can each stand
// beside the very same outcrop).
func findRockOutcropWithTwoStands(m *worldmap.Map) (rx, ry, s1x, s1y, s2x, s2y int, ok bool) {
	dirs := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			if m.At(x, y) != worldmap.Rock {
				continue
			}
			var stands [][2]int
			for _, d := range dirs {
				nx, ny := x+d[0], y+d[1]
				if m.Passable(nx, ny) {
					stands = append(stands, [2]int{nx, ny})
				}
			}
			if len(stands) >= 2 {
				return x, y, stands[0][0], stands[0][1], stands[1][0], stands[1][1], true
			}
		}
	}
	return 0, 0, 0, 0, 0, 0, false
}

// TestQuarryHappyPath is spec 012 US1 AC#3: a villager beside a rock outcrop
// quarries it, gains the pinned stone yield, and the outcrop is depleted
// (permanently, per FR-002) — surviving the reducer application exactly like
// the existing chop/forage completions.
func TestQuarryHappyPath(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	a := &s.Agents[0]
	stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool {
		return m.InBounds(x, y) && effectiveKind(m, s, x, y) == worldmap.Rock
	})
	if !ok {
		t.Fatal("no rock outcrop reachable from agent 0's genesis position on seed 42")
	}
	a.X, a.Y = stand.X, stand.Y
	// WorkStart pre-set (test shortcut, matching the codebase's direct-state-
	// mutation idiom in sim_test.go) so the very first tick already
	// satisfies the work duration — no need to replay the arrival/start beat.
	a.Intent = &Intent{Goal: "quarry", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y, WorkStart: 1 - quarryTicks}

	log := driveTicks(t, s, m, 5, nil)

	var quarried bool
	for _, e := range log {
		if e.Type == "agent.quarried" {
			var p HarvestPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent != 0 || p.X != res.X || p.Y != res.Y {
				t.Errorf("agent.quarried payload = %+v, want agent 0 at (%d,%d)", p, res.X, res.Y)
			}
			quarried = true
		}
	}
	if !quarried {
		t.Fatal("no agent.quarried event emitted")
	}
	if a.Inv.Stone != quarryYield {
		t.Errorf("Inv.Stone = %d, want %d", a.Inv.Stone, quarryYield)
	}
	if a.Intent != nil {
		t.Error("intent should be cleared after quarrying")
	}
	if len(s.Quarried) != 1 || s.Quarried[0] != (Point{X: res.X, Y: res.Y}) {
		t.Errorf("Quarried overlay = %+v, want [{%d %d}]", s.Quarried, res.X, res.Y)
	}
	if effectiveKind(m, s, res.X, res.Y) != worldmap.Depleted {
		t.Errorf("effectiveKind of quarried tile = %v, want Depleted", effectiveKind(m, s, res.X, res.Y))
	}
	if !passable(m, s, res.X, res.Y) {
		t.Error("a depleted outcrop should be passable")
	}
	if buildSite(m, s, res.X, res.Y) {
		t.Error("a depleted outcrop should NOT be buildable")
	}
}

// TestContestedQuarry is spec 012 US1 AC#5: two villagers target the same
// outcrop; the first to complete depletes it, and the second's completion
// re-validates, finds it gone, and resolves without yield — matching today's
// contested-resource pattern (chop/forage/hunt).
func TestContestedQuarry(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	rx, ry, s1x, s1y, s2x, s2y, ok := findRockOutcropWithTwoStands(m)
	if !ok {
		t.Fatal("no rock outcrop with two passable neighbors found on seed 42")
	}

	a0, a1 := &s.Agents[0], &s.Agents[1]
	a0.X, a0.Y = s1x, s1y
	a1.X, a1.Y = s2x, s2y
	// a0 completes at tick 1; a1 completes at tick 2, by which point a0's
	// completion (applied after tick 1) has already appended the outcrop to
	// Quarried — so a1's re-validation at tick 2 sees it depleted.
	a0.Intent = &Intent{Goal: "quarry", TargetX: s1x, TargetY: s1y, ResX: rx, ResY: ry, WorkStart: 1 - quarryTicks}
	a1.Intent = &Intent{Goal: "quarry", TargetX: s2x, TargetY: s2y, ResX: rx, ResY: ry, WorkStart: 2 - quarryTicks}

	log := driveTicks(t, s, m, 5, nil)

	quarriedCount := 0
	a1DoneWithoutYield := false
	for _, e := range log {
		switch e.Type {
		case "agent.quarried":
			var p HarvestPayload
			mustUnmarshal(t, e.Payload, &p)
			quarriedCount++
			if p.Agent != 0 {
				t.Errorf("agent.quarried fired for agent %d, want only agent 0", p.Agent)
			}
		case "agent.intent_done":
			var p AgentPayload
			mustUnmarshal(t, e.Payload, &p)
			if p.Agent == 1 {
				a1DoneWithoutYield = true
			}
		}
	}
	if quarriedCount != 1 {
		t.Errorf("agent.quarried fired %d times, want exactly 1 (contested outcrop)", quarriedCount)
	}
	if !a1DoneWithoutYield {
		t.Error("agent 1's contested quarry should resolve via agent.intent_done with no yield")
	}
	if a1.Inv.Stone != 0 {
		t.Errorf("agent 1 Inv.Stone = %d, want 0 (lost the race)", a1.Inv.Stone)
	}
	if a0.Inv.Stone != quarryYield {
		t.Errorf("agent 0 Inv.Stone = %d, want %d (won the race)", a0.Inv.Stone, quarryYield)
	}
	if len(s.Quarried) != 1 {
		t.Errorf("Quarried overlay has %d entries, want exactly 1 (no double-append)", len(s.Quarried))
	}
}

// TestCollectWaterInexhaustible is spec 012 US1 AC#4: collecting water never
// depletes the source — repeated collects at the same tile always succeed.
func TestCollectWaterInexhaustible(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)

	a := &s.Agents[0]
	stand, res, ok := nearestAdjacentTo(m, s, a.X, a.Y, func(x, y int) bool {
		return m.InBounds(x, y) && effectiveKind(m, s, x, y) == worldmap.Water
	})
	if !ok {
		t.Fatal("no water tile reachable from agent 0's genesis position on seed 42")
	}
	a.X, a.Y = stand.X, stand.Y

	const rounds = 3
	for n := 0; n < rounds; n++ {
		a.Intent = &Intent{Goal: "collect_water", TargetX: stand.X, TargetY: stand.Y, ResX: res.X, ResY: res.Y,
			WorkStart: s.Tick + 1 - collectWaterTicks}
		driveTicks(t, s, m, s.Tick+5, nil)
	}
	if a.Inv.Water != rounds {
		t.Errorf("Inv.Water = %d, want %d after %d collects", a.Inv.Water, rounds, rounds)
	}
	// The source tile is untouched — still Water, still valid to collect
	// from again (no overlay entry anywhere records it).
	if effectiveKind(m, s, res.X, res.Y) != worldmap.Water {
		t.Errorf("water tile effectiveKind = %v, want Water (inexhaustible)", effectiveKind(m, s, res.X, res.Y))
	}
}

// TestReplayDeterminismWithQuarryAndWater is SC-004 over the new US1 event
// pair: a run producing both agent.quarried and agent.collected_water
// replays to byte-identical state, following the codebase's established
// replay-test idiom (sim_test.go TestReplayRebuildsState et al.).
func TestReplayDeterminismWithQuarryAndWater(t *testing.T) {
	const seed = 42
	const ticks = 5000
	m := testMap(seed)
	genesis := NewState(seed, m)

	rockStand, rockRes, ok := nearestAdjacentTo(m, genesis, genesis.Agents[0].X, genesis.Agents[0].Y, func(x, y int) bool {
		return m.InBounds(x, y) && effectiveKind(m, genesis, x, y) == worldmap.Rock
	})
	if !ok {
		t.Fatal("no rock outcrop reachable from agent 0")
	}
	waterStand, waterRes, ok := nearestAdjacentTo(m, genesis, genesis.Agents[1].X, genesis.Agents[1].Y, func(x, y int) bool {
		return m.InBounds(x, y) && effectiveKind(m, genesis, x, y) == worldmap.Water
	})
	if !ok {
		t.Fatal("no water tile reachable from agent 1")
	}

	commands := map[int64][]store.Event{
		0: {
			{Tick: 0, Type: "agent.intent_set", Payload: mustPayload(IntentSetPayload{
				Agent: 0, Goal: "quarry", TargetX: rockStand.X, TargetY: rockStand.Y,
				ResX: rockRes.X, ResY: rockRes.Y, Source: "planner",
			})},
			{Tick: 0, Type: "agent.intent_set", Payload: mustPayload(IntentSetPayload{
				Agent: 1, Goal: "collect_water", TargetX: waterStand.X, TargetY: waterStand.Y,
				ResX: waterRes.X, ResY: waterRes.Y, Source: "planner",
			})},
		},
	}

	live := NewState(seed, m)
	log := driveTicks(t, live, m, ticks, commands)

	var sawQuarried, sawCollected bool
	for _, e := range log {
		switch e.Type {
		case "agent.quarried":
			sawQuarried = true
		case "agent.collected_water":
			sawCollected = true
		}
	}
	if !sawQuarried {
		t.Fatal("scenario never produced agent.quarried — test setup didn't reach the outcrop")
	}
	if !sawCollected {
		t.Fatal("scenario never produced agent.collected_water — test setup didn't reach the water")
	}

	replayed := NewState(seed, m)
	for _, e := range log {
		if err := replayed.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replayed.Tick = e.Tick
	}
	driveTicks(t, replayed, m, ticks, nil) // re-live the quiet tail, as recovery does

	if live.Hash() != replayed.Hash() {
		t.Fatalf("replayed state diverged from live state:\nlive:     %s\nreplayed: %s",
			string(live.Marshal()), string(replayed.Marshal()))
	}
}

func mustUnmarshal(t *testing.T, raw []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
}
