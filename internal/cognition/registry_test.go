package cognition

import (
	"strings"
	"testing"
)

func TestRegistryValidates(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestRegistryContractValues(t *testing.T) {
	// Pinned to contracts/registry.md — a drift here is a doctrine change
	// and must be made there first.
	want := map[string]struct {
		points int
		budget int64
		deg    Degrade
		future bool
	}{
		"planner":       {3, 1200, DegradeReflex, true},
		"conversation":  {13, 7200, DegradeSkip, false},
		"meeting":       {2, 3600, DegradeTemplate, false},
		"consolidation": {5, 28800, DegradeSkip, false},
		"chronicle":     {5, 86400, DegradeSkip, false},
		"metatron":      {5, 86400, DegradeSkip, false},
	}
	if len(registry) != len(want) {
		t.Fatalf("registry has %d classes, contract has %d", len(registry), len(want))
	}
	for name, w := range want {
		dc, ok := ClassFor(name)
		if !ok {
			t.Fatalf("class %q missing", name)
		}
		if dc.Points != w.points || dc.BudgetTicks != w.budget || dc.Degrade != w.deg || dc.FutureDated != w.future {
			t.Errorf("class %q = %+v, want %+v", name, dc, w)
		}
	}
}

func TestClassForKind(t *testing.T) {
	for kind, class := range map[string]string{
		"planner": "planner", "conversation": "conversation",
		"meeting": "meeting", "consolidation": "consolidation",
		"narrator": "chronicle", "drama": "chronicle", "metatron": "metatron",
	} {
		dc, ok := ClassForKind(kind)
		if !ok || dc.Class != class {
			t.Errorf("ClassForKind(%q) = %q, %v; want %q", kind, dc.Class, ok, class)
		}
	}
	if _, ok := ClassForKind("no-such-kind"); ok {
		t.Error("unknown kind resolved")
	}
}

func TestValidateKindsNamesOffender(t *testing.T) {
	if err := ValidateKinds([]string{"planner", "conversation"}); err != nil {
		t.Fatalf("known kinds: %v", err)
	}
	err := ValidateKinds([]string{"planner", "oracle"})
	if err == nil {
		t.Fatal("unregistered kind passed")
	}
	if !strings.Contains(err.Error(), `"oracle"`) {
		t.Errorf("error does not name the offender: %v", err)
	}
}
