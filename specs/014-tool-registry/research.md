# Research: Tool Registry (spec 014)

**Date**: 2026-07-22 · **Input**: spec.md (clarified), full capability-plumbing map of the
codebase (grounded against current main, post-spec-012; TASK-51/spec-013 not yet merged).

## Grounding: the sites being unified

The vocabulary is authored in **four places**, and each verb additionally has per-verb
arms in five more:

| Site | What it holds | Count today |
|---|---|---|
| `internal/mind/prompt.go:15` (`goalVocabulary`) + gloss prose at 27–33 | prompt string + per-verb docs | 19 verbs |
| `internal/mind/parse.go:31` (`validGoals`) | parser whitelist | 19 verbs |
| `internal/sim/plan.go:33` (`planGoals`) | plan-step door whitelist | **10 verbs — drifted** (TASK-55) |
| `internal/sim/policy.go:24–112` (`decideIntent`) | reflex-eligible subset (doctrine) | 8 verbs |
| `internal/sim/policy.go:159–315` (`resolveGoal`) | per-verb goal→Intent resolution | switch, 19 arms |
| `internal/sim/agents.go:267` (`intentDuration`) + consts 255–265 | per-verb work-tick cost | switch |
| `internal/sim/executor.go:405–601` (`executeAtTarget`) ×3 arms + `workDuration` 610–622 | validity, instant-complete, completion-emit, context durations | switches |
| `internal/sim/state.go:205–750` (`Apply`) | per-completion-event reducer arms | one arm per outcome event |
| `internal/mind/mind.go:392–405, 434–446` | per-verb guard construction (`talk_to`) | special case |

Expressive capabilities each carry bespoke caps: utterance 300 bytes (`parse.go:103`),
gist 200 bytes (`parse.go:133`), musing 200 runes (`parse.go:59`), nudge 400 chars
(`sim/metatron.go:23` + `metatron/turn.go:138`). Doors: `InjectIntent`
(`loop.go:127`, ladder at 407–578), `InjectSocial` (`loop.go:187`, whitelist at
146–175, 17 event types). Cost precedent: `internal/cognition/registry.go:37–44`
already registers per-class Points/BudgetTicks — the house pattern for a leaf registry.

Import graph (non-test): `cognition` → nothing internal (leaf); `sim` → clock,
cognition, store, worldmap; `mind` → sim (+ llm, persona, …); `metatron` → sim.
`sim` never imports `mind`/`metatron`/`llm`.

## R1 — Registry placement: new leaf package `internal/tool`

**Decision**: a new leaf package `internal/tool`, following the `internal/cognition`
precedent: pure data + validation, importing nothing internal (or at most `store`).
`sim`, `mind`, and `metatron` all import it.

**Rationale**: gates/executors need `sim.State` + `worldmap.Map`, so executable behavior
cannot live in the registry without a `sim` ↔ registry cycle. The clean split — proven
by `cognition` — is *declarative data in the leaf* (names, param descriptors, effect
class, costs, flags, prompt gloss) and *behavior in `sim`* (resolution, execution,
reduction), keyed by tool name. `sim` imports `tool` exactly as it imports `cognition`.

**Alternatives considered**: (a) registry inside `internal/sim` — no new package, but
`mind` would derive prompt text from `sim` internals and the registry would be buried in
the largest package; rejected for discoverability and because TASK-52's model-facing
tool-call layer must read the registry from `mind` without dragging in sim internals.
(b) Interface-typed gates in the registry with `sim` implementing them — over-abstract
for a formalization layer; deferred until TASK-52 proves the need.

## R2 — Behavior stays where it is; switches become name-keyed tables

**Decision**: `resolveGoal` and `intentDuration` restructure from `switch` statements to
per-tool entries in `sim`-owned tables (`map[toolName]resolver`, durations as registry
cost data), populated at init and cross-checked against the registry (R9).
`executeAtTarget`'s three arms, `workDuration`'s context overrides, the reducer arms,
and the reflex ladder (`decideIntent`) are **not** restructured in this layer — they are
keyed by intent/event, not by capability lookup, and touching them risks the
behavior-identity guarantee for zero derivation value.

**Rationale**: the registry's job is to kill the *vocabulary* duplication (the drift
class of bug). Table-izing resolveGoal/intentDuration lets startup verify "every world
tool resolves and has a duration" — the completeness gate. The executor/reducer arms
are per-*event* logic that a vocabulary registry cannot meaningfully derive; spec FR-011
(behavior-identical) outweighs uniformity there.

**Alternatives considered**: full executor-arm registration (per-tool
validity/completion closures) — the "everything is a plugin" shape; rejected as high-risk
churn in the most replay-sensitive code for no spec requirement.

## R3 — Byte-identical prompts: derivation order is data, plus a golden test

**Decision**: the registry preserves **registration order** matching today's
`goalVocabulary` string order; the derived vocabulary line and the per-verb gloss lines
(carried as `PromptGloss` data on each entry) must reproduce today's prompt text
byte-for-byte. A new golden-prompt test pins the full `systemPrompt` output for a
representative persona against a fixture captured from current main **before** the
refactor lands.

