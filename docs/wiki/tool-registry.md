---
name: tool-registry
description: The single source of truth for agent capabilities (spec 014) — every tool as name + params + gate + effect + cost in one registry; prompt vocabulary, parse validation, sim-door validation, durations, and rosters all derived; boot-time coverage gate
kind: component
sources:
  - internal/tool/tool.go
  - internal/tool/registry.go
  - internal/tool/roster.go
  - internal/tool/derive.go
  - internal/tool/validate.go
  - internal/sim/toolcheck.go
verified_against: 367d689446f502d9351ee48959c5397d4db037a0
---

# Tool registry

`internal/tool` (spec 014, TASK-53) formalizes everything an agent can do — villager
verbs to Metatron nudges — as a `Tool`: name, param schema, gate class, effect class,
and cost, in ONE registry. The core principle it encodes: a tool call is a REQUEST;
an event is the FACT; the gate decides; the executor grounds work in time and space.
The model never asserts outcomes. This layer is deliberately behavior-identical: it
replaced three hand-maintained duplicate maps (`goalVocabulary` in the prompt,
`validGoals` in the parser, `planGoals` at the sim door) that had ALREADY drifted in
shipped code — the plan-step map silently rejected the nine spec-012 verbs (TASK-55);
curing that drift is the migration's sole permitted behavioral delta (FR-012).

## How it works

**The catalog** (`registry.go`): 30 entries in the old vocabulary's registration
order — 24 world verbs (the original ten, spec 012's nine, spec 013's five storage
verbs), the expressive `say`/`gist`/`muse`, and Metatron's
`converse`/`nudge_dream`/`nudge_omen`. Each `Tool` (`tool.go`) carries an
`EffectClass` (`World` → intents, executor-grounded; `Expressive` → immediate
whitelisted event batches; `Read` → data back into cognition, reserved for TASK-52),
`Param`s (`AgentName`/`Text`/`Enum` kinds — storage verbs' `qty` is deliberately
un-modeled pending a numeric ParamKind for TASK-52), a `GateClass`, and a `Cost`
(work `DurationTicks` for world verbs, `TextCapBytes`/`TextCapRunes` for expressive
text, nudge charges). World verbs keep their prompt gloss prose (`PromptGloss`)
byte-exact from the old hand-written prompt block. `converse` is classified
Expressive with empty `Events` (it lands no world events — transcript-only), so
`Validate`'s Events rule is one-directional: Events non-empty ⇒ Expressive.

**Derived surfaces** (`derive.go`): each consumer is one walk of the registry —
`VocabularyLine()` (the prompt's goal list, byte-identical to the old constant),
`PromptGlossBlock()` (the per-verb gloss prose), `WorldGoals()` (the mind parser's
accept set, which also feeds [[agent-mind]]'s TASK-58 structured-output schema), and
`PlanStepGoals()` (the sim door's plan-step accept set). Because all four are the
same walk, the vocabulary can no longer drift between prompt, parser, sampler
schema, and door — adding a verb touches the registry entry plus its sim resolver,
not seven sites in lockstep.

**Rosters** (`roster.go`): capability is roster membership, expressed as data.
`RosterVillager` = the world verbs (derived, registration order) + `say`/`muse`/
`gist`; `RosterMetatron` = `converse`/`nudge_dream`/`nudge_omen`. `OnRoster()` is
the door predicate: [[sim-loop]]'s intent door requires a World tool on the villager
roster (an out-of-roster or unknown name rejects exactly like an unknown goal
today), and [[metatron]]'s nudge form — both the turn-side validation and the
reducer dry-run — must be on the Metatron roster. Future asymmetry (a chief who
proposes laws) becomes a roster edit, not new plumbing.

**Validation** (`validate.go` + `internal/sim/toolcheck.go`): `tool.Validate()`
checks the registry's internal consistency (unique non-empty names, known effect
classes, Events ⇒ Expressive, PlanStep/ReflexEligible only on World tools, roster
names resolve, no Read tools on rosters) and returns ALL violations;
`sim.ValidateToolCoverage()` checks the sim side — every World tool on a roster has
a resolver-table entry and a duration, and every Expressive tool's declared
`Events` ⊆ the `InjectSocial` whitelist. Both run first thing in
[[daemon-lifecycle]]'s `daemon.Run`, before the world opens: a malformed registry
or roster aborts boot with a config error, never a tick-time failure.

**What derives on the sim side** ([[executor]], [[reflex-policy]]): `intentDuration`
reads a table built from the registry's `Cost.DurationTicks` at init (context
overrides — spear-hunt, oven-cook — stay in the executor's `workDuration`, since
the station/inventory is only known at completion time), and `resolveGoal` is a
name-keyed resolver table (`goalResolvers`) with the old switch arms verbatim.
The registry's duration literals are hand-carried mirrors of the sim constants
(R7 — `tool` is a leaf package that imports nothing internal);
`TestWorldToolDurationsMatchSimConstants` pins the two hand-equal so they can
never silently drift.

## Connections

[[agent-mind]] derives its prompt vocabulary, gloss, parser accept set, planner
schema enum, and expressive caps from here; [[sim-loop]]'s injection doors enforce
roster membership at landing; [[reflex-policy]]'s `resolveGoal` table and
[[executor]]'s duration table are the sim-side derivations the coverage gate
cross-checks; [[metatron]]'s nudge cap and form validation read the registry;
[[daemon-lifecycle]] runs the boot gates. The registry formalizes the doors — it
does not relax them: the landing ladder, whitelist, and charge economy are
unchanged enforcers. Spec: `specs/014-tool-registry/` (contracts/registry-api.md,
contracts/tool-catalog.md). TASK-52's tool-use loop and read tools build on this
layer.

## Operational notes

Migration proven behavior-identical: the full replay/determinism suite passed with
zero test-file edits, the golden-prompt fixture (`prompt_golden_test.go`, captured
pre-refactor) passed byte-unchanged through the derivation, and the whitelist was
pinned diff-identical (17 entries). Live smoke on a throwaway world: boot gates
passed, planners landed, and a multi-step plan naming `collect_water` landed — the
TASK-55 drift cure visible live (the old map rejected it). A test-only tool
registered in a test build appears in every derived surface with zero other edits.
