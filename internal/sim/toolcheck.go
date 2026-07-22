package sim

import (
	"errors"
	"fmt"

	"github.com/evanstern/promptworld/internal/tool"
)

// ValidateToolCoverage cross-checks the tool registry against sim's behavior
// tables (spec 014, R9): every GOAL-DOOR World tool must have a resolver-table
// entry (goalResolvers) and a duration entry (intentDurations). These tables
// and the injection whitelist are sim's, so this coverage check lives here
// rather than in tool.Validate — the tool package is a leaf that cannot see
// them.
//
// It is called from daemon boot alongside tool.Validate(); a violation aborts
// before the world runs (FR-003), never at tick time. The same check runs as a
// unit test so CI catches a missing resolver before any daemon does.
//
// Two families of check:
//   - Goal-door World tools: every one has a resolver arm (goalResolvers) and
//     a duration (intentDurations). "Goal-door" means Effect World AND
//     PlanStep true — the same predicate internal/tool calls
//     isLegacyWorldTool (derive.go) and exposes as tool.WorldGoals(): the set
//     of names resolveGoal actually dispatches on. A World tool CAN fall
//     outside this set (spec 017 R11): set_plan is Effect World but grounds
//     through injectPlan (each plan step resolves its own, already-covered
//     goal), never through resolveGoal/goalResolvers — so it carries
//     PlanStep: false and is deliberately exempt from this pair of tables.
//     validateCoverage checks PlanStep directly (not tool.WorldGoals()) so a
//     synthetic fixture list — as the tests below pass — is judged on its own
//     declared shape, not the live package registry.
//   - Expressive tools: every declared event type is ⊆ injectSocialWhitelist
//     (spec 014 T022/FR-013) — the registry formalizes which slice of the
//     whitelist each capability uses; it may never name a type the door would
//     reject. The whitelist itself is unchanged (the door-level backstop).
func ValidateToolCoverage() error {
	return validateCoverage(tool.All())
}

// validateCoverage is the coverage check over an explicit tool list — the
// exported entry passes the live registry; tests pass synthetic lists to prove
// a missing resolver/duration is caught before boot (and that a non-goal-door
// World tool is exempt).
func validateCoverage(tools []tool.Tool) error {
	var errs []error
	for _, t := range tools {
		switch t.Effect {
		case tool.World:
			if !t.PlanStep {
				// Non-goal-door World tool (set_plan): grounds through its own
				// door (injectPlan), never resolveGoal/goalResolvers — exempt.
				continue
			}
			if _, ok := goalResolvers[t.Name]; !ok {
				errs = append(errs, fmt.Errorf("world tool %q has no resolver arm (goalResolvers)", t.Name))
			}
			if _, ok := intentDurations[t.Name]; !ok {
				errs = append(errs, fmt.Errorf("world tool %q has no duration (intentDurations)", t.Name))
			}
		case tool.Expressive:
			for _, ev := range t.Events {
				if !injectSocialWhitelist[ev] {
					errs = append(errs, fmt.Errorf("expressive tool %q declares event %q not in injectSocialWhitelist", t.Name, ev))
				}
			}
		}
	}
	return errors.Join(errs...)
}