**Rationale**: FR-004/SC-003 demand byte-identity (prompt-cache stability and
behavior-identity both hang on it); no prompt snapshot test exists today (verified gap),
so one must be created as the migration's anchor.

**Alternatives considered**: semantic (whitespace-tolerant) comparison — rejected; the
guarantee is byte-level or it is nothing.

## R4 — Rosters: static data in `internal/tool`; doors enforce membership

**Decision**: rosters are declared in the registry package as ordered tool-name lists —
`RosterVillager` (19 world verbs + say + muse + gist), `RosterMetatron` (converse,
nudge_dream, nudge_omen). `InjectIntent` validates the goal against the villager roster
(replacing the raw map lookups — same accept set, same rejection path);
`metatron.landNudge` + the reducer dry-run validate nudge forms against the metatron
roster equivalently. Roster keying is per agent-kind; individuals come later.

**Rationale**: today the "roster" is implicit in which code path an agent's driver calls.
Making it door-checked data satisfies AC #3 with zero behavior change: villagers can
already only reach villager tools, metatron only its own — the check turns a structural
accident into an enforced invariant.

**Alternatives considered**: roster as config file — rejected for this layer; nothing
needs runtime editing, and file config would need migration/versioning for no user.

## R5 — Effect classes and the whitelist relationship

**Decision**: three effect classes as spec'd. Each **expressive** tool entry declares the
event types it may land (say → `social.conversation_turn` + outcome batch types; muse →
`agent.thought`; gist → the conversation-outcome batch; nudge → `metatron.nudged` +
`agent.memory_added`). A startup check asserts every declared type ⊆
`injectSocialWhitelist`. The whitelist itself **stays exactly as-is** as the door-level
backstop (FR-013); non-migrated traffic (consolidation, chronicle, governance rephrase,
cog telemetry) continues to ride it unregistered.

**Rationale**: the whitelist is the isolation boundary and spec-protected; per-tool
declared events formalize *which slice* of it each capability uses without widening or
narrowing anything.

## R6 — Plan-step derivation and the drift cure

**Decision**: `planGoals` dies; the plan-step door derives from a per-tool
`PlanStep bool` flag, set true for all 19 world verbs (`talk_to` included, as today).
This is the FR-012 delta — the sole permitted behavior change, closing TASK-55. A
consistency test (TASK-55 AC#2) asserts the prompt vocabulary and the plan-step accept
set can never diverge again (both derive from one walk of the registry). The reflex
ladder keeps its hand-written doctrine; each entry carries `ReflexEligible bool` as
declarative doctrine data (documentation + wiki derivation), with `decideIntent`
untouched.

**Note**: the exploration agent read the 10-verb `planGoals` as possibly intentional;
spec 012 FR-020 ("every new goal MUST be … expressible as a guarded plan step") and the
map's own comment ("mirrors the planner goal vocabulary") settle it as drift. Clarified
2026-07-22: cure lands here.

## R7 — Costs as registry data

**Decision**: per-tool `Cost` carries what is *declarative*: work duration ticks (world
tools — `intentDuration` derives from it; context-dependent `workDuration` overrides
stay in the executor), text caps (say 300 bytes, gist 200 bytes, muse 200 runes, nudge
400 chars — `parse.go` and metatron validation read the registry constants instead of
local literals), and charge cost (nudges: 1). The charge *economy* (regen 1/6h, cap 3,
event-sourced bank) stays in `internal/sim/metatron.go` — it is world state, not tool
data; the registry entry references the cost, the reducer stays the enforcer.

**Rationale**: moving the literals centralizes without moving the enforcement points —
the dry-run/reducer remain the deciders (spec core principle: the gate decides).

## R8 — Param schemas: minimal Go descriptors, not JSON Schema

**Decision**: a small `Param` descriptor type (name, kind: agent-name/string/none,
required, max size) per tool — enough to describe `talk_to{target}`,
`say{text}`, `nudge{form,target,text}` today and to be consumed by TASK-52's tool-call
parser tomorrow. No JSON-Schema library, no reflection.

**Rationale**: this layer has no model-API change (FR-015); the schema's only current
consumers are validation constants and documentation. A dependency-free descriptor keeps
the leaf package a leaf.

**Alternatives considered**: real JSON Schema — premature; adopt if TASK-52's provider
tool-calling needs it, translating from these descriptors.

## R9 — Startup validation: fail fast, in one place

**Decision**: `tool.Validate()` (called from daemon startup and from a test) enforces:
unique non-empty names; known effect class; every roster name resolves; every world tool
on a roster has a `sim` resolver-table entry and a duration; every expressive tool's
declared events ⊆ the whitelist. Violations return errors that abort startup (FR-003,
edge cases). The same check runs as a plain unit test so CI catches it before any daemon
does.

## Tier decision (constitution Principle V)

Cross-package architectural change touching `internal/sim` door validation and
`internal/mind` orchestration-adjacent code → **Opus 4.8** (senior implementation tier)
for the registry/door/derivation slices; Sonnet for mechanical slices (moving cap
constants, doc reconciliation, wiki re-pin prep) — recorded on TASK-53.
