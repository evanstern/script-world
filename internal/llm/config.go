package llm

import (
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
type Config struct {
	MonthlyBudgetUSD float64     `json:"monthly_budget_usd"`
	Local            LocalConfig `json:"local"`
	Cloud            CloudConfig `json:"cloud"`
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

// DefaultConfig matches the grounding decisions: local Ollama for the
// per-agent chatter, Claude for the nightly/narrative tier, $100/month hard
// ceiling.
func DefaultConfig() Config {
	return Config{
		MonthlyBudgetUSD: 100,
		Local: LocalConfig{
			Endpoint: "http://localhost:11434/v1",
			// The operator's always-on local model; cogito:3b is the
			// lighter "budget" alternative if kept perma-loaded.
			Model: "gemma4:12b-mlx",
		},
		Cloud: CloudConfig{
			Model:            "claude-opus-4-8",
			InputUSDPerMTok:  5,
			OutputUSDPerMTok: 25,
			APIKeyEnv:        "ANTHROPIC_API_KEY",
		},
	}
}

// LoadConfig reads llm.json; (nil, nil) when the file doesn't exist — the
// orchestrator is simply disabled for that world.
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
	switch cfg.Cloud.Provider {
	case "", ProviderAnthropic, ProviderOpenAICompat:
	default:
		return nil, fmt.Errorf("%s: unknown cloud.provider %q", path, cfg.Cloud.Provider)
	}
	if cfg.Cloud.Provider == ProviderOpenAICompat && cfg.Cloud.Endpoint == "" {
		return nil, fmt.Errorf("%s: cloud.provider %q requires cloud.endpoint", path, ProviderOpenAICompat)
	}
	return &cfg, nil
}

// WriteDefault writes the default llm.json (used by `promptworld new`).
func WriteDefault(path string) error {
	data, err := json.MarshalIndent(DefaultConfig(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
