package tool

import (
	"encoding/json"
	"reflect"
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
