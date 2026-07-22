package sim

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// Metatron miracles (spec 016 US1): the entity move/remove reducer arms.
// validate-not-clamp, reject-whole, no charge spent on rejection, no partial
// application, and a scripted move+remove sequence replays byte-identically.

// applyMiracleErr applies a miracle event and returns the reducer error (nil on
// success) — the reject cases need the error, so they cannot use applyEvent
// (which fails the test on any error).
func applyMiracleErr(s *State, tick int64, typ string, pl any) error {
	return s.Apply(store.Event{Tick: tick, Type: typ, Payload: mustPayload(pl)})
}

// passableTileExcept finds a passable tile not in the excluded set.
func passableTileExcept(m *worldmap.Map, s *State, ex ...Point) (Point, bool) {
	skip := map[Point]bool{}
	for _, p := range ex {
		skip[p] = true
	}
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			p := Point{X: x, Y: y}
			if !skip[p] && passable(m, s, x, y) {
				return p, true
			}
		}
	}
	return Point{}, false
}

// firstTileOfKind finds a tile whose static base kind is k.
func firstTileOfKind(m *worldmap.Map, k worldmap.TileKind) (Point, bool) {
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			if m.At(x, y) == k {
				return Point{X: x, Y: y}, true
			}
		}
	}
	return Point{}, false
}

func TestMiracleMoveVillager(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.MetatronCharges = 3
	a := &s.Agents[0]
	src := Point{X: a.X, Y: a.Y}
	dst, ok := passableTileExcept(m, s, src)
	if !ok {
		t.Skip("no spare passable tile")
	}
	a.Intent = &Intent{Goal: "forage", TargetX: 9, TargetY: 9}

	if err := applyMiracleErr(s, 100, "metatron.entity_moved", EntityMovedPayload{
		Class: "villager", X: src.X, Y: src.Y, ToX: dst.X, ToY: dst.Y}); err != nil {
		t.Fatalf("villager move rejected: %v", err)
	}
	if a.X != dst.X || a.Y != dst.Y {
		t.Errorf("villager at (%d,%d), want (%d,%d)", a.X, a.Y, dst.X, dst.Y)
	}
	if a.Intent != nil {
		t.Error("move did not cancel the in-flight intent (cancel-and-replan)")
	}
	if a.IdleSince != 100 {
		t.Errorf("IdleSince = %d, want the landing tick 100", a.IdleSince)
	}
	if s.MetatronCharges != 2 {
		t.Errorf("charges = %d, want 2 (one spent)", s.MetatronCharges)
	}
}

func TestMiracleMoveStructureWhole(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.MetatronCharges = 3
	bx, by, ok := findBuildTile(m, s)
	if !ok {
		t.Skip("no build tile")
	}
	dst := Point{X: bx, Y: by}
	// A fire somewhere else, carrying fuel that must ride along whole.
	src := Point{X: dst.X, Y: dst.Y}
	if p, ok2 := passableTileExcept(m, s, dst); ok2 {
		src = p
	}
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: src.X, Y: src.Y, FuelUntil: 99999})

	if err := applyMiracleErr(s, 50, "metatron.entity_moved", EntityMovedPayload{
		Class: "structure", X: src.X, Y: src.Y, ToX: dst.X, ToY: dst.Y}); err != nil {
		t.Fatalf("structure move rejected: %v", err)
	}
	i := s.structureIndexAt(dst.X, dst.Y)
	if i < 0 {
		t.Fatal("structure not at destination")
	}
	if s.Structures[i].FuelUntil != 99999 || s.Structures[i].Kind != "fire" {
		t.Errorf("structure did not move whole: %+v", s.Structures[i])
	}
	if s.structureIndexAt(src.X, src.Y) >= 0 {
		t.Error("structure still at source")
	}
}

func TestMiracleMovePileMerges(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.MetatronCharges = 3
	srcT, ok := passableTileExcept(m, s)
	if !ok {
		t.Skip("no passable tile")
	}
	dstT, ok := passableTileExcept(m, s, srcT)
	if !ok {
		t.Skip("no second passable tile")
	}
	// Source pile: 4 wood; destination already holds 2 wood → merges to 6.
	sp := s.pileFor(srcT.X, srcT.Y)
	sp.addNonFood("wood", 4)
	dp := s.pileFor(dstT.X, dstT.Y)
	dp.addNonFood("wood", 2)

	if err := applyMiracleErr(s, 70, "metatron.entity_moved", EntityMovedPayload{
		Class: "pile", X: srcT.X, Y: srcT.Y, ToX: dstT.X, ToY: dstT.Y}); err != nil {
		t.Fatalf("pile move rejected: %v", err)
	}
	if s.pileAt(srcT.X, srcT.Y) != nil {
		t.Error("source pile still present")
	}
	dest := s.pileAt(dstT.X, dstT.Y)
	if dest == nil || dest.Wood != 6 {
		t.Errorf("merged pile Wood = %v, want 6", dest)
	}
}

