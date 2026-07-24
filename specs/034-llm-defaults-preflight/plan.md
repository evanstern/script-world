# Implementation Plan: Working fresh-world LLM defaults + loud dead-tier surfacing

**Branch**: `task-84-llm-defaults-preflight` | **Date**: 2026-07-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/034-llm-defaults-preflight/spec.md`

## Summary

A fresh world whose local model is absent (or whose tool-call strategy mismatches the
model) today runs silently brain-dead: calls fail or return tool-free, the breaker's
state is visible only as a glyph in the TUI metatron pane, and villagers sit at the
reflex floor. The fix has three legs:

1. **Boot + periodic preflight** (net-new `internal/llm/preflight.go`): for each
   `openai_compat` provider, GET `{endpoint}/models` and verify the configured model is
   served; classify failures as *model-missing* vs *endpoint-unreachable*, re-check
   periodically while a condition is active so it clears without restart.
2. **Provider health conditions surfaced everywhere**: a per-provider operator condition
   (missing / unreachable / tool-silent) held on the orchestrator, exported through
   `llm.Status → ipc.StatusData`, rendered in `promptworld status` human output, the TUI
   header + metatron pane, emitted as a daemon event on transitions (visible in
   line-mode attach), and logged loudly at boot. A **tool-silence detector** in the
   worker counts consecutive zero-tool-call completions on tool-carrying requests and
   raises the third condition kind.
3. **Working defaults**: `DefaultConfig()` local provider becomes `cogito:3b` +
   `tool_mode: "json"` (+ `parallel: 4`, the live-proven shape); `promptworld new`
   prints the pull command; docs/llm-providers.md and README re-aligned.

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/evanstern/promptworld`)

**Primary Dependencies**: stdlib only for this feature (`net/http` for the models
probe — same client pattern as `providers.go do()`); Bubble Tea already in the TUI

**Storage**: `llm.json` per world (defaults written at `promptworld new`); durable
event log via `store.AppendEvents` (new transition event kind); no schema migration

**Testing**: `go test ./...`; httptest fake endpoints (existing pattern in
`internal/llm` tests); table tests for detector thresholds and status rendering

**Target Platform**: darwin/local daemon (same as existing daemon)

**Project Type**: single Go module — CLI + daemon + TUI

**Performance Goals**: preflight must not delay boot (async goroutine, same pattern as
`go sampler.run(ctx)` at daemon.go:148); zero added latency on the per-call hot path
(counter is an atomic/mutexed int beside `tierHealth`)

**Constraints**: preflight failure NEVER fails boot (FR-002); no false tool-silent
positives on healthy soaks (SC-003); existing worlds byte-identical behavior except
new warnings (FR-010); no `llm.json` migration

**Scale/Scope**: ~6 packages touched (`internal/llm`, `internal/daemon`,
`internal/ipc` (render side only), `cmd/promptworld`, `internal/tui`, docs); net-new
code ≈ 1 file + surgical edits

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Check | Status |
|-----------|-------|--------|
| I. Artifact-grounded action | Spec + plan + research + contracts on disk; default decision derived from TASK-73 eval record (`specs/027-villager-prompt-quality/eval/decision.md`), not preference | PASS |
| II. One task, one PR | TASK-84 → one branch (`task-84-llm-defaults-preflight` in `.worktrees/task-84`) → one PR; spec phases are internal breakdown | PASS |
| III. Gates over assertions | spec-bridge:link before implementation; board status follows artifacts; tasks.md ticks committed on main | PASS |
| IV. Grounding freshness | `internal/llm/*`, `internal/daemon/daemon.go`, `docs/llm-providers.md`, README are wiki-note sources → `/grounding-wiki:wiki-update` + player-docs regen are explicit post-merge tasks | PASS (tracked as tasks) |
| V. Model-tiered workflow | Plan/tasks on Fable 5; implementation delegated to `spec-implementer`. Orchestrator/worker/breaker-adjacent slices (`internal/llm`, daemon wiring) = **Opus 4.8** per rubric (concurrency/scheduling logic in `internal/llm`); status/TUI rendering, `promptworld new` output, docs = **Sonnet** | PASS |

