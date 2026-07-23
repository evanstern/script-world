package cognition

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calibration.json")
	p := &Profile{
		CalibratedAt: "2026-07-20T21:40:00Z",
		Tiers: map[string]TierProfile{
			"local": {
				Model:           "test-model",
				Endpoint:        "http://localhost:11434/v1",
				SecondsPerPoint: 17.2,
				Samples: []ShapeSamples{
					{Shape: "planner-3pt", Points: 3, WallMs: []int64{16100, 17800}},
				},
			},
		},
	}
	if err := p.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if got.Tiers["local"].SecondsPerPoint != 17.2 || got.Tiers["local"].Model != "test-model" {
		t.Errorf("round trip = %+v", got.Tiers["local"])
	}
}

func TestProfileMissingIsLegal(t *testing.T) {
	p, err := LoadProfile(filepath.Join(t.TempDir(), "calibration.json"))
	if err != nil || p != nil {
		t.Errorf("missing file: p=%v err=%v; want nil, nil", p, err)
	}
}

func TestProfileMalformedErrorsWithoutPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calibration.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfile(path); err == nil {
		t.Error("malformed file loaded without error")
	}
}

// TestSeedForBootstrapClasses (spec 024 T008/R5): a profile miss bootstraps by
// PRICING CLASS, not by name — a zero-priced provider seeds from the local
// constant, a priced one from the cloud constant, whatever the provider is
// named. So a fresh v2 provider named "gemma-fast" (zero-priced) or "opus"
// (priced) cold-starts on the right class.
func TestSeedForBootstrapClasses(t *testing.T) {
	if got := SeedFor(nil, "gemma-fast", true); got != BootstrapLocalSecPerPt {
		t.Errorf("zero-priced bootstrap = %g, want local %g", got, BootstrapLocalSecPerPt)
	}
	if got := SeedFor(nil, "opus", false); got != BootstrapCloudSecPerPt {
		t.Errorf("priced bootstrap = %g, want cloud %g", got, BootstrapCloudSecPerPt)
	}
	// The legacy names land on their historical constants via their classes
	// (local is zero-priced, cloud is priced) — byte-identical to the old
	// name-keyed fallback.
	if got := SeedFor(nil, "local", true); got != BootstrapLocalSecPerPt {
		t.Errorf("legacy local bootstrap = %g", got)
	}
	if got := SeedFor(nil, "cloud", false); got != BootstrapCloudSecPerPt {
		t.Errorf("legacy cloud bootstrap = %g", got)
	}
}

// TestSeedForMatchesByProviderName (spec 024 T008): a recorded baseline is keyed
// by provider name, so a pre-spec-024 tier-keyed calibration.json (keys
// "local"/"cloud") keeps matching the derived legacy providers with no
// translation, and a v2 provider's own name keys its own entry. A missing entry
// still falls back to the pricing class.
func TestSeedForMatchesByProviderName(t *testing.T) {
	p := &Profile{Tiers: map[string]TierProfile{
		"local": {SecondsPerPoint: 17.2}, // legacy key, still matches by name
		"opus":  {SecondsPerPoint: 9.4},  // a v2 provider's own name
	}}
	if got := SeedFor(p, "local", true); got != 17.2 {
		t.Errorf("legacy profile key not matched by name: %g", got)
	}
	if got := SeedFor(p, "opus", false); got != 9.4 {
		t.Errorf("v2 provider name not matched: %g", got)
	}
	// Absent entry bootstraps by class, ignoring the recorded siblings.
	if got := SeedFor(p, "cloud", false); got != BootstrapCloudSecPerPt {
		t.Errorf("absent provider must bootstrap by class: %g", got)
	}
}
