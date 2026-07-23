package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Config is llm.json in the world save directory. Endpoints, models, and
// pricing live here; hosted-API keys never do — only the NAME of an
// environment variable that carries one. The one exception is api_key,
// meant for keys that guard LAN-local routers and secure nothing beyond
// the operator's own machines.
//
// Two shapes load (spec 024, decision-5): the v2 registry (`providers` +
// `routes`) and the legacy two-tier (`local`/`cloud`) shape. They are mutually
// exclusive — a legacy file derives a two-provider registry named local/cloud
// with today's routes (resolveRegistry), so every existing world boots byte-
// identically with zero edits (FR-004).
type Config struct {
	MonthlyBudgetUSD float64 `json:"monthly_budget_usd"`
	// Providers is the v2 registry: uniquely named model sources (names are the
	// map keys, so duplicate names are unrepresentable). Presence of a non-empty
	// map selects v2 parsing; empty selects legacy derivation from Local/Cloud.
	Providers map[string]ProviderConfig `json:"providers,omitempty"`
	// Routes maps every accepted call kind to an ordered provider chain (FR-002).
	// The key is a Kind string; load validates it names a real kind and covers
	// exactly Kinds() (completeness, both directions, FR-003).
	Routes map[string]RouteConfig `json:"routes,omitempty"`
	// Local/Cloud are the legacy two-tier shape (mutually exclusive with
	// Providers); retained so untouched llm.json files keep loading forever.
	Local LocalConfig `json:"local,omitempty"`
	Cloud CloudConfig `json:"cloud,omitempty"`
	// LoopMaxRounds is the hard iteration cap for the agent tool-use loop
	// (TASK-52): the maximum number of provider rounds a single cognition may
	// spend before the driver terminates it. Absent or 0 means the default
	// (8); normalized (and clamped) by Rounds(), never an error — a world
	// never fails to boot over a tuning knob (mirrors LocalConfig.Workers()).
	LoopMaxRounds int `json:"loop_max_rounds,omitempty"`
	// MaxTokens carries the per-kind cognition token budgets (spec 025 US2):
	// planner loop / metatron console turn / nightly consolidation. Optional and
	// additive — an absent object (every pre-025 world) yields today's built-in
	// defaults byte-for-byte (FR-010). Each field normalizes independently via
	// PlannerTokens()/MetatronTurnTokens()/ConsolidationTokens(), warn-not-error
	// like Rounds(). A POINTER so json:"omitempty" actually suppresses it —
	// WriteDefault must not emit the object (contracts/llm-json.md: the knob
	// stays opt-in and a default file stays byte-for-byte compatible). A value
	// struct would marshal as "max_tokens":{}, which omitempty cannot drop.
	MaxTokens *TokenBudgets `json:"max_tokens,omitempty"`
}

// TokenBudgets is the optional llm.json max_tokens object (spec 025 US2,
// contracts/llm-json.md): three per-kind response budgets an operator may tune
// without a rebuild. Each is 0 (absent = default) or a positive request budget;
// normalization/clamping lives on Config (PlannerTokens et al.), so packages
// never see a raw operator value.
type TokenBudgets struct {
	Planner       int64 `json:"planner,omitempty"`       // villager planner tool-loop round budget
	MetatronTurn  int64 `json:"metatron_turn,omitempty"` // metatron console-turn round budget
	Consolidation int64 `json:"consolidation,omitempty"` // nightly consolidation call budget
}

// Token-budget defaults and the shared upper clamp (spec 025 US2, R8). The
// defaults MUST equal the former hardcodes so a config without max_tokens is
// byte-for-byte compatible (FR-010): planner 512 (a tool-era round carries a
// tool_use block — the call name + JSON arguments — alongside any prose, so 256
// truncated a structured call mid-arguments; 512 gives headroom without inviting
// rambling); metatron_turn 1024 (a full charter-voiced reply must not crowd out
// a same-round act, spec 017 T020); consolidation 1024 (a night's digest —
// promotions, fades, gist, beliefs, narrative — as one JSON object). The bound
// 4096 is 4–8× the defaults: real headroom for verbose local models while
// bounding pathological configs, mirroring maxLocalWorkers/maxLoopMaxRounds
// (16) an order of magnitude above their sweet spot.
const (
	defaultPlannerTokens       = 512
	defaultMetatronTurnTokens  = 1024
	defaultConsolidationTokens = 1024
	maxTokenBudget             = 4096
)

