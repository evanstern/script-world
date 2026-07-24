# Phase 0 research — spec 034 (fresh-world LLM defaults + dead-tier surfacing)

All findings verified against the working tree on 2026-07-24 (main @ e9213e1).

## R1 — Where the silence actually happens today

- A wrong/absent model name **does** strike the circuit breaker: Ollama returns
  HTTP 404, `do()` errors on any non-200 (providers.go:455-458), the worker calls
  `t.health.fail()` (llm.go:835), and after 3 failures the breaker opens
  (health.go:14,58). So the failure is *detected* — it is just **invisible**: breaker
  state surfaces only as `Up: !p.health.down()` (llm.go:898) rendered as a `●`/`○`
  glyph in the TUI metatron pane (views.go:1475-1478). `promptworld status` human
  output prints no LLM information at all (commands.go:511-513); line-mode attach
  streams raw events only; the boot log says nothing.
- A tool-silent completion (model present, wrong tool mode) is a transport
  **success**: `Response.ToolCalls` nil, `Stop = StopEndTurn`, `health.succeed()`
  (llm.go:840). Nothing counts it anywhere. The planner reports `TermModelDone`
  (toolloop/loop.go:287-294) → `OutcomeUnusable` "loop: model produced no tool call"
  (mind.go:474-475) and deliberately does not re-arm — reflex floor covers the gap.
  The signal exists per-call but is never aggregated per-provider.

**Decision**: two detection legs are required — an existence preflight (catches
absent model *before* traffic, with a precise remedy) and a runtime tool-silence
counter (catches the success-shaped failure preflight cannot see).
**Alternatives considered**: (a) breaker-state surfacing alone — rejected: never
catches tool-silence, and "breaker open" lacks the remedy (pull vs unreachable);
(b) planner-side detection in `internal/mind` — rejected: the per-provider identity
lives in the orchestrator worker; mind sees only the pinned run result.

## R2 — Probe endpoint for model existence

- Nothing in the codebase lists models today (exhaustive search: no `/v1/models`,
  `/api/tags`, `ListModels` callers; the transport only ever hits
  `/chat/completions`, providers.go:441).
- Provider endpoints are OpenAI-compat **base URLs** (e.g.
  `http://localhost:11434/v1`). The OpenAI-compat standard listing is
  `GET {base}/models`; Ollama serves it (`/v1/models`, returning
  `{"data":[{"id":"cogito:3b",…},…]}`). `/api/tags` is Ollama-proprietary and lives
  *off* the compat base — rejected to keep the probe router-agnostic.

**Decision**: `GET {endpoint}/models`, parse `data[].id`, exact-match the configured
model id. Non-2xx or JSON shape mismatch ⇒ "listing unsupported" ⇒ skip silently
(one low-key log line), never a false *model-missing*. Connection/timeout error ⇒
*endpoint-unreachable*. Anthropic transport providers exempt (FR-001).
**Alternatives considered**: Ollama `/api/tags` (proprietary, wrong base); firing a
1-token chat probe (spends tokens, indistinguishable failure classes) — rejected.

## R3 — Condition lifecycle (raise / clear / re-check)

- Existing async boot pattern: `go sampler.run(ctx)` (daemon.go:148) inside the
  LLM-gated block; preflight goroutine follows it exactly — boot never blocks or
  fails on probe results (FR-002).
- Clear paths (FR-004): (a) **traffic is truth** — any successful call on the
  provider clears its conditions (worker already calls `health.succeed()` at
  llm.go:840; clear hook rides the same site); (b) **periodic re-probe** — while a
  preflight-raised condition is active, re-probe every 60s so a pulled model clears
  the warning on a world with no traffic (e.g. paused). No re-probe when healthy
  (zero steady-state overhead).

**Decision**: one condition slot per provider (kinds: `model-missing`,
`endpoint-unreachable`, `tool-silent`), guarded by the provider's existing mutex
discipline; transitions (raise, kind change, clear) fire a hook the daemon wires to
(log line + `daemon.llm_warning` event). Repeat-loudness: the periodic re-probe
re-logs while active (60s cadence), satisfying "repeated, not scrollback" together
with the persistent status/TUI surfaces.
**Alternatives considered**: separate boolean per kind — rejected (operator cares
about the *dominant* problem; unreachable supersedes missing supersedes tool-silent).

## R4 — Tool-silence detector shape

- The worker sees both the request and result: `len(j.req.Tools) > 0` is already the
  transport branch key (providers.go:114), and the worker holds `j.req` at llm.go:828.
  Tool-carrying kinds in production: `KindPlanner` (mind.go:401) and `KindMetatron`
  console turns (metatron/turn.go:190) — both via toolloop. All other kinds never set
  `Tools`, so scoping on `len(req.Tools) > 0` implements FR-005's "kinds that don't
  expect tool calls never count" with zero kind-table maintenance.
