package tool

import (
	"encoding/json"
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
