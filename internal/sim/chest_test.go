package sim

import (
	"strings"
	"testing"

	"github.com/evanstern/script-world/internal/store"
)

// Chests (spec 013 US3, T025): a chest costs 6 planks and records its builder as
// owner permanently; deposits truncate to the chest's free space (a full chest
// leaves the excess carried, US3-AS4); withdrawals truncate to the taker's free
// bulk and to what the chest holds; chest food is plain counts that never spoil
// (FR-010); spear durabilities round-trip agent → chest → agent; and a scripted
// build/deposit/withdraw run replays byte-identically (SC-005).

// TestBuildChestCostAndOwner is US3-AS1 + the "chest owner dies" edge case: the
// build consumes chestPlankCost planks and stamps the builder as owner with an
// empty Store, and the owner record is permanent — a builder's death changes
// nothing on the chest.
func TestBuildChestCostAndOwner(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	a := &s.Agents[0]
	a.Inv = Inventory{Planks: 8}

	applyEvent(t, s, 50, "agent.built", BuiltPayload{Agent: 0, Kind: "chest", X: a.X, Y: a.Y})

	if a.Inv.Planks != 8-chestPlankCost {
		t.Errorf("builder Planks = %d, want %d (chestPlankCost consumed)", a.Inv.Planks, 8-chestPlankCost)
	}
	ch := s.chestAt(a.X, a.Y)
	if ch == nil {
		t.Fatal("no chest structure placed at the build site")
	}
	if ch.Owner != 0 {
		t.Errorf("chest Owner = %d, want 0 (the builder)", ch.Owner)
	}
	if ch.Store == nil {
		t.Fatal("chest Store is nil, want an empty Inventory")
	}
	if bulk(*ch.Store) != 0 {
		t.Errorf("fresh chest bulk = %d, want 0 (empty Store)", bulk(*ch.Store))
	}

	// Owner is permanent: a builder's death leaves the record untouched (no
	// transfer, no inheritance in v1).
	applyEvent(t, s, 60, "agent.died", DiedPayload{Agent: 0, Cause: "starvation"})
	ch = s.chestAt(a.X, a.Y)
	if ch == nil || ch.Owner != 0 {
		t.Errorf("chest owner changed on the owner's death: %+v", ch)
	}
}

// TestDepositTruncatesToChestSpace is US3-AS2/AS4: a deposit into a nearly-full
// chest moves only what fits (chestCap − bulk(*Store)); the excess stays carried,
// never destroyed. Driven through the executor so both the emit truncation and
// the reducer's defensive clamp are exercised.
func TestDepositTruncatesToChestSpace(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	a := &s.Agents[0]
	a.Inv = Inventory{Wood: 10}
	// A chest on the agent's tile with only 3 bulk of free space.
	s.Structures = append(s.Structures, Structure{
		Kind: "chest", X: a.X, Y: a.Y, Owner: 0, Store: &Inventory{Wood: chestCap - 3},
	})
	a.Intent = &Intent{Goal: "deposit", TargetX: a.X, TargetY: a.Y, Kind: "wood"}

	log := driveTicks(t, s, m, 2, nil)

	var got int
	for _, e := range log {
		if e.Type == "agent.deposited" {
			var p DepositedPayload
			mustUnmarshal(t, e.Payload, &p)
			got = p.N
		}
	}
	if got != 3 {
		t.Errorf("deposited N = %d, want 3 (truncated to the chest's free space)", got)
	}
	ch := s.chestAt(a.X, a.Y)
	if ch == nil || bulk(*ch.Store) != chestCap {
		t.Errorf("chest bulk = %d, want %d (filled to capacity)", bulk(*ch.Store), chestCap)
	}
	if a.Inv.Wood != 7 {
		t.Errorf("carried Wood = %d, want 7 (excess stays carried, US3-AS4)", a.Inv.Wood)
	}
}

