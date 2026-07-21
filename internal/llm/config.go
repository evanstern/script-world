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
}

// LocalConfig is the local tier: an OpenAI-compatible chat-completions
// endpoint (Ollama by default). Free; its throughput is the real cap on
// max sim speed once planner calls exist.
type LocalConfig struct {
	Endpoint string `json:"endpoint"` // e.g. http://localhost:11434/v1
	Model    string `json:"model"`
	APIKey   string `json:"api_key,omitempty"` // local routers only
	// ReasoningEffort controls hidden chain-of-thought on thinking-default
	// models (e.g. gemma4 on Ollama). Absent (nil) defaults to "none":
	// interiority prose never needs hidden reasoning, and local latency is
	// the cap on sim speed, so thinking is off unless asked for. Explicit ""
	// sends nothing (escape hatch for backends that reject the field). Any
	// other value is sent verbatim.
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`
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

// WriteDefault writes the default llm.json (used by `scriptworld new`).
func WriteDefault(path string) error {
	data, err := json.MarshalIndent(DefaultConfig(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
