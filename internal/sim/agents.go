package sim

// Agent bodies for the executor layer (TASK-5): deterministic needs, intents,
// and inventories. All values are integers on a 0..1000 scale — integer math
// keeps decay byte-deterministic across platforms (no float rounding drift).

// AgentCount is exported for packages that size per-agent tables.
const AgentCount = 8

const agentCount = AgentCount

// AgentNames is the canonical roster; internal/persona authors a nature for
// each. Order matters (agent index = position here).
var AgentNames = [agentCount]string{"Ash", "Birch", "Cedar", "Rowan", "Fern", "Hazel", "Oak", "Sage"}

// Needs are 0..1000; 0 is lethal territory, 1000 is full.
type Needs struct {
	Health int `json:"health"`
	Food   int `json:"food"`
	Rest   int `json:"rest"`
	Warmth int `json:"warmth"`
	Morale int `json:"morale"`
}

// Inventory is what an agent carries. Spec 012 (resources/food/crafting v2)
// widened it from the legacy {wood, food} pair to the full resource/item set;
// the legacy `Food int` field is gone (the format-version bump to 2 shields old
// v1 snapshots, which never decode under v2). All counts are ints; `Spears`
// holds remaining uses per carried spear, sorted ascending (hunts spend the
// most-worn first). omitempty keeps canonical bytes stable for empty kinds.
type Inventory struct {
	Wood         int   `json:"wood"`
	Stone        int   `json:"stone,omitempty"`
	Water        int   `json:"water,omitempty"`
	Planks       int   `json:"planks,omitempty"`
	RefinedStone int   `json:"refined_stone,omitempty"`
	FoodRaw      int   `json:"food_raw,omitempty"`
	FoodCooked   int   `json:"food_cooked,omitempty"`
	Meals        int   `json:"meals,omitempty"`
	Spears       []int `json:"spears,omitempty"` // remaining uses per spear, sorted ascending
}

// Intent is one multi-step goal being executed unattended: walk to
// (TargetX, TargetY), then perform Goal there for its duration. For chopping,
// the resource (the tree) is adjacent at (ResX, ResY) while the agent stands
// on the passable target tile.
type Intent struct {
	Goal      string `json:"goal"`
	TargetX   int    `json:"target_x"`
	TargetY   int    `json:"target_y"`
	ResX      int    `json:"res_x"`
	ResY      int    `json:"res_y"`
	WorkStart int64  `json:"work_start"` // 0 until work begins at the target
	// Kind/Qty (spec 013 R4) argue the storage goals (drop/pick_up/deposit/
	// withdraw): Kind is an inventory item key ("" = all kinds), Qty the amount
	// (0 = all of kind / as much as fits). Both omitempty keep pre-013 intents
	// and every non-storage intent byte-identical.
	Kind string `json:"kind,omitempty"`
	Qty  int    `json:"qty,omitempty"`
}

type Agent struct {
	Name     string       `json:"name"`
	X        int          `json:"x"`
	Y        int          `json:"y"`
	Needs    Needs        `json:"needs"`
	Inv      Inventory    `json:"inv"`
	Asleep   bool         `json:"asleep"`
	Dead     bool         `json:"dead"`
	Intent   *Intent      `json:"intent,omitempty"`
	LastTalk int64        `json:"last_talk"`
	LastGive int64        `json:"last_give,omitempty"`
	Known    []KnownRumor `json:"known,omitempty"`
	// Memories accrete via agent.memory_added events (TASK-7); soul.md is a
	// rendered view of this list. Bounded later by TASK-9 consolidation.
	Memories []Memory `json:"memories,omitempty"`
	// IdleSince is the tick this agent last became idle/awake — reducer-
	// maintained so the reflex grace is a pure function of event history.
	IdleSince int64 `json:"idle_since"`
	// NearDeath latches the "nearly died" memory once per health collapse.
	NearDeath bool `json:"near_death,omitempty"`
	// Generation counts high-salience interrupts (TASK-32, FR-014): bumped
	// by the reducer on memories at/above GenerationBumpSalience. In-flight
	// thoughts snapshotted under an older generation are superseded at
	// landing. omitempty keeps pre-TASK-32 snapshots byte-stable.
	Generation int64 `json:"generation,omitempty"`
	// Plan is the pending guarded steps of a conditional plan (TASK-32
	// US4): the executor evaluates the head step each idle tick.
	Plan []PlanStep `json:"plan,omitempty"`
	// Nightly consolidation (TASK-9). Night/mark values of 0 mean "never" —
	// NightIndex is 1-based — so pre-TASK-9 snapshots stay correct.
	Beliefs               []Belief `json:"beliefs,omitempty"`
	Narrative             string   `json:"narrative,omitempty"`
	LastConsolidatedNight int64    `json:"last_consolidated_night,omitempty"`
	ConsolidatedUpTo      int64    `json:"consolidated_up_to,omitempty"`
	LastConsolidateMark   int64    `json:"last_consolidate_mark,omitempty"`
	// Hail (TASK-47) is the target-side pause: nil unless a talk_to landing
	// flagged this agent down. Pointer + omitempty so pre-feature snapshots
	// and un-hailed agents keep byte-identical canonical state (determinism
	// hash). Written only by the reducer.
	Hail *AgentHail `json:"hail,omitempty"`
}