// TestWithdrawTruncatesToBulkAndContents is US3-AS3 + the "withdraw more than
// fits" edge case: a withdrawal is bounded by BOTH the taker's free bulk and what
// the chest actually holds — never an error, never a loss.
func TestWithdrawTruncatesToBulkAndContents(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	drive := func(t *testing.T, carried Inventory, store Inventory, qty int) (n, invWood, chestWood int) {
		t.Helper()
		s := NewState(seed, m)
		a := &s.Agents[0]
		a.Inv = carried
		st := store
		s.Structures = append(s.Structures, Structure{Kind: "chest", X: a.X, Y: a.Y, Owner: 0, Store: &st})
		a.Intent = &Intent{Goal: "withdraw", TargetX: a.X, TargetY: a.Y, Kind: "wood", Qty: qty}
		log := driveTicks(t, s, m, 2, nil)
		for _, e := range log {
			if e.Type == "agent.withdrew" {
				var p WithdrewPayload
				mustUnmarshal(t, e.Payload, &p)
				n = p.N
			}
		}
		ch := s.chestAt(a.X, a.Y)
		return n, a.Inv.Wood, ch.Store.Wood
	}

	t.Run("bounded_by_free_bulk", func(t *testing.T) {
		// Free bulk 3, chest holds 10 → only 3 leaves.
		n, invWood, chestWood := drive(t, Inventory{Wood: bulkCap - 3}, Inventory{Wood: 10}, 0)
		if n != 3 {
			t.Errorf("withdrew N = %d, want 3 (bounded by free bulk)", n)
		}
		if invWood != bulkCap {
			t.Errorf("carried Wood = %d, want %d (filled to the cap)", invWood, bulkCap)
		}
		if chestWood != 7 {
			t.Errorf("chest Wood = %d, want 7 (remainder left)", chestWood)
		}
	})

	t.Run("bounded_by_chest_contents", func(t *testing.T) {
		// Plenty of free bulk, chest holds only 2 → only 2 leaves.
		n, invWood, chestWood := drive(t, Inventory{}, Inventory{Wood: 2}, 0)
		if n != 2 {
			t.Errorf("withdrew N = %d, want 2 (bounded by chest contents)", n)
		}
		if invWood != 2 || chestWood != 0 {
			t.Errorf("carried/chest Wood = %d/%d, want 2/0 (chest emptied)", invWood, chestWood)
		}
	})
}

// TestChestFoodNeverSpoils is US3-AS5 / FR-010: food deposited into a chest is
// stored as plain counts — no rot batch, no spoil deadline anywhere — so it
// survives arbitrary time passing (the T032 rot sweep only ever touches ground
// piles). Proven both structurally (no spoil_at in the chest's bytes) and by
// driving well past the rot window with the food untouched.
func TestChestFoodNeverSpoils(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	a := &s.Agents[0]
	a.Inv = Inventory{FoodRaw: 6}
	s.Structures = append(s.Structures, Structure{Kind: "chest", X: a.X, Y: a.Y, Owner: 0, Store: &Inventory{}})
	a.Intent = &Intent{Goal: "deposit", TargetX: a.X, TargetY: a.Y, Kind: "food_raw"}

	driveTicks(t, s, m, 2, nil)

	ch := s.chestAt(a.X, a.Y)
	if ch == nil || ch.Store.FoodRaw != 6 {
		t.Fatalf("chest FoodRaw = %v, want 6 stored as a plain count", ch)
	}
	// Structural guarantee: a chest's contents are an Inventory with no batch /
	// spoil_at field — unlike a ground pile's FoodBatch. The bytes prove it.
	if b := mustPayload(ch); strings.Contains(string(b), "spoil_at") {
		t.Errorf("chest bytes carry a spoil deadline (%s) — chest food must never batch", b)
	}

	// Drive past the full rot window (all others dead so the long run is cheap);
	// no sweep touches the chest, so the count is exactly preserved.
	for i := 1; i < len(s.Agents); i++ {
		s.Agents[i].Dead = true
	}
	s.Agents[0].Dead = true // no reflex churn; the chest stands on its own
	driveTicks(t, s, m, rotWindowTicks+120, nil)

	ch = s.chestAt(a.X, a.Y)
	if ch == nil || ch.Store.FoodRaw != 6 {
		t.Errorf("after %d ticks chest FoodRaw = %v, want 6 unchanged (FR-010)", rotWindowTicks+120, ch)
	}
}

