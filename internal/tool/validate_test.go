package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestValidateRejectsMalformed (FR-003, R9): each malformed registry/roster
// fixture produces a validation error, so a bad edit fails fast at startup
// rather than at tick time. Validate returns ALL violations, so we assert the
// specific message surfaces.
func TestValidateRejectsMalformed(t *testing.T) {
	cases := []struct {
		name     string
		reg      []Tool
		villager []string
		metatron []string
		want     string
	}{
		{
			name: "empty name",
			reg:  []Tool{{Name: "", Effect: World}},
			want: "empty name",
		},
		{
			name: "duplicate name",
			reg:  []Tool{{Name: "chop", Effect: World}, {Name: "chop", Effect: World}},
			want: "duplicate tool name",
		},
		{
			name: "unknown effect class",
			reg:  []Tool{{Name: "chop", Effect: EffectClass(99)}},
			want: "unknown effect class",
		},
		{
			name: "events on a world tool",
			reg:  []Tool{{Name: "chop", Effect: World, Events: []string{"agent.thought"}}},
			want: "declares Events",
		},
		{
			name: "planstep on a non-world tool",
			reg:  []Tool{{Name: "say", Effect: Expressive, PlanStep: true}},
			want: "PlanStep set on a non-world tool",
		},
		{
			name: "enum param without values",
			reg:  []Tool{{Name: "drop", Effect: World, Params: []Param{{Name: "kind", Kind: Enum}}}},
			want: "has no values",
		},
		{
			name:     "roster names a missing tool",
			reg:      []Tool{{Name: "chop", Effect: World}},
			villager: []string{"chop", "ghost_verb"},
			want:     "not in the registry",
		},
		{
			name: "number param with inverted bounds",
			reg:  []Tool{{Name: "drop", Effect: World, Params: []Param{{Name: "qty", Kind: Number, Min: 5, Max: 1}}}},
			want: "Min 5 > Max 1",
		},
		{
			name: "InputSchemaJSON is not valid JSON",
			reg:  []Tool{{Name: "set_plan", Effect: World, InputSchemaJSON: json.RawMessage(`{not json`)}},
			want: "InputSchemaJSON is not valid JSON",
		},
		{
			name: "InputSchemaJSON is not a JSON object",
			reg:  []Tool{{Name: "set_plan", Effect: World, InputSchemaJSON: json.RawMessage(`["array", "not object"]`)}},
			want: "InputSchemaJSON must be a JSON object",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := swapRegistry(tc.reg, tc.villager, tc.metatron)
			defer restore()

			err := Validate()
			if err == nil {
				t.Fatalf("expected a validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.want)
			}
		})
	}
}

// TestValidateAllViolations: Validate collects every violation, not just the
// first — two independent faults both surface.
func TestValidateAllViolations(t *testing.T) {
	restore := swapRegistry(
		[]Tool{{Name: "", Effect: World}, {Name: "say", Effect: Expressive, PlanStep: true}},
		nil, nil,
	)
	defer restore()

	err := Validate()
	if err == nil {
		t.Fatal("expected violations, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "empty name") || !strings.Contains(msg, "PlanStep set on a non-world tool") {
		t.Errorf("expected both violations, got: %s", msg)
	}
}

// TestValidateAdmitsLegalFixtures (spec 017 R12/T002): fixtures that would
// have failed under spec-014's rules but are now legal — a roster naming a
// Read tool, one-sided Number bounds, and a well-formed InputSchemaJSON
// override — all validate clean.
func TestValidateAdmitsLegalFixtures(t *testing.T) {
	cases := []struct {
		name     string
		reg      []Tool
		villager []string
	}{
		{
			name:     "Read tool on a roster is now legal",
			reg:      []Tool{{Name: "peek", Effect: Read}},
			villager: []string{"peek"},
		},
		{
			name: "Number param with only Min set",
			reg:  []Tool{{Name: "drop", Effect: World, Params: []Param{{Name: "qty", Kind: Number, Min: 1}}}},
		},
		{
			name: "Number param with only Max set",
			reg:  []Tool{{Name: "drop", Effect: World, Params: []Param{{Name: "qty", Kind: Number, Max: 10}}}},
		},
		{
			name: "Number param with equal Min and Max",
			reg:  []Tool{{Name: "drop", Effect: World, Params: []Param{{Name: "qty", Kind: Number, Min: 3, Max: 3}}}},
		},
		{
			name: "well-formed InputSchemaJSON override",
			reg:  []Tool{{Name: "set_plan", Effect: World, InputSchemaJSON: json.RawMessage(`{"type":"object"}`)}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := swapRegistry(tc.reg, tc.villager, nil)
			defer restore()

			if err := Validate(); err != nil {
				t.Errorf("expected no violation, got: %v", err)
			}
		})
	}
}
