package mind

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// updateGolden regenerates the planner-prompt fixture instead of asserting
// against it: `go test ./internal/mind -run TestGoldenPrompt -update-golden`.
var updateGolden = flag.Bool("update-golden", false, "rewrite the planner-prompt golden fixture")

// TestGoldenPrompt is the byte-identity anchor for the tool-registry migration
// (spec 014, R3/SC-003): it pins the full systemPrompt output — persona +
// goal vocabulary + gloss block — byte-for-byte. Captured against pre-refactor
// code; it MUST pass UNCHANGED once the vocabulary/gloss are derived from the
// registry (T011). A diff here means the prompt bytes moved, which would break
// prompt-cache stability and the behavior-identity guarantee.
func TestGoldenPrompt(t *testing.T) {
	// A representative persona: multi-paragraph text exercising the persona
	// block and the vocabulary/gloss that follows it.
	const name = "Hazel"
	const persona = "You are Hazel, level-headed and patient. You keep the fire lit and\nremember who helped when times were lean."

	got := systemPrompt(name, persona)

	path := filepath.Join("testdata", "planner_prompt.golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("wrote golden fixture %s (%d bytes)", path, len(got))
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update-golden to create it): %v", err)
	}
	if got != string(want) {
		t.Errorf("systemPrompt output drifted from the golden fixture.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
