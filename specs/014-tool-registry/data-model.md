# Data Model: Tool Registry (spec 014)

**Date**: 2026-07-22 · Grounded in research.md R1–R9. All types live in the new leaf
package `internal/tool` unless noted. Nothing here is persisted — registry and rosters
are static compiled data; no event log or snapshot format changes (spec assumption).

## Tool

One capability an agent can request. The unit of the registry.

| Field | Type | Notes |
|---|---|---|
| `Name` | string | unique, non-empty, snake_case; the goal/tool identifier models emit (`chop`, `talk_to`, `say`, `muse`, `gist`, `converse`, `nudge_dream`, `nudge_omen`) |
| `Effect` | EffectClass | `World` \| `Expressive` \| `Read` |
| `Params` | []Param | ordered descriptors; empty for param-less verbs |
| `Gate` | GateClass + prose | declarative statement of the precondition class checked at landing (see Gate) |
| `Cost` | Cost | see Cost |
| `PlanStep` | bool | may appear as a guarded plan step (world tools: all true post-FR-012) |
| `ReflexEligible` | bool | doctrine data only; `decideIntent` remains hand-written (R6) |
| `PromptGloss` | string | the verb's prompt documentation line(s); empty when the current prompt has none. Byte-exact source for derived prompt text (R3) |
| `Events` | []string | expressive tools only: event types this tool may land; must be ⊆ `injectSocialWhitelist` (R5) |

**Validation rules** (enforced by `tool.Validate`, R9): unique names; known effect
class; `Events` non-empty iff `Effect == Expressive`; `PlanStep`/`ReflexEligible` false
unless `Effect == World`; every `Events` entry whitelisted.

## EffectClass

| Value | Meaning | Landing path |
|---|---|---|
| `World` | produces an Intent the executor grounds in time/space | `Loop.InjectIntent` |
| `Expressive` | immediate bounded batch of whitelisted events | `Loop.InjectSocial` |
| `Read` | data back into cognition, no events | none in this layer (declared, zero entries — TASK-52) |

## Param

Minimal descriptor (R8): `Name` string · `Kind` (`agentName` \| `text` \| `enum`) ·
`Required` bool · `MaxBytes`/`MaxRunes` int (0 = n/a) · `Enum` []string (forms, etc.).

Examples: `talk_to{target: agentName, required}`; `say{text: text, ≤300 bytes}`;
`nudge_dream{target: agentName, required; text: text, ≤400 chars}`.

## Gate

Declarative, not executable (R1/R2): `GateClass` names the precondition family the
door/reducer enforces — `resolvable` (world tools: `resolveGoal` must produce an intent
against live state), `charge` (nudges: bank ≥ cost, reducer dry-run enforces),
`scene` (say/gist: active conversation scene), `none` (muse). The enforcing code stays
in `sim`/`metatron`; the registry states which family applies so the derivation tests
can assert coverage.

## Cost

| Field | Type | Used by |
|---|---|---|
| `DurationTicks` | int64 | world tools → `sim` duration table (derives `intentDuration`); 0 = instant-on-arrival, context overrides stay in `workDuration` (R7) |
| `Charges` | int | nudges: 1; all others 0 |
| `TextCapBytes` / `TextCapRunes` | int | say 300B, gist 200B, nudge 400, muse 200 runes — parse/validation sites read these instead of local literals |

## Roster

Ordered list of registry tool names per agent kind (R4). Static data in `internal/tool`:

- **`RosterVillager`**: the 19 world verbs (order = today's `goalVocabulary` order,
  byte-identity anchor) + `say`, `muse`, `gist`.
- **`RosterMetatron`**: `converse`, `nudge_dream`, `nudge_omen`.

**Validation**: every roster name resolves to a registry entry; rosters may not name
`Read` tools in this layer. Membership is checked at the doors: `InjectIntent` (goal ∈
villager roster ∧ PlanStep for plan steps), metatron nudge validation (form ∈ metatron
roster). Rejection path identical to today's unknown-goal handling.

## Derived surfaces (not stored — computed from one registry walk)

| Surface | Replaces | Derivation |
|---|---|---|
| Prompt vocabulary string + gloss block | `goalVocabulary` const + prose (`prompt.go:15,27–33`) | join World-class villager-roster names in registration order; append `PromptGloss` lines. Byte-identical to today (SC-003) |
| Mind parse accept set | `validGoals` (`parse.go:31`) | set of World-class villager-roster names |
| Plan-step accept set | `planGoals` (`plan.go:33`) | names with `PlanStep == true` (the FR-012 delta: 19, was 10) |
| Duration table | `intentDuration` switch (`agents.go:267`) | `Cost.DurationTicks` per world tool |
| Text-cap constants | literals in `parse.go`, `sim/metatron.go`, `metatron/turn.go` | `Cost.TextCap*` per expressive tool |

**Invariant** (TASK-55 AC#2, tested): prompt vocabulary set ≡ parse accept set ≡
plan-step accept set — all three are the same walk, so divergence is impossible by
construction and asserted by test anyway.

## State transitions

None. No entity here has runtime state: the registry is immutable after `Validate()` at
startup; the tool-call → gate → event lifecycle remains owned by the existing doors and
reducer, unchanged (FR-013/FR-014).

## Relationships

```
Registry 1—n Tool
Roster   n—n Tool (by name; villager, metatron)
Tool(World)      —derives→ prompt vocab, parse set, plan-step set, duration table
Tool(Expressive) —declares→ Events ⊆ injectSocialWhitelist (backstop unchanged)
Tool(Read)       —declared only; zero entries until TASK-52
```
