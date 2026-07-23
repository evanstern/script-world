package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// mustUnmarshalSchema decodes an InputSchema result into a generic map for
// structural comparison — InputSchema's own byte-stability guarantee is
// tested separately (TestInputSchemaDeterministic); tests here assert shape,
// not literal bytes.
func mustUnmarshalSchema(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("InputSchema produced invalid JSON: %v\nraw: %s", err, raw)
	}
	return m
}

// TestInputSchemaPerParamKind (spec 017 T003, data-model.md §1): each
// ParamKind derives its documented JSON Schema fragment, and a tool with no
// params derives the bare object shape.
func TestInputSchemaPerParamKind(t *testing.T) {
	cases := []struct {
		name string
		tool Tool
		want map[string]any
	}{
		{
			name: "AgentName",
			tool: Tool{Name: "talk_to", Params: []Param{{Name: "target", Kind: AgentName, Required: true}}},
			want: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"target": map[string]any{"type": "string"}},
				"required":             []any{"target"},
				"additionalProperties": false,
			},
		},
		{
			name: "Text with MaxRunes",
			tool: Tool{Name: "muse", Params: []Param{{Name: "text", Kind: Text, MaxRunes: 200}}},
			want: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"text": map[string]any{"type": "string", "maxLength": float64(200)}},
				"additionalProperties": false,
			},
		},
		{
			name: "Text with MaxBytes and no MaxRunes",
			tool: Tool{Name: "say", Params: []Param{{Name: "text", Kind: Text, MaxBytes: 300}}},
			want: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"text": map[string]any{"type": "string", "maxLength": float64(300)}},
				"additionalProperties": false,
			},
		},
		{
			name: "Text with no bound",
			tool: Tool{Name: "converse", Params: []Param{{Name: "text", Kind: Text}}},
			want: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"text": map[string]any{"type": "string"}},
				"additionalProperties": false,
			},
		},
		{
			name: "Enum",
			tool: Tool{Name: "probe", Params: []Param{{Name: "kind", Kind: Enum, Enum: []string{"a", "b"}}}},
			want: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"kind": map[string]any{"type": "string", "enum": []any{"a", "b"}}},
				"additionalProperties": false,
			},
		},
		{
			name: "Number with both bounds",
			tool: Tool{Name: "probe", Params: []Param{{Name: "qty", Kind: Number, Min: 1, Max: 10}}},
			want: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"qty": map[string]any{"type": "integer", "minimum": float64(1), "maximum": float64(10)}},
				"additionalProperties": false,
			},
		},
		{
			name: "Number with only Min",
			tool: Tool{Name: "probe", Params: []Param{{Name: "qty", Kind: Number, Min: 1}}},
			want: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"qty": map[string]any{"type": "integer", "minimum": float64(1)}},
				"additionalProperties": false,
			},
		},
		{
			name: "Number unbounded",
			tool: Tool{Name: "probe", Params: []Param{{Name: "qty", Kind: Number}}},
			want: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"qty": map[string]any{"type": "integer"}},
				"additionalProperties": false,
			},
		},
		{
			name: "no params",
			tool: Tool{Name: "forage"},
			want: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustUnmarshalSchema(t, InputSchema(tc.tool))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("InputSchema(%s) =\n%#v\nwant\n%#v", tc.tool.Name, got, tc.want)
			}
		})
	}
}

// TestInputSchemaOverridePassthrough (R11): an InputSchemaJSON override is
// returned verbatim, bypassing Params derivation entirely — set_plan's
// contract.
func TestInputSchemaOverridePassthrough(t *testing.T) {
	override := json.RawMessage(`{"type":"object","properties":{"steps":{"type":"array"}},"required":["steps"]}`)
	tl := Tool{Name: "set_plan", Params: []Param{{Name: "ignored", Kind: Text}}, InputSchemaJSON: override}

	got := InputSchema(tl)
	if string(got) != string(override) {
		t.Errorf("InputSchema override not passed through verbatim:\ngot  %s\nwant %s", got, override)
	}
}

// TestInputSchemaDeterministic: two calls for the same Tool produce
// byte-identical output.
func TestInputSchemaDeterministic(t *testing.T) {
	tl, ok := Lookup("drop")
	if !ok {
		t.Fatal("drop not in registry")
	}
	a := InputSchema(tl)
	b := InputSchema(tl)
	if string(a) != string(b) {
		t.Errorf("InputSchema not deterministic across calls:\n%s\nvs\n%s", a, b)
	}
}

// TestInputSchemaRealCatalogEntry: drop (Enum "kind" + Number "qty") derives
// the full documented shape from a real registry entry.
func TestInputSchemaRealCatalogEntry(t *testing.T) {
	tl, ok := Lookup("drop")
	if !ok {
		t.Fatal("drop not in registry")
	}
	got := mustUnmarshalSchema(t, InputSchema(tl))

	if got["type"] != "object" || got["additionalProperties"] != false {
		t.Errorf("top-level shape wrong: %#v", got)
	}
	if _, present := got["required"]; present {
		t.Errorf("drop has no Required params, expected no \"required\" key, got %#v", got["required"])
	}

	props, ok := got["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties is not an object: %#v", got["properties"])
	}

	kind, ok := props["kind"].(map[string]any)
	if !ok || kind["type"] != "string" {
		t.Errorf("kind property = %#v, want a string schema", kind)
	}
	if _, present := kind["enum"]; !present {
		t.Errorf("kind property missing enum: %#v", kind)
	}

	qty, ok := props["qty"].(map[string]any)
	if !ok || qty["type"] != "integer" || qty["minimum"] != float64(1) {
		t.Errorf("qty property = %#v, want integer with minimum 1", qty)
	}
	if _, present := qty["maximum"]; present {
		t.Errorf("qty property has an unexpected maximum: %#v", qty)
	}
}

