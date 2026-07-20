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
					{Shape: "musing-1pt", Points: 1, WallMs: []int64{16100, 17800}},
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

func TestSeedForBootstrap(t *testing.T) {
	if got := SeedFor(nil, "local"); got != BootstrapLocalSecPerPt {
		t.Errorf("local bootstrap = %g", got)
	}
	if got := SeedFor(nil, "cloud"); got != BootstrapCloudSecPerPt {
		t.Errorf("cloud bootstrap = %g", got)
	}
	p := &Profile{Tiers: map[string]TierProfile{"local": {SecondsPerPoint: 17.2}}}
	if got := SeedFor(p, "local"); got != 17.2 {
		t.Errorf("profile seed = %g", got)
	}
	if got := SeedFor(p, "cloud"); got != BootstrapCloudSecPerPt {
		t.Errorf("absent tier must bootstrap: %g", got)
	}
}
