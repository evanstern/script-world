package tool

import (
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
			name:     "read tool on a roster",
			reg:      []Tool{{Name: "peek", Effect: Read}},
			villager: []string{"peek"},
			want:     "read tool",
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
