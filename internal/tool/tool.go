// Package tool is the single source of truth for agent capabilities (spec
// 014, Layer 1). Every capability an agent can request — the world verbs,
// speaking, musing, conversation gists, the metatron's converse and nudges —
// is one Tool entry: a name, a parameter schema, a gate class, an effect
// class, and a cost. The prompt vocabulary the models see, the mind-side
// validation of replies, and the sim-door validation of arriving actions are
// all DERIVED from these entries (derive.go), so none of them can drift.
//
// This is a leaf package: it imports nothing internal (research R1), following
// the internal/cognition precedent. Declarative data lives here; the behavior
// that grounds a tool in time and space (resolution, execution, reduction)
// stays in internal/sim and internal/metatron, keyed by tool name. A tool call
// is a REQUEST; an event is the FACT; the gate decides; the executor grounds
// the work. The model never asserts an outcome.
package tool

// EffectClass is how a tool's use reaches the world (data-model.md).
type EffectClass int

const (
	// World tools produce an Intent the executor grounds in time and space
	// (landing path: Loop.InjectIntent).
	World EffectClass = iota
	// Expressive tools produce an immediate, bounded batch of whitelisted
	// events (landing path: Loop.InjectSocial).
	Expressive
	// Read tools return data into cognition and produce no events. Declared as
	// a class (FR-002) but carry zero entries in this layer — the tool-use loop
	// (TASK-52) populates them.
	Read
)

func (e EffectClass) String() string {
	switch e {
	case World:
		return "world"
	case Expressive:
		return "expressive"
	case Read:
		return "read"
	}
	return "unknown"
}

// ParamKind is the minimal descriptor vocabulary (research R8): enough to
// describe today's arguments and to feed TASK-52's tool-call parser. No JSON
// Schema, no reflection.
type ParamKind int

const (
	// AgentName is a villager name (talk_to's target, nudge_dream's target).
	AgentName ParamKind = iota
	// Text is free text, bounded by MaxBytes/MaxRunes (say, muse, nudge text).
	Text
	// Enum is a closed set of string values, listed in Param.Enum (the storage
	// verbs' item kind).
	Enum
)

// Param is one argument a tool accepts.
type Param struct {
	Name     string
	Kind     ParamKind
	Required bool
	MaxBytes int      // 0 = n/a
	MaxRunes int      // 0 = n/a
	Enum     []string // the allowed values when Kind == Enum; nil otherwise
}

// GateClass names the precondition family checked against live state before
// anything lands (data-model.md Gate). It is declarative: the enforcing code
// stays in sim/metatron; the class lets the derivation tests assert coverage.
type GateClass int

const (
	// Resolvable — resolveGoal must produce an intent against live state
	// (every world tool).
	Resolvable GateClass = iota
	// Charge — the nudge bank must hold at least Cost.Charges (the reducer
	// dry-run enforces it).
	Charge
	// Scene — an active conversation scene must exist (say, gist).
	Scene
	// None — no precondition (muse, converse).
	None
)

// Cost is what using a tool spends: time (world-tool work duration), charges
// (metatron nudges), or a text-size budget (utterance/gist/musing/nudge caps).
type Cost struct {
	DurationTicks int64 // world tools: base work ticks (0 = instant on arrival)
	Charges       int   // nudges: 1; all others 0
	TextCapBytes  int   // say 300, gist 200, nudge 400 (bytes)
	TextCapRunes  int   // muse 200 (runes)
}

// Tool is one capability an agent can request — the unit of the registry.
type Tool struct {
	Name           string
	Effect         EffectClass
	Params         []Param
	Gate           GateClass
	Cost           Cost
	PlanStep       bool     // may appear as a guarded plan step (world tools)
	ReflexEligible bool     // doctrine data only; decideIntent stays hand-written (R6)
	PromptGloss    string   // the verb's prompt documentation line(s); "" when none
	Events         []string // expressive tools: event types this tool may land (⊆ whitelist)
}
