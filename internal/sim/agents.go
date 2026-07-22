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
type Structure struct {
	Kind      string `json:"kind"` // "fire" | "shelter" | "oven"
	X         int    `json:"x"`
	Y         int    `json:"y"`
	FuelUntil int64  `json:"fuel_until,omitempty"` // fires only
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

	eatFoodValue = 350 // one food item

	// Thresholds the reflex policy keys on.
	hungryAt    = 350
	tiredAt     = 250
	stockFoodTo = 3

	// Action durations in ticks (game seconds).
	forageTicks       = 120
	chopTicks         = 300
	buildFireTicks    = 600
	buildShelterTicks = 1200
	huntTicks         = 900

	// Yields and costs.
	chopWood        = 2
	forageYield     = 1
	huntYield       = 3
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
	// forageYield (1) / huntYield (3) above stay in force through Phase 2 so
	// agent.foraged/agent.hunted behavior is unchanged until the food rewrite.
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
	}
	return 0 // sleep / goto_warmth / wander / seek complete on arrival
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
)
