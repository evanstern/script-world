# Data Model: Norms and Votes

## World state (event-sourced, in `sim.State`)

All new fields JSON-tagged in fixed order (canonical bytes for hashing); slices never
maps (deterministic JSON). Pre-TASK-13 snapshots unmarshal with zero values — a world
upgrades to "no norms yet, no meeting held" harmlessly.

### New State fields

| Field | Type | Notes |
|---|---|---|
| `MeetingPlace` | `*Point` (`json:"meeting_place,omitempty"`) | nil until first convene; set once by `meeting.place_designated`, never moves |
| `Meeting` | `MeetingState` (`json:"meeting"`) | current assembly lifecycle (zero value = no meeting) |
| `Norms` | `[]Norm` (`json:"norms,omitempty"`) | the law, in enactment order; repealed norms stay (Active=false) — the ledger never forgets |
| `NextNormID` | `int` (`json:"next_norm_id,omitempty"`) | monotonic, `NextXxxID` convention |
| `NextProposalID` | `int` (`json:"next_proposal_id,omitempty"`) | monotonic; failed proposals have IDs too (chronicle references them) |

### MeetingState

| Field | Type | Notes |
|---|---|---|
| `Phase` | `string` | `""` (none) \| `"convening"` \| `"open"` |
| `OpenedTick` | `int64` | tick of `meeting.opened`; 0 outside a meeting |
| `Attendees` | `[]int` | attendance snapshot at open (living, awake, within `meetingRadius`, not exiled) |
| `NextSpeaker` | `int` | index into `Attendees` of the next turn; == len(Attendees) when all have spoken |
| `LastMeetingDay` | `int64` | `DayIndex` of the last opened meeting — the once-per-day guard (consolidation-marker pattern) |

**Invariants**: `Phase=="open"` ⇒ `OpenedTick>0` and `MeetingPlace!=nil`; `Attendees`
unique, living at snapshot time; reducer resets Phase/OpenedTick/Attendees/NextSpeaker
on `meeting.closed`, keeps `LastMeetingDay`.

### Norm

| Field | Type | Notes |
|---|---|---|
| `ID` | `int` | from `NextNormID` |
| `Kind` | `string` | closed vocabulary: `"curfew"` \| `"repay_debts"` \| `"exile"` |
| `Target` | `int` | exile only: the judged villager; −1 otherwise |
| `Param` | `int` | kind-scoped: curfew `startSecond` (second-of-day); 0 otherwise |
| `Text` | `string` | template text, replaced by rephrased text when the flavor event lands; ≤ `normTextMax` (280) |
| `Proposer` | `int` | agent index |
| `DayPassed` | `int64` | `DayIndex` at enactment |
| `Tally` | `string` | e.g. `"5-2"` — display provenance for charter/chronicle |
| `Active` | `bool` | false after repeal (or after amendment supersedes wording — amend edits in place, stays active) |
| `DayRepealed` | `int64` | 0 while active |
| `Amended` | `bool` | curfew may be amended once (`+2h`); guards re-amendment |
| `Violations` | `[]NormViolation` | bounded ring, cap 16 — fodder for amend/repeal rules |

### NormViolation

| Field | Type | Notes |
|---|---|---|
| `Agent` | `int` | violator |
| `Tick` | `int64` | when |

**Validation / invariants** (enforced in reducer, caught by `InjectSocial` dry-run for
the injected type): kinds only from the vocabulary; at most one Active norm per kind
(exile: per target); exile target alive at enactment; `Param` sane for kind; text
capped. Duplicate-of-active proposals are never tabled (fodder rules check first) and
the reducer rejects them as a second line of defense.

### Computed (never stored)

- `ActiveNorms(s) []Norm` — filter, for prompt/charter/detectors.
- `ExiledIndex(s, agent) bool` — any active exile norm targeting agent.
- `ViolationCount(norm, agent) int` — count in the bounded ring.

## State transitions

```
(no meeting) ──executor @ SecondOfDay 41400, DayIndex > LastMeetingDay──▶ convening
     │              meeting.place_designated (first time only) + meeting.convened
     ▼
convening ──executor @ SecondOfDay 43200──▶ open  (meeting.opened{attendees}; LastMeetingDay=DayIndex)
     ▼
open ──turn beat every 360 ticks──▶ meeting.turn_taken{agent, raised}
     │        └─ fodder rule fires ──▶ meeting.proposal_tabled{proposal}
     │                                     └─ same beat ──▶ meeting.proposal_resolved{votes, tally, passed}
     │                                                          └─ reducer: enact/amend/repeal Norm,
     │                                                             pairwise voter edge deltas
     ▼
open ──all spoke, or timebox 3600 (+grace ≤900) ──▶ (closed)  (meeting.closed; Phase="")
```

Norm lifecycle: `enacted (Active) ──amend──▶ enacted (Param/Text updated, Amended=true)
──repeal──▶ inactive (DayRepealed set)`. Exile norms are repealable like any norm
(the village can forgive).

## Event payloads (structs in `internal/sim/governance.go`)

See [contracts/governance-events.md](contracts/governance-events.md) for the full
per-event table (fields, emitter, reducer effect, whitelist status).

## Files (derived, bound to the run)

### `village_charter.md` (save-dir root — distinct from Metatron's `charter.md`)

Scribe-rendered view of `State.Norms` + `MeetingPlace`; dirty-marked by governance
event types, rendered on scribe start. Sections: header (village name, meeting place,
day count), Rules in force (text, proposer, day, tally, amendment note), Standing
judgments (exiles), Repealed (struck through, day span). Regenerable at any time from
state — never authoritative, never hand-edited (the sim overwrites).

## In-memory (mind driver, `internal/mind/meeting.go`)

| Entity | Fields | Notes |
|---|---|---|
| meeting driver state | `phrasingBusy` (single-flight), pending proposal queue (bounded) | convo-driver pattern; absorb-side enqueue, worker-side one `KindMeeting` call, `InjectSocial` one `meeting.proposal_rephrased` |
| planner suppression | attendee set while meeting open | absorb sees `meeting.convened`/`meeting.closed`; the asleep-agent precedent |

## Relationships

```
gru attacks / broken debts / hostility ──fodder rules (pure fns of State)──▶ proposal tabled
                                                                                │
Relation edges (trust/affection) ──vote function (pure int fn)──▶ votes ──strict majority──▶ Norm
                                                                                │
Norm ──prompt "Village law" section──▶ planner context ──persona──▶ obey / skirt / defy
                                                                                │
defiance + witness ──deterministic detectors──▶ norm.violated ──▶ witness memories (gossip-seed,
                                                  TellableFor) + edge penalties + Violations ring
                                                                                │
Violations ring ──amend/repeal fodder──▶ next meeting's proposals   (the loop closes)
```
