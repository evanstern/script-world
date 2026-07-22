# Contract: Tool Catalog (spec 014)

The complete enumeration of registry entries this layer ships. This is the migration
contract: every row must exist as a registry entry, and NOTHING else may be registered.
Attribute values marked "= today" are carried verbatim from the named source at
implementation time (TASK-51 may have grown the verb set by then — the catalog extends
to whatever `goalVocabulary` holds on main when implementation branches; the shape of
each row is fixed here).

## World tools (Effect: World → InjectIntent; villager roster)

Registration order = today's `goalVocabulary` order (`internal/mind/prompt.go:15`) —
this order is the byte-identity anchor for the derived prompt string (SC-003).

| Name | Params | Gate | DurationTicks | PlanStep | ReflexEligible | PromptGloss |
|---|---|---|---|---|---|---|
| forage | — | resolvable | = today (`agents.go:255–265`) | true | true | — |
| chop | — | resolvable | = today | true | true | — |
| hunt | — | resolvable | = today | true | true (via foodIntent) | — |
| build_fire | — | resolvable | = today | true | true | — |
| build_shelter | — | resolvable | = today | true | false | — |
| eat | — | resolvable | = today | true | true | — |
| sleep | — | resolvable | 0 (instant) | true | true | — |
| wander | — | resolvable | 0 (instant) | true | true | — |
| goto_warmth | — | resolvable | 0 (instant) | true | true | — |
| talk_to | target: agentName, required | resolvable (+ alive/present guards built mind-side, unchanged) | 0 (instant) | true | false | — |
| quarry | — | resolvable | = today | true | false | = today (`prompt.go:27`) |
| collect_water | — | resolvable | = today | true | false | = today |
| cook | — | resolvable | = today (+ `workDuration` oven override stays executor-side) | true | false | = today (`prompt.go:28`) |
| refuel_fire | — | resolvable | 0 (instant) | true | true (shared goal, spec 012 FR-020) | = today |
| craft_planks | — | resolvable | = today | true | false | = today (`prompt.go:29`) |
| craft_stone | — | resolvable | = today | true | false | = today |
| craft_spear | — | resolvable | = today (+ spear hunt override stays executor-side) | true | false | = today |
| build_oven | — | resolvable | = today | true | false | = today (`prompt.go:30`) |
| bathe | — | resolvable | = today | true | false | = today |
| drop | kind: enum (item keys), optional | resolvable | 0 (instant) | true | false | = today (`prompt.go:31`) |
| pick_up | kind: enum (item keys), optional | resolvable | 0 (instant) | true | false | — (shares drop's gloss line) |
| build_chest | — | resolvable | = today (`buildFireTicks`, 600) | true | false | = today (`prompt.go:32`) |
| deposit | kind: enum (item keys), optional | resolvable | 0 (instant) | true | false | — (shares build_chest's gloss line) |
| withdraw | kind: enum (item keys), optional | resolvable | 0 (instant) | true | false | — (shares build_chest's gloss line) |

**T002 re-enumeration (post-TASK-51 / spec 013 merged, 2026-07-22)**: `goalVocabulary`
now holds 24 world verbs — the 19 above plus the 5 storage verbs (`drop`, `pick_up`,
`build_chest`, `deposit`, `withdraw`) added by spec 013. The catalog shape is fixed; these
rows extend it. Their `kind`/`qty` argument surface is carried by
`validateKindQty`/`validKinds` (`internal/mind/parse.go`) and `Kind`/`Qty` on
`sim.InjectArgs`/`sim.PlanStep`; `validKinds` is NOT migrated in this layer (parse.go
keeps it — it is not a capability-vocabulary list). The `qty` integer argument has no
representable `ParamKind` in the fixed contract (`AgentName`/`Text`/`Enum`), so only
`kind` is modeled as a Param here (flagged for TASK-52, which consumes Params for
tool-call parsing).

`PlanStep: true` across all rows IS the FR-012 delta — but post-TASK-51 the delta is the
**9 spec-012 verbs** (`quarry`, `collect_water`, `cook`, `refuel_fire`, `craft_planks`,
`craft_stone`, `craft_spear`, `build_oven`, `bathe`): today's `planGoals`
(`internal/sim/plan.go`) admits 15 — the first 10 plus the 5 spec-013 storage verbs,
which TASK-51 added to `planGoals` correctly. Only the 9 spec-012 verbs remain missing
from `planGoals`, so they alone are the cured drift. `ReflexEligible` is declarative
doctrine data mirroring `decideIntent` (`policy.go:24–112`), which stays hand-written.

## Expressive tools (Effect: Expressive → InjectSocial; villager roster)

| Name | Params | Gate | Cost | Events (⊆ whitelist) |
|---|---|---|---|---|
| say | text ≤ 300 bytes (`parse.go:103`) | scene | TextCapBytes 300 | `social.conversation_turn` |
| gist | gist ≤ 200 bytes, topics ≤ 3×40, tones ∈ [-2,2] (`parse.go:110–168`) | scene | TextCapBytes 200 | `social.conversation`, `social.relation_changed`, `social.rumor_told`, `agent.memory_added` (the outcome batch as landed today at `convo.go:363`) |
| muse | text ≤ 200 runes (`parse.go:59`) | none | TextCapRunes 200 | `agent.thought` |

Scheduling/triggering of all three is untouched (spec: muse cadence stays; scenes stay
driver-run). The registry describes what each lands, not when.

## Metatron tools (roster: metatron)

| Name | Params | Gate | Cost | Events |
|---|---|---|---|---|
| converse | text | none (refusals free) | — | — (transcript only; no world events) |
| nudge_dream | target: agentName, required; text ≤ 400 (`sim/metatron.go:23`) | charge (bank ≥ 1; reducer dry-run enforces) | Charges 1, TextCap 400 | `metatron.nudged`, `agent.memory_added` |
| nudge_omen | targets; text ≤ 400 | charge | Charges 1, TextCap 400 | `metatron.nudged`, `agent.memory_added` |

The charge *economy* (regen 1/6h, cap 3, genesis 1) remains world state in
`internal/sim/metatron.go` — the registry references the cost; the reducer stays the
enforcer (R7).

## Read tools

Zero entries. The `Read` effect class exists in the type system only (FR-002); startup
validation rejects any roster referencing a Read tool in this layer.

## Explicitly NOT in the catalog (clarified 2026-07-22)

Nightly-consolidation memory writes, chronicle entries, governance rephrase
(`meeting.proposal_rephrased`), and cognition telemetry (`cog.*`) keep riding the
preserved `injectSocialWhitelist` door unregistered. The whitelist itself does not
change by one entry.
