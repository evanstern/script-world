package tool

import (
	"errors"
	"fmt"
)

// Validate enforces the registry's structural invariants (research R9,
// FR-003). It is called from daemon boot and from a unit test, so a malformed
// registry or roster fails fast — before the world runs, never at tick time.
// It returns ALL violations (joined), not just the first, so a bad edit is
// diagnosed in one pass.
//
// The whitelist-subset check (every Expressive tool's Events ⊆
// injectSocialWhitelist) and the resolver/duration coverage check live sim-side
// (internal/sim/toolcheck.go): the whitelist and the resolver table are sim's,
// and this package is a leaf that imports nothing internal.
func Validate() error {
	var errs []error

	seen := make(map[string]bool, len(registry))
	for _, t := range registry {
		if t.Name == "" {
			errs = append(errs, errors.New("tool with empty name"))
			continue
		}
		if seen[t.Name] {
			errs = append(errs, fmt.Errorf("duplicate tool name %q", t.Name))
		}
		seen[t.Name] = true

		switch t.Effect {
		case World, Expressive, Read:
		default:
			errs = append(errs, fmt.Errorf("tool %q: unknown effect class %d", t.Name, t.Effect))
		}

		// Events may be declared only by Expressive tools. (An Expressive tool
		// MAY be eventless — converse produces a transcript, no world events —
		// so non-emptiness is not required; that direction of data-model.md's
		// "iff" is deliberately relaxed for converse, see registry.go.)
		if len(t.Events) > 0 && t.Effect != Expressive {
			errs = append(errs, fmt.Errorf("tool %q: %s tool declares Events (only expressive tools may)", t.Name, t.Effect))
		}

		// PlanStep / ReflexEligible are world-tool doctrine only.
		if t.PlanStep && t.Effect != World {
			errs = append(errs, fmt.Errorf("tool %q: PlanStep set on a non-world tool", t.Name))
		}
		if t.ReflexEligible && t.Effect != World {
			errs = append(errs, fmt.Errorf("tool %q: ReflexEligible set on a non-world tool", t.Name))
		}

		// Well-formed params: an Enum descriptor must list its allowed values.
		for _, p := range t.Params {
			if p.Kind == Enum && len(p.Enum) == 0 {
				errs = append(errs, fmt.Errorf("tool %q: enum param %q has no values", t.Name, p.Name))
			}
		}
	}

	// Rosters: every name resolves to a registry entry, and no roster may name
	// a Read tool in this layer.
	for _, r := range []struct {
		name  string
		names []string
	}{{"villager", RosterVillager}, {"metatron", RosterMetatron}} {
		for _, n := range r.names {
			t, ok := Lookup(n)
			if !ok {
				errs = append(errs, fmt.Errorf("%s roster names %q, which is not in the registry", r.name, n))
				continue
			}
			if t.Effect == Read {
				errs = append(errs, fmt.Errorf("%s roster names Read tool %q (read tools are not callable in this layer)", r.name, n))
			}
		}
	}

	return errors.Join(errs...)
}
