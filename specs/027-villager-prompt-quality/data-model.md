# Data Model: Villager Prompt Quality

**Date**: 2026-07-23 | **Plan**: [plan.md](plan.md)

No persisted entities change. Two logical structures matter.

## 1. The system prompt frame (rendered text, not stored)

`systemPrompt(name, personaText string) string` — a pure function; same inputs →
byte-identical output (FR-005, SC-005). The rendered frame has ordered parts:

| # | Part | Content rules |
|---|------|---------------|
| 1 | Identity statement | One sentence; contains `name` — the ONLY occurrence of `name` in the frame (FR-001). Establishes: a villager in a small settlement. |
| 2 | Persona block | `personaText` verbatim, set off as its own block; when empty, part 2 vanishes cleanly (no dangling separators / doubled blank lines) (FR-002). |
| 3 | Task framing | The decision contract, doctrine-preserving (FR-003): decide by calling exactly one acting tool; read-only tools may precede it; `set_plan` and `muse` are actions (a beat spent thinking is a beat not spent doing); no free-text action path. Second-person voice ("you"), no name repetition. |
| 4 | Worked exemplar *(variant-conditional)* | One generic tool-selection example per research D5: hypothetical situation shape, no real names, no literal JSON args, not `muse`-featuring. Present only in the `new+exemplar` variant; ships only if it wins the eval (FR-004). |

**Invariants**

- Frame text contains `name` exactly once (persona text exempt).
- No dynamic content (ticks, needs, world state) — cacheable-prefix purity.
- Doctrine meaning fixed; wording free.

## 2. The eval record (tracked artifact, `specs/027-villager-prompt-quality/eval/`)

One record per variant (`old`, `new`, `new-exemplar`), produced by the soak driver
and summarized on TASK-73:

| Field | Source |
|-------|--------|
| `variant`, `git_ref` | eval driver invocation (research D2) |
| `seed`, `game_hours`, `provider/model` | soak setup (research D3) |
| `planner_tool_calls` (denominator) | `cog.tool_call` events joined to villager planner jobs (research D1) |
| `rejected_malformed`, `rejected_cardinality` (counts + rates) | verdict tally (D1) |
| `tool_distribution` | per-acting-tool selection shares (D1, screens SC-003) |
| `prompt_bytes`, `prompt_words`, `prompt_tokens_approx` | render helper (research D4) |

**Ship gate (SC-001)**: `new*.rate ≤ old.rate` for both rejection rates, for the
shipped variant. **Exemplar decision (FR-004)**: shipped variant = better (or
equal-and-cheaper) of `new` vs `new-exemplar`; decision + numbers recorded.

## State transitions

None — no store schema, event types, or sim state change in this feature.
