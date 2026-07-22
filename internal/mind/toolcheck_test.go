package mind

import (
	"sort"
	"testing"

	"github.com/evanstern/promptworld/internal/tool"
)

// TestPlanStepCapMirrorsTool (spec 017 T004): internal/tool declares
// PlanStepCap for set_plan's authored schema separately from this package's
// own planStepCap constant — internal/tool is a leaf package (research R1)
// and cannot import internal/mind to share the literal. This test pins the
// two constants equal so a change to one that isn't mirrored in the other
// fails loudly here instead of drifting silently.
func TestPlanStepCapMirrorsTool(t *testing.T) {
	if tool.PlanStepCap != planStepCap {
		t.Errorf("tool.PlanStepCap = %d, internal/mind planStepCap = %d — must be kept equal", tool.PlanStepCap, planStepCap)
	}
}

// TestToolItemKindsSubsetOfValidKinds (spec 017 T004): internal/tool's
// ItemKinds() (the storage verbs' Enum descriptor, and set_plan's `kind`
// schema enum) is a real subset of this package's own validKinds set — every
// item kind the registry names is also legal to this package's free-text
// parser. validKinds carries exactly one extra member, "" (the parser's "all
// kinds" sentinel), which has no ParamKind/JSON-Schema representation and is
// deliberately absent from ItemKinds() — spec 014 chose NOT to migrate
// validKinds into internal/tool (it is not a capability-vocabulary list), so
// the two lists are related but not identical; this test pins that relation
// so it can't silently drift into an actual mismatch.
func TestToolItemKindsSubsetOfValidKinds(t *testing.T) {
	kinds := tool.ItemKinds()
	for _, k := range kinds {
		if !validKinds[k] {
			t.Errorf("tool.ItemKinds() has %q, which is not in validKinds", k)
		}
	}

	got := append([]string{}, kinds...)
	sort.Strings(got)
	want := sortedKeys(validKinds)
	// want has exactly one extra member: "".
	filteredWant := want[:0:0]
	for _, k := range want {
		if k != "" {
			filteredWant = append(filteredWant, k)
		}
	}
	if len(got) != len(filteredWant) {
		t.Fatalf("tool.ItemKinds() has %d members, validKinds (minus \"\") has %d: got %v, want %v", len(got), len(filteredWant), got, filteredWant)
	}
	for i := range got {
		if got[i] != filteredWant[i] {
			t.Errorf("tool.ItemKinds() sorted = %v, validKinds (minus \"\") sorted = %v", got, filteredWant)
			break
		}
	}
}
