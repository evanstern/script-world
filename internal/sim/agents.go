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

type Inventory struct {
	Wood int `json:"wood"`
	Food int `json:"food"`
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
	// Nightly consolidation (TASK-9). Night/mark values of 0 mean "never" —
	// NightIndex is 1-based — so pre-TASK-9 snapshots stay correct.
	Beliefs               []Belief `json:"beliefs,omitempty"`
	Narrative             string   `json:"narrative,omitempty"`
	LastConsolidatedNight int64    `json:"last_consolidated_night,omitempty"`
	ConsolidatedUpTo      int64    `json:"consolidated_up_to,omitempty"`
	LastConsolidateMark   int64    `json:"last_consolidate_mark,omitempty"`
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
type Structure struct {
	Kind string `json:"kind"` // "fire" | "shelter"
	X    int    `json:"x"`
	Y    int    `json:"y"`
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
)
