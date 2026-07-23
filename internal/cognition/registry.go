package cognition

import "fmt"

// Degrade names the deterministic action a decision class falls to when the
// router suppresses it: the model is not consulted, and the class's floor
// behavior runs instead.
type Degrade string

const (
	DegradeSkip     Degrade = "skip"     // nothing replaces the thought (recorded, not silent)
	DegradeReflex   Degrade = "reflex"   // the deterministic reflex floor covers
	DegradeTemplate Degrade = "template" // pre-authored text stands (meeting rephrase)
	// DegradeFasterTier is registry-expressible but not wired in v1: a class
	// declaring it is treated as DegradeSkip until a faster tier exists.
	DegradeFasterTier Degrade = "faster-tier"
)

// DecisionClass is one registered category of model-reaching decision: its
// thought cost in Fibonacci points (a property of the prompt shape,
// host-independent) and its staleness budget in game ticks (a property of
// the fiction). Values are doctrine (decision-4): changing one is a reviewed
// code change, never runtime tuning.
type DecisionClass struct {
	Class       string
	Points      int
	BudgetTicks int64
	Degrade     Degrade
	FutureDated bool
}

// fibonacci is the closed set of legal point values.
var fibonacci = map[int]bool{1: true, 2: true, 3: true, 5: true, 8: true, 13: true}

// registry holds the initial values from
// specs/007-cognition-horizon/contracts/registry.md.
var registry = map[string]DecisionClass{
	"planner":       {Class: "planner", Points: 3, BudgetTicks: 1200, Degrade: DegradeReflex, FutureDated: true},
	"conversation":  {Class: "conversation", Points: 13, BudgetTicks: 7200, Degrade: DegradeSkip},
	"meeting":       {Class: "meeting", Points: 2, BudgetTicks: 3600, Degrade: DegradeTemplate},
	"consolidation": {Class: "consolidation", Points: 5, BudgetTicks: 28800, Degrade: DegradeSkip},
	"chronicle":     {Class: "chronicle", Points: 5, BudgetTicks: 86400, Degrade: DegradeSkip},
	"metatron":      {Class: "metatron", Points: 5, BudgetTicks: 86400, Degrade: DegradeSkip},
}

// kindToClass maps every llm call kind (as a string, keeping this package
// leaf) to its decision class. Completeness against the orchestrator's
// accepted kinds is enforced at daemon start via ValidateKinds (FR-002).
var kindToClass = map[string]string{
	"planner":       "planner",
	"conversation":  "conversation",
	"meeting":       "meeting",
	"consolidation": "consolidation",
	"narrator":      "chronicle",
	"drama":         "chronicle",
	"metatron":      "metatron",
}

// ClassFor returns the registered class by name.
func ClassFor(class string) (DecisionClass, bool) {
	dc, ok := registry[class]
	return dc, ok
}

// ClassForKind resolves an llm call kind to its decision class.
func ClassForKind(kind string) (DecisionClass, bool) {
	name, ok := kindToClass[kind]
	if !ok {
		return DecisionClass{}, false
	}
	return ClassFor(name)
}

// Validate checks registry invariants: Fibonacci point membership, positive
// budgets, kind mappings that resolve. Fatal at daemon start on failure.
func Validate() error {
	for name, dc := range registry {
		if dc.Class != name {
			return fmt.Errorf("cognition registry: class %q keyed as %q", dc.Class, name)
		}
		if !fibonacci[dc.Points] {
			return fmt.Errorf("cognition registry: class %q points %d not in the Fibonacci set", name, dc.Points)
		}
		if dc.BudgetTicks <= 0 {
			return fmt.Errorf("cognition registry: class %q budget %d not positive", name, dc.BudgetTicks)
		}
	}
	for kind, class := range kindToClass {
		if _, ok := registry[class]; !ok {
			return fmt.Errorf("cognition registry: kind %q maps to unregistered class %q", kind, class)
		}
	}
	return nil
}

// ValidateKinds enforces intentional categorization (FR-002): every kind the
// orchestrator accepts must resolve to a registered decision class, or the
// daemon must not start. The error names the offender.
func ValidateKinds(kinds []string) error {
	if err := Validate(); err != nil {
		return err
	}
	for _, k := range kinds {
		if _, ok := ClassForKind(k); !ok {
			return fmt.Errorf("cognition registry: llm kind %q has no registered decision class — register it in internal/cognition/registry.go (FR-002)", k)
		}
	}
	return nil
}