// TestSpearRoundTripThroughChest is the "spear in storage" edge case: a tool
// carries its remaining durability through the chest. A deposit moves the
// most-worn spears first (front of the ascending slice) and both sides stay
// sorted; a withdrawal brings the exact durabilities back.
func TestSpearRoundTripThroughChest(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	a := &s.Agents[0]
	a.Inv = Inventory{Spears: []int{1, 3}}
	// A pre-stocked chest proves merge + sort on deposit.
	s.Structures = append(s.Structures, Structure{Kind: "chest", X: a.X, Y: a.Y, Owner: 0, Store: &Inventory{Spears: []int{2}}})

	// Deposit both carried spears.
	a.Intent = &Intent{Goal: "deposit", TargetX: a.X, TargetY: a.Y, Kind: "spears"}
	driveTicks(t, s, m, 2, nil)

	ch := s.chestAt(a.X, a.Y)
	if len(ch.Store.Spears) != 3 || ch.Store.Spears[0] != 1 || ch.Store.Spears[1] != 2 || ch.Store.Spears[2] != 3 {
		t.Fatalf("chest Spears = %v, want [1 2 3] (durabilities merged, sorted ascending)", ch.Store.Spears)
	}
	if len(a.Inv.Spears) != 0 {
		t.Errorf("carried Spears = %v, want none after depositing both", a.Inv.Spears)
	}

	// Withdraw the two most-worn (Qty 2): [1 2] leave, [3] stays.
	a.Intent = &Intent{Goal: "withdraw", TargetX: a.X, TargetY: a.Y, Kind: "spears", Qty: 2}
	driveTicks(t, s, m, 5, nil)

	ch = s.chestAt(a.X, a.Y)
	if len(a.Inv.Spears) != 2 || a.Inv.Spears[0] != 1 || a.Inv.Spears[1] != 2 {
		t.Errorf("carried Spears = %v, want [1 2] (most-worn-first, durabilities preserved)", a.Inv.Spears)
	}
	if len(ch.Store.Spears) != 1 || ch.Store.Spears[0] != 3 {
		t.Errorf("chest Spears = %v, want [3] remaining", ch.Store.Spears)
	}
}

// TestReplayByteIdentityChests is SC-005 over US3: a scripted build → deposit →
// withdraw run replays from genesis (log only) to a byte-identical state hash.
func TestReplayByteIdentityChests(t *testing.T) {
	const seed = 42
	m := testMap(seed)

	bx, by, ok := findBuildTile(m, NewState(seed, m))
	if !ok {
		t.Skip("no buildable tile on this map")
	}

	genesis := func() *State {
		s := NewState(seed, m)
		for i := 1; i < len(s.Agents); i++ {
			s.Agents[i].Dead = true // quiet the run to the lone builder
		}
		a := &s.Agents[0]
		a.X, a.Y = bx, by // stand on the build tile: no walking to schedule
		a.Inv = Inventory{Planks: 8, Wood: 5, FoodRaw: 3, Spears: []int{2}}
		return s
	}

	pl := func(v any) []byte { return mustPayload(v) }
	commands := map[int64][]store.Event{
		30: {{Tick: 30, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
			Agent: 0, Goal: "build_chest", TargetX: bx, TargetY: by, Source: "planner"})}},
		700: {{Tick: 700, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
			Agent: 0, Goal: "deposit", TargetX: bx, TargetY: by, Kind: "wood", Qty: 3, Source: "planner"})}},
		760: {{Tick: 760, Type: "agent.intent_set", Payload: pl(IntentSetPayload{
			Agent: 0, Goal: "withdraw", TargetX: bx, TargetY: by, Kind: "wood", Qty: 2, Source: "planner"})}},
	}

	const ticks = 820
	live := genesis()
	log := driveTicks(t, live, m, ticks, commands)

	var sawBuilt, sawDeposit, sawWithdraw bool
	for _, e := range log {
		switch e.Type {
		case "agent.built":
			sawBuilt = true
		case "agent.deposited":
			sawDeposit = true
		case "agent.withdrew":
			sawWithdraw = true
		}
	}
	if !sawBuilt || !sawDeposit || !sawWithdraw {
		t.Fatalf("scripted run missing chest events (built %v, deposit %v, withdraw %v)", sawBuilt, sawDeposit, sawWithdraw)
	}
	ch := live.chestAt(bx, by)
	if ch == nil || ch.Owner != 0 {
		t.Fatalf("no owner-0 chest after the build: %+v", ch)
	}
	if ch.Store.Wood != 1 {
		t.Errorf("chest Wood = %d, want 1 (deposited 3, withdrew 2)", ch.Store.Wood)
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
