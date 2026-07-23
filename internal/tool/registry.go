package tool

import "encoding/json"

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

// ItemKinds returns a copy of the storage verbs' item-kind vocabulary, in the
// order above. Exported for set_plan's authored schema (below) and for
// internal/mind's cross-check test pinning this list against its own
// (deliberately unmigrated, per spec 014) validKinds set.
func ItemKinds() []string {
	out := make([]string, len(itemKinds))
	copy(out, itemKinds)
	return out
}

// PlanStepCap is the maximum number of steps set_plan's `steps` array may
// carry. It mirrors sim.PlanStepCap and internal/mind's own planStepCap
// constant (parse.go) — declared here, not imported, because internal/tool
// is a leaf package (research R1) and cannot import internal/mind or
// internal/sim. internal/mind's TestPlanStepCapMirrorsTool pins the two
// constants equal so this literal cannot drift silently.
const PlanStepCap = 3

// setPlanSchema is set_plan's authored InputSchemaJSON override (spec 017
// R11): the registry's scalar Param model has no ParamKind for a steps
// array, so this tool's schema is hand-built instead of derived from Params.
// goal's enum is legacyWorldNamesFrom(worldTools) — the SAME filter
// (isLegacyWorldTool, derive.go) that builds VocabularyLine/WorldGoals — so
// set_plan's step vocabulary can never drift from the legacy goal vocabulary
// it re-expresses. It takes goals as a parameter, rather than calling
// derive.go's legacyWorldNames() directly, because that walks the package
// `registry` var — and registry's own literal (below) needs this schema to
// build set_plan, which would be an initialization cycle (registry ->
// setPlanSchema -> legacyWorldNames -> registry). Building from worldTools
// (a plain literal with no such dependency) breaks the cycle while deriving
// the identical set. kind's enum is itemKinds, the storage verbs' own Enum
// descriptor.
func setPlanSchema(goals []string) json.RawMessage {
	step := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"goal": map[string]any{"type": "string", "enum": goals},
			"kind": map[string]any{"type": "string", "enum": itemKinds},
			"qty":  map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"goal"},
		"additionalProperties": false,
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"steps": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": PlanStepCap,
				"items":    step,
			},
			// Optional plan-level reason (spec 019 R12 / T024): the model's why
			// for the whole plan, threaded to InjectArgs.Reason (agent.thought
			// narration). Capability-only description, not required.
			"reason": map[string]any{"type": "string", "maxLength": ReasonCapRunes,
				"description": "optionally, why you're doing this"},
		},
		"required":             []string{"steps"},
		"additionalProperties": false,
	}
	b, err := json.Marshal(schema)
	if err != nil {
		// schema is built from literal Go data; marshaling cannot fail.
		panic("tool: setPlanSchema marshal: " + err.Error())
	}
	return b
}

// miracleKinds is the metatron turn's miracle vocabulary — the four kinds the
// angel's work_miracle tool (spec 017 T019b) and internal/metatron's landMiracle
// / BuildMiracleBatch (spec 016 turn contract) accept. It is declared here as
// work_miracle's Enum descriptor rather than imported: internal/tool is a leaf
// (research R1) and cannot see internal/metatron or internal/sim, so the
// canonical list is MIRRORED, and internal/metatron's TestMiracleKindsMirrorTool
// pins it equal to BuildMiracleBatch's accepted set so it cannot drift.
var miracleKinds = []string{"move", "remove", "give_item", "time_snap"}

// MiracleKinds returns a copy of work_miracle's kind vocabulary, in the order
// above. Exported for internal/metatron's drift cross-check test.
func MiracleKinds() []string {
	out := make([]string, len(miracleKinds))
	copy(out, miracleKinds)
	return out
}

// miracleCosts is THE authoritative per-kind charge price for a miracle (spec
// 021 R7 / FR-009 / SC-004): the single source from which the reducer's
// enforcement (sim.miracleCost, derived via MiracleCostsByEvent) and every
// model/player-facing rendering (MetatronToolGuidance) both draw, so a price
// change here propagates everywhere with no second edit. The time snap is the
// dear one (2 charges); every other miracle costs 1. Keyed lookups only —
// never iterated into ordered output (determinism).
var miracleCosts = map[string]int{
	"move":      1,
	"remove":    1,
	"give_item": 1,
	"time_snap": 2,
}

