---
id: TASK-85
title: 'Scriptable agent tools: pluggable script-defined tools over an engine API'
status: To Do
assignee: []
created_date: '2026-07-24 03:02'
labels:
  - idea
dependencies: []
ordinal: 73000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Idea capture (2026-07-23): make agent/angel tools scriptable and pluggable instead of hard-coded. An embedded scripting layer (e.g. Lua) calls a stable engine API surface (move_agent, emit_event, broadcast, heal, ...). A 'tool' becomes a script + manifest: e.g. a teleport tool that calls move_agent on itself and emits 'vanished in a poof of smoke'. Existing built-in tools would be converted to the same form (major shift, highly extensible). Personas become installable bundles dropped into the world folder, e.g. gandalf/{SOUL.md, tools/cast_light.lua, influence_verbal.lua, water_magic.lua}. Key work: (1) design the engine API surface area, (2) pick/sandbox the scripting runtime, (3) tool manifest -> LLM tool schema, (4) convert existing tools, (5) bundle install/validation. Needs a spec before implementation.
<!-- SECTION:DESCRIPTION:END -->
