---
id: TASK-4
title: Procgen village map v1
status: Done
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 03:33'
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
- [x] #1 Seeded generation produces a village area with wood, water, forage, and animals
- [x] #2 No structures at seed; buildable sites exist
- [x] #3 Same seed reproduces the same map
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

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented on branch task-4-procgen-map (off the completed main — note: PR #2 had merged into 001-world-daemon because the repo default branch was never switched; completed that merge into main first). internal/worldmap: integer-hash value-noise generation — water (lowest 18% elevation), woods (moistest 24% of land), forage scatter, 4 spread animal dens; flat []TileKind slice (DF-scale-ready); no structure tile kind exists, so cold start holds by construction with Buildable()=grass. Manifest gains map_width/map_height (64x64 default); terrain regenerated from manifest everywhere, never persisted. Sim moves wanderers against Passable(); TUI map pane renders terrain with a panning camera. AC#1/#2 proven by worldmap tests across 6 seeds (resources present, >=25% buildable, no structures); AC#3 by same-seed Hash() equality. go test -race ./... green; PTY smoke shows the rendered village (forest edge, clearings, dens). Wiki: worldmap-generation note added, 10 notes re-pinned, gate green (19 notes).

PR: https://github.com/evanstern/script-world/pull/3 (base main)
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Seeded procgen village map shipped: deterministic terrain (water, woods, forage, animal dens) from integer-hash value noise; cold start guaranteed by the type system (no structure tiles exist); DF-scale-ready flat-slice representation; sim movement and TUI rendering are terrain-aware with a panning camera. All three ACs proven by tests across multiple seeds.
<!-- SECTION:FINAL_SUMMARY:END -->
