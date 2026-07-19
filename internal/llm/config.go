package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Config is llm.json in the world save directory. Endpoints, models, and
// pricing live here; API keys never do — only the NAME of an environment
// variable that carries one.
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
}

// CloudConfig is the cloud tier: the Anthropic Messages API via the
// official SDK. Pricing feeds the spend meter.
type CloudConfig struct {
	Model            string  `json:"model"`
	InputUSDPerMTok  float64 `json:"input_usd_per_mtok"`
	OutputUSDPerMTok float64 `json:"output_usd_per_mtok"`
	APIKeyEnv        string  `json:"api_key_env"` // env var NAME holding the key
	// Endpoint overrides the API base URL (tests, proxies); empty = default.
	Endpoint string `json:"endpoint,omitempty"`
}

// DefaultConfig matches the grounding decisions: local Ollama for the
// per-agent chatter, Claude for the nightly/narrative tier, $100/month hard
// ceiling.
func DefaultConfig() Config {
	return Config{
		MonthlyBudgetUSD: 100,
		Local: LocalConfig{
			Endpoint: "http://localhost:11434/v1",
			Model:    "qwen3:8b",
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
