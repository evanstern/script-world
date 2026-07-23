package llm

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeConfigFile writes raw JSON to a temp llm.json and returns the path.
func writeConfigFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "llm.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLegacyShapeDerivesRegistry (spec 024 FR-004, US1 AC#1): an untouched
// legacy llm.json derives two providers named local/cloud — local is zero-priced
// openai_compat carrying parallel/tool_mode, cloud is anthropic carrying pricing
// and keys — with today's kind→tier table as single-entry routes. This is the
// P1 equivalence guarantee at the config layer.
func TestLegacyShapeDerivesRegistry(t *testing.T) {
	p := writeConfigFile(t, `{
		"monthly_budget_usd": 100,
		"local": {"endpoint": "http://localhost:11434/v1", "model": "gemma", "parallel": 4, "tool_mode": "json"},
		"cloud": {"model": "claude-opus-4-8", "input_usd_per_mtok": 5, "output_usd_per_mtok": 25, "api_key_env": "K"}
	}`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	providers, routes, err := cfg.resolveRegistry()
	if err != nil {
		t.Fatalf("resolveRegistry: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("derived %d providers, want 2 (local, cloud)", len(providers))
	}
	local := providers["local"]
	if local.Transport != ProviderOpenAICompat || local.Model != "gemma" ||
		!local.zeroPriced() || local.Parallel != 4 || local.ToolMode != "json" {
		t.Errorf("derived local = %+v", local)
	}
	cloud := providers["cloud"]
	if cloud.Transport != ProviderAnthropic || cloud.zeroPriced() ||
		cloud.InputUSDPerMTok != 5 || cloud.OutputUSDPerMTok != 25 || cloud.APIKeyEnv != "K" {
		t.Errorf("derived cloud = %+v", cloud)
	}

	// Routes cover exactly the accepted kinds, each a single-entry chain, with
	// today's placement: planner/conversation/meeting → local; the rest → cloud.
	if len(routes) != len(acceptedKinds) {
		t.Fatalf("derived %d routes, want %d", len(routes), len(acceptedKinds))
	}
	wantLocal := map[Kind]bool{KindPlanner: true, KindConversation: true, KindMeeting: true}
	for kind := range acceptedKinds {
		rc, ok := routes[kind]
		if !ok {
			t.Errorf("kind %q has no derived route", kind)
			continue
		}
		if len(rc.Chain) != 1 || rc.NoFallback {
			t.Errorf("route %q = %+v, want a single-entry fallback-free chain", kind, rc)
			continue
		}
		want := "cloud"
		if wantLocal[kind] {
			want = "local"
		}
		if rc.Chain[0] != want {
			t.Errorf("route %q → %q, want %q", kind, rc.Chain[0], want)
		}
	}
}

// TestLegacyCloudProviderTransportMapping: cloud.provider "" | "anthropic" maps
// to the anthropic transport; "openai_compat" maps to openai_compat (FR-004
// transport mapping).
func TestLegacyCloudProviderTransportMapping(t *testing.T) {
	cases := []struct {
		provider string
		want     string
	}{
		{"", ProviderAnthropic},
		{"anthropic", ProviderAnthropic},
		{"openai_compat", ProviderOpenAICompat},
	}
	for _, c := range cases {
		cfg := Config{
			MonthlyBudgetUSD: 100,
			Local:            LocalConfig{Endpoint: "http://x/v1", Model: "m"},
			Cloud:            CloudConfig{Provider: c.provider, Model: "m", Endpoint: "http://cloud/v1"},
		}
		providers, _, err := cfg.resolveRegistry()
		if err != nil {
			t.Fatalf("provider %q: %v", c.provider, err)
		}
		if got := providers["cloud"].Transport; got != c.want {
			t.Errorf("cloud.provider %q → transport %q, want %q", c.provider, got, c.want)
		}
	}
}

// TestDefaultV2SemanticallyEqualsLegacy (spec 024 FR-017, T006): the v2 default
// WriteDefault emits derives the SAME registry a legacy default would — proving
// the new-world scaffold is semantically identical to today's two-tier defaults.
func TestDefaultV2SemanticallyEqualsLegacy(t *testing.T) {
	vp, vr, err := DefaultConfig().resolveRegistry()
	if err != nil {
		t.Fatalf("v2 default resolveRegistry: %v", err)
	}
	legacy := Config{
		MonthlyBudgetUSD: 100,
		Local:            LocalConfig{Endpoint: "http://localhost:11434/v1", Model: "gemma4:12b-mlx"},
		Cloud:            CloudConfig{Model: "claude-opus-4-8", InputUSDPerMTok: 5, OutputUSDPerMTok: 25, APIKeyEnv: "ANTHROPIC_API_KEY"},
	}
	lp, lr, err := legacy.resolveRegistry()
	if err != nil {
		t.Fatalf("legacy default resolveRegistry: %v", err)
	}
	if !reflect.DeepEqual(vp, lp) {
		t.Errorf("v2 default providers differ from legacy:\n v2=%+v\nleg=%+v", vp, lp)
	}
	if !reflect.DeepEqual(vr, lr) {
		t.Errorf("v2 default routes differ from legacy:\n v2=%+v\nleg=%+v", vr, lr)
	}
}

// v2Body composes a valid two-provider (gemma/anthropic) v2 config with a
// caller-supplied routes object, for the validation matrix's route mutations.
func v2Body(routes string) string {
	return `{"monthly_budget_usd":100,` +
		`"providers":{` +
		`"gemma":{"transport":"openai_compat","endpoint":"http://x/v1","model":"g"},` +
		`"anthropic":{"transport":"anthropic","model":"claude","input_usd_per_mtok":5}},` +
		`"routes":` + routes + `}`
}

// validRoutes covers exactly the seven accepted kinds — the baseline a matrix
// case mutates.
const validRoutes = `{"planner":["gemma"],"conversation":["gemma"],"meeting":["gemma"],` +
	`"consolidation":["anthropic"],"narrator":["anthropic"],"drama":["anthropic"],"metatron":["anthropic"]}`

// TestValidationMatrix (spec 024 FR-003, SC-008, contracts/llm-config.md): every
// row of the boot-error matrix fails LoadConfig with an error naming the
// offending entry — never a runtime surprise. Warn-and-clamp knobs (parallel,
// tool_mode) are NOT in this matrix; they clamp, never error.
func TestValidationMatrix(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string // substring the boot error must contain
	}{
		{
			"route names undeclared provider",
			v2Body(`{"planner":["ghost"],"conversation":["gemma"],"meeting":["gemma"],"consolidation":["anthropic"],"narrator":["anthropic"],"drama":["anthropic"],"metatron":["anthropic"]}`),
			"ghost",
		},
		{
			"accepted kind missing from routes",
			v2Body(`{"planner":["gemma"],"conversation":["gemma"],"meeting":["gemma"],"consolidation":["anthropic"],"narrator":["anthropic"],"drama":["anthropic"]}`),
			"metatron",
		},
		{
			"unknown kind key in routes",
			v2Body(`{"planner":["gemma"],"conversation":["gemma"],"meeting":["gemma"],"consolidation":["anthropic"],"narrator":["anthropic"],"drama":["anthropic"],"metatron":["anthropic"],"sorcery":["gemma"]}`),
			"sorcery",
		},
		{
			"duplicate provider within a chain",
			v2Body(`{"planner":["gemma","gemma"],"conversation":["gemma"],"meeting":["gemma"],"consolidation":["anthropic"],"narrator":["anthropic"],"drama":["anthropic"],"metatron":["anthropic"]}`),
			"planner",
		},
		{
			"empty chain",
			v2Body(`{"planner":[],"conversation":["gemma"],"meeting":["gemma"],"consolidation":["anthropic"],"narrator":["anthropic"],"drama":["anthropic"],"metatron":["anthropic"]}`),
			"planner",
		},
		{
			"no_fallback with chain length > 1",
			v2Body(`{"planner":{"chain":["gemma","anthropic"],"no_fallback":true},"conversation":["gemma"],"meeting":["gemma"],"consolidation":["anthropic"],"narrator":["anthropic"],"drama":["anthropic"],"metatron":["anthropic"]}`),
			"planner",
		},
		{
			"provider missing model",
			`{"monthly_budget_usd":100,"providers":{"gemma":{"transport":"openai_compat","endpoint":"http://x/v1"},"anthropic":{"transport":"anthropic","model":"c"}},"routes":` + validRoutes + `}`,
			"model",
		},
		{
			"provider missing transport",
			`{"monthly_budget_usd":100,"providers":{"gemma":{"endpoint":"http://x/v1","model":"g"},"anthropic":{"transport":"anthropic","model":"c"}},"routes":` + validRoutes + `}`,
			"transport",
		},
		{
			"openai_compat missing endpoint",
			`{"monthly_budget_usd":100,"providers":{"gemma":{"transport":"openai_compat","model":"g"},"anthropic":{"transport":"anthropic","model":"c"}},"routes":` + validRoutes + `}`,
			"endpoint",
		},
		{
			"both providers and legacy present",
			`{"monthly_budget_usd":100,"providers":{"gemma":{"transport":"openai_compat","endpoint":"http://x/v1","model":"g"},"anthropic":{"transport":"anthropic","model":"c"}},"routes":` + validRoutes + `,"local":{"model":"m"}}`,
			"mutually exclusive",
		},
		{
			"monthly_budget_usd not positive",
			`{"monthly_budget_usd":0,"providers":{"gemma":{"transport":"openai_compat","endpoint":"http://x/v1","model":"g"},"anthropic":{"transport":"anthropic","model":"c"}},"routes":` + validRoutes + `}`,
			"monthly_budget_usd",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := writeConfigFile(t, c.body)
			_, err := LoadConfig(p)
			if err == nil {
				t.Fatalf("expected a boot error naming %q, got nil", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("boot error %q does not name the offender %q", err.Error(), c.want)
			}
		})
	}
}

// TestValidV2Loads: the valid baseline the matrix mutates loads cleanly and its
// registry resolves — a positive control for the matrix.
func TestValidV2Loads(t *testing.T) {
	p := writeConfigFile(t, v2Body(validRoutes))
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("valid v2 config rejected: %v", err)
	}
	providers, routes, err := cfg.resolveRegistry()
	if err != nil {
		t.Fatalf("resolveRegistry on valid v2: %v", err)
	}
	if len(providers) != 2 || len(routes) != len(acceptedKinds) {
		t.Errorf("valid v2 resolved to %d providers / %d routes", len(providers), len(routes))
	}
}
