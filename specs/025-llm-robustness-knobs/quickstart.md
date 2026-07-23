# Quickstart Validation: llm.json robustness knobs (TASK-72)

Prerequisites: Go 1.26.4, repo root (or the task worktree), no network needed —
every scenario runs against test stubs or a local boot.

## 1. Retry semantics (spec US1 / SC-001, SC-002)

```sh
go test ./internal/toolloop/ -run 'Retry' -v
```

Expected (test names indicative — final names in tasks.md):
- fail-once stub → run completes, `Result.Retried == true`, termination in the
  success family (SC-001);
- fail-twice stub → `Term == provider_error`, second error propagated, no third
  Submit (SC-002);
- admission-refusal stub (`ErrTierBusy` et al.) → zero retries, terminates as
  today;
- estimator invariance: recovered run feeds exactly one `ObserveCognition`
  observation, twice-failed run feeds zero (SC-004);
- round-cap invariance: a run that failed once still completes `MaxRounds` rounds.

## 2. Retry visibility in the trail (spec SC-003)

```sh
go test ./internal/mind/ ./internal/metatron/ -run 'Retr' -v
```

Expected: a recovered planner cognition and a recovered metatron turn each emit a
non-terminal `cog.outcome` event with outcome `retried` carrying the first
failure's reason; digest catalog sweep stays green:

```sh
go test ./... -run 'TestCatalogSweep'
```

## 3. Config normalization (spec US2 / SC-006)

```sh
go test ./internal/llm/ -run 'TokenBudget|MaxTokens' -v
```

Expected table (per key — planner / metatron_turn / consolidation):

| llm.json value | effective | warning |
|----------------|-----------|---------|
| absent         | default (512/1024/1024) | none |
| 0              | default   | none |
| 2048           | 2048      | none |
| -5             | default   | `out of range (min 1)` |
| 999999         | 4096      | `out of range (max 4096)` |

## 4. Boot behavior — never fatal (spec SC-005, SC-006)

In a scratch world dir, set an out-of-range knob and boot:

```sh
promptworld new scratch-072 && cd "$(promptworld path scratch-072)"
# edit llm.json → "max_tokens": {"planner": 999999}
promptworld daemon .
```

Expected: the daemon boots; boot output contains
`daemon: llm.json max_tokens.planner 999999 out of range (max 4096) — clamped to 4096`.
A valid value (e.g. 768) boots silently and planner requests carry
`max_tokens: 768` on the wire (assertable via the existing httptest wire-shape
pattern, `internal/llm/providers_test.go:31`).

## 5. Regression gates (spec SC-007, TASK-72 AC #5)

```sh
go test -race ./...
```

Expected: full suite green. A config with no `max_tokens` object produces
wire-identical requests to today (defaults byte-for-byte, spec FR-010).

After merge: `/grounding-wiki:wiki-update` re-pins the touched notes
(tool-loop, llm-orchestrator, agent-mind, metatron, nightly-consolidation,
daemon-lifecycle — exact set as computed against the diff).
