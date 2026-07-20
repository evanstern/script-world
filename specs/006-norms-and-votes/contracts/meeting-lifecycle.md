# Contract: Meeting Lifecycle, Vote Function, Fodder Rules, Renders

The behavioral contract for the daily meeting — everything here is deterministic
(pure functions of state/map/tick) and is what `internal/sim/governance_test.go`
asserts.

## Schedule (all seconds are `clock.SecondOfDay`, epoch-aware)

| Beat | When | Effect |
|---|---|---|
| Convene | `41400` (11:30), once per day (`DayIndex > Meeting.LastMeetingDay`), only when ≥2 living villagers | `meeting.place_designated` (first time) + `meeting.convened`; pinning starts |
| Open | `43200` (noon) | `meeting.opened{attendees}`; `LastMeetingDay` advances |
| Turn | every `meetingTurnTicks = 360` while open, starting `OpenedTick+360` | next attendee speaks; possibly tables; tabled proposals resolve same beat |
| Close | when `NextSpeaker == len(Attendees)`, or `tick − OpenedTick ≥ 3600` (timebox), hard cap `≥ 3600 + meetingGraceTicks (900)` | `meeting.closed`; pinning stops |

Grace semantics: at the timebox, if the current turn's proposal has been tabled but
not yet resolved (cannot happen in v1's same-beat design) or a speaking turn beat is
due within the grace window with agenda remaining, the meeting runs to at most
+900 ticks; otherwise it closes exactly at the timebox. A meeting with 8 attendees
uses 8×360 = 2880 ticks — the timebox binds only when attendance is full and future
turn cadences slow.

Attendance snapshot at open: living ∧ awake ∧ `abs(dx)+abs(dy) ≤ meetingRadius (3)`
of the place ∧ not exiled. No late joins in v1 (the snapshot IS the roll).

## Pinning (convening → close)

On the regular staggered intent beat, each living, awake, non-exiled villager whose
intent is not already `attend_meeting` gets
`Intent{Goal: "attend_meeting", TargetX/Y: meeting place}` (source `"meeting"`).
Arrived agents idle at the place. `resolveGoal` gains no case — the goal is never
planner-choosable. The mind suppresses planner scheduling for pinned agents
(`meeting.convened` → until `meeting.closed`), the asleep-agent precedent. Asleep
villagers are left asleep and miss the meeting; exiled villagers are never pinned.

## Vote function (pure, integer)

For proposal P by proposer `pr`, voter `v` (attendee, `v ≠ exile target`):

```
base(v)  = Trust(v→pr) + Affection(v→pr)                    // RelationBetween, clamped edges
score(v) =
  add_curfew / add_repay_debts:  base(v)
  amend / repeal of norm N:      base(v) + (ViolationCount(N, v) > 0 ? selfInterestBonus : 0)
  exile of target T:             −(Trust(v→T) + Affection(v→T)) + base(v)/4

vote(v)  = yea  iff score(v) ≥ 0        // pr always yea, regardless of score
passed   = yeas > eligible/2            // strict majority; ties fail
eligible = len(Attendees) − (exile ? 1 : 0)
```

Constants (in `governance.go` const block): `selfInterestBonus = 400`,
`meetingAlignAffection = 8`, `meetingOpposeTrust = 10`, `normViolationTrust = 40`,
`normViolationAffection = 25`, `meetingRadius = 3`, `exileShunRadius = 6`,
`exileHostilityGate = −600`, `repealViolationCount = 2`, `normTextMax = 280`,
`meetingTurnTicks = 360`, `meetingTimeboxTicks = 3600`, `meetingGraceTicks = 900`,
`meetingConveneSecond = 41400`, `meetingOpenSecond = 43200`. Exported only if the
mind/TUI needs the value (expected: none in v1).

## Fodder rules (tabling, checked in order; first match tables; else `raised` note)

1. **add_curfew** — proposer holds a gru memory (kind: attack/sighting text match on
   the recorded gru memory events) from the last 3 game days ∧ no active curfew.
   Param `nightStartSecond`; template: "No one out after nightfall — the night hunts us."
2. **add_repay_debts** — proposer is creditor of a `broken` debt ∧ no active
   repay_debts norm. Template: "Debts must be repaid — a promise is a promise."
3. **amend/repeal** — proposer has ≥ `repealViolationCount` entries in an active
   norm's ring: affection(proposer→norm.Proposer) ≥ 0 → **amend** (curfew only,
   `Param += 7200`, once — `Amended` guards); else → **repeal**.
