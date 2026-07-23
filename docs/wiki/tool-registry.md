---
name: tool-registry
description: The single source of truth for agent capabilities (spec 014, extended specs 017/019/021) — every tool as name + params + gate + effect + cost in one registry; prompt vocabulary, parse validation, sim-door validation, durations, and rosters all derived; the tool-use loop's declared rosters and InputSchema derivation; the authoritative miracle cost table, RestrictEnum, and the derived Metatron tool guidance (spec 021); boot-time coverage gate
kind: component
sources:
  - internal/tool/tool.go
  - internal/tool/registry.go
  - internal/tool/roster.go
  - internal/tool/derive.go
  - internal/tool/validate.go
  - internal/sim/toolcheck.go
verified_against: 8c44bf21ad22c0f1bad07ae7f2a08072a0cb5544
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

**The catalog** (`registry.go`): the pre-spec-017 30 entries, plus two spec-017
additions — `set_plan` (a loop-only planning tool) and `work_miracle` (Metatron's
fourth tool) — plus four spec-019 additions, the journal tools — assembled in
order: `worldTools` (the 24 legacy world verbs, exactly the old goal-vocabulary
order), `set_plan`, `expressiveTools` (`say`/`gist`/`muse`), `metatronTools`
(`converse`/`nudge_dream`/`nudge_omen`/`work_miracle`), `journalTools`
(`write_journal_entry`/`delete_from_journal`/`search_journal`/`read_journal`,
appended last so no existing tool's position shifts). The tool groups are
declared as separate literals (`worldTools`, `expressiveTools`, `metatronTools`,
`journalTools`) rather than one, so `set_plan`'s schema can be built from
`worldTools` alone and spliced in — building it from the assembled `registry` would
be an initialization cycle. Each `Tool` (`tool.go`) carries an `EffectClass`
(`World` → intents, executor-grounded; `Expressive` → immediate whitelisted event
batches; `Read` → data back into cognition, consumed by [[tool-loop]] — spec 019
ships the first production Read entries, `search_journal`/`read_journal`),
`Param`s (`AgentName`/`Text`/`Enum`/`Number` kinds — `Number` (spec 017 R12) pays
the spec-014 debt that left the storage verbs' `qty` unmodeled: bounded by
optional `Min`/`Max`, 0/0 meaning unbounded; every `Param` also carries an
optional `Description`, spec 019 T024, emitted verbatim as the derived JSON
Schema property's `"description"` — `""` means no description), a `GateClass`,
and a `Cost` (work `DurationTicks` for world verbs, `TextCapBytes`/
`TextCapRunes` for expressive text, nudge/miracle charges). World verbs keep
their prompt gloss prose (`PromptGloss`) byte-exact from the old hand-written
prompt block. `converse` is classified Expressive with empty `Events` (it lands
no world events — transcript-only), so `Validate`'s Events rule is
one-directional: Events non-empty ⇒ Expressive.

**The journal tools** (`registry.go`, spec 019 US3): `write_journal_entry` and
`delete_from_journal` are Expressive (their `journal.entry_written`/
`journal.entry_deleted` Events land through the `InjectSocial` door like any
other whitelisted batch); `search_journal` and `read_journal` are `Read` — data
returned into cognition, grounding nothing. All four carry `Gate: None` (no
scene, no charge — the reducer dry-run, budget or existence, is the only gate)
and are villager-only: they join `LoopRosterVillager` alone, never a Metatron
roster, since journals are private. Every acting villager world tool also gains
an optional bounded `reason` param (spec 019 R12 / T024) — the model's free-text
"why" for the action — via a post-declaration pass: `worldTools` wraps a new
`worldToolsBase` literal, appending `reasonParam()` (`Kind: Text, Required:
false, MaxRunes: ReasonCapRunes` = 200, with a capability-only description) to
every entry's `Params`, so the shared param is defined once and no verb's
literal repeats it. `reason` is deliberately absent from `muse` (interiority is
already free-standing) and every Metatron tool. `set_plan`'s authored
`InputSchemaJSON` (`setPlanSchema`) separately gains an optional top-level
`reason` string (same `ReasonCapRunes` cap) alongside its `steps` array — the
plan-level why, threaded to `InjectArgs.Reason`.

**The legacy/loop split** (`isLegacyWorldTool`, `derive.go`, spec 017 R11): a World
tool no longer automatically belongs to the free-text goal vocabulary — the
discriminator is `Effect == World && PlanStep`. Every pre-spec-017 World tool
already carries `PlanStep: true` (the TASK-55 single-walk invariant), so the
filter changes nothing for them; `set_plan` is `Effect World` (it lands through
the same `InjectIntent` path) but carries `PlanStep: false` because it is
loop-only vocabulary, not a legacy free-text goal — so it is excluded from
`VocabularyLine()`, `WorldGoals()`, and `RosterVillager` for free, with no
separate exclusion list anywhere.

**`set_plan` and `work_miracle`'s schemas** (`registry.go`): `set_plan` needs an
authored `InputSchemaJSON` override — the registry's scalar `Param` model has no
`ParamKind` for a `steps` array — built by `setPlanSchema(legacyWorldNamesFrom(worldTools))`:
a `steps` array (1..`PlanStepCap` (3) items) of `{goal, kind, qty}` objects, `goal`'s
enum drawn from the SAME legacy-World-tool filter `VocabularyLine`/`WorldGoals` use,
so the plan vocabulary can never drift from the free-text one even though the two
can't share one function call (an initialization-cycle constraint). `work_miracle`
needs no override: its flat parameter surface (`kind` required Enum over
`miracleKinds` = `move`/`remove`/`give_item`/`time_snap`, plus every per-kind field
as an optional scalar) is fully Params-derived — deliberately, because the loop
driver's `validateArgs` routes every `InputSchemaJSON` tool through `set_plan`'s
structural validator, so an override here would validate `work_miracle` calls
against the wrong shape. `work_miracle` is `Effect Expressive` (not `World`): it
lands a bounded event batch through the SAME `InjectSocial` door the nudges use
(`metatron.landMiracle` → `BuildMiracleBatch`), has no intent and no work duration,
and — decisively — `Validate` forbids a World tool from declaring `Events`, which
`work_miracle` must (so the sim-side coverage check can pin its event set ⊆ the
whitelist). There is deliberately no `gratis` parameter: the angel can never waive
a charge (spec 016 FR-007/SC-005) — structural absence, not a sanitized field.

**The miracle cost source and the spec-021 derivations** (`registry.go`,
`derive.go`; TASK-64): the per-kind miracle cost table is declared HERE, beside
`miracleKinds` — `MiracleCost(kind) (int, bool)` and `MiracleCostsByEvent()`
(kind↔event-type mapping, fresh map per call) are the ONE authoritative price
source: `sim.miracleCost` derives from `MiracleCostsByEvent()` (the import
direction already existed — [[metatron-miracles]]) and the angel's prompt renders
costs from `MiracleCost`, so a price edit propagates to enforcement and prose in
one edit (`work_miracle.Cost.Charges` stays 1 — the Charge gate's minimum, not a
price). Two new derive.go surfaces serve [[metatron]]'s per-world capability
gating: `RestrictEnum(t, param, allowed)` returns a copy-on-write `Tool` whose
named Enum param keeps only the allowed values (registry never mutated; the
tool's own Enum order preserved; `InputSchema` of the restricted copy declares
only granted values), and `MetatronToolGuidance(roster)` renders the acting-tool
guidance prose — per tool its name, argument surface (from `Params`, the same
source `InputSchema` walks), and charge cost — replacing the hand-written prose
list `turnSystemPrompt` used to carry, so described ≡ declared ≡ priced by
construction (drift tests in `derive_test.go`).

**Derived surfaces** (`derive.go`): each consumer is one walk of the registry —
`VocabularyLine()` (the prompt's goal list, byte-identical to the old constant,
now over the legacy-only filter), `PromptGlossBlock()` (the per-verb gloss
prose — scoped to `isLegacyWorldTool` tools only, since this IS the world-verb
goal prose; the journal tools' glosses, spec 019, are model-facing tool
descriptions delivered per-tool through the loop's `ToolDecl`, not this legacy
prose surface), `WorldGoals()` (the mind parser's accept set), and
`PlanStepGoals()` (the sim door's plan-step accept set) all walk
`legacyWorldNames()`. `InputSchema(t)` (spec 017 data-model.md §1) is the
tool-use loop's new consumer: returns `t.InputSchemaJSON` verbatim when set,
else derives a JSON Schema object from `t.Params` (`paramSchema` per-kind:
`AgentName`/`Text` → string, +`maxLength` from `MaxRunes`/`MaxBytes`; `Enum` →
string with an `enum`; `Number` → integer, +`minimum`/`maximum` from `Min`/
`Max`; every kind then gains `"description"` from `Param.Description` when
non-empty, spec 019 T024) — deterministic output since `Params` is already
registration-ordered and the one Go map in play (`properties`) holds only
property-name keys, which `encoding/json` sorts lexicographically.

**Rosters** (`roster.go`): capability is roster membership, expressed as data.
`RosterVillager` = the legacy world verbs (derived via `isLegacyWorldTool`,
registration order — `set_plan` excluded) + `say`/`muse`/`gist`; `RosterMetatron`
= `converse`/`nudge_dream`/`nudge_omen` (unchanged — `work_miracle` is not on this
door roster). `OnRoster()` is the door predicate: [[sim-loop]]'s intent door
requires a World tool on the villager roster, and [[metatron]]'s nudge form — both
the turn-side validation and the reducer dry-run — must be on the Metatron roster.
Two new roster exports serve [[tool-loop]] specifically, returning full `Tool`
values (not just names, since `InputSchema` needs `Params`/`InputSchemaJSON`):
`LoopRosterVillager()` = every legacy World tool, then `set_plan`, then `muse`
(`say`/`gist` stay scene-gated and out of the loop roster this task — scenes
remain driver-run, not model-initiated), then the four spec-019 journal tools
(`write_journal_entry`, `delete_from_journal`, `search_journal`, `read_journal`
— appended last so no existing declared tool's position shifts);
`LoopRosterMetatron()` = `nudge_dream`, `nudge_omen`, `work_miracle` —
deliberately NOT `RosterMetatron`, because `converse` is excluded: it is the
angel's final-answer channel (the loop's `Result.Final`), not a callable tool,
and declaring it would trap a `converse` call as `rejected_unknown` (Metatron
installs no `converse` handler by design). The journal tools are villager-only
— `LoopRosterMetatron` is untouched, since journals are private.

**Validation** (`validate.go` + `internal/sim/toolcheck.go`): `tool.Validate()`
checks the registry's internal consistency (unique non-empty names, known effect
classes, Events ⇒ Expressive, PlanStep/ReflexEligible only on World tools, Number
params' Min/Max not inverted, a set `InputSchemaJSON` is valid-JSON object shape,
roster names resolve) and returns ALL violations. Spec 017 lifts the spec-014
restriction barring Read tools from a roster (`tool-loop` is now the Read
consumer; spec 017 itself shipped zero production Read entries, but a roster
naming one was no longer a `Validate` error — spec 019 ships the first two,
`search_journal`/`read_journal`, both on `LoopRosterVillager`). `sim.ValidateToolCoverage()` checks the sim side —
every GOAL-DOOR World tool (Effect World AND PlanStep true — the same
`isLegacyWorldTool` predicate) has a resolver-table entry and a duration, and
every Expressive tool's declared `Events` ⊆ the `InjectSocial` whitelist.
`set_plan` is a World tool that deliberately carries `PlanStep: false`, so
`validateCoverage` skips it — it grounds through its own door (`injectPlan`, each
step resolving its own already-covered goal), never through `resolveGoal`/
`goalResolvers`. Both `tool.Validate()` and `sim.ValidateToolCoverage()` run
first thing in [[daemon-lifecycle]]'s `daemon.Run`, before the world opens: a
malformed registry or roster aborts boot with a config error, never a tick-time
failure.

**What derives on the sim side** ([[executor]], [[reflex-policy]]): `intentDuration`
reads a table built from the registry's `Cost.DurationTicks` at init, filtered to
goal-door (World && PlanStep) tools (context overrides — spear-hunt, oven-cook —
stay in the executor's `workDuration`, since the station/inventory is only known
at completion time), and `resolveGoal` is a name-keyed resolver table
(`goalResolvers`) with the old switch arms verbatim. The registry's duration
literals are hand-carried mirrors of the sim constants (R7 — `tool` is a leaf
package that imports nothing internal); `TestWorldToolDurationsMatchSimConstants`
pins the two hand-equal so they can never silently drift.

## Connections

[[agent-mind]] derived its prompt vocabulary, gloss, and parser accept set from
here pre-spec-017 (retired with the free-text planner reply); [[tool-loop]] is the
new consumer: `Job.Roster` is `tool.LoopRosterVillager()`/`LoopRosterMetatron()`,
and `InputSchema(t)` builds each declared tool's wire schema. [[sim-loop]]'s
injection doors enforce roster membership at landing; [[reflex-policy]]'s
`resolveGoal` table and [[executor]]'s duration table are the sim-side
derivations the coverage gate cross-checks; [[metatron]]'s nudge cap, form
validation, and `work_miracle` dispatch read the registry; [[daemon-lifecycle]]
runs the boot gates; [[agent-journal]] is the spec-019 consumer of the four
journal tools (`write_journal_entry`/`delete_from_journal`/`search_journal`/
`read_journal`) declared here. The registry formalizes the doors — it does not
relax them: the landing ladder, whitelist, and charge economy are unchanged
enforcers. Spec: `specs/014-tool-registry/` (contracts/registry-api.md,
contracts/tool-catalog.md); the tool-use loop additions are spec 017
(`data-model.md` §1-2, R11-R13); the journal tools and reason param are spec 019
(R12, T024).

## Operational notes

Migration proven behavior-identical: the full replay/determinism suite passed with
zero test-file edits, the golden-prompt fixture (`prompt_golden_test.go`, captured
pre-refactor — retired with spec 017 once the free-text planner reply it pinned
was replaced by native tool declarations) passed byte-unchanged through the
derivation, and the whitelist was pinned diff-identical (17 entries). Live smoke
on a throwaway world: boot gates
passed, planners landed, and a multi-step plan naming `collect_water` landed — the
TASK-55 drift cure visible live (the old map rejected it). A test-only tool
registered in a test build appears in every derived surface with zero other edits.
