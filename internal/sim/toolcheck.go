package sim

import (
	"errors"
	"fmt"

	"github.com/evanstern/promptworld/internal/tool"
)

// ValidateToolCoverage cross-checks the tool registry against sim's behavior
// tables (spec 014, R9): every World tool must have a resolver-table entry
// (goalResolvers) and a duration entry (intentDurations). These tables and the
// injection whitelist are sim's, so this coverage check lives here rather than
// in tool.Validate — the tool package is a leaf that cannot see them.
//
// It is called from daemon boot alongside tool.Validate(); a violation aborts
// before the world runs (FR-003), never at tick time. The same check runs as a
// unit test so CI catches a missing resolver before any daemon does.
//
// Two families of check:
//   - World tools: every one has a resolver arm (goalResolvers) and a duration
//     (intentDurations).
//   - Expressive tools: every declared event type is ⊆ injectSocialWhitelist
//     (spec 014 T022/FR-013) — the registry formalizes which slice of the
//     whitelist each capability uses; it may never name a type the door would
//     reject. The whitelist itself is unchanged (the door-level backstop).
func ValidateToolCoverage() error {
	var errs []error
	for _, t := range tool.All() {
		switch t.Effect {
		case tool.World:
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