**Post-Phase-1 re-check**: no new violations introduced by the design — no new
packages, no new dependencies, no config migration. PASS.

## Project Structure

### Documentation (this feature)

```text
specs/034-llm-defaults-preflight/
├── spec.md
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── provider-conditions.md   # condition state machine + status/event wire shapes
│   └── fresh-world-defaults.md  # the llm.json a fresh world gets + new/docs alignment
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/llm/
├── preflight.go         # NET-NEW: models-list probe + periodic re-check loop
├── preflight_test.go    # NET-NEW: httptest endpoints (missing model / unreachable / no listing)
├── llm.go               # provider conditions map; tool-silence counter in worker;
│                        #   StatusSnapshot exports conditions; condition-transition hook
├── health.go            # unchanged (breaker stays as-is per spec assumption)
├── config.go            # DefaultConfig(): local → cogito:3b + tool_mode json + parallel 4
└── llm_test.go / config_test.go  # detector + defaults tests

internal/daemon/
└── daemon.go            # boot wiring: start preflight goroutine after llm.New;
                         #   condition-transition hook → daemon event + log line

internal/ipc/
└── protocol.go          # no struct change needed (StatusData.LLM already carries
                         #   llm.Status); conditions ride inside llm.ProviderStatus

cmd/promptworld/
└── commands.go          # cmdStatus human output: render active provider warnings;
                         #   cmdNew: print expected model + pull command

internal/tui/
└── views.go             # headerView: persistent [llm: …] badge while any condition
                         #   active; metatron-pane provider lines annotate condition

docs/llm-providers.md    # default = cogito:3b + json; gemma documented as upgrade
README.md                # same alignment (line ~86)
```

**Structure Decision**: everything lands in existing packages; the only net-new file
is `internal/llm/preflight.go` (+ its test). Conditions live on the `provider` struct
beside `tierHealth` (llm.go:312-336) so status export, the worker hot path, and the
preflight loop all share one home without new plumbing.

## Design decisions (Phase 0 summary — full trail in research.md)

- **D1 — probe endpoint**: `GET {endpoint}/models` (OpenAI-compat standard; Ollama
  serves it at `/v1/models`). A 404/unsupported listing → probe skipped gracefully
  (edge case), never a false "missing".
- **D2 — condition semantics**: one active condition per provider, kinds
  `model-missing` | `endpoint-unreachable` | `tool-silent`; boot preflight sets the
  first two, worker detector sets the third; ANY successful call on the provider
  clears all conditions (traffic is truth), and a periodic re-probe (60s while a
  preflight condition is active) clears without traffic (FR-004).
- **D3 — tool-silence detector**: counts *consecutive* completed calls that carried
  tools (`len(req.Tools) > 0`) but returned zero tool calls; threshold 8; any
  tool-carrying call that yields a tool call resets it. Remedy text varies by the
  provider's resolved tool mode (native → "set tool_mode json"; json → "model appears
  unsuited for tool work").
- **D4 — surfaces**: (a) boot/daemon log lines (`daemon: WARNING …` — same stream as
  existing knob warnings, lands in daemon.log); (b) `StatusData.LLM.Providers[i]`
  gains condition fields → `promptworld status` prints a WARNING block, TUI header
  gains a red badge, metatron pane annotates the provider line; (c) a
  `daemon.llm_warning` event appended/broadcast on transitions (raise + clear) via the
  existing `appendDaemonEvent` pattern, so line-mode attach streams it and it's
  durable for post-hoc audit.
- **D5 — default config**: local provider `cogito:3b`, `tool_mode: "json"`,
  `parallel: 4` — exactly the live-proven shape from docs/llm-providers.md +
  TASK-73 soaks; cloud tier untouched. `promptworld new` prints
  `ollama pull cogito:3b` guidance.

## Complexity Tracking

No constitution violations — table intentionally empty.