// kindToEvent maps a miracle kind to the store event type it lands as — the
// bridge that lets sim (whose reducer keys its enforcement by the arriving
// event's type, not the model's kind string) derive its cost table from this
// kind-keyed one. Declared beside miracleKinds so the kind vocabulary and its
// event mapping stay together. Keyed lookups only.
var kindToEvent = map[string]string{
	"move":      "metatron.entity_moved",
	"remove":    "metatron.entity_removed",
	"give_item": "metatron.item_granted",
	"time_snap": "metatron.time_snapped",
}

// MiracleCost returns the charge price of a miracle kind; ok is false for an
// unknown kind. THE authoritative price accessor (SC-004): the guidance prose
// renders from here, so a described cost can never drift from the enforced one.
func MiracleCost(kind string) (int, bool) {
	c, ok := miracleCosts[kind]
	return c, ok
}

// MiracleCostsByEvent returns a fresh map from miracle EVENT TYPE to charge
// price — the shape sim's reducer keys on. Built by walking the ordered kind
// vocabulary and resolving each kind's event and cost through keyed lookups
// only, so the result carries exactly the four metatron.* miracle event types
// and is deterministic. sim.miracleCost IS this map (spec 021 R7), replacing
// the literal that used to be sim's own second copy of the table.
func MiracleCostsByEvent() map[string]int {
	out := make(map[string]int, len(miracleKinds))
	for _, k := range miracleKinds {
		out[kindToEvent[k]] = miracleCosts[k]
	}
	return out
}

// miracleParams is work_miracle's flat parameter surface (spec 017 T019b,
// mirroring spec 016's turn contract). `kind` is the sole required argument;
// every other field is optional because the needed set is per-kind (move:
// class/x/y/to_x/to_y; remove: class/x/y; give_item: villager/item/qty;
// time_snap: day/time). The reducer dry-run enforces the per-kind requirements
// at the door (metatron.landMiracle → BuildMiracleBatch → InjectSocial), and the
// loop feeds a rejection back for repair, so a permissive flat schema is exactly
// right — the door is the semantic authority, this only rejects shapes the model
// can fix (wrong scalar type, missing kind, unknown kind).
//
// There is deliberately NO `gratis` parameter: the angel can NEVER waive a
// charge (spec 016 FR-007/SC-005). Structural absence — not sanitizing a field
// out — is the guarantee, exactly as the retired turnReply.Miracle struct had no
// gratis field. The scalar Param model expresses this whole surface
// (kind/class/villager/item as strings, day/qty/x/y/to_x/to_y as integers), so
// work_miracle needs NO authored InputSchemaJSON: its schema is Params-derived
// (InputSchema, derive.go), unlike set_plan whose steps ARRAY the scalar model
// cannot express. This is also load-bearing for the loop driver — its
// validateArgs routes every InputSchemaJSON tool through set_plan's structural
// validator (toolloop/loop.go), so an authored override here would be validated
// against set_plan's `steps` shape and every work_miracle call rejected; Params
// keeps InputSchema derivation and validateArgs in agreement.
func miracleParams() []Param {
	return []Param{
		{Name: "kind", Kind: Enum, Required: true, Enum: miracleKinds},
		{Name: "class", Kind: Text},
		{Name: "villager", Kind: AgentName},
		{Name: "item", Kind: Text},
		{Name: "qty", Kind: Number, Min: 1},
		{Name: "day", Kind: Number},
		{Name: "time", Kind: Text},
		{Name: "x", Kind: Number},
		{Name: "y", Kind: Number},
		{Name: "to_x", Kind: Number},
		{Name: "to_y", Kind: Number},
	}
}

