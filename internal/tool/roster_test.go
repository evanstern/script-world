package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// namesOf extracts Tool.Name in order, for comparing a []Tool loop roster
// against a []string name list.
func namesOf(tools []Tool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name
	}
	return out
}

// TestLoopRosterVillagerContents (spec 017 contracts/loop-api.md, extended by
// spec 019): the villager loop roster is the legacy world verbs (registration
// order), then set_plan, then muse, then the four journal tools (spec 019, US3).
// say/gist stay scene-gated and out of the loop roster.
func TestLoopRosterVillagerContents(t *testing.T) {
	want := append(append([]string{}, wantWorldOrder...), "set_plan", "muse",
		"write_journal_entry", "delete_from_journal", "search_journal", "read_journal")
	got := namesOf(LoopRosterVillager())
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoopRosterVillager() names =\n%v\nwant\n%v", got, want)
	}

	for _, tl := range LoopRosterVillager() {
		if tl.Name == "" {
			t.Errorf("LoopRosterVillager returned a zero-value Tool")
		}
	}
}

// TestLoopRosterMetatronContents (spec 017 T020): the metatron loop roster is
// nudge_dream, nudge_omen, work_miracle — the DECLARED loop surface, which
// differs from RosterMetatron: converse is excluded (it is the final-answer
// text channel, not a callable tool) and work_miracle is included (the R13
// post-#38 amendment).
func TestLoopRosterMetatronContents(t *testing.T) {
	want := []string{"nudge_dream", "nudge_omen", "work_miracle"}
	got := namesOf(LoopRosterMetatron())
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoopRosterMetatron() names = %v, want %v", got, want)
	}
	// converse must NOT be declared to the loop (it has no handler by design).
	if OnRoster(got, "converse") {
		t.Error("converse leaked into the metatron loop roster — it is the text channel, not a tool")
	}
	for _, tl := range LoopRosterMetatron() {
		if tl.Name == "" {
			t.Error("LoopRosterMetatron returned a zero-value Tool")
		}
	}
}

