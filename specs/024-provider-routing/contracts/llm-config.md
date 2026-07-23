# Contract: llm.json v2 — provider registry + routes

The operator-facing configuration surface. See [data-model.md](../data-model.md) for
field rules; this file fixes the concrete JSON shapes.

## v2 example (the measured division of labor)

```json
{
  "monthly_budget_usd": 100,
  "loop_max_rounds": 8,
  "providers": {
    "gemma": {
      "transport": "openai_compat",
      "endpoint": "http://localhost:11434/v1",
      "model": "gemma4:12b-mlx",
      "parallel": 2,
      "endpoint_capacity": 4
    },
    "cogito": {
      "transport": "openai_compat",
      "endpoint": "http://localhost:11434/v1",
      "model": "cogito:3b",
      "parallel": 4,
      "endpoint_capacity": 4
    },
    "anthropic": {
      "transport": "anthropic",
      "model": "claude-opus-4-8",
      "input_usd_per_mtok": 5,
      "output_usd_per_mtok": 25,
      "api_key_env": "ANTHROPIC_API_KEY"
    },
    "niner": {
      "transport": "openai_compat",
      "endpoint": "http://10.0.0.9:9999/v1",
      "model": "some-router-model",
      "input_usd_per_mtok": 1,
      "output_usd_per_mtok": 4,
      "api_key": "lan-router-key"
    }
  },
  "routes": {
    "planner":       ["gemma"],
    "conversation":  ["cogito", "gemma"],
    "meeting":       ["cogito", "gemma"],
    "consolidation": ["anthropic", "niner"],
    "narrator":      ["anthropic", "niner"],
    "drama":         ["anthropic", "niner"],
    "metatron":      { "chain": ["anthropic"], "no_fallback": true }
  }
}
```

## Default written by `promptworld new` (v2, semantically today's defaults)

```json
{
  "monthly_budget_usd": 100,
  "providers": {
    "local": {
      "transport": "openai_compat",
      "endpoint": "http://localhost:11434/v1",
      "model": "gemma4:12b-mlx"
    },
    "cloud": {
      "transport": "anthropic",
      "model": "claude-opus-4-8",
      "input_usd_per_mtok": 5,
      "output_usd_per_mtok": 25,
      "api_key_env": "ANTHROPIC_API_KEY"
    }
  },
  "routes": {
    "planner": ["local"], "conversation": ["local"], "meeting": ["local"],
    "consolidation": ["cloud"], "narrator": ["cloud"], "drama": ["cloud"],
    "metatron": ["cloud"]
  }
}
```

## Kind-scoped top-level knobs (spec 025 composition)

`loop_max_rounds` and `max_tokens` (`{planner, metatron_turn, consolidation}`, spec 025)
are properties of the thought class, not the provider: they stay top-level in both
shapes, apply unchanged whichever chain candidate serves, and round-trip byte-for-byte
through the v2 marshal/parse. There are no per-provider token fields.

## Legacy shape (loads forever, no edits)

Today's `{"monthly_budget_usd", "local": {...}, "cloud": {...}}` derives exactly the
default registry above (with the operator's own values, `cloud.provider` →
`transport`). `providers` and `local`/`cloud` together in one file → load error.

## Load-time validation matrix (boot errors)

| Config | Error names |
|--------|-------------|
| route → undeclared provider | the route kind + unknown name |
| accepted kind missing from routes | the kind |
| unknown kind key in routes | the key |
| duplicate provider within a chain | the route kind + name |
| empty chain | the route kind |
| `no_fallback` with chain length > 1 | the route kind |
| provider missing transport/model, openai_compat missing endpoint | the provider name + field |
| both `providers` and legacy `local`/`cloud` present | ambiguity |
| `monthly_budget_usd` ≤ 0 | unchanged |

Warn-and-clamp (never errors): `parallel` (1–16), `tool_mode`, `loop_max_rounds` —
unchanged doctrine, applied per provider.