// normalizeTokenBudget clamps one max_tokens field into an effective request
// budget and an optional operator-facing warning, mirroring Rounds():
//
//	absent / 0  → kind default, no warning (safe compat default)
//	1 … 4096    → as given, no warning
//	< 0         → kind default, warning
//	> 4096      → 4096, warning
//
// It never errors — a world can never fail to boot over these knobs (FR-008).
func normalizeTokenBudget(key string, raw, def int64) (n int64, warn string) {
	switch {
	case raw == 0:
		return def, ""
	case raw < 0:
		return def, fmt.Sprintf("llm.json max_tokens.%s %d out of range (min 1) — using %d", key, raw, def)
	case raw > maxTokenBudget:
		return maxTokenBudget, fmt.Sprintf("llm.json max_tokens.%s %d out of range (max %d) — clamped to %d", key, raw, maxTokenBudget, maxTokenBudget)
	default:
		return raw, ""
	}
}

// tokenBudgets returns the config's budgets, nil-safe: an absent max_tokens
// object reads as the all-zero struct, so every field falls to its default.
func (c Config) tokenBudgets() TokenBudgets {
	if c.MaxTokens == nil {
		return TokenBudgets{}
	}
	return *c.MaxTokens
}

// PlannerTokens / MetatronTurnTokens / ConsolidationTokens resolve each per-kind
// budget to (effective, warning), independently and accumulating (spec 025
// US2). The daemon prints any warning on its boot channel and passes the
// effective value into mind.New / metatron.New (data-model.md §5).
func (c Config) PlannerTokens() (int64, string) {
	return normalizeTokenBudget("planner", c.tokenBudgets().Planner, defaultPlannerTokens)
}

func (c Config) MetatronTurnTokens() (int64, string) {
	return normalizeTokenBudget("metatron_turn", c.tokenBudgets().MetatronTurn, defaultMetatronTurnTokens)
}

func (c Config) ConsolidationTokens() (int64, string) {
	return normalizeTokenBudget("consolidation", c.tokenBudgets().Consolidation, defaultConsolidationTokens)
}

// ProviderConfig is one declared model source in the v2 registry (FR-001): the
// same knob set the two fixed tiers expose today, now per named entry. Both
// pricing fields zero ⇒ a zero-priced provider (never budget-refused; seeds
// the local estimator bootstrap class).
type ProviderConfig struct {
	Transport        string  `json:"transport"` // "openai_compat" | "anthropic"
	Endpoint         string  `json:"endpoint,omitempty"`
	Model            string  `json:"model"`
	InputUSDPerMTok  float64 `json:"input_usd_per_mtok,omitempty"`
	OutputUSDPerMTok float64 `json:"output_usd_per_mtok,omitempty"`
	APIKeyEnv        string  `json:"api_key_env,omitempty"` // env var NAME holding the key
	APIKey           string  `json:"api_key,omitempty"`     // inline key, local routers only
	Parallel         int     `json:"parallel,omitempty"`    // clamp 1–16, warn-not-error (workers())
	ReasoningEffort  *string `json:"reasoning_effort,omitempty"`
	ToolMode         string  `json:"tool_mode,omitempty"`
	// EndpointCapacity, when > 0, enables the advisory cross-process lease pool
	// on this provider's normalized endpoint (US5). Absent/0 = today's behavior.
	EndpointCapacity int `json:"endpoint_capacity,omitempty"`
}

// zeroPriced reports whether a provider bills nothing — the surviving local vs
// cloud distinction (decision-5): zero-priced providers are never budget-
// refused and seed the local estimator bootstrap class.
func (pc ProviderConfig) zeroPriced() bool {
	return pc.InputUSDPerMTok == 0 && pc.OutputUSDPerMTok == 0
}