// AgentHail is the courtesy pause a talk_to landing lays on its target: who
// hailed it (By) and the tick the pause lifts (Until). Denominated in game
// ticks so wall-speed changes never stretch or shrink the window.
type AgentHail struct {
	By    int   `json:"by"`
	Until int64 `json:"until"`
}

// Memory is one episodic record; salience 1..10 weights the working-memory
// window. Subject/Tone (TASK-8) mark gossip-worthy memories about another
// agent — the seeds rumors are born from (−1 subject = purely personal).
type Memory struct {
	Text     string `json:"text"`
	Salience int    `json:"salience"`
	Tick     int64  `json:"tick"`
	Subject  int    `json:"subject"`
	Tone     int    `json:"tone,omitempty"`
}

// Structure is player-visible built stuff; the map itself never contains
// structures ([[worldmap]] cold start) — they exist only as event-sourced state.
//
// FuelUntil (spec 012) applies to fires only: a fire is lit iff tick <
// FuelUntil. It is set at build (build tick + fire's initial burn window) and
// pushed forward by refuel, capped at now + fireFuelCap. Lit-ness is always
// derived, never stored as a flag. omitempty keeps shelter/oven and pre-012
// snapshots byte-identical. NOTE: warmth/burnout behavior is NOT yet wired to
// FuelUntil — that lands in Phase 4 (T019).
//
// Owner and Store (spec 013, research R8) apply to chests only: a chest rides
// the structure lifecycle rather than a parallel entity. Owner is the builder's
// agent index (permanent — no transfer/inheritance in v1); its zero-value
// round-trips unambiguously to agent 0 because every chest has an owner and
// non-chests never read the field. Store is the chest's contents, capped at
// chestCap via the same derived bulk() used for agents — chests preserve food
// indefinitely, so it needs no batches. Both omitempty keep non-chest and
// pre-013 snapshots byte-identical.
type Structure struct {
	Kind      string     `json:"kind"` // "fire" | "shelter" | "oven" | "chest"
	X         int        `json:"x"`
	Y         int        `json:"y"`
	FuelUntil int64      `json:"fuel_until,omitempty"` // fires only
	Owner     int        `json:"owner,omitempty"`      // chests only: builder agent index, permanent
	Store     *Inventory `json:"store,omitempty"`      // chests only: contents (no rot inside)
}

// FoodBatch is one drop of food on the ground with its own spoilage deadline —
// rot is per-drop (spec 013 US5), so ground food is batch-tracked (chests
// preserve food and need no batches). Kind ∈ food_raw|food_cooked|meals.
type FoodBatch struct {
	Kind    string `json:"kind"`
	N       int    `json:"n"`
	SpoilAt int64  `json:"spoil_at"` // drop/death tick + rotWindowTicks
}

