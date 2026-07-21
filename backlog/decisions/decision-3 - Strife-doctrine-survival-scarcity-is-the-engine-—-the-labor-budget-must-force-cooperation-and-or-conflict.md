---
id: decision-3
title: >-
  Strife doctrine: survival scarcity is the engine — the labor budget must force
  cooperation and/or conflict
date: '2026-07-20 19:53'
status: proposed
---
## Context

Roguelike design session (user, 2026-07-20). Survival in script-world v1 is trivially cheap: foraging yields a food unit in 2 game-minutes, a fire costs 10 minutes + 2 wood and then burns forever, a shelter is 20 minutes of work, and health regenerates while awake. Agents can satisfy all needs with minutes of labor per day, so nothing forces them to depend on — or exploit — each other. The social systems built in TASK-8/TASK-13 (relationships, debts, rumors, norms, votes) have little material substrate to be *about*.

## Decision

Survival must be difficult enough that the daily labor budget is the engine of the drama. Calibration targets (game-time):

- **Solo break-even ≈ a full working day.** Feeding yourself ≈ 4h of foraging; fueling one fire through an 8h night ≈ 8 wood ≈ 4h of chopping. A lone agent spends ~8h/day just not dying, with no slack for building, hoarding, or socializing.
- **Cooperation creates the only surplus.** A shared fire divides the fuel bill by the number of agents around it; a successful hunt feeds the hunter plus one other; a hut is ~2 person-days that can be parallelized. Village-of-8 coordination drops the per-agent cost to ~4–5h/day — surplus exists, but only through shared infrastructure and division of labor.
- **Seasons turn the screw.** The cold season raises fuel needs and suppresses food regrowth, pushing solo agents into structural deficit. Surviving winter requires warm-season stockpiles — which creates hoarding, sharing, theft, and norms worth voting on.
- **Permadeath makes the stakes real.** Death is already permanent per agent; runs get run-level outcomes (a village can die).

Strife — cooperation *and/or* conflict — is the success metric of the survival economy. Whether agents respond to scarcity by organizing fire-duty rosters or by stealing from a neighbor's stockpile, both are wins; comfort is the failure mode.

## Consequences

- Every effort/yield/decay constant is tuned against the labor-budget targets above, not against "feels fun in isolation" (TASK-30 owns the calibration worksheet).
- New survival mechanics (seasons TASK-28, fire fuel TASK-29, run outcomes TASK-31) and the resource/inventory designs (TASK-25/26) must each state how they serve this doctrine.
- The norms-and-votes system (specs/006) and social fabric become load-bearing: fire duty, food sharing, and hoarding are exactly the things a village legislates about.
