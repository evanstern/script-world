package tool

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The derived surfaces: each is one walk of the registry, computed live so a
// tool added to the registry (or, in tests, appended via the registry hook)
// flows to all of them with no other edit. These replace the four
// hand-maintained vocabularies (the prompt goal line, the mind parse set, the
// sim plan-step map, and the per-verb gloss prose) whose drift this feature
// exists to kill.
//
// The single-walk invariant (TASK-55 AC#2): VocabularyLine names ≡ WorldGoals
// keys ≡ PlanStepGoals keys — divergence is impossible by construction because
// all three walk the same World-tool set, and every World tool carries
// PlanStep == true.

// isLegacyWorldTool reports whether t belongs to the legacy free-text goal
// vocabulary: Effect World AND PlanStep true.
//
// PlanStep is the discriminator that excludes set_plan (spec 017 R11):
// set_plan is Effect World (it lands through the same InjectIntent path) but
// is loop-only vocabulary, not a legacy free-text goal — it carries
// PlanStep: false precisely so this filter (and everything built from it)
// stays byte-stable without a separate exclusion list. Every OTHER World
// tool already carries PlanStep: true (the TASK-55 single-walk invariant),
// so this filter changes nothing for them.
func isLegacyWorldTool(t Tool) bool {
	return t.Effect == World && t.PlanStep
}

// legacyWorldNamesFrom returns the ordered names of legacy World tools in
// tools (registration order preserved). Takes an explicit slice, rather than
// walking the package registry, so it can be called while registry itself
// is still being built (registry.go's setPlanTool needs this list before
// registry exists — calling the registry-walking legacyWorldNames() there
// would be an initialization cycle).
func legacyWorldNamesFrom(tools []Tool) []string {
	var names []string
	for _, t := range tools {
		if isLegacyWorldTool(t) {
			names = append(names, t.Name)
		}
	}
	return names
}

// legacyWorldNames returns the ordered names of legacy World tools in the
// package registry. This is the one walk VocabularyLine and WorldGoals share
// (both call it) — and, at init time, registry.go's setPlanTool builds the
// identical set via legacyWorldNamesFrom(worldTools) — so the free-text
// vocabulary and set_plan's authored `goal` enum can never drift from each
// other even though they can't share this exact function call.
func legacyWorldNames() []string {
	return legacyWorldNamesFrom(registry)
}

// VocabularyLine returns the comma-joined legacy World-tool names in
// registration order — byte-identical to the old goal-vocabulary line.
func VocabularyLine() string {
	return strings.Join(legacyWorldNames(), ", ")
}