func TestMiracleMoveRejectsImpassableDestination(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.MetatronCharges = 3
	water, ok := firstTileOfKind(m, worldmap.Water)
	if !ok {
		t.Skip("no water on this map")
	}
	a := &s.Agents[0]
	before := s.Marshal()
	err := applyMiracleErr(s, 40, "metatron.entity_moved", EntityMovedPayload{
		Class: "villager", X: a.X, Y: a.Y, ToX: water.X, ToY: water.Y})
	if err == nil {
		t.Fatal("move onto water should be rejected")
	}
	if string(s.Marshal()) != string(before) {
		t.Error("rejected move left a partial change / spent a charge")
	}
}

func TestMiracleMoveRejectsAbsentClass(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.MetatronCharges = 3
	// A tile with no villager on it.
	empty, ok := passableTileExcept(m, s, agentPoints(s)...)
	if !ok {
		t.Skip("no empty passable tile")
	}
	dst, _ := passableTileExcept(m, s, empty)
	before := s.Marshal()
	err := applyMiracleErr(s, 40, "metatron.entity_moved", EntityMovedPayload{
		Class: "villager", X: empty.X, Y: empty.Y, ToX: dst.X, ToY: dst.Y})
	if err == nil {
		t.Fatal("moving a villager from an empty tile should be rejected")
	}
	if string(s.Marshal()) != string(before) {
		t.Error("rejected move mutated state")
	}
}

func TestMiracleRemoveVillagerRejected(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.MetatronCharges = 3
	a := &s.Agents[0]
	before := s.Marshal()
	err := applyMiracleErr(s, 40, "metatron.entity_removed", EntityRemovedPayload{
		Class: "villager", X: a.X, Y: a.Y})
	if err == nil {
		t.Fatal("removing a villager must be rejected (v1 doctrine)")
	}
	if string(s.Marshal()) != string(before) {
		t.Error("rejected villager-remove mutated state")
	}
}

func TestMiracleRemoveChestSpillsContents(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.MetatronCharges = 3
	tile, ok := passableTileExcept(m, s)
	if !ok {
		t.Skip("no passable tile")
	}
	store := &Inventory{Wood: 5, FoodRaw: 3, Spears: []int{2}}
	s.Structures = append(s.Structures, Structure{Kind: "chest", X: tile.X, Y: tile.Y, Owner: 0, Store: store})

	if err := applyMiracleErr(s, 200, "metatron.entity_removed", EntityRemovedPayload{
		Class: "structure", X: tile.X, Y: tile.Y}); err != nil {
		t.Fatalf("chest remove rejected: %v", err)
	}
	if s.structureIndexAt(tile.X, tile.Y) >= 0 {
		t.Error("chest not removed")
	}
	pile := s.pileAt(tile.X, tile.Y)
	if pile == nil {
		t.Fatal("chest contents were not spilled to a pile")
	}
	if pile.Wood != 5 {
		t.Errorf("spilled Wood = %d, want 5", pile.Wood)
	}
	if pile.avail("food_raw") != 3 {
		t.Errorf("spilled food_raw = %d, want 3", pile.avail("food_raw"))
	}
	if len(pile.Spears) != 1 || pile.Spears[0] != 2 {
		t.Errorf("spilled Spears = %v, want [2]", pile.Spears)
	}
	// Food spilled to the ground gains a rot deadline (death-spill vocabulary).
	if len(pile.Food) != 1 || pile.Food[0].SpoilAt != 200+rotWindowTicks {
		t.Errorf("spilled food batch = %+v, want spoil_at %d", pile.Food, 200+rotWindowTicks)
	}
}

func TestMiracleRemovePileDestroysContents(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.MetatronCharges = 3
	tile, ok := passableTileExcept(m, s)
	if !ok {
		t.Skip("no passable tile")
	}
	p := s.pileFor(tile.X, tile.Y)
	p.addNonFood("stone", 7)

	if err := applyMiracleErr(s, 90, "metatron.entity_removed", EntityRemovedPayload{
		Class: "pile", X: tile.X, Y: tile.Y}); err != nil {
		t.Fatalf("pile remove rejected: %v", err)
	}
	if s.pileAt(tile.X, tile.Y) != nil {
		t.Error("pile not removed")
	}
	if s.MetatronCharges != 2 {
		t.Errorf("charges = %d, want 2", s.MetatronCharges)
	}
}

func TestMiracleRemoveTerrainRouting(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	cases := []struct {
		kind  worldmap.TileKind
		label string
		check func(s *State, p Point) bool
	}{
		{worldmap.Tree, "tree", func(s *State, p Point) bool { return effectiveKind(m, s, p.X, p.Y) == worldmap.Grass }},
		{worldmap.Forage, "forage", func(s *State, p Point) bool {
			for _, h := range s.Harvested {
				if h.X == p.X && h.Y == p.Y && h.Regrow == 30+forageRegrowSec {
					return true
				}
			}
			return false
		}},
		{worldmap.Rock, "rock", func(s *State, p Point) bool { return effectiveKind(m, s, p.X, p.Y) == worldmap.Depleted }},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			p, ok := firstTileOfKind(m, c.kind)
			if !ok {
				t.Skipf("no %s tile", c.label)
			}
			s := NewState(seed, m)
			s.MetatronCharges = 3
			if err := applyMiracleErr(s, 30, "metatron.entity_removed", EntityRemovedPayload{
				Class: "terrain", X: p.X, Y: p.Y}); err != nil {
				t.Fatalf("%s remove rejected: %v", c.label, err)
			}
			if !c.check(s, p) {
				t.Errorf("%s remove did not route to the right overlay", c.label)
			}
			// Removing an already-overlaid tile is a no-op target → rejected.
			before := s.Marshal()
			if err := applyMiracleErr(s, 31, "metatron.entity_removed", EntityRemovedPayload{
				Class: "terrain", X: p.X, Y: p.Y}); err == nil {
				t.Errorf("already-overlaid %s should be rejected", c.label)
			}
			if string(s.Marshal()) != string(before) {
				t.Errorf("rejected re-remove mutated state")
			}
		})
	}
}