// Pile is the per-tile commons of dropped/spilled goods (spec 013 US2,
// research R1) — event-sourced overlay state like Quarried, never a tile
// mutation. Non-food is flat counts (it never decays); food is batch-tracked
// in drop order (batches with identical (Kind, SpoilAt) merge). Spears carry
// their remaining uses, sorted ascending (most-worn moves first). One pile per
// tile is a reducer invariant; a pile drained to nothing is removed in the same
// reducer application. omitempty keeps the canonical bytes stable for empty
// kinds.
type Pile struct {
	X            int         `json:"x"`
	Y            int         `json:"y"`
	Wood         int         `json:"wood,omitempty"`
	Stone        int         `json:"stone,omitempty"`
	Water        int         `json:"water,omitempty"`
	Planks       int         `json:"planks,omitempty"`
	RefinedStone int         `json:"refined_stone,omitempty"`
	Spears       []int       `json:"spears,omitempty"` // remaining uses, sorted ascending
	Food         []FoodBatch `json:"food,omitempty"`   // drop order; same (Kind,SpoilAt) merges
}

// empty reports whether a pile holds nothing — the reducer removes such a pile
// in the same application that drains it (one pile per tile, zero-content piles
// removed; data-model.md).
func (p *Pile) empty() bool {
	return p.Wood == 0 && p.Stone == 0 && p.Water == 0 && p.Planks == 0 &&
		p.RefinedStone == 0 && len(p.Spears) == 0 && len(p.Food) == 0
}

// addFood merges n of a food kind into the pile: an existing batch with the
// identical (Kind, SpoilAt) absorbs the count, else a new batch appends in drop
// order (data-model.md: "same (Kind,SpoilAt) merges"). A non-positive count is
// a no-op.
func (p *Pile) addFood(kind string, n int, spoilAt int64) {
	if n <= 0 {
		return
	}
	for i := range p.Food {
		if p.Food[i].Kind == kind && p.Food[i].SpoilAt == spoilAt {
			p.Food[i].N += n
			return
		}
	}
	p.Food = append(p.Food, FoodBatch{Kind: kind, N: n, SpoilAt: spoilAt})
}

// canonicalKinds is the fixed iteration order for "all kinds" storage transfers
// (data-model.md): the Inventory field order. Determinism depends on it — a
// Kind-empty pick_up/withdraw walks these in this exact order (spec 013 US2).
var canonicalKinds = []string{
	"wood", "stone", "water", "planks", "refined_stone",
	"food_raw", "food_cooked", "meals", "spears",
}

// isFoodKind reports whether a kind is one of the batch-tracked food forms
// (the only kinds that rot in ground piles).
func isFoodKind(kind string) bool {
	return kind == "food_raw" || kind == "food_cooked" || kind == "meals"
}

// carriedCount is how many units of a kind an agent carries: spears counted
// (durability lives in the slice), every other kind its flat inventory field.
func carriedCount(inv Inventory, kind string) int {
	if kind == "spears" {
		return len(inv.Spears)
	}
	return invField(inv, kind)
}

// avail is how many units of a kind the pile holds — food summed across
// batches, spears counted, non-food the flat field. The executor reads it to
// size a pick_up; the reducer to clamp defensively (staying total).
func (p *Pile) avail(kind string) int {
	switch kind {
	case "wood":
		return p.Wood
	case "stone":
		return p.Stone
	case "water":
		return p.Water
	case "planks":
		return p.Planks
	case "refined_stone":
		return p.RefinedStone
	case "spears":
		return len(p.Spears)
	case "food_raw", "food_cooked", "meals":
		n := 0
		for _, b := range p.Food {
			if b.Kind == kind {
				n += b.N
			}
		}
		return n
	}
	return 0
}

// addNonFood adds n units of a flat (non-food, non-spear) kind. Food rides
// addFood (batches + rot deadlines); spears carry durabilities and are
// appended directly by the caller. A non-positive count is a no-op.
func (p *Pile) addNonFood(kind string, n int) {
	if n <= 0 {
		return
	}
	switch kind {
	case "wood":
		p.Wood += n
	case "stone":
		p.Stone += n
	case "water":
		p.Water += n
	case "planks":
		p.Planks += n
	case "refined_stone":
		p.RefinedStone += n
	}
}

// takeNonFood removes up to n of a flat kind, returning the actual amount
// removed (clamped to what the pile holds — the reducer stays total).
func (p *Pile) takeNonFood(kind string, n int) int {
	if a := p.avail(kind); n > a {
		n = a
	}
	if n <= 0 {
		return 0
	}
	switch kind {
	case "wood":
		p.Wood -= n
	case "stone":
		p.Stone -= n
	case "water":
		p.Water -= n
	case "planks":
		p.Planks -= n
	case "refined_stone":
		p.RefinedStone -= n
	}
	return n
}

