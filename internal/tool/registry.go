package tool

// registry is the authoritative collection of every tool, in registration
// order. The world tools come first, in exactly today's goal-vocabulary order
// (internal/mind/prompt.go) — that order is the byte-identity anchor for the
// derived prompt string (SC-003, R3). The expressive tools and the metatron
// tools follow.
//
// itemKinds are the inventory keys the storage verbs (drop/pick_up/deposit/
// withdraw) accept as their `kind` argument — mirrors internal/sim's canonical
// kinds. Declared here as the storage verbs' Enum descriptor; the parser's
// own validKinds set (internal/mind/parse.go) is NOT migrated in this layer
// (it is not a capability-vocabulary list). The empty "all kinds" case is the
// omitted argument, not an enum value.
var itemKinds = []string{
	"wood", "stone", "water", "planks", "refined_stone",
	"food_raw", "food_cooked", "meals", "spears",
}

// storageParams is the shared `kind`+`qty` descriptor for the four storage
// verbs (drop/pick_up/deposit/withdraw; build_chest takes neither). `qty` is
// a Number param (Min 1, Max unbounded) — spec 017 R12 pays the spec-014 debt
// this comment used to flag (qty had no representable ParamKind). Both stay
// optional: validateKindQty (internal/mind/parse.go) remains the free-text
// path's enforcer; these Params now also drive InputSchema (derive.go) for
// the tool-use loop.
func storageParams() []Param {
	return []Param{
		{Name: "kind", Kind: Enum, Required: false, Enum: itemKinds},
		{Name: "qty", Kind: Number, Required: false, Min: 1},
	}
}

// The gloss lines carried byte-exact from internal/mind/prompt.go (the prose
// between "Goals:" and "For a short sequence"). Each is attached to the first
// verb it describes so PromptGlossBlock, walking registration order, rebuilds
// the block exactly (R3). Raw string literals preserve the embedded quotes and
// em-dash verbatim.
const (
	glossQuarry     = `quarry gathers stone from a rock outcrop; collect_water gathers water from a water tile.`
	glossCook       = `cook turns raw food into fire-cooked food (worth double) at a lit fire, or into meals (the best food) at an oven; refuel_fire feeds one wood to a fire to keep it burning (or relight a cold one).`
	glossCraft      = `craft_planks turns 1 wood into 4 planks; craft_stone turns 1 stone into 1 refined stone; craft_spear needs 1 wood + 1 refined stone and makes a spear (breaks after 3 hunts) — all hand-crafted anywhere, no travel needed.`
	glossBuildOven  = `build_oven needs 4 refined stone + 2 planks and lets you cook meals and bathe; bathe at an oven spends 1 water + 1 wood for warmth and morale.`
	glossDrop       = `drop puts down goods where you stand, creating or adding to a ground pile there anyone can take from; pick_up takes from a pile on or next to you. Name what with "kind" (wood, stone, water, planks, refined_stone, food_raw, food_cooked, meals, or spears) and how much with "qty" (omit or 0 = all of that kind); pick_up with no "kind" takes everything that fits.`
	glossBuildChest = `build_chest needs 6 planks; chests preserve food (it never rots there, unlike a ground pile) and you keep the chest permanently once built. deposit stores goods in the nearest chest — always name a "kind" or nothing moves; withdraw takes goods back out of the nearest chest that has them ("kind" omitted or empty takes everything that fits). Taking from another villager's chest is possible too, but they will remember who took it.`
)