- Threshold: planner completions legitimately end tool-free sometimes
  (`TermModelDone` is a normal terminal). TASK-73 healthy soaks showed sustained
  tool-call streams (789–982 decisions / 8 game-hours); a *consecutive* run of 8
  tool-free completions on a tool-carrying chain does not occur on a healthy
  provider but is hit within the first minutes by a never-function-calling model
  (cogito-native emits zero, every time). 8 balances SC-005 (minutes to flag)
  against SC-003 (zero false positives in a soak window).

**Decision**: per-provider consecutive counter beside `tierHealth` on the `provider`
struct (llm.go:312-336); increment on completed tool-carrying call with zero tool
calls, reset on any tool call; at ≥8, raise `tool-silent` (unless a preflight
condition already active). In JSON mode a `tool:"none"` envelope is still a
zero-tool-call completion and counts — 8 consecutive declines means the tier is
effectively dead for planning regardless of mode; remedy text differs by resolved
mode (native → suggest `tool_mode: "json"`; json → model unsuited for tool work).
**Alternatives considered**: windowed rate (N of last M) — rejected: more state, no
better signal for the "never function-calls" failure class; counting
`TermModelDone` in mind — rejected per R1.

## R5 — Surfacing routes (FR-002/003 + SC-001 "all three surfaces")

- **Daemon log**: boot warnings already stream `fmt.Printf("daemon: …")` →
  daemon.log (stdout redirect wired at commands.go:429-435). Preflight + transitions
  log there as `daemon: WARNING llm provider …`. `configWarnf`/`leaseWarnf` stay
  untouched (stderr → same file).
- **Status wire**: `StatusData.LLM` already carries `llm.Status` with
  `[]ProviderStatus` (protocol.go:114, llm.go:264-284) and the TUI already polls it
  1/s — adding `Condition`/`ConditionDetail`/`Remedy` (omitempty) to
  `ProviderStatus` reaches every client with no protocol surgery. `status --json`
  gets it for free.
- **`promptworld status` human**: currently renders zero LLM info — add a warning
  block (only when conditions are active) after the clock line; healthy worlds print
  exactly what they print today.
- **TUI**: header badge (pattern: the red `[degraded]` badge, views.go:121-122) —
  `[llm: local model missing]`; metatron-pane provider lines (views.go:1468-1497)
  annotate the condition + remedy.
- **Line-mode attach + durability**: `appendDaemonEvent` (daemon.go:276-287,
  precedent: `daemon.started`/`daemon.stopped`) appends + broadcasts a
  `daemon.llm_warning` event on raise/clear with provider, kind, detail, remedy,
  `active` flag. Whitelisting follows the existing daemon-event pattern (no-op
  reducer arm; events are operator-facing, never world-state).

**Decision**: all five routes above; conditions are operator-facing only — nothing
reaches chronicle/narrator surfaces (spec assumption honored).

## R6 — Default alignment (FR-007/008/009)

- Evidence: TASK-73 ship-gate record (specs/027-villager-prompt-quality/eval/
  decision.md) — three 8-game-hour soaks on `cogito:3b` + `tool_mode: "json"`,
  789/896/982 planner decisions, shipped prompt variant chosen on this config. The
  bring-up caveat records that **no** planner call succeeded until the eval worlds
  were switched off the default. Current default `gemma4:12b-mlx` (config.go:454) is
  a machine-local MLX build; not a stock registry pull. docs/llm-providers.md's v2
  example already documents cogito's `tool_mode: "json"` requirement (line 71,
  measured in TASK-52); README names gemma4 at line 86.

**Decision**: `DefaultConfig()` local provider → `{model: "cogito:3b",
tool_mode: "json", parallel: 4}` (parallel 4 = the documented live-proven cogito
shape; endpoint_capacity stays unset — opt-in cross-world knob). Cloud tier
untouched. `cmdNew` prints the expected model + `ollama pull cogito:3b`.
docs/llm-providers.md presents cogito+json as *the fresh-world default* and
gemma-class as the documented upgrade; README line ~86 aligned. Existing worlds:
untouched by construction (config is per-world `llm.json`, read at boot; FR-010).
**Alternatives considered**: keep gemma4 + prominent pull docs — rejected: the model
isn't a public registry name, so no pull command can be printed that works; a
default that requires editing config before first light contradicts SC-002.

## R7 — Testing strategy

- `httptest` fake OpenAI-compat servers (existing pattern in internal/llm tests):
  (a) `/models` with/without the configured id, (b) 404 on `/models` (listing
  unsupported), (c) connection refused → unreachable; assert condition kind,
  remedy text, boot-never-fails, and clear-on-re-probe.
- Worker detector: fake caller returning tool-free completions on tool-carrying
  requests → assert raise at threshold, reset on tool call, clear on success,
  non-tool kinds never count.
- Status wire: `StatusSnapshot()` includes condition fields; `cmdStatus` render
  golden-ish assertions; TUI header badge unit test (existing views test pattern).
- Defaults: `DefaultConfig()`/`WriteDefault` golden assertions + docs greps live in
  quickstart validation (SC-004).
