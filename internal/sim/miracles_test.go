package sim

import (
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
