# Recipe & Tool Contract Additions: spec 032 (walls, axes, paths)

Additions to the authoritative recipe table (`internal/sim/recipes.go`) and tool registry
(`internal/tool/registry.go`). The machine tables must agree with these literal numbers;
`recipes_test.go` asserts the mirror (spec 012 convention).

## New recipes

| Goal | Inputs | Outputs | Structure | Duration (ticks) | Site |
|---|---|---|---|---|---|
| `craft_axe` | 1 plank + 1 stone | 1 axe (10 uses) | — | 240 | anywhere |
| `build_wall_plank` | 2 planks | — | `wall_plank` (HP 200) | 600 | on_site (adjacent-stand) |
| `build_wall_stone` | 2 refined_stone | — | `wall_stone` (HP 600) | 600 | on_site (adjacent-stand) |
| `build_path` | 1 stone | — | `path` | 240 | on_site |

`demolish` and `repair` are world verbs but not recipes (no inputs table entry needed for
demolish; repair consumes material per cycle in the reducer): demolish 300 ticks/cycle
chipping 100 HP; repair 240 ticks/cycle consuming 1 matching material (planks /
refined_stone) restoring 100 HP, clamped to the derived max.

## Changed yields (economy rebalance)

| Verb | Old yield | New bare-handed | New with axe | Axe durability spent |
|---|---|---|---|---|
| `chop` | 2 wood | **1 wood** | **3 wood** | 1 use |
| `quarry` | 2 stone | **1 stone** | **3 stone** | 1 use |

Free-bulk truncation (`minInt(yield, freeBulk)`) still applies after the axe branch.
Axe presence is judged against pre-mutation state by emitter and reducer independently
(spear-hunt precedent). Duration of chop/quarry is unchanged by the axe.

## New tool-registry rows (`worldToolsBase`)

| Name | DurationTicks | PlanStep | ReflexEligible | Notes |
|---|---|---|---|---|
| `craft_axe` | 240 | yes | no | gloss: axe cost + 3x harvest + durability |
| `build_wall_plank` | 600 | yes | no | gloss: walls block movement, HP, repairable |
| `build_wall_stone` | 600 | yes | no | (covered by wall gloss) |
| `build_path` | 240 | yes | no | gloss: 2x walking speed on paths |
| `demolish` | 300 | yes | no | gloss: tear down nearest wall, cycle by cycle |
| `repair` | 240 | yes | no | gloss: mend nearest damaged wall, 1 material/cycle |

Every row must satisfy the boot coverage gate (`ValidateToolCoverage`): a `goalResolvers`
arm + an `intentDurations` entry each. The storage-param enum `itemKinds` gains `"axes"`.
