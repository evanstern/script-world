# Contract: Governance Events

New event domains: `meeting.*` (assembly lifecycle) and `norm.*` (law effects). All
payloads are structs (deterministic JSON), carry agent **indices** (TASK-17
convention), and are reducer-total: events whose subjects have vanished degrade to
no-ops, never errors.

Dispatch: `State.Apply` gains
`case "meeting.place_designated", "meeting.convened", ... : return s.applyGovernance(e)`.

## Emitters

Every governance event is **executor-emitted** (pure function of state+map+tick)
except `meeting.proposal_rephrased`, the single **injected** (whitelisted) type.

| Event | Emitter | Whitelisted |
|---|---|---|
| `meeting.place_designated` | executor (first convene) | no |
| `meeting.convened` | executor (11:30) | no |
| `meeting.opened` | executor (noon) | no |
| `meeting.turn_taken` | executor (turn beat) | no |
| `meeting.proposal_tabled` | executor (turn beat, fodder rule) | no |
| `meeting.proposal_resolved` | executor (same beat as tabled) | no |
| `meeting.closed` | executor (agenda done / timebox+grace) | no |
| `meeting.proposal_rephrased` | mind driver via `InjectSocial` | **yes** |
| `norm.violated` | executor (detectors) | no |

`injectSocialWhitelist` delta: `+ "meeting.proposal_rephrased"`. Nothing else becomes
injectable.

## Payloads and reducer effects

### `meeting.place_designated`

| Field | Type | Notes |
|---|---|---|
| `x`, `y` | `int` | derived: first fire's tile, else first shelter's, else map-center-nearest passable |

Reducer: `MeetingPlace = &Point{X,Y}`. Emitted at most once per world (guard:
`MeetingPlace == nil`).

### `meeting.convened`

| Field | Type | Notes |
|---|---|---|
| `x`, `y` | `int` | the place (denormalized for narration) |

Reducer: `Meeting.Phase = "convening"`. Executor behavior while convening/open: pin
awake, living, non-exiled agents' intents to the place (goal `attend_meeting`,
source `"meeting"`).

### `meeting.opened`

| Field | Type | Notes |
|---|---|---|
| `attendees` | `[]int` | living, awake, within `meetingRadius` of place, not exiled |

Reducer: `Phase="open"`, `OpenedTick=e.Tick`, `Attendees=payload`, `NextSpeaker=0`,
`LastMeetingDay=DayIndex(e.Tick)`. Empty attendance is legal (everyone asleep/dead):
the meeting opens and the next beat closes it.

### `meeting.turn_taken`

| Field | Type | Notes |
|---|---|---|
| `agent` | `int` | the speaker |
| `raised` | `string` | grievance note when no proposal fires (template from loudest negative memory); "" when a proposal follows |

Reducer: `NextSpeaker++`. Companion (same beat, executor-emitted): a low-salience
personal memory for the speaker ("You spoke at the meeting…").

### `meeting.proposal_tabled`

| Field | Type | Notes |
|---|---|---|
| `proposal_id` | `int` | from `NextProposalID` (reducer increments) |
| `kind` | `string` | `add_curfew` \| `add_repay_debts` \| `amend` \| `repeal` \| `exile` |
| `norm_id` | `int` | amend/repeal: the norm addressed; 0 otherwise |
| `target` | `int` | exile: the named villager; −1 otherwise |
| `param` | `int` | curfew startSecond (add: `nightStartSecond`; amend: +7200 over current) |
| `proposer` | `int` | agent index |
| `text` | `string` | deterministic template text (the floor) |

Reducer: `NextProposalID++` (proposal itself is not stored state — it resolves in the
same beat; the log row is the record).

### `meeting.proposal_resolved`

| Field | Type | Notes |
|---|---|---|
| `proposal_id` | `int` | matches the tabled event |
| `kind`,`norm_id`,`target`,`param`,`proposer`,`text` | | denormalized from the tabled proposal (reducer needs no cross-event lookup — replay-total) |
| `yeas`, `nays` | `[]int` | voter indices by position (exile target excluded from both) |
| `passed` | `bool` | strict majority of eligible attendees; ties fail |

Reducer, when `passed`:
- `add_*`: append `Norm{ID: NextNormID++, Kind, Target, Param, Text, Proposer, DayPassed, Tally, Active: true}`.
- `amend`: update norm's `Param`/`Text`, set `Amended=true` (no-op if norm missing/inactive).
- `repeal`: `Active=false`, `DayRepealed=DayIndex` (no-op if already inactive).
- `exile`: as `add_*` with `Kind:"exile"`, `Target` (no-op if target dead).

Always (passed or failed): pairwise voter edge deltas applied reducer-internally —
aligned pairs `+meetingAlignAffection`, opposed pairs `−meetingOpposeTrust`
(both directions; the `social.gave` internal-effect precedent). Companion events
(same beat, executor-emitted): one toned `agent.memory_added` per attendee about the
proposer (subject-tagged — gossip-seed shape).

### `meeting.proposal_rephrased` (INJECTED — the only whitelisted governance type)

| Field | Type | Notes |
|---|---|---|
| `proposal_id` | `int` | must reference a proposal already resolved this world |
| `norm_id` | `int` | the enacted norm to re-text; 0 if the proposal failed (then only the log/chronicle benefit) |
| `text` | `string` | proposer-voiced phrasing, ≤ `normTextMax` (280 chars) |

Reducer: if `norm_id` names an existing norm, replace `Text` (flavor only — Kind,
Param, Target, votes untouched). Dry-run validation rejects: unknown norm, empty or
oversized text. Injection failure or model absence leaves template text standing.

### `meeting.closed`

| Field | Type | Notes |
|---|---|---|
| `proposals` | `int` | count tabled this meeting (narration) |
| `graced` | `bool` | whether grace extended past the timebox |

Reducer: `Phase=""`, `OpenedTick=0`, `Attendees=nil`, `NextSpeaker=0`. Executor stops
pinning; mind resumes planning for ex-attendees.

### `norm.violated`

| Field | Type | Notes |
|---|---|---|
| `norm_id` | `int` | the norm breached |
| `violator` | `int` | agent index |
| `witnesses` | `[]int` | non-empty by contract (unwitnessed breaches emit nothing) |

Reducer: append `{violator, tick}` to the norm's bounded `Violations` ring; apply
witness→violator edge deltas internally (`−normViolationTrust`,
`−normViolationAffection`). Companion events (same beat): one toned, subject-tagged
`agent.memory_added` per witness ("X broke the village's law: …") — `TellableFor`
picks these up as rumor seeds with zero new rumor machinery.

## Determinism notes

- Every emitter beat is a pure function of (state, map, tick); no wall-clock, no RNG
  (no roll is needed anywhere in v1 governance — ties and orderings are structural).
- The one injected type carries flavor text only; outcomes are identical with the
  model on, off, or failing (SC-005, SC-007).
- Replay: reducer arms are total; denormalized `proposal_resolved` payloads mean no
  reducer ever needs to look back at a prior event.
