# Eval records — 027 villager prompt quality

The ship-gate evidence for the villager system-prompt rewrite (TASK-73). One
record per prompt variant, produced by `scripts/eval-prompt-73.sh` (research
D1–D3) plus token counts from `TestPromptFrameReport` (research D4).

## Variants (research D2 — git refs, no runtime toggle)

| file | variant | git ref |
|------|---------|---------|
| `old.md` | `old` | merge-base with `origin/main` (pre-rewrite prompt) |
| `new.md` | `new` | rewrite commit, no exemplar |
| `new-exemplar.md` | `new+exemplar` | exemplar commit stacked on `new` |
| `decision.md` | — | the gate verdict + exemplar ship/reject decision |

## Record shape (data-model.md §2)

Each `<variant>.md` records:

- `variant`, `git_ref` — driver invocation
- `seed`, `game_hours`, `provider/model` — soak setup (research D3)
- `planner_tool_calls` — denominator: `cog.tool_call` events joined to villager
  planner jobs (class `planner` via `cog.thought` job id; excludes
  metatron/conversation)
- `rejected_malformed`, `rejected_cardinality` — counts + rates over the denominator
- `tool_distribution` — per-acting-tool selection shares (screens SC-003)
- `prompt_bytes`, `prompt_words`, `prompt_tokens_approx` — render helper (research D4)

## Gate (data-model.md §2)

- **Ship (SC-001)**: shipped variant's `rejected_malformed` and
  `rejected_cardinality` rates are each `≤ old`'s.
- **Distribution screen (SC-003)**: every acting tool at `≥5%` share under `old`
  keeps a nonzero share; no tool's share grows by more than `2×`, unless
  explained + accepted in `decision.md`.
- **Exemplar (FR-004)**: shipped variant = better (or equal-and-cheaper-in-tokens)
  of `new` vs `new+exemplar`; decision + numbers recorded in `decision.md`.
