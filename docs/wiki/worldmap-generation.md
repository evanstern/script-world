---
name: worldmap-generation
description: Seeded village terrain — integer-hash value noise generates water, woods, forage, and animal dens; never persisted, regenerated from the manifest anywhere
kind: component
sources:
  - internal/worldmap/worldmap.go
  - internal/worldmap/noise.go
verified_against: cdb24b60395f9f75d86df545df7dcc027f384bcb
---

# World map generation

`internal/worldmap` generates the village terrain as a **pure function of
(seed, width, height)**. The map is never persisted or sent over the wire: the daemon,
the TUI, and any future tool regenerate the identical map from the manifest
(`world.Map()`). Only dynamic changes (buildings, when TASK-5+ adds them) will be
event-sourced on top of this static base.

## How it works

Representation: `Map{W, H, Tiles []TileKind, Dens []Point}` — a flat slice indexed
`y*W+x`, the shape that scales to DF-style sizes later (the engine requirement from
the grounding session). `TileKind` is `Grass | Water | Tree | Forage`. **There is
deliberately no structure kind**: worlds start cold (Minecraft-style), so "no
structures at seed" holds by construction, and `Buildable(x,y)` = plain grass.

Noise (`noise.go`): integer-hash value noise — lattice values from FNV-64a of
(seed, purpose, lattice point), smoothstep-bilinear interpolation, summed over three
octaves (cells 16/8/4, halving amplitude) in `fbm`. Pure integer hashing keeps
generation byte-identical across platforms and Go versions, the same discipline as
[[deterministic-rng]].

`Generate` pipeline: elevation and moisture fields (`fbm` with different purpose
tags) → water floods the lowest `waterFraction = 18%` of elevation (percentile
threshold, so every seed gets real water) → trees claim the moistest
`treeFraction = 24%` of dry land (correlated noise ⇒ woods, not salt-and-pepper) →
forage scatters over remaining grass at ~4.5% per-tile hash probability → four animal
`Dens` are picked from a deterministic candidate stream, grass-only, ≥12 Manhattan
apart. Zero dims default to `DefaultSize = 64`.

`Passable(x,y)` = in-bounds grass or forage (water and standing trees block);
`Hash()` fingerprints tiles + dens for determinism tests.

## Connections

[[world-save-directory]]'s manifest carries `map_width`/`map_height` and `world.Map()`
regenerates; the [[executor]] overlays dynamic terrain on it and moves agents against effective passability;
[[sim-state-reducer]]'s genesis places them on passable tiles; the [[tui-client]] map
pane renders tiles and dens. Dens are huntable food sites with cooldowns as of TASK-5.

## Operational notes

Tuning constants live at the top of `worldmap.go`. Tests assert, across a seed
spread: same seed ⇒ identical `Hash()` (AC#3); water/trees/forage/dens all present
(AC#1); ≥25% buildable open grass (AC#2). Changing any tuning constant or the noise
changes every existing world's terrain on next daemon start — treat generation as
format-versioned behavior once real saves matter.