// Durations carried verbatim from internal/sim/agents.go (the intentDuration
// switch and its constants). The registry is now the source; the sim duration
// table (agents.go) derives from these. Context overrides (spear hunt, oven
// cook) stay in the executor's workDuration and are not registry data (R7).
var registry = []Tool{
	// --- World tools (villager roster; goal-vocabulary order) ---
	{Name: "forage", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 120}, PlanStep: true, ReflexEligible: true},
	{Name: "chop", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 300}, PlanStep: true, ReflexEligible: true},
	{Name: "hunt", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 900}, PlanStep: true, ReflexEligible: true},
	{Name: "build_fire", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 600}, PlanStep: true, ReflexEligible: true},
	{Name: "build_shelter", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 1200}, PlanStep: true},
	{Name: "eat", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 0}, PlanStep: true, ReflexEligible: true},
	{Name: "sleep", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 0}, PlanStep: true, ReflexEligible: true},
	{Name: "wander", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 0}, PlanStep: true, ReflexEligible: true},
	{Name: "goto_warmth", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 0}, PlanStep: true, ReflexEligible: true},
	{Name: "talk_to", Effect: World, Gate: Resolvable, Params: []Param{{Name: "target", Kind: AgentName, Required: true}}, Cost: Cost{DurationTicks: 0}, PlanStep: true},
	{Name: "quarry", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 400}, PlanStep: true, PromptGloss: glossQuarry},
	{Name: "collect_water", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 60}, PlanStep: true},
	{Name: "cook", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 240}, PlanStep: true, PromptGloss: glossCook},
	{Name: "refuel_fire", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 0}, PlanStep: true, ReflexEligible: true},
	{Name: "craft_planks", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 180}, PlanStep: true, PromptGloss: glossCraft},
	{Name: "craft_stone", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 180}, PlanStep: true},
	{Name: "craft_spear", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 240}, PlanStep: true},
	{Name: "build_oven", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 900}, PlanStep: true, PromptGloss: glossBuildOven},
	{Name: "bathe", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 240}, PlanStep: true},
	{Name: "drop", Effect: World, Gate: Resolvable, Params: storageParams(), Cost: Cost{DurationTicks: 0}, PlanStep: true, PromptGloss: glossDrop},
	{Name: "pick_up", Effect: World, Gate: Resolvable, Params: storageParams(), Cost: Cost{DurationTicks: 0}, PlanStep: true},
	{Name: "build_chest", Effect: World, Gate: Resolvable, Cost: Cost{DurationTicks: 600}, PlanStep: true, PromptGloss: glossBuildChest},
	{Name: "deposit", Effect: World, Gate: Resolvable, Params: storageParams(), Cost: Cost{DurationTicks: 0}, PlanStep: true},
	{Name: "withdraw", Effect: World, Gate: Resolvable, Params: storageParams(), Cost: Cost{DurationTicks: 0}, PlanStep: true},

	// --- Expressive tools (villager roster) ---
	{Name: "say", Effect: Expressive, Gate: Scene,
		Params: []Param{{Name: "text", Kind: Text, MaxBytes: 300}},
		Cost:   Cost{TextCapBytes: 300},
		Events: []string{"social.conversation_turn"}},
	{Name: "gist", Effect: Expressive, Gate: Scene,
		// The gist's topics/tones sub-fields (parseOutcome) are not modeled as
		// Params in this layer; only the bounded gist text is. The parser stays
		// the enforcer of the full shape.
		Params: []Param{{Name: "gist", Kind: Text, MaxBytes: 200}},
		Cost:   Cost{TextCapBytes: 200},
		Events: []string{"social.conversation", "social.relation_changed", "social.rumor_told", "agent.memory_added"}},
	{Name: "muse", Effect: Expressive, Gate: None,
		Params: []Param{{Name: "text", Kind: Text, MaxRunes: 200}},
		Cost:   Cost{TextCapRunes: 200},
		Events: []string{"agent.thought"}},

	// --- Metatron tools (metatron roster) ---
	// converse produces a transcript reply and lands NO world events. It is the
	// metatron's expressive speech channel, so it is Effect Expressive with an
	// empty Events set — the one eventless expressive tool (see Validate: Events
	// are required to be ⊆ whitelist but are not required to be non-empty, so an
	// eventless expressive tool is legal).
	{Name: "converse", Effect: Expressive, Gate: None,
		Params: []Param{{Name: "text", Kind: Text}}},
	{Name: "nudge_dream", Effect: Expressive, Gate: Charge,
		Params: []Param{{Name: "target", Kind: AgentName, Required: true}, {Name: "text", Kind: Text, MaxBytes: 400}},
		Cost:   Cost{Charges: 1, TextCapBytes: 400},
		Events: []string{"metatron.nudged", "agent.memory_added"}},
	{Name: "nudge_omen", Effect: Expressive, Gate: Charge,
		Params: []Param{{Name: "text", Kind: Text, MaxBytes: 400}},
		Cost:   Cost{Charges: 1, TextCapBytes: 400},
		Events: []string{"metatron.nudged", "agent.memory_added"}},
}

// byName indexes the registry for O(1) Lookup, built once at init.
var byName = func() map[string]Tool {
	m := make(map[string]Tool, len(registry))
	for _, t := range registry {
		m[t.Name] = t
	}
	return m
}()

// All returns every registered tool in registration order (stable; the
// prompt-derivation order anchor). The returned slice is a copy — callers may
// not mutate the registry.
func All() []Tool {
	out := make([]Tool, len(registry))
	copy(out, registry)
	return out
}

// Lookup returns the tool by name; ok is false for unknown names.
func Lookup(name string) (Tool, bool) {
	t, ok := byName[name]
	return t, ok
}