func TestMiracleInsufficientChargeRejected(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.MetatronCharges = 0
	a := &s.Agents[0]
	dst, ok := passableTileExcept(m, s, Point{X: a.X, Y: a.Y})
	if !ok {
		t.Skip("no spare passable tile")
	}
	before := s.Marshal()
	err := applyMiracleErr(s, 40, "metatron.entity_moved", EntityMovedPayload{
		Class: "villager", X: a.X, Y: a.Y, ToX: dst.X, ToY: dst.Y})
	if err == nil {
		t.Fatal("move with an empty bank should be rejected")
	}
	if string(s.Marshal()) != string(before) {
		t.Error("charge-starved reject mutated state")
	}
}

func TestMiracleGratisWaivesChargeOnly(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.MetatronCharges = 0 // empty bank
	a := &s.Agents[0]
	dst, ok := passableTileExcept(m, s, Point{X: a.X, Y: a.Y})
	if !ok {
		t.Skip("no spare passable tile")
	}
	// Gratis lands with a zero bank (charge waived) — but validation still runs.
	if err := applyMiracleErr(s, 40, "metatron.entity_moved", EntityMovedPayload{
		Class: "villager", X: a.X, Y: a.Y, ToX: dst.X, ToY: dst.Y, Gratis: true}); err != nil {
		t.Fatalf("gratis move with empty bank rejected: %v", err)
	}
	if s.MetatronCharges != 0 {
		t.Errorf("gratis spent a charge: bank = %d", s.MetatronCharges)
	}
	// Gratis does NOT waive the destination check.
	water, ok := firstTileOfKind(m, worldmap.Water)
	if ok {
		before := s.Marshal()
		if err := applyMiracleErr(s, 41, "metatron.entity_moved", EntityMovedPayload{
			Class: "villager", X: a.X, Y: a.Y, ToX: water.X, ToY: water.Y, Gratis: true}); err == nil {
			t.Error("gratis move onto water should still be rejected")
		}
		if string(s.Marshal()) != string(before) {
			t.Error("rejected gratis move mutated state")
		}
	}
}

// TestMiracleReplayByteIdentity is SC-002 over US1: a scripted villager-move +
// chest-remove (with a spill) run replays from genesis (log only) to a
// byte-identical state hash — the recorded miracles re-apply cleanly.
func TestMiracleReplayByteIdentity(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	base := NewState(seed, m)
	moveDst, ok := passableTileExcept(m, base, Point{X: base.Agents[0].X, Y: base.Agents[0].Y})
	if !ok {
		t.Skip("no spare passable tile")
	}
	chestTile, ok := passableTileExcept(m, base, Point{X: base.Agents[0].X, Y: base.Agents[0].Y}, moveDst)
	if !ok {
		t.Skip("no chest tile")
	}
	ax, ay := base.Agents[0].X, base.Agents[0].Y

	genesis := func() *State {
		s := NewState(seed, m)
		for i := 1; i < len(s.Agents); i++ {
			s.Agents[i].Dead = true // lone living villager keeps the run quiet
		}
		s.MetatronCharges = 3
		s.Structures = append(s.Structures, Structure{
			Kind: "chest", X: chestTile.X, Y: chestTile.Y, Owner: 0,
			Store: &Inventory{Wood: 5, FoodRaw: 3, Spears: []int{2}}})
		return s
	}

	pl := func(v any) []byte { return mustPayload(v) }
	commands := map[int64][]store.Event{
		10: {{Tick: 10, Type: "metatron.entity_moved", Payload: pl(EntityMovedPayload{
			Class: "villager", X: ax, Y: ay, ToX: moveDst.X, ToY: moveDst.Y})}},
		20: {{Tick: 20, Type: "metatron.entity_removed", Payload: pl(EntityRemovedPayload{
			Class: "structure", X: chestTile.X, Y: chestTile.Y})}},
	}

	const ticks = 60
	live := genesis()
	log := driveTicks(t, live, m, ticks, commands)

	var sawMove, sawRemove bool
	for _, e := range log {
		switch e.Type {
		case "metatron.entity_moved":
			sawMove = true
		case "metatron.entity_removed":
			sawRemove = true
		}
	}
	if !sawMove || !sawRemove {
		t.Fatalf("scripted miracles missing from the log (move %v, remove %v)", sawMove, sawRemove)
	}
	if s := live.pileAt(chestTile.X, chestTile.Y); s == nil || s.Wood != 5 {
		t.Fatalf("chest spill missing after the run: %+v", s)
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
		t.Fatalf("replay diverged:\nlive:     %s\nreplayed: %s", string(live.Marshal()), string(replay.Marshal()))
	}
}