// ReasonCapRunes bounds the optional per-action `reason` text — the same rune
// budget muse's text carries (a wire sanity cap, not usage guidance). Exported
// so the mind handler's defensive truncation reads the one authoritative value.
const ReasonCapRunes = 200

// reasonParam is the OPTIONAL free-text "why" every acting villager world tool
// carries (spec 019 R12 / T024): the model's reason for the action, threaded to
// InjectArgs.Reason and baked into the completion memory's Why (situated
// " — <why>" clause). Capability-only description — no cadence/format/content
// guidance. NOT added to muse (interiority is already free-standing) or any
// metatron tool.
func reasonParam() Param {
	return Param{Name: "reason", Kind: Text, Required: false, MaxRunes: ReasonCapRunes,
		Description: "optionally, why you're doing this"}
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

	// Journal tool glosses (spec 019, US3). Deliberately capability + budget
	// ONLY — the ONE rule the system imposes is the size budget, so the prompt
	// imposes no cadence, format, or content guidance either (FR-010 applies to
	// prompts too; the reviewer checks for smuggled "when/why/how to journal"
	// guidance). The 4000/1000 numbers mirror sim.JournalBudgetRunes /
	// JournalWriteCapRunes (internal/tool is a leaf and cannot import sim).
	glossWriteJournal  = `write_journal_entry adds a new entry to your private journal, a personal notebook only you can read. The journal holds up to 4000 characters total; a write that would exceed that (or is over 1000 characters on its own) is rejected and leaves the journal unchanged.`
	glossDeleteJournal = `delete_from_journal removes one journal entry by its id number, freeing the space it used.`
	glossSearchJournal = `search_journal returns the entries in your journal whose text contains a given word or phrase, most recent first.`
	glossReadJournal   = `read_journal returns one journal entry by its id number, or the whole journal when no id is given.`
)

// Durations carried verbatim from internal/sim/agents.go (the intentDuration
// switch and its constants). The registry is now the source; the sim duration
// table (agents.go) derives from these. Context overrides (spear hunt, oven
// cook) stay in the executor's workDuration and are not registry data (R7).
//
// worldTools, expressiveTools, and metatronTools (below) are declared
// separately, rather than as one registry literal, so that set_plan (spec
// 017 R11) can be built from worldTools alone and then spliced in after it —
// see setPlanTool and the registry assembly below.
// worldTools carries every acting villager world verb. Each entry gains the
// optional `reason` param (spec 019 R12 / T024) via a post-declaration pass, so
// the shared param is defined once and no verb's literal repeats it — every
// acting tool can carry a why without touching 24 rows.
var worldTools = func() []Tool {
	tools := worldToolsBase
	for i := range tools {
		tools[i].Params = append(append([]Param(nil), tools[i].Params...), reasonParam())
	}
	return tools
}()

var worldToolsBase = []Tool{
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
}

// setPlanTool is the loop-only planning tool (spec 017 R11): Effect World
// (it lands through InjectIntent's existing Plan path, unchanged), Gate
// Resolvable, no Cost/PromptGloss — it carries no PlanStep (deliberately:
// see legacyWorldNamesFrom's doc in derive.go) so it stays out of the legacy
// prose surfaces and RosterVillager, appearing only in LoopRosterVillager
// (roster.go).
var setPlanTool = Tool{
	Name:            "set_plan",
	Effect:          World,
	Gate:            Resolvable,
	InputSchemaJSON: setPlanSchema(legacyWorldNamesFrom(worldTools)),
}

// expressiveTools are the villager roster's expressive verbs (registration
// order = catalog table order; the villager roster's own expressive tail
// order — say, muse, gist — is expressed separately in roster.go).
var expressiveTools = []Tool{
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
}

