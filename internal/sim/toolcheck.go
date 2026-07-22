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
// The whitelist-subset check (every Expressive tool's Events ⊆
// injectSocialWhitelist) is added in Phase 4 (T022) once the expressive
// capabilities read the registry.
func ValidateToolCoverage() error {
	var errs []error
	for _, t := range tool.All() {
		if t.Effect != tool.World {
			continue
		}
		if _, ok := goalResolvers[t.Name]; !ok {
			errs = append(errs, fmt.Errorf("world tool %q has no resolver arm (goalResolvers)", t.Name))
		}
		if _, ok := intentDurations[t.Name]; !ok {
			errs = append(errs, fmt.Errorf("world tool %q has no duration (intentDurations)", t.Name))
		}
	}
	return errors.Join(errs...)
}