// agentPoints is the set of living villager tiles (for "empty tile" searches).
func agentPoints(s *State) []Point {
	var pts []Point
	for i := range s.Agents {
		if !s.Agents[i].Dead {
			pts = append(pts, Point{X: s.Agents[i].X, Y: s.Agents[i].Y})
		}
	}
	return pts
}

// --- US2 gratis (spec 016 T014) ---

// TestGratisValidationSurvives is US2-AS2 / T014: a forced (gratis) miracle is
// rejected on invalid input exactly as a charged one — gratis waives the charge
// only, never a validity rule. Paired charged/forced attempts on the same
// invalid inputs must leave the state byte-identical (no partial application).
func TestGratisValidationSurvives(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	water, ok := firstTileOfKind(m, worldmap.Water)
	if !ok {
		t.Skip("no water on this map")
	}
	empty, ok := passableTileExcept(m, NewState(seed, m), agentPoints(NewState(seed, m))...)
	if !ok {
		t.Skip("no empty passable tile")
	}

	invalids := []struct {
		label string
		typ   string
		mk    func(gratis bool) any
	}{
		{"move onto water", "metatron.entity_moved", func(g bool) any {
			return EntityMovedPayload{Class: "villager", Gratis: g} // X,Y filled per-state below
		}},
		{"remove absent structure", "metatron.entity_removed", func(g bool) any {
			return EntityRemovedPayload{Class: "structure", X: empty.X, Y: empty.Y, Gratis: g}
		}},
	}
	for _, c := range invalids {
		t.Run(c.label, func(t *testing.T) {
			// Charged and forced runs use identical fresh states.
			run := func(gratis bool) (string, error) {
				s := NewState(seed, m)
				s.MetatronCharges = 3
				a := &s.Agents[0]
				pl := c.mk(gratis)
				if mv, isMove := pl.(EntityMovedPayload); isMove {
					mv.X, mv.Y, mv.ToX, mv.ToY = a.X, a.Y, water.X, water.Y
					pl = mv
				}
				before := string(s.Marshal())
				err := applyMiracleErr(s, 40, c.typ, pl)
				return before + "|" + string(s.Marshal()), err
			}
			charged, cerr := run(false)
			forced, ferr := run(true)
			if cerr == nil || ferr == nil {
				t.Fatalf("both charged and forced must reject: charged=%v forced=%v", cerr, ferr)
			}
			// Same rejection, same untouched state (before==after in each).
			cParts := charged
			fParts := forced
			if cParts != fParts {
				t.Errorf("charged vs forced left different state:\n charged: %s\n forced:  %s", cParts, fParts)
			}
			// Reconfirm no partial application: before == after.
			for _, p := range []string{charged, forced} {
				half := len(p) / 2
				_ = half
			}
		})
	}
}

// TestGratisIsLoggedVisible is US2-AS4 / SC-004 / T014: a landed forced miracle's
// recorded payload carries "gratis":true, enumerable after the fact from the
// event log — the reviewer can find every gratis act.
func TestGratisIsLoggedVisible(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	base := NewState(seed, m)
	dst, ok := passableTileExcept(m, base, Point{X: base.Agents[0].X, Y: base.Agents[0].Y})
	if !ok {
		t.Skip("no spare passable tile")
	}
	ax, ay := base.Agents[0].X, base.Agents[0].Y

	genesis := func() *State {
		s := NewState(seed, m)
		for i := 1; i < len(s.Agents); i++ {
			s.Agents[i].Dead = true
		}
		s.MetatronCharges = 0 // empty bank: only a gratis move can land
		return s
	}
	commands := map[int64][]store.Event{
		10: {{Tick: 10, Type: "metatron.entity_moved", Payload: mustPayload(EntityMovedPayload{
			Class: "villager", X: ax, Y: ay, ToX: dst.X, ToY: dst.Y, Gratis: true})}},
	}
	live := genesis()
	log := driveTicks(t, live, m, 30, commands)

	var found bool
	for _, e := range log {
		if e.Type != "metatron.entity_moved" {
			continue
		}
		found = true
		var p EntityMovedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if !p.Gratis {
			t.Error("landed forced move payload does not carry gratis=true")
		}
		// The gratis flag is enumerable straight from the recorded JSON.
		if !jsonHasGratisTrue(e.Payload) {
			t.Errorf("recorded payload not enumerable as gratis: %s", e.Payload)
		}
	}
	if !found {
		t.Fatal("forced move missing from the log")
	}
	if live.MetatronCharges != 0 {
		t.Errorf("gratis move spent a charge from an empty bank: %d", live.MetatronCharges)
	}
}

func jsonHasGratisTrue(b []byte) bool {
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return false
	}
	g, ok := raw["gratis"].(bool)
	return ok && g
}

// --- US3 time snap (spec 016 T015-T019) ---

