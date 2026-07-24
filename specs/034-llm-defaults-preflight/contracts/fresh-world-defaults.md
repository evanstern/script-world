# Contract — fresh-world default LLM config (spec 034)

## llm.json written by `promptworld new`

```json
{
  "monthly_budget_usd": 100,
  "providers": {
    "local": {
      "transport": "openai_compat",
      "endpoint": "http://localhost:11434/v1",
      "model": "cogito:3b",
      "parallel": 4,
      "tool_mode": "json"
    },
    "cloud": {
      "transport": "anthropic",
      "model": "claude-opus-4-8",
      "input_usd_per_mtok": 5,
      "output_usd_per_mtok": 25,
      "api_key_env": "ANTHROPIC_API_KEY"
    }
  },
  "routes": { /* unchanged defaultRoutes() */ }
}
```

Only the `local` provider changes vs today (model, tool_mode, parallel).
Existing worlds' llm.json files are never touched.

## `promptworld new` stdout (appended guidance)

After the existing three lines, print:

```
local model: cogito:3b — pull it first if you haven't: ollama pull cogito:3b
```

Exact wording may be tuned at implementation; MUST name the model and give a
copy-pasteable pull command (FR-008).

## Docs alignment (FR-009 / SC-004)

- `docs/llm-providers.md`: state that fresh worlds get `cogito:3b` +
  `tool_mode: "json"` + `parallel: 4` as the default local provider; keep the
  gemma-class entry as the documented upgrade path for operators who serve it.
- `README.md` (~line 86): name `cogito:3b` (with pull command) as the model a
  fresh world expects instead of `gemma4:12b-mlx`.
- Verification: the model+mode named by `DefaultConfig()`, docs/llm-providers.md,
  and README must be identical (quickstart V5 grep).