// key resolves the credential exactly as CloudConfig.key: an inline local-router
// key wins, else the named environment variable, else empty (open endpoints).
func (pc ProviderConfig) key() string {
	if pc.APIKey != "" {
		return pc.APIKey
	}
	if pc.APIKeyEnv != "" {
		return os.Getenv(pc.APIKeyEnv)
	}
	return ""
}

// workers normalizes a provider's Parallel into an effective worker count and
// an optional warning, mirroring LocalConfig.Workers() per provider (FR-003
// warn-and-clamp): absent/0 → 1 silent; 1–16 verbatim; out-of-range clamped
// with a warning; never an error.
func (pc ProviderConfig) workers(name string) (int, string) {
	switch {
	case pc.Parallel == 0:
		return 1, ""
	case pc.Parallel < 0:
		return 1, fmt.Sprintf("llm.json providers.%s.parallel %d out of range (min 1) — using 1", name, pc.Parallel)
	case pc.Parallel > maxLocalWorkers:
		return maxLocalWorkers, fmt.Sprintf("llm.json providers.%s.parallel %d out of range (max %d) — clamped to %d", name, pc.Parallel, maxLocalWorkers, maxLocalWorkers)
	default:
		return pc.Parallel, ""
	}
}

// toolModeResolved normalizes a provider's tool_mode (openai_compat only; the
// anthropic transport is always native and ignores it), mirroring the warn-not-
// error clamp of the tier config (resolveToolMode), scoped by provider name.
func (pc ProviderConfig) toolModeResolved(name string) (mode, warn string) {
	return resolveToolMode("providers."+name, pc.ToolMode)
}

// RouteConfig is one call kind's ordered provider chain plus the no-fallback
// flag (FR-002). Two JSON forms unmarshal into it (research R6): the bare-array
// shorthand `["a","b"]` (the common case — the chain IS the operator's ruling)
// and the object form `{"chain": ["a","b"], "no_fallback": true}`.
type RouteConfig struct {
	Chain      []string
	NoFallback bool
}