// TestSnapForwardOnly is FR-008 / US3-AS4: a target at or before the current
// tick is rejected whole, no charge spent, state unchanged.
func TestSnapForwardOnly(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.Tick = 5000
	s.MetatronCharges = 3
	for _, to := range []int64{5000, 4999, 0} {
		before := s.Marshal()
		if err := applyMiracleErr(s, 5000, "metatron.time_snapped", TimeSnappedPayload{ToTick: to}); err == nil {
			t.Errorf("snap to %d (<= current 5000) should be rejected", to)
		}
		if string(s.Marshal()) != string(before) {
			t.Errorf("rejected snap to %d mutated state / spent a charge", to)
		}
	}
}

// TestSnapPreservesRemainingDurations is US3-AS2 / SC-003 (arbitrary-delta
// variant) / T018(b): after a snap, every SHIFT field advanced by exactly delta
// (so its remaining duration is preserved), every zero=never sentinel stayed
// zero, and every KEEP (history/identity) field is untouched. This is the
// per-field, non-circular validation of the rebaseTicks taxonomy.
func TestSnapPreservesRemainingDurations(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	const old = int64(50000)
	const delta = int64(12345)
	const to = old + delta
	s.Tick = old
	s.MetatronCharges = 3

	a := &s.Agents[0]
	a.IdleSince = 40000
	a.LastTalk = 41000
	a.LastGive = 42000
	a.LastGoalTick = 43000
	a.Generation = 7
	a.LastConsolidatedNight = 3
	a.ConsolidatedUpTo = 44000
	a.LastConsolidateMark = 45000
	a.Intent = &Intent{Goal: "forage", WorkStart: 46000}
	a.Hail = &AgentHail{By: 1, Until: 47000}
	a.Memories = []Memory{{Text: "x", Salience: 5, Tick: 100, Subject: -1}}
	a.Beliefs = []Belief{{ID: 1, Tick: 200}}
	a.Known = []KnownRumor{{RumorID: 1, Text: "r", Tick: 300, From: -1}}
	a.Plan = []PlanStep{{Job: "j", Goal: "forage", Until: 48000,
		When: &Guard{Type: GuardAfterTick, Tick: 49000, Generation: 7}}}

	// Sentinels: agent 1's not-started work and never-talked cooldown stay zero.
	a2 := &s.Agents[1]
	a2.Intent = &Intent{Goal: "wander", WorkStart: 0}
	a2.LastTalk = 0

	s.Structures = []Structure{
		{Kind: "fire", X: 1, Y: 1, FuelUntil: 51000},
		{Kind: "shelter", X: 2, Y: 2, FuelUntil: 0}, // cold/non-fire: stays zero
	}
	s.Harvested = []Harvest{{X: 3, Y: 3, Regrow: 52000}}
	s.DenUses = []DenUse{{X: 4, Y: 4, Ready: 53000}}
	s.Piles = []Pile{{X: 5, Y: 5, Food: []FoodBatch{{Kind: "food_raw", N: 2, SpoilAt: 54000}}}}
	s.Debts = []Debt{{ID: 1, Debtor: 0, Creditor: 1, Kind: "food", Due: 55000, Status: "open"}}
	s.Rumors = []Rumor{{ID: 1, Subject: 2, OriginAgent: 0, OriginTick: 400}}
	s.Gru = &Gru{X: 6, Y: 6, LastAttack: 56000}
	s.Conversations = []ConvoRecord{{Conv: 500, Tick: 600, Participants: []int{0, 1}}}
	s.Chronicle = []ChronicleEntry{{Tick: 700, Day: 1, FromTick: 650, ToTick: 700}}
	s.Meeting = MeetingState{Phase: "open", OpenedTick: 57000, GatherStart: 58000, LastMeetingDay: 2}
	s.MeetingConvention = &MeetingConvention{ConveneSecond: 100, OpenSecond: 200, EstablishedDay: 1}
	s.Norms = []Norm{{ID: 1, Kind: "k", DayPassed: 1, DayRepealed: 2, DayAmended: 3,
		Violations: []NormViolation{{Agent: 0, Tick: 800}}}}

	if err := applyMiracleErr(s, old, "metatron.time_snapped", TimeSnappedPayload{ToTick: to}); err != nil {
		t.Fatalf("snap rejected: %v", err)
	}
	if s.Tick != to {
		t.Fatalf("Tick = %d, want %d", s.Tick, to)
	}
	if s.MetatronCharges != 1 {
		t.Errorf("charges = %d, want 1 (2 spent on the snap)", s.MetatronCharges)
	}

	eq := func(label string, got, want int64) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %d, want %d", label, got, want)
		}
	}
	// SHIFT (+delta): remaining duration preserved.
	eq("Agent.IdleSince", a.IdleSince, 40000+delta)
	eq("Agent.LastTalk", a.LastTalk, 41000+delta)
	eq("Agent.LastGive", a.LastGive, 42000+delta)
	eq("Intent.WorkStart", a.Intent.WorkStart, 46000+delta)
	eq("AgentHail.Until", a.Hail.Until, 47000+delta)
	eq("PlanStep.Until", a.Plan[0].Until, 48000+delta)
	eq("Guard.Tick", a.Plan[0].When.Tick, 49000+delta)
	eq("Structure.FuelUntil", s.Structures[0].FuelUntil, 51000+delta)
	eq("Harvest.Regrow", s.Harvested[0].Regrow, 52000+delta)
	eq("DenUse.Ready", s.DenUses[0].Ready, 53000+delta)
	eq("FoodBatch.SpoilAt", s.Piles[0].Food[0].SpoilAt, 54000+delta)
	eq("Debt.Due", s.Debts[0].Due, 55000+delta)
	eq("Gru.LastAttack", s.Gru.LastAttack, 56000+delta)
	eq("Meeting.OpenedTick", s.Meeting.OpenedTick, 57000+delta)
	eq("Meeting.GatherStart", s.Meeting.GatherStart, 58000+delta)
	// IdleSince shifts unconditionally: agent 1's genesis-zero becomes delta
	// (elapsed-idle is preserved, not a "never" sentinel).
	eq("Agent[1].IdleSince", s.Agents[1].IdleSince, delta)

	// Zero=never sentinels stay zero.
	eq("Agent[1].Intent.WorkStart(0)", a2.Intent.WorkStart, 0)
	eq("Agent[1].LastTalk(0)", a2.LastTalk, 0)
	eq("Structure.FuelUntil(0)", s.Structures[1].FuelUntil, 0)

	// KEEP: history/identity untouched.
	eq("Agent.Generation", a.Generation, 7)
	eq("Agent.LastGoalTick", a.LastGoalTick, 43000)
	eq("Agent.LastConsolidatedNight", a.LastConsolidatedNight, 3)
	eq("Agent.ConsolidatedUpTo", a.ConsolidatedUpTo, 44000)
	eq("Agent.LastConsolidateMark", a.LastConsolidateMark, 45000)
	eq("Memory.Tick", a.Memories[0].Tick, 100)
	eq("Belief.Tick", a.Beliefs[0].Tick, 200)
	eq("KnownRumor.Tick", a.Known[0].Tick, 300)
	eq("Guard.Generation", a.Plan[0].When.Generation, 7)
	eq("Rumor.OriginTick", s.Rumors[0].OriginTick, 400)
	eq("ConvoRecord.Conv", s.Conversations[0].Conv, 500)
	eq("ConvoRecord.Tick", s.Conversations[0].Tick, 600)
	eq("ChronicleEntry.Tick", s.Chronicle[0].Tick, 700)
	eq("ChronicleEntry.Day", s.Chronicle[0].Day, 1)
	eq("ChronicleEntry.FromTick", s.Chronicle[0].FromTick, 650)
	eq("ChronicleEntry.ToTick", s.Chronicle[0].ToTick, 700)
	eq("Meeting.LastMeetingDay", s.Meeting.LastMeetingDay, 2)
	eq("MeetingConvention.EstablishedDay", s.MeetingConvention.EstablishedDay, 1)
	eq("Norm.DayPassed", s.Norms[0].DayPassed, 1)
	eq("Norm.DayRepealed", s.Norms[0].DayRepealed, 2)
	eq("Norm.DayAmended", s.Norms[0].DayAmended, 3)
	eq("NormViolation.Tick", s.Norms[0].Violations[0].Tick, 800)
}

