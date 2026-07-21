---
name: governance
description: Norms and votes (TASK-13) — the daily noon meeting, relationship-deterministic votes, event-sourced norms with witnessed-violation teeth, and the scribe-rendered village charter
kind: component
sources:
  - internal/sim/governance.go
  - internal/mind/meeting.go
verified_against: a49d615ec26d41ff14784f5a8f03f89d0e6c96f9
---

# Governance (norms and votes)

TASK-13's self-legislating village: villagers gather once per game day at noon,
table rules born from their grievances, and vote them into a persistent charter
— with every outcome a pure function of state, so replay never asks a model who
won. The model's only role is phrasing. This is also the substrate for
exile-by-vote, the grounding session's miscast valve of last resort.

## How it works

**The meeting** (`sim/governance.go`, beats emitted from the [[executor]]'s
`stepEvents`): at 11:30 (`meetingConveneSecond`), once per day
(`Meeting.LastMeetingDay` vs `DayIndex`, the consolidation-marker pattern),
`meeting.convened` fires and awake, non-exiled villagers are pinned to an
`attend_meeting` intent toward the meeting place — event-sourced state
(`State.MeetingPlace`), designated exactly once as the first fire's tile (else
first shelter, else map center) since the cold-start map has no landmarks. At
noon `meeting.opened` snapshots attendance (living ∧ awake ∧ within
`meetingRadius` 3 ∧ not exiled); speaking turns fire every 360 ticks in seating
order; the meeting closes when the agenda is done or at the 3600-tick timebox
(+900 grace), and stale pins clear. The [[agent-mind]] suppresses
planner/musing traffic for attendees (`sim.AtMeeting`) until close.

**Proposals** are deterministic fodder rules, first match tables, at most one
per turn: a gru memory within 3 days → curfew; a broken debt owed to you →
repay-debts; ≥2 of your own violations of an active norm → amend (curfew +2h,
once, if you like its proposer) or repeal; village-wide hostility toward
someone (mean trust+affection < −600) plus your own grudge → exile. Turn-takers
with no fodder raise their loudest negative memory as a grievance.

**Votes** resolve in the same beat (no open-proposal state survives a tick):
per attendee, an integer score over [[social-fabric]] Relation edges — trust +
affection toward the proposer; amend/repeal add a self-interest bonus for
fellow violators; exile inverts to feelings about the *target* (who does not
vote). Proposer always yea; yea iff score ≥ 0; strict majority passes, ties
fail. `meeting.proposal_resolved` carries the denormalized proposal + per-voter
positions; its reducer enacts/amends/repeals `State.Norms` and applies pairwise
voter edge deltas (aligned +affection, opposed −trust) reducer-internally.
Attendees remember outcomes (subject-tagged, toned — gossip seeds).

**Teeth**: norms are a closed vocabulary (`curfew`, `repay_debts`, `exile`)
because only observable behavior can be judged. Detectors are deterministic and
witnessed-only (≥1 awake villager in `witnessRadius`, else nothing happens):
curfew rides the per-minute beat (night, uncovered, latch once per night);
repay-debts piggybacks the hourly due-check's `promise_broken`; exile-defiance
fires when the exile lingers near the village (latch hourly). `norm.violated`
appends the norm's bounded violation ring (amend/repeal fodder) and moves
witness→violator edges; companion witness memories are `TellableFor`-eligible,
so violations become rumors with zero new machinery. Agents are *not*
hard-enforced: norms enter planner context as a "Village law" block
(`prompt.go`), and obey/skirt/defy is persona; the [[reflex-policy]] stays
norm-blind so defiance survives degraded mode.

**Phrasing, the one model touch** (`mind/meeting.go`): an enacted proposal gets
one best-effort `llm.KindMeeting` (local tier) call to restate the template in
the proposer's voice, injected as `meeting.proposal_rephrased` — the single
whitelisted governance type, validated by the [[sim-loop]] dry-run (norm
exists, text ≤ 280), swapping text and nothing else. Since TASK-32 the call
first passes the deterministic [[cognition]] router (`routeVerdict` on the
`meeting` class): a disallow emits a suppressed `cog.outcome` record and
returns — the degrade action is the template itself, so enacted law never
waits on a model. Any failure likewise leaves the template standing.

**The charter**: authoritative law is event-sourced state; the scribe renders
`village_charter.md` (rules in force with proposer/day/tally, standing
judgments, repealed rules struck through) on governance events and at start —
reconstructible from the log by construction, and deliberately distinct from
Metatron's player-editable `charter.md` ([[world-save-directory]]).

## Connections

[[executor]] emits every governance beat; [[sim-state-reducer]] carries
MeetingPlace/Meeting/Norms; [[social-fabric]]'s edges are the vote substrate
and receive the consequences; [[agent-mind]] renders the law into prompts,
suppresses convened planners, and hosts the phrasing driver; [[chronicle]]
narrates assemblies, tallies, exiles, and violations; [[event-types]] catalogs
the `meeting.*`/`norm.*` families; [[metatron]] owns the *other* charter;
[[gru]] attacks are the canonical curfew fodder.

## Operational notes

Live proof (seed 13, model-free, ~19 game days at max speed —
`specs/006-norms-and-votes/quickstart-results.md`): 18/18 noon meetings at
full attendance, closed at +2881 ticks; 7 organic proposals in which the
self-interested-legislator loop emerged unprompted — Fern, twice-caught,
amended then repealed the curfew, Oak re-tabled it the same meeting, and Fern
cast the run's only nay. 14 violations, all witnessed. Exile has not fired
organically (the −600 gate is meant to be rare); it is proven by tests.