// UnmarshalJSON accepts both the bare-array shorthand and the {chain,
// no_fallback} object form.
func (r *RouteConfig) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return json.Unmarshal(trimmed, &r.Chain)
	}
	var obj struct {
		Chain      []string `json:"chain"`
		NoFallback bool     `json:"no_fallback"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	r.Chain, r.NoFallback = obj.Chain, obj.NoFallback
	return nil
}

// MarshalJSON emits the bare-array shorthand for a plain chain and the object
// form only when no_fallback is set, so WriteDefault's v2 output stays as terse
// as an operator would hand-write it.
func (r RouteConfig) MarshalJSON() ([]byte, error) {
	if !r.NoFallback {
		return json.Marshal(r.Chain)
	}
	return json.Marshal(struct {
		Chain      []string `json:"chain"`
		NoFallback bool     `json:"no_fallback"`
	}{r.Chain, r.NoFallback})
}

// MarshalJSON writes the v2 registry shape when Providers is populated and the
// legacy two-tier shape otherwise — one Config type, two on-disk shapes. Only
// WriteDefault (v2) and any legacy passthrough exercise this; the daemon never
// rewrites llm.json.
func (c Config) MarshalJSON() ([]byte, error) {
	if len(c.Providers) > 0 {
		out := map[string]any{
			"monthly_budget_usd": c.MonthlyBudgetUSD,
			"providers":          c.Providers,
			"routes":             c.Routes,
		}
		if c.LoopMaxRounds != 0 {
			out["loop_max_rounds"] = c.LoopMaxRounds
		}
		return json.Marshal(out)
	}
	type legacy struct {
		MonthlyBudgetUSD float64     `json:"monthly_budget_usd"`
		Local            LocalConfig `json:"local"`
		Cloud            CloudConfig `json:"cloud"`
		LoopMaxRounds    int         `json:"loop_max_rounds,omitempty"`
	}
	return json.Marshal(legacy{c.MonthlyBudgetUSD, c.Local, c.Cloud, c.LoopMaxRounds})
}

// loop iteration-cap bounds. 8 rounds covers read-then-act patterns with
// headroom (TASK-52 R14); 16 bounds adversarial loops. Both mirror the
// warn-not-error clamp doctrine of LocalConfig.Workers().
const (
	defaultLoopMaxRounds = 8
	maxLoopMaxRounds     = 16
)

// Rounds normalizes LoopMaxRounds into an effective round cap and an optional
// operator-facing warning, mirroring LocalConfig.Workers() (never errors;
// out-of-range values are clamped, not rejected).
//
//	absent / 0 → 8, no warning (safe default)
//	1–16       → as given, no warning
//	< 0        → 8, warning
//	> 16       → 16, warning
func (c Config) Rounds() (n int, warn string) {
	switch {
	case c.LoopMaxRounds == 0:
		return defaultLoopMaxRounds, ""
	case c.LoopMaxRounds < 0:
		return defaultLoopMaxRounds, fmt.Sprintf("llm.json loop_max_rounds %d out of range (min 1) — using %d", c.LoopMaxRounds, defaultLoopMaxRounds)
	case c.LoopMaxRounds > maxLoopMaxRounds:
		return maxLoopMaxRounds, fmt.Sprintf("llm.json loop_max_rounds %d out of range (max %d) — clamped to %d", c.LoopMaxRounds, maxLoopMaxRounds, maxLoopMaxRounds)
	default:
		return c.LoopMaxRounds, ""
	}
}

// Tool-call strategy for a tier (TASK-52). "native" uses the provider's
// first-class function-calling wire (Anthropic tools / OpenAI tool_calls);
// "json" engages the schema-constrained fallback envelope (provider-wire.md
// §3) for models whose native function calling is unreliable.
const (
	ToolModeNative = "native"
	ToolModeJSON   = "json"
)

// resolveToolMode normalizes a tool_mode field, mirroring the warn-not-error
// clamp of Workers()/Rounds(): "" (absent) is native, the two legal values
// pass through, and any other value falls back to native with an operator
// warning — never an error, so a world never fails to boot over the knob.
func resolveToolMode(scope, raw string) (mode, warn string) {
	switch raw {
	case "", ToolModeNative:
		return ToolModeNative, ""
	case ToolModeJSON:
		return ToolModeJSON, ""
	default:
		return ToolModeNative, fmt.Sprintf("llm.json %s.tool_mode %q unknown (want %q or %q) — using %q",
			scope, raw, ToolModeNative, ToolModeJSON, ToolModeNative)
	}
}

// LocalConfig is the local tier: an OpenAI-compatible chat-completions
// endpoint (Ollama by default). Free; its throughput is the real cap on
// max sim speed once planner calls exist.
type LocalConfig struct {
	Endpoint string `json:"endpoint"` // e.g. http://localhost:11434/v1
	Model    string `json:"model"`
	APIKey   string `json:"api_key,omitempty"` // local routers only
	// Parallel is the requested local-tier concurrency: the number of
	// worker goroutines (in-flight calls) against the local model. Absent
	// or 0 means 1 — behavior indistinguishable from pre-feature builds.
	// Normalized (and clamped) by Workers(); never an error, so a world
	// can never fail to boot over this field.
	Parallel int `json:"parallel,omitempty"`
	// ReasoningEffort controls hidden chain-of-thought on thinking-default
	// models (e.g. gemma4 on Ollama). Absent (nil) defaults to "none":
	// interiority prose never needs hidden reasoning, and local latency is
	// the cap on sim speed, so thinking is off unless asked for. Explicit ""
	// sends nothing (escape hatch for backends that reject the field). Any
	// other value is sent verbatim.
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`
	// ToolMode selects the local tier's tool-call strategy (TASK-52): absent
	// or "" is "native" (OpenAI-compatible tool_calls); "json" engages the
	// schema-constrained fallback envelope for backends whose native function
	// calling is unreliable (llama.cpp grammar notes, parse.go:122). Same
	// per-model knob shape as ReasoningEffort; normalized by ToolModeResolved().
	ToolMode string `json:"tool_mode,omitempty"`
}