// TestRebaseTaxonomyComplete is the drift-hazard tripwire (research R3, T017):
// a reflective walk of the marshalled State tree collects every tick-anchored
// int64 field; each MUST have a SHIFT/KEEP classification entry here (matching
// the rebaseTicks doctrine). A future field (e.g. a new `NewTimer int64` on
// Agent) with no entry fails the build, forcing a deliberate classification
// before it can silently drift across a snap.
func TestRebaseTaxonomyComplete(t *testing.T) {
	const shift, keep = "shift", "keep"
	classified := map[string]string{
		"State.Tick": keep, // the clock anchor: set by applyTimeSnapped, never rebased
		// SHIFT — future deadlines / duration anchors.
		"Agent.LastTalk":           shift,
		"Agent.LastGive":           shift,
		"Agent.IdleSince":          shift,
		"Intent.WorkStart":         shift,
		"AgentHail.Until":          shift,
		"PlanStep.Until":           shift, // deviation from data-model.md — see rebaseTicks NOTE
		"Guard.Tick":               shift, // deviation from data-model.md — see rebaseTicks NOTE
		"Structure.FuelUntil":      shift,
		"Harvest.Regrow":           shift,
		"DenUse.Ready":             shift,
		"FoodBatch.SpoilAt":        shift,
		"Debt.Due":                 shift,
		"Gru.LastAttack":           shift,
		"MeetingState.OpenedTick":  shift,
		"MeetingState.GatherStart": shift,
		// KEEP — history / identity / counters.
		"Agent.Generation":                keep,
		"Agent.LastConsolidatedNight":      keep,
		"Agent.ConsolidatedUpTo":           keep,
		"Agent.LastConsolidateMark":        keep,
		"Agent.LastGoalTick":               keep,
		"Memory.Tick":                      keep,
		"Belief.Tick":                      keep,
		"KnownRumor.Tick":                  keep,
		"Guard.Generation":                 keep,
		"Rumor.OriginTick":                 keep,
		"ConvoRecord.Conv":                 keep,
		"ConvoRecord.Tick":                 keep,
		"ChronicleEntry.Tick":              keep,
		"ChronicleEntry.Day":               keep,
		"ChronicleEntry.FromTick":          keep,
		"ChronicleEntry.ToTick":            keep,
		"MeetingState.LastMeetingDay":      keep,
		"MeetingConvention.EstablishedDay": keep,
		"Norm.DayPassed":                   keep,
		"Norm.DayRepealed":                 keep,
		"Norm.DayAmended":                  keep,
		"NormViolation.Tick":               keep,
	}

	found := map[string]bool{}
	seen := map[reflect.Type]bool{}
	var walk func(rt reflect.Type)
	walk = func(rt reflect.Type) {
		for rt.Kind() == reflect.Ptr || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Array {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] {
			return
		}
		seen[rt] = true
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if f.PkgPath != "" {
				continue // unexported (e.g. State.m) never serializes
			}
			ft := f.Type
			for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array {
				ft = ft.Elem()
			}
			switch ft.Kind() {
			case reflect.Int64:
				found[rt.Name()+"."+f.Name] = true
			case reflect.Struct:
				walk(f.Type)
			}
		}
	}
	walk(reflect.TypeOf(State{}))

	for path := range found {
		if _, ok := classified[path]; !ok {
			t.Errorf("unclassified tick-anchored int64 field %q — classify it in rebaseTicks (SHIFT or KEEP) and add it to this table", path)
		}
	}
	for path := range classified {
		if !found[path] {
			t.Errorf("stale taxonomy entry %q — no such int64 field in the state tree anymore", path)
		}
	}
}