4. **exile** — some living villager T (≠ proposer): mean over living others of
   (Trust(o→T)+Affection(o→T)) < `exileHostilityGate` ∧ proposer's own edge sum
   toward T < 0 ∧ no active exile of T. Lowest-index qualifying T.

Duplicate-of-active is structurally untablable (rules 1/2/4 check first). One
proposal max per turn, per meeting speaker order = ascending agent index over the
attendance snapshot.

## Violation detectors

| Norm kind | Beat | Condition (all require ≥1 witness in `witnessRadius`, latched) |
|---|---|---|
| curfew | per-game-minute needs beat | night ∧ awake ∧ outside warmth/shelter cover ∧ `SecondOfDay ≥ Param`; latch: once per agent per night |
| repay_debts | hourly due-check (existing) | a `promise_broken` lands while norm active → same-beat `norm.violated` for the debtor; witnesses = agents within radius of debtor |
| exile | per-game-minute | exiled T within `exileShunRadius` of meeting place or any structure; latch: once per game hour |

Witness = living, awake, not the violator, Chebyshev/Manhattan distance per the
existing `witnessRadius` convention (reuse the executor's helper).

## Planner-context block (prompt.go)

Rendered after `socialContext` when relevant:

```
Village law (decided at the daily noon meeting):
- Everyone inside by nightfall. (passed day 3, Birch's proposal, 5-2)
- Debts must be repaid. (passed day 4, Alder's proposal, 6-1)
Standing judgment: Rowan is exiled from the village. (day 6, 4-2)
The village gathers at noon to decide its rules; you may raise grievances there.
```

Empty when no norms and no meeting imminent. The reflex policy remains norm-blind
(defiance must stay possible model-off).

## Charter render (scribe → `village_charter.md`)

Dirty on any `meeting.*`/`norm.*` event; rendered on scribe start. Shape:

```markdown
# Village charter
Meeting place: (x, y) — where the first fire was lit.

## Rules in force
1. Everyone inside by nightfall. — proposed by Birch, passed day 3 (5-2), amended day 9 (start moved 2h later)
2. Debts must be repaid. — proposed by Alder, passed day 4 (6-1)

## Standing judgments
- Rowan is exiled. — proposed by Cedar, passed day 6 (4-2)

## Repealed
- ~~Everyone shares the stockpile.~~ — passed day 2, repealed day 5
```

Never hand-edited; the scribe overwrites. Metatron's `charter.md` is untouched by
this feature.

## Test contract (what governance_test.go proves)

1. Full-day drive: convene → place designated once → open with correct snapshot →
   turns in seating order → close before timebox+grace; second day reuses the place.
2. Vote table: given authored Relations, each fodder kind resolves to the exact
   yea/nay sets and pass/fail above (table-driven over edge fixtures).
3. Enact/amend/repeal reducer effects; charter-relevant state matches after replay
   (`canonicalLog` hash equality, driveTicks harness).
4. Detectors: witnessed curfew breach → violated + witness memory + edge deltas;
   unwitnessed → nothing; latches hold.
5. Exile: target excluded from vote; passed exile stops pinning/attendance;
   proximity violation fires.
6. Whitelist: `meeting.proposal_rephrased` injects (valid) and rejects (unknown norm,
   oversized text); nothing else governance-shaped injects.
7. Degraded mode: whole cycle with no orchestrator — outcomes identical to
   model-on run except norm Text.
