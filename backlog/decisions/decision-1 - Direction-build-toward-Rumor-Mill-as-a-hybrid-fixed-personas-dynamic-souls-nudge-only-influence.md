---
id: decision-1
title: >-
  Direction: build toward Rumor Mill as a hybrid - fixed personas, dynamic
  souls, nudge-only influence
date: '2026-07-18 20:49'
status: Accepted
---
## Context

`research/project-plans.html` (2026-07-18) fleshed out three concepts for script-world, each a
different stance on what a prompt is: Homestead (a conversation — edit charters live), Rumor Mill
(a personality — cast a society, read the chronicle), Sealed Orders (a program — seal orders,
survive a run). Evan reviewed all three and picked a direction.

## Decision

Build toward **Rumor Mill**, modified into a hybrid with these four rules:

1. **~20 agents** at world seed (up from the concept's 12).
2. **Persona + soul:** each agent starts from a specific authored personality prompt (persona)
   plus a dynamic meta-layer (a `soul.md` of memories, relationships, beliefs). Agents live and
   grow — experience accretes into the soul.
3. **Personas are fixed.** Direct modification of an agent's personality is not a player verb —
   the "recast" verb from the original Rumor Mill pitch is removed.
4. **All influence is indirect,** via suggestion/nudge (the whisper mechanic): planted thoughts,
   dreams, rumors that agents interpret through their fixed persona. You can nudge; you cannot
   edit.

Homestead remains a mine for mechanics (needs, growth-through-use, journal → soul); Sealed Orders
is shelved.

## Consequences

- TASK-1 (Socratic grounding session) narrows: it interrogates this hybrid, not three open
  concepts. The "Chosen direction" section of `research/project-plans.html` is its input.
- Character drama must come from drift between fixed nature and lived experience, since recasting
  is gone; a badly-cast agent cannot be fixed, only influenced — the mitigation surface (nudges,
  social pressure, possibly generational turnover) is a required design topic.
- 20 agents raises the LLM cost ceiling; tiered models and prompt caching graduate from
  optimizations to requirements.