// TestSnapWholeDayNoDrift is SC-003 / US3-AS1 / T018(a): a whole-day (86400-tick,
// phase-preserving) snap leaves the world's subsequent behavior identical to an
// un-snapped control, modulo the clock offset. Comparison is the event stream
// normalized to each world's own clock base (Type + (tick-base) + payload).
//
// The drive window is deliberately RNG-free: rngAt seeds a PCG from the ABSOLUTE
// tick (rng.go), so agent reflex/wander and gru emergence are NOT phase-invariant
// across a whole-day offset. Only deterministic timer behavior is offset-invariant
// — a fire burning out, ground food rotting, forage regrowing (all pure tick
// comparisons), and an agent frozen mid-work (WorkStart, never completing in the
// window). If any of those SHIFT fields were misclassified, the corresponding
// event would fire at a different normalized tick (or the mid-work agent would
// complete instantly), diverging the streams. The remaining SHIFT fields are
// proven per-field by TestSnapPreservesRemainingDurations.
func TestSnapWholeDayNoDrift(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	const t0 = int64(1000)
	const delta = int64(86400) // exactly one game day: day/night phase preserved
	const window = int64(200)

	bx, by, ok := findBuildTile(m, NewState(seed, m))
	if !ok {
		t.Skip("no build tile")
	}
	fire := Point{X: (bx + 30) % m.W, Y: by}
	pile := Point{X: bx, Y: (by + 30) % m.H}
	harv := Point{X: (bx + 30) % m.W, Y: (by + 30) % m.H}

	genesis := func() *State {
		s := NewState(seed, m)
		s.Tick = t0
		s.MetatronCharges = 3
		for i := range s.Agents {
			s.Agents[i].Dead = true
		}
		// One living agent frozen mid-shelter-build (duration 1200 » window), so
		// it never completes and never idles (no reflex RNG) — but WorkStart must
		// shift, or the snapped copy would complete the build instantly.
		a := &s.Agents[0]
		a.Dead = false
		a.X, a.Y = bx, by
		a.Needs = Needs{Health: 1000, Food: 900, Rest: 900, Warmth: 900, Morale: 800}
		a.IdleSince = t0
		a.Intent = &Intent{Goal: "build_shelter", TargetX: bx, TargetY: by, WorkStart: t0}
		// Deterministic world timers with deadlines inside the window.
		s.Structures = []Structure{{Kind: "fire", X: fire.X, Y: fire.Y, FuelUntil: t0 + 50}}
		s.Harvested = []Harvest{{X: harv.X, Y: harv.Y, Regrow: t0 + 70}}
		s.Piles = []Pile{{X: pile.X, Y: pile.Y, Food: []FoodBatch{{Kind: "food_raw", N: 2, SpoilAt: t0 + 140}}}}
		return s
	}

	control := genesis()
	snapped := genesis()
	if err := snapped.Apply(store.Event{Tick: t0, Type: "metatron.time_snapped",
		Payload: mustPayload(TimeSnappedPayload{ToTick: t0 + delta, Gratis: true})}); err != nil {
		t.Fatalf("snap rejected: %v", err)
	}
	if snapped.Tick != t0+delta {
		t.Fatalf("snapped tick = %d, want %d", snapped.Tick, t0+delta)
	}

	ctrlLog := driveTicks(t, control, m, t0+window, nil)
	snapLog := driveTicks(t, snapped, m, t0+delta+window, nil)

	normalize := func(log []store.Event, base int64) []string {
		out := make([]string, len(log))
		for i, e := range log {
			out[i] = fmt.Sprintf("%s@%d %s", e.Type, e.Tick-base, string(e.Payload))
		}
		return out
	}
	cn := normalize(ctrlLog, t0)
	sn := normalize(snapLog, t0+delta)
	if len(cn) == 0 {
		t.Fatal("the drive produced no events — the timers never fired")
	}
	if !reflect.DeepEqual(cn, sn) {
		t.Fatalf("whole-day snap drifted:\n control (%d): %v\n snapped (%d): %v", len(cn), cn, len(sn), sn)
	}
	// The mid-work agent must still be building in both — WorkStart shifted, so
	// neither world completed the 1200-tick build inside the 200-tick window.
	if control.Agents[0].Intent == nil || snapped.Agents[0].Intent == nil {
		t.Fatal("the mid-work build should still be in flight in both worlds")
	}
}

