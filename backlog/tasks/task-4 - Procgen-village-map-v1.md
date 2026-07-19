---
id: TASK-4
title: Procgen village map v1
status: In Progress
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 03:25'
labels:
  - engine
  - procgen
dependencies:
  - TASK-2
ordinal: 4000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
One village area: terrain with wood, water, forage, huntable animals; NO starting buildings (Minecraft-style cold start). Map representation must scale to DF-style sizes later (engine requirement, not v1 feature). Grounding: grounded-assumptions.md (The world).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Seeded generation produces a village area with wood, water, forage, and animals
- [ ] #2 No structures at seed; buildable sites exist
- [ ] #3 Same seed reproduces the same map
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Branch task-4-procgen-map off main
2. internal/worldmap package: Generate(seed, w, h) — hand-rolled seeded value-noise heightmap + moisture; water (low percentile), tree patches (moist land), forage scatter, animal dens; flat []TileKind slice (DF-scale-ready representation); Passable/At/Hash API; no structure tile kind exists at all (cold start by construction)
3. Manifest: map_width/map_height (default 64x64; absent fields in old saves default safely)
4. sim integration: NewState places wanderers on passable tiles; stepEvents moves against the map (impassable target = stay; escape rule if standing on impassable after old-save migration)
5. TUI map pane: terrain glyphs + camera panning (arrows, c recenter) since 64x64 exceeds a terminal
6. Tests: worldmap determinism (same seed = identical hash), AC coverage (wood/water/forage/dens present, buildable sites, no structures), sim movement respects passability; full -race suite
7. Wiki-update re-pin + new worldmap note; PR off main; board close-out
<!-- SECTION:PLAN:END -->