// PromptGlossBlock returns the concatenated per-verb gloss lines in
// registration order, each terminated by a newline — byte-identical to the old
// hand-written prose block (the lines between "Goals:" and "For a short
// sequence" in internal/mind/prompt.go). Empty when no world verb carries a
// gloss.
//
// Scoped to the legacy world verbs (isLegacyWorldTool): this block IS the
// world-verb goal prose, so a non-world tool's gloss must never enter it. The
// journal tools (spec 019) carry glosses too — but those are their model-facing
// tool DESCRIPTIONS, delivered per-tool through the loop's ToolDecl (toolloop),
// not this legacy prose surface. Today every glossed tool except the journal
// tools is a world verb, so the filter is behavior-preserving for the shipped
// prose block while keeping it pure as non-world glosses are added.
func PromptGlossBlock() string {
	var b strings.Builder
	for _, t := range registry {
		if isLegacyWorldTool(t) && t.PromptGloss != "" {
			b.WriteString(t.PromptGloss)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// WorldGoals returns the set of legacy World-tool names — the mind-side parse
// accept set (replaces the mind's old hand-maintained goal map). A fresh map
// per call; callers on a hot path cache it once.
func WorldGoals() map[string]bool {
	m := make(map[string]bool)
	for _, n := range legacyWorldNames() {
		m[n] = true
	}
	return m
}

// PlanStepGoals returns the set of names with PlanStep == true — the sim-door
// plan-step accept set (replaces the sim's old hand-maintained plan-step map).
// The FR-012 drift cure lives in the catalog data (every World tool carries
// PlanStep: true), not in code here.
func PlanStepGoals() map[string]bool {
	m := make(map[string]bool)
	for _, t := range registry {
		if t.PlanStep {
			m[t.Name] = true
		}
	}
	return m
}

// InputSchema derives the JSON Schema object a tool declares to a model
// (spec 017 data-model.md §1) from t.Params — or, when t.InputSchemaJSON is
// set, returns that authored override verbatim (set_plan, R11), bypassing
// Params derivation entirely.
//
// Output is deterministic: Params is already registration-ordered, and every
// list this function builds (required names, enum values) walks that slice
// or a Param's own Enum slice directly — never a Go map — so two calls for
// the same Tool marshal to byte-identical JSON. The one map in play
// (properties, and the schema object itself) holds only property-name keys,
// which encoding/json sorts lexicographically before marshaling, so their
// presence does not reintroduce nondeterminism.
func InputSchema(t Tool) json.RawMessage {
	if len(t.InputSchemaJSON) > 0 {
		return t.InputSchemaJSON
	}

	properties := make(map[string]any, len(t.Params))
	var required []string
	for _, p := range t.Params {
		properties[p.Name] = paramSchema(p)
		if p.Required {
			required = append(required, p.Name)
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	b, err := json.Marshal(schema)
	if err != nil {
		// schema is built from literal Go data (strings, bools, string
		// slices); marshaling it cannot fail. Panic is the honest response to
		// an impossible state, matching mind's plannerReplySchema precedent.
		panic("tool: InputSchema marshal: " + err.Error())
	}
	return b
}

// RestrictEnum returns a copy of t whose named Enum param is narrowed to the
// intersection of its declared Enum values and allowed (spec 021 R5.1) — the
// per-world capability manifest uses it to offer work_miracle with only the
// granted `kind` values. It is copy-on-write: a FRESH Params slice is built and
// the target param's Enum is replaced with a fresh slice, so the registry's
// Tool and its shared backing arrays are never mutated (the caller may treat
// the result as owned). Survivors keep the tool's OWN Enum order — not allowed's
// — so the declared surface stays deterministic regardless of how the manifest
// listed the kinds; an allowed name the tool never declared is dropped. A tool
// with no such Enum param is returned structurally unchanged (still a fresh
// Params copy). Feeding InputSchema the result declares only the surviving enum
// values, which IS the structural-absence guarantee (FR-005 layer 1).
func RestrictEnum(t Tool, param string, allowed []string) Tool {
	keep := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		keep[a] = true
	}
	params := make([]Param, len(t.Params))
	copy(params, t.Params)
	for i := range params {
		if params[i].Name != param || params[i].Kind != Enum {
			continue
		}
		var narrowed []string
		for _, v := range params[i].Enum { // the tool's own declared order
			if keep[v] {
				narrowed = append(narrowed, v)
			}
		}
		params[i].Enum = narrowed
	}
	t.Params = params
	return t
}

// metatronToolDesc is the one-line human gloss rendered beside each metatron
// acting tool's name in the derived guidance. Keyed, so a tool absent from the
// granted roster contributes no line (spec 021 FR-005/FR-008). This map supplies
// only the "what it does" prose; the tool NAMES, ARGUMENT surfaces, and COSTS in
// the guidance all derive from the registry, so described ≡ declared.
var metatronToolDesc = map[string]string{
	"send_vision":     "a waking vision for ONE living villager, at any hour",
	"send_omen":       "an omen at night for one villager, a named group, or everyone",
	"monitor_and_act": "place a standing order: watch for a condition, then act",
	"cancel_order":    "cancel a standing order by its id",
	"pause":           "pause the world clock",
	"start":           "start the world clock (optionally at a named speed)",
	"adjust_speed":    "change the world clock's speed",
	"work_miracle":    "a direct world edit",
}

// miracleKindArgs is the per-kind argument hint rendered under work_miracle,
// keyed by kind so only GRANTED kinds — the work_miracle tool's possibly-
// restricted `kind` Enum — ever appear (FR-005). The kind vocabulary and its
// price derive from the registry (the Enum and MiracleCost); this map only
// supplies the human argument gloss for a kind that is offered.
var miracleKindArgs = map[string]string{
	"move":      `class ("villager"|"structure"|"pile"), x, y, to_x, to_y`,
	"remove":    `class ("structure"|"pile"|"terrain"), x, y`,
	"give_item": `villager, item, qty`,
	"time_snap": `day and time ("HH:MM")`,
}

// MetatronToolGuidance renders the human-shaped acting-tool guidance for the
// metatron turn prompt FROM the granted roster (spec 021 R6 / FR-008): one
// bullet per tool, in roster order, naming the tool, its argument surface (from
// Params — the same source InputSchema derives from), and its charge cost (from
// the authoritative MiracleCost table for miracle kinds, Cost.Charges for the
// nudges). Because it walks the SAME roster that feeds Job.Roster, a tool or a
// miracle kind absent from the world's grant is absent here too — the prose can
// never describe a capability the model was not offered, and the derived cost
// can never drift from the enforced one. Output is deterministic: roster order
// and each tool's own Enum/Params slices drive every list; no map is iterated
// into the output. Empty for an empty roster (a conversation-only world).
func MetatronToolGuidance(roster []Tool) string {
	var b strings.Builder
	for _, t := range roster {
		desc := metatronToolDesc[t.Name]
		if t.Name == "work_miracle" {
			fmt.Fprintf(&b, "  • %s(kind, …) — %s; kind is\n", t.Name, desc)
			for _, k := range enumValues(t, "kind") {
				cost, _ := MiracleCost(k)
				fmt.Fprintf(&b, "      %q with %s — %d %s\n", k, miracleKindArgs[k], cost, chargeWord(cost))
			}
			continue
		}
		cost := t.Cost.Charges
		fmt.Fprintf(&b, "  • %s(%s) — %s (%d %s)\n", t.Name, paramNameList(t), desc, cost, chargeWord(cost))
	}
	return b.String()
}

// chargeWord pluralizes the charge count for the guidance prose.
func chargeWord(n int) string {
	if n == 1 {
		return "charge"
	}
	return "charges"
}

// enumValues returns the named param's declared Enum values in the tool's own
// order (a restricted copy keeps its narrowed order — RestrictEnum preserves
// it), or nil when the tool has no such Enum param.
func enumValues(t Tool, param string) []string {
	for _, p := range t.Params {
		if p.Name == param && p.Kind == Enum {
			return p.Enum
		}
	}
	return nil
}

// paramNameList joins a tool's declared parameter names in Params order — the
// argument surface a guidance bullet shows (e.g. "target, text").
func paramNameList(t Tool) string {
	names := make([]string, 0, len(t.Params))
	for _, p := range t.Params {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}

// paramSchema derives one property's JSON Schema fragment from its Param
// descriptor, per the data-model.md §1 derivation rules:
//   - AgentName -> {"type":"string"}
//   - Text      -> {"type":"string"}, +maxLength from MaxRunes, else MaxBytes
//   - Enum      -> {"type":"string","enum":[...]}
//   - Number    -> {"type":"integer"}, +minimum/maximum when Min/Max set
//     (a bound of 0 means unset, matching Param's own 0,0-is-unbounded
//     convention — the qty param is never legitimately bounded to exactly 0).
func paramSchema(p Param) map[string]any {
	var s map[string]any
	switch p.Kind {
	case AgentName:
		s = map[string]any{"type": "string"}
	case Text:
		s = map[string]any{"type": "string"}
		switch {
		case p.MaxRunes > 0:
			s["maxLength"] = p.MaxRunes
		case p.MaxBytes > 0:
			s["maxLength"] = p.MaxBytes
		}
	case Enum:
		s = map[string]any{"type": "string", "enum": p.Enum}
	case Number:
		s = map[string]any{"type": "integer"}
		if p.Min != 0 {
			s["minimum"] = p.Min
		}
		if p.Max != 0 {
			s["maximum"] = p.Max
		}
	default:
		s = map[string]any{"type": "string"}
	}
	if p.Description != "" {
		s["description"] = p.Description
	}
	return s
}