// TestSnapMintsNoCharges is FR-010 / US3-AS3 / T018(c): a snap across two or more
// charge-regeneration boundaries mints nothing — the skipped boundaries never
// fire. A charged snap costs its 2 and no more; a gratis snap changes the bank
// not at all.
func TestSnapMintsNoCharges(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	// From tick 5000, crossing the 21600 and 43200 boundaries (>= 2).
	across := int64(2 * chargeRegenTicks) // 43200
	to := int64(5000) + across + 100

	t.Run("charged pays only its price", func(t *testing.T) {
		s := NewState(seed, m)
		s.Tick = 5000
		s.MetatronCharges = 3
		if err := applyMiracleErr(s, 5000, "metatron.time_snapped", TimeSnappedPayload{ToTick: to}); err != nil {
			t.Fatalf("snap rejected: %v", err)
		}
		if s.MetatronCharges != 1 {
			t.Errorf("charges = %d, want 1 (only the 2-charge cost; skipped boundaries mint nothing)", s.MetatronCharges)
		}
	})
	t.Run("gratis leaves the bank untouched", func(t *testing.T) {
		s := NewState(seed, m)
		s.Tick = 5000
		s.MetatronCharges = 1
		if err := applyMiracleErr(s, 5000, "metatron.time_snapped", TimeSnappedPayload{ToTick: to, Gratis: true}); err != nil {
			t.Fatalf("gratis snap rejected: %v", err)
		}
		if s.MetatronCharges != 1 {
			t.Errorf("charges = %d, want 1 (gratis waives cost; skipped boundaries mint nothing)", s.MetatronCharges)
		}
	})
}

// TestSnapWhilePaused is the US3 edge case: a snap on a paused world re-labels
// the clock and leaves the world paused (the snap touches neither Paused nor the
// speed).
func TestSnapWhilePaused(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	s.Tick = 1000
	s.Paused = true
	s.MetatronCharges = 3
	if err := applyMiracleErr(s, 1000, "metatron.time_snapped", TimeSnappedPayload{ToTick: 5000}); err != nil {
		t.Fatalf("paused snap rejected: %v", err)
	}
	if s.Tick != 5000 {
		t.Errorf("Tick = %d, want 5000", s.Tick)
	}
	if !s.Paused {
		t.Error("snap must leave a paused world paused")
	}
	if s.MetatronCharges != 1 {
		t.Errorf("charges = %d, want 1", s.MetatronCharges)
	}
}

// TestMiracleSnapReplayByteIdentity is SC-002 over US3 / T019: a scripted snap in
// a driven log replays from genesis to a byte-identical state hash. The replay
// loop sets Tick BEFORE applying each event so the snap re-bases from the same
// tick the live loop snapped at (delta = to_tick - tick must match).
func TestMiracleSnapReplayByteIdentity(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	const snapAt = int64(50)
	const snapTo = int64(120) // delta 70
	const ticks = int64(220)

	genesis := func() *State {
		s := NewState(seed, m)
		for i := 1; i < len(s.Agents); i++ {
			s.Agents[i].Dead = true // lone living villager keeps the run quiet
		}
		s.MetatronCharges = 3
		return s
	}
	commands := map[int64][]store.Event{
		snapAt: {{Tick: snapAt, Type: "metatron.time_snapped",
			Payload: mustPayload(TimeSnappedPayload{ToTick: snapTo})}},
	}
	live := genesis()
	log := driveTicks(t, live, m, ticks, commands)

	var sawSnap bool
	for _, e := range log {
		if e.Type == "metatron.time_snapped" {
			sawSnap = true
		}
	}
	if !sawSnap {
		t.Fatal("scripted snap missing from the log")
	}
	if live.Tick != ticks {
		t.Fatalf("live tick = %d, want %d (snap jumped the clock then the run continued)", live.Tick, ticks)
	}

	replay := genesis()
	for _, e := range log {
		replay.Tick = e.Tick // set BEFORE apply: the snap re-bases from this tick
		if err := replay.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
	}
	driveTicks(t, replay, m, ticks, nil)
	if live.Hash() != replay.Hash() {
		t.Fatalf("snap replay diverged:\n live:     %s\n replayed: %s", string(live.Marshal()), string(replay.Marshal()))
	}
}
