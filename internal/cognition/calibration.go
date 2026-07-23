package cognition

import (
	"encoding/json"
	"fmt"
	"os"
)

// ShapeSamples is the audit trail for one reference-workload shape. WallMs is
// per-sample wall time: one model call for a single-shot shape, the whole
// tool-use loop for a loop shape (spec 017).
type ShapeSamples struct {
	Shape  string  `json:"shape"`
	Points int     `json:"points"`
	WallMs []int64 `json:"wall_ms"`
}

// TierProfile is one tier's measured identity and baseline.
//
// SecondsPerPoint is the tier's normalized cost per Fibonacci point. For a
// single-shot cognition kind it is one model call's wall time per point; for a
// loop cognition (the villager planner on the local tier, spec 017) it is the
// WHOLE tool-use loop's wall time per point — the same unit the live estimator
// observes via Orchestrator.ObserveCognition — so a seeded baseline and a live
// observation are directly comparable and the router's suppression arithmetic
// stays truthful when a cognition is N model calls.
type TierProfile struct {
	Model           string         `json:"model"`
	Endpoint        string         `json:"endpoint,omitempty"`
	SecondsPerPoint float64        `json:"seconds_per_point"`
	Samples         []ShapeSamples `json:"samples"`
}

// Profile is calibration.json (specs/007-cognition-horizon/contracts/
// calibration.md): written only by `promptworld calibrate`, read once at
// daemon start to seed the live estimators.
type Profile struct {
	CalibratedAt string                 `json:"calibrated_at"`
	Tiers        map[string]TierProfile `json:"tiers"`
}

// LoadProfile reads calibration.json. A missing file returns (nil, nil) —
// legal; bootstrap defaults apply. A malformed file returns an error the
// caller downgrades to a warning + bootstrap, never a crash.
func LoadProfile(path string) (*Profile, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("calibration profile: %w", err)
	}
	var p Profile
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("calibration profile %s: %w", path, err)
	}
	return &p, nil
}

// Save writes the profile as a full-file replace. Only the calibrate command
// calls this; the daemon never writes the file (auditability — the recorded
// baseline moves only under a human's hand).
func (p *Profile) Save(path string) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// SeedFor returns the seconds-per-point seed for a provider (spec 024 R5): the
// profile's recorded baseline when present and positive, else a bootstrap by
// pricing class. The profile is keyed by PROVIDER NAME now — legacy worlds
// derive providers named "local"/"cloud", so a pre-spec-024 tier-keyed
// calibration.json keeps matching by name with no translation table.
//
// The miss fallback is by pricing class (decision-5's surviving local/cloud
// distinction), not by name: a zero-priced provider seeds from the local
// bootstrap constant, a priced one from the cloud constant. So a fresh v2
// provider named neither "local" nor "cloud" still cold-starts sanely, and the
// legacy names land on their historical constants (local is zero-priced, cloud
// is priced) — byte-identical to the pre-spec-024 name-keyed fallback.
func SeedFor(p *Profile, name string, zeroPriced bool) float64 {
	if p != nil {
		if tp, ok := p.Tiers[name]; ok && tp.SecondsPerPoint > 0 {
			return tp.SecondsPerPoint
		}
	}
	if zeroPriced {
		return BootstrapLocalSecPerPt
	}
	return BootstrapCloudSecPerPt
}
