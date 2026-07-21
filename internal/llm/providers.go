package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// caller is one tier's transport.
type caller interface {
	call(ctx context.Context, req Request) (text string, inTok, outTok int64, err error)
}

// --- OpenAI-compatible chat completions (Ollama, 9router, et al.) ---
// Serves the local tier always, and the cloud tier when
// cloud.provider = "openai_compat".

type openaiCompat struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
	// reasoningEffort is already resolved (see resolveReasoningEffort);
	// empty means omit the field from the request body entirely.
	reasoningEffort string
}

func newOpenAICompat(endpoint, model, apiKey, reasoningEffort string) *openaiCompat {
	return &openaiCompat{
		endpoint:        strings.TrimRight(endpoint, "/"),
		model:           model,
		apiKey:          apiKey,
		client:          &http.Client{Timeout: 120 * time.Second},
		reasoningEffort: reasoningEffort,
	}
}

func (o *openaiCompat) call(ctx context.Context, req Request) (string, int64, int64, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var msgs []msg
	if req.System != "" {
		msgs = append(msgs, msg{Role: "system", Content: req.System})
	}
	msgs = append(msgs, msg{Role: "user", Content: req.Prompt})

	payload := map[string]any{
		"model":    o.model,
		"messages": msgs,
		// Some routers (9router) stream by default; this decoder wants one
		// JSON object, so pin it.
		"stream": false,
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if o.reasoningEffort != "" {
		payload["reasoning_effort"] = o.reasoningEffort
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", 0, 0, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", 0, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", 0, 0, fmt.Errorf("chat-completions HTTP %d: %s", resp.StatusCode, snippet)
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", 0, 0, fmt.Errorf("chat-completions response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", 0, 0, fmt.Errorf("chat-completions returned no choices")
	}
	return out.Choices[0].Message.Content, out.Usage.PromptTokens, out.Usage.CompletionTokens, nil
}

// --- cloud tier: Anthropic Messages API via the official SDK ---

type anthropicCaller struct {
	client anthropic.Client
	model  string
}

func newAnthropicCaller(cfg CloudConfig) *anthropicCaller {
	var opts []option.RequestOption
	if key := cfg.key(); key != "" {
		opts = append(opts, option.WithAPIKey(key))
	}
	if cfg.Endpoint != "" {
		opts = append(opts, option.WithBaseURL(cfg.Endpoint))
	}
	return &anthropicCaller{client: anthropic.NewClient(opts...), model: cfg.Model}
}

// newCloudCaller picks the cloud tier's transport from the config.
func newCloudCaller(cfg CloudConfig) caller {
	if cfg.Provider == ProviderOpenAICompat {
		return newOpenAICompat(cfg.Endpoint, cfg.Model, cfg.key(),
			resolveReasoningEffort(cfg.ReasoningEffort, ""))
	}
	return newAnthropicCaller(cfg)
}

func (a *anthropicCaller) call(ctx context.Context, req Request) (string, int64, int64, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.Prompt)),
		},
	}
	if req.System != "" {
		// Stable system prompts (agent souls, the narrator charter) are the
		// cacheable prefix — mark them so repeat calls bill at cache-read
		// rates.
		params.System = []anthropic.TextBlockParam{{
			Text:         req.System,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}}
	}
	resp, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return "", 0, 0, err
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(b.Text)
		}
	}
	return text.String(), resp.Usage.InputTokens, resp.Usage.OutputTokens, nil
}
