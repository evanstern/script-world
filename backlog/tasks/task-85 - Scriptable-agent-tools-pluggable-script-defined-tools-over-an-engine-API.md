---
id: TASK-85
title: 'Scriptable agent tools: pluggable script-defined tools over an engine API'
status: To Do
assignee: []
created_date: '2026-07-24 03:02'
updated_date: '2026-07-24 03:49'
labels:
  - idea
dependencies: []
priority: high
ordinal: 5500
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Idea capture (2026-07-23): make agent/angel tools scriptable and pluggable instead of hard-coded. An embedded scripting layer (e.g. Lua) calls a stable engine API surface (move_agent, emit_event, broadcast, heal, ...). A 'tool' becomes a script + manifest: e.g. a teleport tool that calls move_agent on itself and emits 'vanished in a poof of smoke'. Existing built-in tools would be converted to the same form (major shift, highly extensible). Personas become installable bundles dropped into the world folder, e.g. gandalf/{SOUL.md, tools/cast_light.lua, influence_verbal.lua, water_magic.lua}. Key work: (1) design the engine API surface area, (2) pick/sandbox the scripting runtime, (3) tool manifest -> LLM tool schema, (4) convert existing tools, (5) bundle install/validation. Needs a spec before implementation.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Full Spec Kit spec (specify -> clarify -> plan -> tasks) authored and linked to this task via spec-bridge:link before implementation starts
- [ ] #2 Script tools are pure functions (args + read-only world view -> event batch + narration); every emitted event is validated against the InjectSocial whitelist — scripts cannot mutate state directly or invent event types
- [ ] #3 v1 scope holds: instantaneous angel/expressive tools are scriptable; tick-simulated villager world verbs (hunt/forage/build...) remain native Go
- [ ] #4 Persona bundle (SOUL/charter fragment + capabilities.json + tools/ manifests+scripts) installs by dropping a folder into the world dir, with boot-time validation: manifest schema, declared events subset-of whitelist, script parses, step/memory caps set
- [ ] #5 Determinism preserved: scripts have no wall clock and no unseeded RNG; replaying a world containing scripted-tool events reproduces identical state hashes
- [ ] #6 At least one existing metatron tool is re-expressed as a loadable bundle tool (dogfood) proving the manifest -> registry -> derive -> handler pipeline
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Grounded design sketch (from codebase exploration 2026-07-23, pre-spec):

**Core reframe — scripts emit events, they don't call an API.** The engine is event-sourced with two doors (sim.Loop.InjectIntent / InjectSocial) and a whitelist (internal/sim/loop.go:190 injectSocialWhitelist); replay determinism is load-bearing (State.Hash). So a tool script is a pure function: (args, read-only world view) -> event batch + narration. The engine validates the batch against the whitelist and lands it through the existing door. Teleport = script returns [entity_moved, broadcast("vanished in a poof of smoke")]. Safer and simpler than an imperative move_agent() API: no re-entrancy, trivially sandboxable, deterministic by construction, and ValidateToolCoverage (declared Events subset-of whitelist) extends unchanged.

**Scope cut — two species of tool; only one is scriptable in v1:**
- Instantaneous event-batch tools (metatron miracles/nudges, expressive/social) — already "build batch + inject" (sim.BuildMiracleBatch is the hand-coded prototype). v1 target.
- Tick-simulated world verbs (hunt = 900 ticks + spear durability, via InjectIntent + executor) — NOT scriptable in v1; scripting those means scripting the executor. Stay native Go.

**Runtime:** recommend starlark-go over gopher-lua — hermetic/deterministic by design (no I/O, no clock, no ambient randomness; built-in step limits). gopher-lua needs hand-stripping os/io/math.random. Either way: no wall clock, RNG seeded from event log only, hard step/memory caps. Decide in spec.

**Bundle shape:**
gandalf/
  SOUL.md              (persona/charter fragment)
  capabilities.json    (existing grant mechanism, internal/metatron/charter.go)
  tools/<name>/{tool.json, tool.star}   (manifest: name, description, params, declared events, charge cost)
Manifest maps onto the existing Tool struct so derive.go keeps generating LLM schemas + prompt gloss with no new machinery; loaded tools append to registry + roster. Precedent for world-dir file loading: charter.md / skills/*.md loaders (per-turn fresh).

**Phasing:**
1. Manifest-only declarative tools — parameterized macros over existing primitives (metatron.entity_moved / entity_removed / item_granted / time_snapped + broadcast/dream), zero script runtime. Proves load -> register -> schema -> handler pipeline.
2. Script runtime for conditional logic, sandboxed as above.
3. Widen the primitive event vocabulary one audited event at a time — e.g. heal does not exist today (needs only change via gameplay events); heal = new bounded needs_changed-style event = new store.Event type + State.Apply reducer arm + whitelist entry + charge cost. The "API surface" IS the whitelisted event vocabulary; grow it deliberately. Charge costs plug into the existing miracle economy so bundles can't be free-cast god mode.

**Key code anchors:** internal/tool/{registry.go,derive.go,roster.go,validate.go}; internal/toolloop/loop.go (Handler map); internal/sim/loop.go:190 (whitelist), internal/sim/miracles.go (primitives), internal/sim/toolcheck.go; internal/metatron/charter.go (charter/skills/capabilities loaders).
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Priority/ordering rationale (2026-07-23): placed HIGH, tail of the HIGH group (ordinal 76000) — above all mediums, below the four live-defect highs (TASK-84/40/86/87). Reason: very large surface area (internal/tool, toolloop, sim whitelist/reducers, metatron loaders, world-dir format) means longer drift = more staleness risk on a bigger issue; do it sooner rather than let the design sketch rot. Constitution tiering: architectural/cross-package -> Opus 4.8 implementation slices; full Spec Kit mandatory (already an AC).
<!-- SECTION:NOTES:END -->
