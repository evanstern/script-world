---
id: TASK-89
title: >-
  world-01 local tier runs cogito:3b — upgrade to gemma4:12b-mlx (Thornspire
  gist confabulation is model-tier, not prompt)
status: To Do
assignee: []
created_date: '2026-07-24 04:31'
labels:
  - emergent-lore
  - epistemics
  - operations
dependencies: []
priority: medium
ordinal: 76000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Found during spec 030 (TASK-79) US3 eval, 2026-07-24. The spec 030 gist-attribution eval (specs/030-epistemic-hygiene/eval/decision.md) proved the fact-flattening / asserted-unperformed-action confabulation class ('discussed the glowy tendrils after investigating') is a property of the weak local model, not the outcome prompt: gemma4:12b-mlx (repo default local tier) produces 0/18 defects with the CURRENT prompt (controls 12/12), while cogito:3b — which world-01 actually runs per ~/.promptworld/worlds/world-01/llm.json — produces 3/18 with no improvement from attribution-preserving wording (5/18). Remediation is operational: point world-01's llm.json local tier at gemma4:12b-mlx (endpoint localhost:11434 already serves it). Historical world-01 lore already laundered into memories/beliefs is handled by spec 030's decay + provenance machinery once merged.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 world-01 llm.json local.model updated to gemma4:12b-mlx and the daemon restarted against it
- [ ] #2 A post-upgrade multi-scene gist sample from world-01 shows zero fact-flattened / asserted-unperformed-action shapes (spot-check recorded on this task)
<!-- AC:END -->