// takeFood removes up to n units of a food kind from the OLDEST matching
// batches first (drop order = creation order = oldest), returning the actual
// amount removed. Emptied batches are compacted out, preserving drop order.
func (p *Pile) takeFood(kind string, n int) int {
	if n <= 0 {
		return 0
	}
	taken := 0
	out := p.Food[:0]
	for _, b := range p.Food {
		if b.Kind == kind && taken < n {
			t := b.N
			if t > n-taken {
				t = n - taken
			}
			b.N -= t
			taken += t
		}
		if b.N > 0 {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		p.Food = nil
	} else {
		p.Food = out
	}
	return taken
}

// takeSpears removes the n most-worn spears (front of the ascending-sorted
// slice) and returns their durabilities; the pile stays sorted ascending.
func (p *Pile) takeSpears(n int) []int {
	if n > len(p.Spears) {
		n = len(p.Spears)
	}
	if n <= 0 {
		return nil
	}
	taken := append([]int(nil), p.Spears[:n]...)
	rest := append([]int(nil), p.Spears[n:]...)
	if len(rest) == 0 {
		p.Spears = nil
	} else {
		p.Spears = rest
	}
	return taken
}

// Harvest marks a foraged tile regrowing at Regrow.
type Harvest struct {
	X      int   `json:"x"`
	Y      int   `json:"y"`
	Regrow int64 `json:"regrow"`
}

// DenUse marks a hunted den not huntable again until Ready.
type DenUse struct {
	X     int   `json:"x"`
	Y     int   `json:"y"`
	Ready int64 `json:"ready"`
}

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// --- executor tuning (game-minutes are the decay heartbeat) ---

const (
	// Per-game-minute needs deltas.
	foodDecay      = 1 // full → empty in ~16.6 game-hours
	restDecayAwake = 1
	restRegenSleep = 4 // full recharge in ~4 game-hours
	warmthLossCold = 4 // night, outdoors, no fire: full → 0 in ~4 game-hours
	warmthGainFire = 6
	warmthGainDay  = 2
	healthLoss     = 3 // per minute while starving or freezing (~5.5h to die)
	healthRegen    = 1 // fed and rested

	// Thresholds the reflex policy keys on.
	hungryAt = 350
	tiredAt  = 250

	// Action durations in ticks (game seconds).
	forageTicks       = 120
	chopTicks         = 300
	buildFireTicks    = 600
	buildShelterTicks = 1200
	huntTicks         = 900

	// Yields and costs.
	chopWood        = 2
	fireWoodCost    = 2
	shelterWoodCost = 5

	// Cadences and ranges.
	moveEveryTicks = 5 // 12 tiles per game-minute
	fireWarmRadius = 2 // Manhattan
	// TASK-7: the reflex is the fallback mind — it only acts on agents idle
	// past this grace, leaving room for planner injections.
	reflexGraceTicks = 120 // 2 game-minutes
	// PlannerCadenceTicks is the mind driver's per-agent baseline.
	PlannerCadenceTicks = 1800 // 30 game-minutes
	witnessRadius       = 8
	nearDeathBelow      = 200
	nearDeathResetAt    = 400
	coldNightBelow      = 350
	forageRegrowSec     = 12 * 3600
	denCooldownSec      = 6 * 3600
	talkCooldownSec     = 2 * 3600
	talkMoraleBonus     = 50
)

// --- spec 012 resources/food/crafting v2 tuning ---
//
// The single scalar tuning surface for the v2 economy, mirrored in
// specs/012-resources-food-crafting/contracts/recipes.md and the recipe table
// (recipes.go). Ticks are game-seconds (a game-hour is 3600 ticks). Phase 2
// only declares these; behavior is wired to them in later phases (T013–T037),
// so several are intentionally unused until then (package-level constants may
// be unused without a compile error).
const (
	// Food restore per unit eaten, on the 0..1000 need scale (cooking ~doubles
	// raw; the meal is the best food). Eating stops at Food >= satietyAt.
	foodRawRestore    = 40
	foodCookedRestore = 80
	mealRestore       = 100
	satietyAt         = 900

	// Reflex larder target (T018): idle agents top up carried raw food to this
	// many units before wandering. Restates the legacy stock-3-meals prep rule
	// over the finer raw unit (contracts/recipes.md sizing).
	stockFoodRawTo = 8

	// refuelDyingBelow (T020): the reflex refuels a fire whose remaining fuel
	// has dropped below this window (1 game-hour) when carrying wood.
	refuelDyingBelow = 3600

	// Fire fuel window. A fresh fire (2 wood) burns 2×fireBurnPerWood; each
	// refuel (1 wood) adds fireBurnPerWood, truncated to now + fireFuelCap.
	fireBurnPerWood = 4 * 3600  // 4 game-hours per wood
	fireFuelCap     = 12 * 3600 // remaining fuel ceiling

	// Spear durability: hunts a spear lasts before breaking.
	spearDurability = 3

	// Rest regen per game-minute while asleep on a shelter tile (else
	// restRegenSleep = 4).
	restRegenShelter = 6

	// Bath effects at an oven (absolute post-values are carried in the event;
	// these are the pre-cap bumps applied, gru-pattern).
	bathMorale = 150
	bathWarmth = 300

	// v2 gather rescale (wired T013 quarry/water, T017 forage/hunt). The legacy
	// forageYield/huntYield constants are gone (T017): agent.foraged now yields
	// forageYieldV2 FoodRaw, agent.hunted huntYieldBare (spear boost is T027).
	quarryYield       = 2
	quarryTicks       = 400
	collectWaterYield = 1
	collectWaterTicks = 60
	forageYieldV2     = 2
	huntYieldBare     = 8
	huntYieldSpear    = 12
	huntTicksSpear    = 600

	// Hand-craft / build / station recipe magnitudes (mirrored by recipes.go;
	// wired T026/T030–T037).
	plankYield       = 4
	craftPlanksTicks = 180
	craftStoneTicks  = 180
	craftSpearTicks  = 240
	shelterPlankCost = 8
	buildOvenTicks   = 900
	ovenBatchSize    = 8
	cookFireTicks    = 240
	cookOvenTicks    = 360
	batheTicks       = 240
)

// --- spec 013 inventory/storage v1 tuning ---
//
// The scalar tuning surface for the storage layer, mirrored in
// specs/013-inventory-storage/data-model.md. Ticks are game-seconds. Phase 2
// only declares these; behavior is wired to them in later phases (US1–US5,
// migration), so several are intentionally unused until then.
const (
	bulkCap        = 24     // per-villager carried bulk ceiling
	chestCap       = 48     // per-chest stored bulk ceiling
	chestPlankCost = 6      // build_chest recipe input
	rotWindowTicks = 172800 // 2 game days: ground-pile food batch lifetime

	// Taking (theft) social marks — the deltas a non-owner withdrawal lays
	// through the existing relation/memory machinery (research R5).
	theftTrustDelta     = -120 // owner→taker trust on a taking
	theftAffectionDelta = -40  // owner→taker affection on a taking
	theftMemoryTone     = -60  // owner/witness memory tone (gossip seed)
)

// bulk is an agent's (or a chest's) carried load: one per unit of every
// inventory kind plus one per carried spear (data-model.md). Derived, never
// stored — same doctrine as fire lit-ness from FuelUntil: a derived value
// cannot drift from its parts. bulkCap (24) exceeds the largest single yield
// (spear hunt, 12), so no completion is unsatisfiable from empty (research R2).
// Chest capacity uses this same function over *Store.
func bulk(inv Inventory) int {
	return inv.Wood + inv.Stone + inv.Water + inv.Planks + inv.RefinedStone +
		inv.FoodRaw + inv.FoodCooked + inv.Meals + len(inv.Spears)
}

// freeBulk is the remaining carry capacity under the cap: bulkCap − bulk(inv),
// floored at zero (a defensively over-cap inventory reports no free space, never
// a negative). The reducer yield clamps (US1-AS2) and the executor completion
// re-validation (US1-AS1) share it: a full pouch reports zero, a partially full
// one the exact remainder (research R2). Pure function of pre-event Inv, so
// replay is byte-identical.
func freeBulk(inv Inventory) int {
	if f := bulkCap - bulk(inv); f > 0 {
		return f
	}
	return 0
}

// BulkCap and Bulk are bulkCap/bulk exported for internal/tui (SC-006: "how
// full a villager's hands are" must be answerable from the TUI alone),
// mirroring the MetatronChargeCap export pattern for the same purpose —
// the sim package stays the single source of truth for the derived value
// and its ceiling.
const BulkCap = bulkCap

// ChestCap is chestCap exported for internal/tui (SC-006: "what's in a given
// chest, and is it full" must be answerable from the TUI alone), mirroring the
// BulkCap export — sim stays the single source of truth for the per-chest
// stored-bulk ceiling and its display.
const ChestCap = chestCap

func Bulk(inv Inventory) int {
	return bulk(inv)
}

func intentDuration(goal string) int64 {
	switch goal {
	case "forage":
		return forageTicks
	case "chop":
		return chopTicks
	case "build_fire":
		return buildFireTicks
	case "build_shelter":
		return buildShelterTicks
	case "hunt":
		return huntTicks
	case "quarry":
		return quarryTicks
	case "collect_water":
		return collectWaterTicks
	case "cook":
		// Fire cooking base case; a cook intent resolved at an oven takes
		// longer — that override happens in the executor (workDuration,
		// T031), since the station is only known from the intent's target.
		return cookFireTicks
	case "craft_planks":
		return craftPlanksTicks
	case "craft_stone":
		return craftStoneTicks
	case "craft_spear":
		return craftSpearTicks
	case "build_oven":
		return buildOvenTicks
	case "build_chest":
		// Spec 013 US3: fire-comparable build time (the build_chest recipe row's
		// Duration). Timed work, unlike the instant deposit/withdraw goals.
		return buildFireTicks
	case "bathe":
		return batheTicks
	}
	return 0 // sleep / goto_warmth / wander / seek / refuel_fire complete on arrival
}

// --- event payloads ---

type (
	IntentSetPayload struct {
		Agent   int    `json:"agent"`
		Goal    string `json:"goal"`
		TargetX int    `json:"target_x"`
		TargetY int    `json:"target_y"`
		ResX    int    `json:"res_x"`
		ResY    int    `json:"res_y"`
		Source  string `json:"source,omitempty"` // "reflex" | "planner"
		// Kind/Qty (spec 013 R4) carry a storage goal's argument onto the intent;
		// omitempty keeps pre-013 and non-storage intent_set payloads byte-identical.
		Kind string `json:"kind,omitempty"`
		Qty  int    `json:"qty,omitempty"`
	}
	WorkStartedPayload struct {
		Agent int   `json:"agent"`
		Tick  int64 `json:"tick"`
	}
	HarvestPayload struct { // foraged / chopped / hunted / built site
		Agent int `json:"agent"`
		X     int `json:"x"`
		Y     int `json:"y"`
	}
	BuiltPayload struct {
		Agent int    `json:"agent"`
		Kind  string `json:"kind"`
		X     int    `json:"x"`
		Y     int    `json:"y"`
	}
	NeedsPayload struct {
		Agent  int `json:"agent"`
		Health int `json:"health"`
		Food   int `json:"food"`
		Rest   int `json:"rest"`
		Warmth int `json:"warmth"`
		Morale int `json:"morale"`
	}
	DiedPayload struct {
		Agent int    `json:"agent"`
		Cause string `json:"cause"` // "starvation" | "exposure" | "collapse"
	}
	TalkedPayload struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	RegrownPayload struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	MemoryAddedPayload struct {
		Agent    int    `json:"agent"`
		Text     string `json:"text"`
		Salience int    `json:"salience"`
		Subject  int    `json:"subject"`
		Tone     int    `json:"tone,omitempty"`
	}
	ThoughtPayload struct {
		Agent  int    `json:"agent"`
		Text   string `json:"text"`
		Source string `json:"source"` // "planner" (reflex acts without narrating)
	}
	// Hail lifecycle (TASK-47). from = hailer, to = target — the field names
	// the chronicle grammar already resolves to agent names, so tail/TUI
	// visibility lands with no view-layer change.
	HailedPayload struct {
		From  int   `json:"from"`
		To    int   `json:"to"`
		Until int64 `json:"until"`
	}
	HailMetPayload struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	HailExpiredPayload struct {
		From int `json:"from"`
		To   int `json:"to"`
	}

	// --- spec 012 resources/food/crafting v2 payloads ---
	// Field order below is the canonical serialization order (see
	// contracts/events.md); all outcomes are absolute (no deltas, no dice).

	// CraftedPayload: a completed hand-craft. Kind ∈ planks|refined_stone|spear;
	// the reducer applies the recipe delta from recipes.go.
	CraftedPayload struct {
		Agent int    `json:"agent"`
		Kind  string `json:"kind"`
	}
	// AtePayload replaces the old empty AgentPayload for agent.ate (the format
	// bump shields old logs): counts consumed per form plus the absolute
	// post-eat food need. Wired in Phase 4 (T018).
	AtePayload struct {
		Agent     int `json:"agent"`
		Meals     int `json:"meals"`
		Cooked    int `json:"cooked"`
		Raw       int `json:"raw"`
		FoodAfter int `json:"food_after"`
	}
	// CookedPayload: a cook batch. Station ∈ fire|oven; Kind ∈
	// food_cooked|meals. Consumed FoodRaw → Produced of Kind.
	CookedPayload struct {
		Agent    int    `json:"agent"`
		Station  string `json:"station"`
		Consumed int    `json:"consumed"`
		Produced int    `json:"produced"`
		Kind     string `json:"kind"`
	}
	// BathedPayload: a bath at an oven — absolute post-cap need values
	// (gru-pattern).
	BathedPayload struct {
		Agent       int `json:"agent"`
		MoraleAfter int `json:"morale_after"`
		WarmthAfter int `json:"warmth_after"`
	}
	// RefueledPayload: a fire refuel (planner or reflex). FuelUntil is the
	// absolute new deadline (already capped by the emitter).
	RefueledPayload struct {
		Agent     int   `json:"agent"`
		X         int   `json:"x"`
		Y         int   `json:"y"`
		FuelUntil int64 `json:"fuel_until"`
	}
	// SpearBrokePayload: the spear that spent its last use, alongside the hunt
	// completion; a companion memory rides the same batch.
	SpearBrokePayload struct {
		Agent int `json:"agent"`
	}
	// FireBurnedOutPayload: the fuel sweep's once-per-burnout signal. No state
	// effect (lit-ness is derived from FuelUntil); chronicle/TUI material.
	FireBurnedOutPayload struct {
		X int `json:"x"`
		Y int `json:"y"`
	}

	// --- spec 013 inventory/storage v1 payloads ---
	// Field order below is the canonical serialization order (see
	// contracts/events.md); every count is the ACTUAL post-clamp moved amount
	// (outcome-only), never a request.

	// DroppedPayload: n of kind left on the agent's tile, created-or-merged into
	// the tile's pile (food becomes a batch stamped tick + rotWindowTicks;
	// spears move most-worn-first with their durabilities).
	DroppedPayload struct {
		Agent int    `json:"agent"`
		X     int    `json:"x"`
		Y     int    `json:"y"`
		Kind  string `json:"kind"`
		N     int    `json:"n"`
	}
	// PickedUpPayload: n of kind taken from the tile's pile (food oldest-batch-
	// first), truncated to free bulk; one event per kind moved in the batch.
	PickedUpPayload struct {
		Agent int    `json:"agent"`
		X     int    `json:"x"`
		Y     int    `json:"y"`
		Kind  string `json:"kind"`
		N     int    `json:"n"`
	}
	// DepositedPayload: n of kind moved from inventory into the chest at (x,y),
	// truncated to chest free space (chestCap − bulk(*Store)).
	DepositedPayload struct {
		Agent int    `json:"agent"`
		X     int    `json:"x"`
		Y     int    `json:"y"`
		Kind  string `json:"kind"`
		N     int    `json:"n"`
	}
	// WithdrewPayload: n of kind taken from the chest at (x,y) into inventory,
	// truncated to the taker's free bulk. Owner is the chest's owner index; a
	// non-owner taker co-emits the theft companion batch (contracts/events.md).
	WithdrewPayload struct {
		Agent int    `json:"agent"`
		X     int    `json:"x"`
		Y     int    `json:"y"`
		Kind  string `json:"kind"`
		N     int    `json:"n"`
		Owner int    `json:"owner"`
	}
	// FoodRottedPayload: n of a food kind removed from the pile at (x,y) by the
	// per-game-minute rot sweep (same-kind batches merged per pile per sweep).
	FoodRottedPayload struct {
		X    int    `json:"x"`
		Y    int    `json:"y"`
		Kind string `json:"kind"`
		N    int    `json:"n"`
	}
)