// TestWorkMiracleSchema (spec 017 T019b): work_miracle's schema is Params-derived
// (no authored InputSchemaJSON, unlike set_plan): a flat object, additionalProperties
// false, required exactly ["kind"], kind an enum of MiracleKinds(), the integer/
// string types per the turn contract, and — critically (spec 016 FR-007/SC-005) —
// NO gratis property.
func TestWorkMiracleSchema(t *testing.T) {
	tl, ok := Lookup("work_miracle")
	if !ok {
		t.Fatal("work_miracle not registered")
	}
	if len(tl.InputSchemaJSON) != 0 {
		t.Error("work_miracle must not carry an authored InputSchemaJSON — its schema is Params-derived")
	}
	if tl.Effect != Expressive {
		t.Errorf("work_miracle.Effect = %v, want Expressive", tl.Effect)
	}
	if tl.Gate != Charge {
		t.Errorf("work_miracle.Gate = %v, want Charge", tl.Gate)
	}
	raw := InputSchema(tl)
	var schema struct {
		Type       string   `json:"type"`
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
		AdditionalProperties bool `json:"additionalProperties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("work_miracle schema did not decode: %v\nraw: %s", err, raw)
	}
	if schema.Type != "object" || schema.AdditionalProperties {
		t.Errorf("top-level shape wrong: type=%q additionalProperties=%v", schema.Type, schema.AdditionalProperties)
	}
	if !reflect.DeepEqual(schema.Required, []string{"kind"}) {
		t.Errorf("required = %v, want [kind]", schema.Required)
	}
	if _, ok := schema.Properties["gratis"]; ok {
		t.Error("work_miracle schema declares a gratis property — the angel can never waive a charge (SC-005)")
	}
	if !reflect.DeepEqual(schema.Properties["kind"].Enum, MiracleKinds()) {
		t.Errorf("kind enum = %v, want %v", schema.Properties["kind"].Enum, MiracleKinds())
	}
	for _, name := range []string{"day", "qty", "x", "y", "to_x", "to_y"} {
		if schema.Properties[name].Type != "integer" {
			t.Errorf("property %q type = %q, want integer", name, schema.Properties[name].Type)
		}
	}
	for _, name := range []string{"class", "villager", "item", "time"} {
		if schema.Properties[name].Type != "string" {
			t.Errorf("property %q type = %q, want string", name, schema.Properties[name].Type)
		}
	}
}

// TestSetPlanExcludedFromLegacySurfaces (spec 017 T004 CRITICAL constraint):
// set_plan is Effect World but must not appear in any of the legacy
// prose/derivation surfaces — VocabularyLine, WorldGoals, PlanStepGoals, or
// RosterVillager (the pre-loop door roster). These are the byte-stability
// pins: the golden prompt test and single-walk invariant test already assert
// the surfaces are unchanged; this test pins the mechanism (set_plan's
// absence) directly.
func TestSetPlanExcludedFromLegacySurfaces(t *testing.T) {
	if strings.Contains(VocabularyLine(), "set_plan") {
		t.Errorf("set_plan leaked into VocabularyLine: %s", VocabularyLine())
	}
	if WorldGoals()["set_plan"] {
		t.Error("set_plan leaked into WorldGoals")
	}
	if PlanStepGoals()["set_plan"] {
		t.Error("set_plan leaked into PlanStepGoals")
	}
	if OnRoster(RosterVillager, "set_plan") {
		t.Error("set_plan leaked into RosterVillager")
	}
	// But it IS reachable via Lookup and the loop roster — it is a real
	// catalog entry, just not a legacy one.
	if _, ok := Lookup("set_plan"); !ok {
		t.Error("set_plan is not registered at all")
	}
	if !OnRoster(namesOf(LoopRosterVillager()), "set_plan") {
		t.Error("set_plan is missing from LoopRosterVillager")
	}
}

// TestSetPlanSchema: set_plan's authored InputSchemaJSON is valid JSON,
// passes Validate (exercised via the live registry in
// TestValidateRealRegistry too), and carries the documented shape — a
// `steps` array, 1..PlanStepCap items of {goal: enum, kind?: enum, qty?:
// integer >= 1}, required ["goal"] per step and ["steps"] at top level, both
// levels additionalProperties: false.
func TestSetPlanSchema(t *testing.T) {
	tl, ok := Lookup("set_plan")
	if !ok {
		t.Fatal("set_plan not registered")
	}
	raw := tl.InputSchemaJSON
	if !json.Valid(raw) {
		t.Fatalf("set_plan schema is not valid JSON: %s", raw)
	}

	var schema struct {
		Type       string   `json:"type"`
		Required   []string `json:"required"`
		Properties struct {
			Steps struct {
				Type     string `json:"type"`
				MinItems int    `json:"minItems"`
				MaxItems int    `json:"maxItems"`
				Items    struct {
					Type       string   `json:"type"`
					Required   []string `json:"required"`
					Properties struct {
						Goal struct {
							Enum []string `json:"enum"`
						} `json:"goal"`
						Kind struct {
							Enum []string `json:"enum"`
						} `json:"kind"`
						Qty struct {
							Type    string `json:"type"`
							Minimum int    `json:"minimum"`
						} `json:"qty"`
					} `json:"properties"`
					AdditionalProperties bool `json:"additionalProperties"`
				} `json:"items"`
			} `json:"steps"`
		} `json:"properties"`
		AdditionalProperties bool `json:"additionalProperties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("set_plan schema did not decode: %v\nraw: %s", err, raw)
	}

	if schema.Type != "object" || schema.AdditionalProperties {
		t.Errorf("top-level shape wrong: type=%q additionalProperties=%v", schema.Type, schema.AdditionalProperties)
	}
	if !reflect.DeepEqual(schema.Required, []string{"steps"}) {
		t.Errorf("top-level required = %v, want [steps]", schema.Required)
	}
	if schema.Properties.Steps.Type != "array" || schema.Properties.Steps.MinItems != 1 || schema.Properties.Steps.MaxItems != PlanStepCap {
		t.Errorf("steps array shape wrong: %+v", schema.Properties.Steps)
	}
	item := schema.Properties.Steps.Items
	if item.Type != "object" || item.AdditionalProperties {
		t.Errorf("step item shape wrong: type=%q additionalProperties=%v", item.Type, item.AdditionalProperties)
	}
	if !reflect.DeepEqual(item.Required, []string{"goal"}) {
		t.Errorf("step item required = %v, want [goal]", item.Required)
	}
	if !reflect.DeepEqual(item.Properties.Goal.Enum, wantWorldOrder) {
		t.Errorf("step goal enum =\n%v\nwant\n%v", item.Properties.Goal.Enum, wantWorldOrder)
	}
	if !reflect.DeepEqual(item.Properties.Kind.Enum, ItemKinds()) {
		t.Errorf("step kind enum =\n%v\nwant\n%v", item.Properties.Kind.Enum, ItemKinds())
	}
	if item.Properties.Qty.Type != "integer" || item.Properties.Qty.Minimum != 1 {
		t.Errorf("step qty shape wrong: %+v", item.Properties.Qty)
	}
}
