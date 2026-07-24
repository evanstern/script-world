package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/cognition"
)

// TestSeedCalibrationRecordsProvenanceFullProfile (spec 035 T005): a profile
// entry for every provider records that provider's CalibratedAt.
func TestSeedCalibrationRecordsProvenanceFullProfile(t *testing.T) {
	o := newOrch(t, testConfig("http://127.0.0.1:1", "http://127.0.0.1:1", 100), testStore(t))
	profile := &cognition.Profile{
		CalibratedAt: "2026-07-20T21:40:00Z",
		Tiers: map[string]cognition.TierProfile{
			"local": {SecondsPerPoint: 0.94},
			"cloud": {SecondsPerPoint: 1.2},
		},
	}
	o.SeedCalibration(profile)

	if got := o.CalibratedAt("local"); got != profile.CalibratedAt {
		t.Errorf("local CalibratedAt = %q, want %q", got, profile.CalibratedAt)
	}
	if got := o.CalibratedAt("cloud"); got != profile.CalibratedAt {
		t.Errorf("cloud CalibratedAt = %q, want %q", got, profile.CalibratedAt)
	}
}

// TestSeedCalibrationPartialProfile: a profile covering only "local" leaves
// "cloud" on bootstrap (empty CalibratedAt) — per-provider truth (US3
// scenario 3, contracts/warnings.md Test obligations).
func TestSeedCalibrationPartialProfile(t *testing.T) {
	o := newOrch(t, testConfig("http://127.0.0.1:1", "http://127.0.0.1:1", 100), testStore(t))
	profile := &cognition.Profile{
		CalibratedAt: "2026-07-20T21:40:00Z",
		Tiers: map[string]cognition.TierProfile{
			"local": {SecondsPerPoint: 0.94},
		},
	}
	o.SeedCalibration(profile)

	if got := o.CalibratedAt("local"); got != profile.CalibratedAt {
		t.Errorf("local CalibratedAt = %q, want %q", got, profile.CalibratedAt)
	}
	if got := o.CalibratedAt("cloud"); got != "" {
		t.Errorf("cloud CalibratedAt = %q, want empty (uncovered by profile)", got)
	}
}

// TestCalibratedAtNilProfileAllBootstrap: SeedCalibration(nil) — and never
// calling it at all — both leave every provider bootstrap (empty
// CalibratedAt); a never-seeded provider is bootstrap from construction.
func TestCalibratedAtNilProfileAllBootstrap(t *testing.T) {
	o := newOrch(t, testConfig("http://127.0.0.1:1", "http://127.0.0.1:1", 100), testStore(t))
	if got := o.CalibratedAt("local"); got != "" {
		t.Errorf("never-seeded local CalibratedAt = %q, want empty", got)
	}
	o.SeedCalibration(nil)
	if got := o.CalibratedAt("local"); got != "" {
		t.Errorf("SeedCalibration(nil) local CalibratedAt = %q, want empty", got)
	}
	if got := o.CalibratedAt("cloud"); got != "" {
		t.Errorf("SeedCalibration(nil) cloud CalibratedAt = %q, want empty", got)
	}
}

// TestCalibratedAtUnknownProvider: a name with no provider reports empty
// rather than panicking.
func TestCalibratedAtUnknownProvider(t *testing.T) {
	o := newOrch(t, testConfig("http://127.0.0.1:1", "http://127.0.0.1:1", 100), testStore(t))
	if got := o.CalibratedAt("nonexistent"); got != "" {
		t.Errorf("unknown provider CalibratedAt = %q, want empty", got)
	}
}

// TestStatusSnapshotCarriesCalibratedAt (spec 035 T012): the wire field
// mirrors SeedCalibration's per-provider provenance decision.
func TestStatusSnapshotCarriesCalibratedAt(t *testing.T) {
	o := newOrch(t, testConfig("http://127.0.0.1:1", "http://127.0.0.1:1", 100), testStore(t))
	profile := &cognition.Profile{
		CalibratedAt: "2026-07-20T21:40:00Z",
		Tiers: map[string]cognition.TierProfile{
			"local": {SecondsPerPoint: 0.94},
		},
	}
	o.SeedCalibration(profile)

	st := o.StatusSnapshot()
	local := provStatus(st, "local")
	if local.CalibratedAt != profile.CalibratedAt {
		t.Errorf("local status CalibratedAt = %q, want %q", local.CalibratedAt, profile.CalibratedAt)
	}
	cloud := provStatus(st, "cloud")
	if cloud.CalibratedAt != "" {
		t.Errorf("cloud status CalibratedAt = %q, want empty (bootstrap)", cloud.CalibratedAt)
	}
}

// TestProviderStatusCalibratedAtOmitempty: marshaling a bootstrap provider
// omits calibrated_at entirely (FR-008 byte-identity); a calibrated provider
// carries it verbatim.
func TestProviderStatusCalibratedAtOmitempty(t *testing.T) {
	bootstrap := ProviderStatus{Name: "local", Model: "m"}
	b, err := json.Marshal(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "calibrated_at") {
		t.Errorf("bootstrap ProviderStatus must omit calibrated_at, got %s", b)
	}

	calibrated := ProviderStatus{Name: "local", Model: "m", CalibratedAt: "2026-07-20T21:40:00Z"}
	b, err = json.Marshal(calibrated)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"calibrated_at":"2026-07-20T21:40:00Z"`) {
		t.Errorf("calibrated ProviderStatus missing calibrated_at, got %s", b)
	}
}