// metatronTools are the metatron roster's tools. converse produces a
// transcript reply and lands NO world events. It is the metatron's
// expressive speech channel, so it is Effect Expressive with an empty Events
// set — the one eventless expressive tool (see Validate: Events are
// required to be ⊆ whitelist but are not required to be non-empty, so an
// eventless expressive tool is legal).
var metatronTools = []Tool{
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
	// work_miracle is the metatron's fourth loop tool (spec 017 T019b, R13
	// amendment): a direct, charge-priced world edit landed through the SAME
	// InjectSocial door the nudges use (metatron.landMiracle → the shared
	// BuildMiracleBatch → Loop.InjectSocial). It is Effect EXPRESSIVE, not World,
	// decided by the same rule that makes the nudges Expressive: it produces an
	// immediate, bounded batch of whitelisted events through the social door —
	// verbatim the EffectClass Expressive contract (tool.go). A World tool would
	// instead produce an executor-grounded Intent through InjectIntent (a miracle
	// has no intent and no work duration), and — decisively — Validate forbids a
	// World tool from declaring Events, which work_miracle must, so that the
	// sim-side coverage check (ValidateToolCoverage) can pin its event set ⊆ the
	// whitelist. Being Expressive also keeps it out of every legacy villager
	// World surface for free (isLegacyWorldTool requires Effect World), so those
	// stay byte-stable with no exclusion flag.
	//
	// Gate Charge, like the nudges: the bank must hold at least one charge, and
	// the reducer dry-run enforces the real per-kind price (2 for time_snap, 1
	// for the rest, keyed by event type in sim.miracleCost). Cost.Charges is 1 —
	// the gate's minimum, not the per-kind price. Its Events are the four miracle
	// event types plus the FR-018 perception memory, all already on
	// injectSocialWhitelist (spec 016).
	{Name: "work_miracle", Effect: Expressive, Gate: Charge,
		Params: miracleParams(),
		Cost:   Cost{Charges: 1},
		Events: []string{"metatron.time_snapped", "metatron.item_granted",
			"metatron.entity_moved", "metatron.entity_removed", "agent.memory_added"}},
}

// journalTools are the villager-only journal capabilities (spec 019, US3): two
// Expressive tools that land the journal.* mutations through the InjectSocial
// door (their Events are pinned ⊆ injectSocialWhitelist by
// sim.ValidateToolCoverage) and two Read tools that return journal content into
// cognition and ground nothing — the first PRODUCTION Read tools (spec 017 lifted
// the roster restriction and specified Read dispatch, so the loop needs no
// change). All four join LoopRosterVillager only; the metatron never sees them
// (journals are private). Gate None: write/delete need no scene and no charge —
// the reducer dry-run (budget / existence) is their only gate.
var journalTools = []Tool{
	{Name: "write_journal_entry", Effect: Expressive, Gate: None,
		Params:      []Param{{Name: "text", Kind: Text, Required: true, MaxRunes: 1000}},
		Cost:        Cost{TextCapRunes: 1000},
		Events:      []string{"journal.entry_written"},
		PromptGloss: glossWriteJournal},
	{Name: "delete_from_journal", Effect: Expressive, Gate: None,
		Params:      []Param{{Name: "entry", Kind: Number, Required: true}},
		Events:      []string{"journal.entry_deleted"},
		PromptGloss: glossDeleteJournal},
	{Name: "search_journal", Effect: Read,
		Params:      []Param{{Name: "query", Kind: Text, Required: true, MaxRunes: 200}},
		PromptGloss: glossSearchJournal},
	{Name: "read_journal", Effect: Read,
		Params:      []Param{{Name: "entry", Kind: Number, Required: false}},
		PromptGloss: glossReadJournal},
}

// registry is the authoritative collection of every tool, in registration
// order: worldTools (exactly today's goal-vocabulary order — the byte-
// identity anchor for the derived prompt string, SC-003/R3), then set_plan
// (appended immediately after the world verbs so no existing tool's position
// shifts — spec 017 T004), then expressiveTools, then metatronTools, then the
// journal tools (spec 019, appended last so no existing tool's position shifts).
var registry = func() []Tool {
	out := make([]Tool, 0, len(worldTools)+1+len(expressiveTools)+len(metatronTools)+len(journalTools))
	out = append(out, worldTools...)
	out = append(out, setPlanTool)
	out = append(out, expressiveTools...)
	out = append(out, metatronTools...)
	out = append(out, journalTools...)
	return out
}()

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
