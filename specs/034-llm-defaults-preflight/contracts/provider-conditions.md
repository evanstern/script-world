# Contract — provider health conditions (spec 034)

## Preflight probe (outbound)

- Request: `GET {provider.endpoint}/models` with the provider's auth header if a
  key resolves (same header rules as chat-completions); timeout ≤ 5s.
- Applies to `openai_compat` transport providers only; `anthropic` exempt.
- Interpretation:
  - transport error / timeout → condition `endpoint-unreachable`
  - 2xx + JSON `{"data":[{"id":…}]}` containing the configured model id → healthy
  - 2xx + valid shape, id absent → condition `model-missing`
  - non-2xx, or 2xx with unparseable shape → **listing unsupported**: no
    condition, one low-key log line, provider treated as unverifiable (runtime
    net still applies)
- Schedule: once at daemon boot (async — boot never blocks/fails on it), then
  every 60s **only while** a preflight-raised condition is active; each active
  re-probe re-logs the warning line.

## Status wire (promptworld status --json / TUI poll)

`StatusData.LLM.Providers[i]` (existing) gains:

```json
{
  "name": "local",
  "model": "cogito:3b",
  "endpoint": "http://localhost:11434/v1",
  "up": true,
  "condition": "model-missing",
  "condition_detail": "model \"cogito:3b\" not served by http://localhost:11434/v1",
  "condition_remedy": "ollama pull cogito:3b"
}
```

All three fields omitempty; healthy provider serializes exactly as today.

## Human surfaces

- `promptworld status`: when any provider has an active condition, print after
  the clock line, one line per affected provider:
  `WARNING llm provider "local": model "cogito:3b" not served by http://localhost:11434/v1 — ollama pull cogito:3b`
  Healthy worlds: output byte-identical to today.
- Daemon log: same line prefixed `daemon: ` at boot, on every transition, and on
  each 60s re-probe while active.
- TUI header: red badge `[llm: <provider> <kind>]` appended while any condition
  is active (pattern: the `[degraded]` badge); metatron-pane provider line gains
  the condition + remedy in red.

## Event (durable + broadcast; visible in line-mode attach)

Type: `daemon.llm_warning`

```json
{"provider":"local","kind":"model-missing","detail":"model \"cogito:3b\" not served by http://localhost:11434/v1","remedy":"ollama pull cogito:3b","active":true}
```

- Emitted on raise, reclassify, and clear (`active:false`, remedy empty).
- NOT emitted per re-probe repeat (the repeat surface is the log + persistent
  status fields) — transitions only, so the durable log stays quiet.
- No-op reducer: never mutates world state; never appears in chronicle/narrator.

## Tool-silence detection (worker-side)

- Scope: completed calls whose request carried tool declarations
  (`len(req.Tools) > 0`) — production kinds: planner, metatron console turns.
- Counter: consecutive zero-tool-call completions per provider; any returned
  tool call resets; transport failures don't count (breaker's job).
- Threshold 8 → raise `tool-silent` (unless a preflight condition holds the slot).
- Remedy text by resolved tool mode:
  - native → `set providers.<name>.tool_mode to "json" and restart`
  - json → `model never emits tool calls even in json mode — use a model suited for tool work`
- Clear: first tool-carrying completion that returns a tool call.