// TestRestrictEnum (spec 021 T005): RestrictEnum narrows a tool's Enum param to
// the intersection with the allowed set, preserving the tool's OWN order, and
// is copy-on-write — the registry's Tool is never mutated. The restricted copy's
// InputSchema declares only the surviving enum values (structural absence, FR-005
// layer 1).
func TestRestrictEnum(t *testing.T) {
	wm, ok := Lookup("work_miracle")
	if !ok {
		t.Fatal("work_miracle missing from registry")
	}
	before := append([]string(nil), enumValues(wm, "kind")...)

	// Restrict to a subset, listed out of registry order — survivors keep the
	// tool's own order, not the caller's.
	got := RestrictEnum(wm, "kind", []string{"give_item", "move", "not_a_kind"})
	want := []string{"move", "give_item"} // registry order: move before give_item
	if !reflect.DeepEqual(enumValues(got, "kind"), want) {
		t.Errorf("restricted kind enum = %v, want %v (tool order, unknown dropped)", enumValues(got, "kind"), want)
	}

	// Copy-on-write: the registry Tool's enum is untouched.
	again, _ := Lookup("work_miracle")
	if !reflect.DeepEqual(enumValues(again, "kind"), before) {
		t.Errorf("registry work_miracle kind enum mutated: %v, want %v", enumValues(again, "kind"), before)
	}

	// The restricted copy's InputSchema declares only the surviving kinds.
	schema := mustUnmarshalSchema(t, InputSchema(got))
	props := schema["properties"].(map[string]any)
	kind := props["kind"].(map[string]any)
	enumAny := kind["enum"].([]any)
	var gotEnum []string
	for _, v := range enumAny {
		gotEnum = append(gotEnum, v.(string))
	}
	if !reflect.DeepEqual(gotEnum, want) {
		t.Errorf("restricted InputSchema kind enum = %v, want %v", gotEnum, want)
	}

	// A param with no Enum (or absent) is returned unchanged but with a fresh
	// Params slice (owned by the caller).
	unchanged := RestrictEnum(wm, "villager", []string{"whatever"})
	if !reflect.DeepEqual(enumValues(unchanged, "kind"), before) {
		t.Errorf("restricting a non-enum param disturbed the kind enum: %v", enumValues(unchanged, "kind"))
	}
}

// TestMetatronToolGuidanceDrift (spec 021 T007 / FR-008 / SC-004 / INV-3): the
// derived guidance names every roster tool, renders every cost from the single
// authoritative table, mentions no non-roster tool or ungranted kind, and is a
// byte-identical pure function of its input.
func TestMetatronToolGuidanceDrift(t *testing.T) {
	roster := LoopRosterMetatron()
	g := MetatronToolGuidance(roster)

	// Every roster tool name appears.
	for _, tl := range roster {
		if !strings.Contains(g, tl.Name) {
			t.Errorf("guidance omits roster tool %q", tl.Name)
		}
	}
	// Every miracle kind appears with its authoritative cost, and no cost is
	// hand-written wrong.
	for _, k := range MiracleKinds() {
		cost, _ := MiracleCost(k)
		line := k + `" with ` // the kind label as rendered
		if !strings.Contains(g, line) {
			t.Errorf("guidance omits miracle kind %q", k)
		}
		want := fmt.Sprintf("%d %s", cost, chargeWord(cost))
		if !strings.Contains(g, want) {
			t.Errorf("guidance omits cost %q for kind %q", want, k)
		}
	}
	// No non-roster tool name leaks in (e.g. villager verbs, converse).
	for _, bad := range []string{"converse", "forage", "say", "muse", "set_plan", "write_journal_entry"} {
		if strings.Contains(g, bad) {
			t.Errorf("guidance mentions non-roster tool %q", bad)
		}
	}
	// Pure function: two calls are byte-identical.
	if MetatronToolGuidance(LoopRosterMetatron()) != g {
		t.Error("MetatronToolGuidance is not deterministic across calls")
	}

	// A restricted roster: only granted tools/kinds appear.
	wm, _ := Lookup("work_miracle")
	restricted := []Tool{RestrictEnum(wm, "kind", []string{"give_item"})}
	rg := MetatronToolGuidance(restricted)
	if !strings.Contains(rg, `give_item" with `) {
		t.Error("restricted guidance omits the granted give_item kind")
	}
	// Ungranted kinds are checked in their rendered label form (`"<kind>" with`)
	// so "remove" ⊃ "move" cannot cause a false match either way.
	for _, gone := range []string{`time_snap" with `, `move" with `, `remove" with `} {
		if strings.Contains(rg, gone) {
			t.Errorf("restricted guidance leaks ungranted kind label %q", gone)
		}
	}
	for _, gone := range []string{"nudge_dream", "nudge_omen"} {
		if strings.Contains(rg, gone) {
			t.Errorf("restricted guidance leaks ungranted tool %q", gone)
		}
	}
	// time_snap's dear price never appears when time_snap is ungranted.
	if strings.Contains(rg, "2 charges") {
		t.Error("restricted guidance leaks the time_snap 2-charge cost")
	}

	// Empty roster (conversation-only world) → empty guidance.
	if MetatronToolGuidance(nil) != "" {
		t.Error("empty roster should render empty guidance")
	}
}