// ToolModeResolved normalizes local.tool_mode into an effective strategy and
// an optional operator warning (see resolveToolMode).
func (c LocalConfig) ToolModeResolved() (mode, warn string) {
	return resolveToolMode("local", c.ToolMode)
}

// maxLocalWorkers caps local-tier concurrency. queueCap is 32; more than
// 16 in-flight calls would let concurrency exceed half the queue and mostly
// measures server-side queueing, not parallelism. 16 leaves an order of
// magnitude of headroom over the measured sweet spot (4) while bounding
// pathological configs (parallel: 10000).
const maxLocalWorkers = 16

// Workers normalizes Parallel into an effective worker count and an optional
// operator-facing warning. The knob never errors (FR-007): out-of-range
// values are clamped, not rejected.
//
//	absent / 0 → 1, no warning (byte-for-byte compat default)
//	1–16       → as given, no warning
//	< 0        → 1, warning
//	> 16       → 16, warning
func (c LocalConfig) Workers() (n int, warn string) {
	switch {
	case c.Parallel == 0:
		return 1, ""
	case c.Parallel < 0:
		return 1, fmt.Sprintf("llm.json local.parallel %d out of range (min 1) — using 1", c.Parallel)
	case c.Parallel > maxLocalWorkers:
		return maxLocalWorkers, fmt.Sprintf("llm.json local.parallel %d out of range (max %d) — clamped to %d", c.Parallel, maxLocalWorkers, maxLocalWorkers)
	default:
		return c.Parallel, ""
	}
}

// Cloud provider values. Empty means ProviderAnthropic.
const (
	ProviderAnthropic    = "anthropic"     // the Anthropic Messages API via the official SDK
	ProviderOpenAICompat = "openai_compat" // any OpenAI-compatible chat-completions router
)

