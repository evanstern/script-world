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

// TestLoopRosterVillagerContents (spec 017 contracts/loop-api.md): the
// villager loop roster is exactly the legacy world verbs (registration
// order), then set_plan, then muse — say/gist stay scene-gated and out of
// the loop roster this task (data-model.md §2).
func TestLoopRosterVillagerContents(t *testing.T) {
	want := append(append(append([]string{}, wantWorldOrder...), "set_plan"), "muse")
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

// TestLoopRosterMetatronContents: the metatron loop roster is converse,
// nudge_dream, nudge_omen — the same membership and order as RosterMetatron,
// resolved to full Tool values.
func TestLoopRosterMetatronContents(t *testing.T) {
	want := append([]string{}, RosterMetatron...)
	got := namesOf(LoopRosterMetatron())
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoopRosterMetatron() names = %v, want %v", got, want)
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
