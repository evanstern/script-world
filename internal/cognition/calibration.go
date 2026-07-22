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

// SeedFor returns the seconds-per-point seed for a tier: the profile's
// recorded baseline when present and positive, else the pessimistic
// bootstrap default.
func SeedFor(p *Profile, tier string) float64 {
	if p != nil {
		if tp, ok := p.Tiers[tier]; ok && tp.SecondsPerPoint > 0 {
			return tp.SecondsPerPoint
		}
	}
	if tier == "cloud" {
		return BootstrapCloudSecPerPt
	}
	return BootstrapLocalSecPerPt
}