// CloudConfig is the cloud tier: the Anthropic API by default, or any
// OpenAI-compatible router (e.g. a LAN-local 9router) when provider says
// so. Pricing feeds the spend meter either way.
type CloudConfig struct {
	Provider         string  `json:"provider,omitempty"` // "anthropic" (default) | "openai_compat"
	Model            string  `json:"model"`
	InputUSDPerMTok  float64 `json:"input_usd_per_mtok"`
	OutputUSDPerMTok float64 `json:"output_usd_per_mtok"`
	APIKeyEnv        string  `json:"api_key_env"`       // env var NAME holding the key
	APIKey           string  `json:"api_key,omitempty"` // inline key, local routers only
	// Endpoint overrides the API base URL (required for openai_compat;
	// tests/proxies for anthropic); empty = the Anthropic default.
	Endpoint string `json:"endpoint,omitempty"`
	// ReasoningEffort only applies when Provider is openai_compat (the
	// Anthropic SDK path is untouched). Absent or "" sends nothing — cloud
	// models are chosen for quality, not latency, so there is no default
	// reasoning posture to impose. Any other value is sent verbatim.
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`
	// ToolMode selects the cloud tier's tool-call strategy (TASK-52), honored
	// only when Provider is openai_compat — the Anthropic SDK path is always
	// native and ignores this knob. Absent or "" is "native"; "json" engages
	// the fallback envelope. Normalized by ToolModeResolved().
	ToolMode string `json:"tool_mode,omitempty"`
}

// ToolModeResolved normalizes cloud.tool_mode into an effective strategy and
// an optional operator warning (see resolveToolMode). Applies only to the
// openai_compat cloud provider; the Anthropic path is always native.
func (c CloudConfig) ToolModeResolved() (mode, warn string) {
	return resolveToolMode("cloud", c.ToolMode)
}

// key resolves the credential: an inline local-router key wins, else the
// named environment variable, else empty (Ollama-style open endpoints).
func (c CloudConfig) key() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	if c.APIKeyEnv != "" {
		return os.Getenv(c.APIKeyEnv)
	}
	return ""
}

// resolveReasoningEffort applies the nil/"" convention shared by both
// tiers' ReasoningEffort fields: nil takes the tier's default, "" (or any
// other explicit value) is returned verbatim.
func resolveReasoningEffort(v *string, def string) string {
	if v == nil {
		return def
	}
	return *v
}

// DefaultConfig matches the grounding decisions in the v2 registry shape
// (FR-017): local Ollama for the per-agent chatter, Claude for the nightly/
// narrative work, $100/month hard ceiling — semantically identical to today's
// two-tier defaults, expressed as two named providers with today's routes.
func DefaultConfig() Config {
	return Config{
		MonthlyBudgetUSD: 100,
		Providers: map[string]ProviderConfig{
			// The operator's always-on local model; cogito:3b is the lighter
			// "budget" alternative if kept perma-loaded.
			"local": {Transport: ProviderOpenAICompat, Endpoint: "http://localhost:11434/v1", Model: "gemma4:12b-mlx"},
			"cloud": {Transport: ProviderAnthropic, Model: "claude-opus-4-8", InputUSDPerMTok: 5, OutputUSDPerMTok: 25, APIKeyEnv: "ANTHROPIC_API_KEY"},
		},
		Routes: defaultRoutes(),
	}
}

// defaultRoutes is today's kind→tier table expressed as single-entry chains:
// high-volume ambient cognition local (free), low-volume high-quality work
// cloud. Shared by DefaultConfig (v2) and legacy derivation.
func defaultRoutes() map[string]RouteConfig {
	return map[string]RouteConfig{
		string(KindPlanner):       {Chain: []string{"local"}},
		string(KindConversation):  {Chain: []string{"local"}},
		string(KindMeeting):       {Chain: []string{"local"}},
		string(KindConsolidation): {Chain: []string{"cloud"}},
		string(KindNarrator):      {Chain: []string{"cloud"}},
		string(KindDrama):         {Chain: []string{"cloud"}},
		string(KindMetatron):      {Chain: []string{"cloud"}},
	}
}

// LoadConfig reads llm.json; (nil, nil) when the file doesn't exist — the
// orchestrator is simply disabled for that world. The full validation matrix
// (contracts/llm-config.md) runs here so a bad registry dies at boot with an
// error naming the offending entry, never a runtime surprise (FR-003).
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.MonthlyBudgetUSD <= 0 {
		return nil, fmt.Errorf("%s: monthly_budget_usd must be positive", path)
	}
	if _, _, err := cfg.resolveRegistry(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

// resolveRegistry normalizes either config shape into the runtime registry:
// the validated provider set and the kind→chain routes New() builds machinery
// from. It is the single validation authority — LoadConfig calls it for boot
// errors, New() calls it to build. Warn-and-clamp knobs (parallel, tool_mode)
// are NOT validated here; they clamp per provider at construction (FR-003).
func (c Config) resolveRegistry() (map[string]ProviderConfig, map[Kind]RouteConfig, error) {
	legacyPresent := c.Local != (LocalConfig{}) || c.Cloud != (CloudConfig{})
	v2Present := len(c.Providers) > 0
	switch {
	case v2Present && legacyPresent:
		return nil, nil, fmt.Errorf("`providers` and legacy `local`/`cloud` are mutually exclusive — declare one shape")
	case v2Present:
		return c.validateV2()
	default:
		return c.deriveLegacy()
	}
}

// deriveLegacy builds the two-provider registry (named local/cloud) and today's
// routes from the legacy shape, so an untouched llm.json behaves byte-identically
// (FR-004). Legacy-specific validation (cloud.provider, openai_compat endpoint)
// is preserved verbatim from the pre-feature LoadConfig.
func (c Config) deriveLegacy() (map[string]ProviderConfig, map[Kind]RouteConfig, error) {
	switch c.Cloud.Provider {
	case "", ProviderAnthropic, ProviderOpenAICompat:
	default:
		return nil, nil, fmt.Errorf("unknown cloud.provider %q", c.Cloud.Provider)
	}
	if c.Cloud.Provider == ProviderOpenAICompat && c.Cloud.Endpoint == "" {
		return nil, nil, fmt.Errorf("cloud.provider %q requires cloud.endpoint", ProviderOpenAICompat)
	}
	// cloud.provider "" | "anthropic" → anthropic transport; "openai_compat" →
	// openai_compat. Local is always openai_compat, zero-priced. Cloud carries
	// its pricing and keys; parallel is left absent (workers() → 1, cloud's
	// single-worker rule).
	cloudTransport := ProviderAnthropic
	if c.Cloud.Provider == ProviderOpenAICompat {
		cloudTransport = ProviderOpenAICompat
	}
	providers := map[string]ProviderConfig{
		"local": {
			Transport: ProviderOpenAICompat, Endpoint: c.Local.Endpoint, Model: c.Local.Model,
			APIKey: c.Local.APIKey, Parallel: c.Local.Parallel,
			ReasoningEffort: c.Local.ReasoningEffort, ToolMode: c.Local.ToolMode,
		},
		"cloud": {
			Transport: cloudTransport, Endpoint: c.Cloud.Endpoint, Model: c.Cloud.Model,
			InputUSDPerMTok: c.Cloud.InputUSDPerMTok, OutputUSDPerMTok: c.Cloud.OutputUSDPerMTok,
			APIKeyEnv: c.Cloud.APIKeyEnv, APIKey: c.Cloud.APIKey,
			ReasoningEffort: c.Cloud.ReasoningEffort, ToolMode: c.Cloud.ToolMode,
		},
	}
	routes := make(map[Kind]RouteConfig, len(acceptedKinds))
	for kind, rc := range defaultRoutes() {
		routes[Kind(kind)] = rc
	}
	return providers, routes, nil
}

// validateV2 runs the full boot-error matrix over the v2 registry and returns
// the routes keyed by Kind. Every failure names the offending entry (FR-003).
func (c Config) validateV2() (map[string]ProviderConfig, map[Kind]RouteConfig, error) {
	for name, pc := range c.Providers {
		switch pc.Transport {
		case ProviderOpenAICompat, ProviderAnthropic:
		default:
			return nil, nil, fmt.Errorf("provider %q: unknown transport %q (want %q or %q)", name, pc.Transport, ProviderOpenAICompat, ProviderAnthropic)
		}
		if pc.Model == "" {
			return nil, nil, fmt.Errorf("provider %q: missing model", name)
		}
		if pc.Transport == ProviderOpenAICompat && pc.Endpoint == "" {
			return nil, nil, fmt.Errorf("provider %q: transport %q requires endpoint", name, ProviderOpenAICompat)
		}
	}
	routes := make(map[Kind]RouteConfig, len(c.Routes))
	for key, rc := range c.Routes {
		kind := Kind(key)
		if _, ok := acceptedKinds[kind]; !ok {
			return nil, nil, fmt.Errorf("route %q: unknown call kind", key)
		}
		if len(rc.Chain) == 0 {
			return nil, nil, fmt.Errorf("route %q: empty chain", key)
		}
		if rc.NoFallback && len(rc.Chain) > 1 {
			return nil, nil, fmt.Errorf("route %q: no_fallback with a chain of %d — a no-fallback route may name only its head", key, len(rc.Chain))
		}
		seen := make(map[string]struct{}, len(rc.Chain))
		for _, name := range rc.Chain {
			if _, ok := c.Providers[name]; !ok {
				return nil, nil, fmt.Errorf("route %q: names undeclared provider %q", key, name)
			}
			if _, dup := seen[name]; dup {
				return nil, nil, fmt.Errorf("route %q: provider %q listed twice (a chain is an ordered set)", key, name)
			}
			seen[name] = struct{}{}
		}
		routes[kind] = rc
	}
	// Completeness: every accepted kind must have a route (FR-003).
	for kind := range acceptedKinds {
		if _, ok := routes[kind]; !ok {
			return nil, nil, fmt.Errorf("call kind %q has no route", kind)
		}
	}
	return c.Providers, routes, nil
}

// WriteDefault writes the default llm.json (used by `promptworld new`).
func WriteDefault(path string) error {
	data, err := json.MarshalIndent(DefaultConfig(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
