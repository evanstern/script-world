# Quickstart: Norms and Votes — validation guide

How to prove the feature works, from unit tests to a live world. Contracts:
[governance-events.md](contracts/governance-events.md),
[meeting-lifecycle.md](contracts/meeting-lifecycle.md). Data model:
[data-model.md](data-model.md).

## Prerequisites

- Go 1.26 toolchain; repo root.
- For live checks: a runnable `promptworld` binary (`go build ./cmd/promptworld`)
  and, optionally, an `llm.json` in the world dir (phrasing flavor only — every
  scenario below must also pass with no LLM configured).

## 1. Unit / reducer level (fast, deterministic)

```sh
go test ./internal/sim/ -run 'Governance|Meeting|Norm' -race -count=1
go test ./internal/mind/ -run 'Meeting|Prompt' -race -count=1
```

Expected: the test contract in meeting-lifecycle.md holds — full-day meeting
lifecycle via `driveTicks`, vote tables over authored Relations, enact/amend/repeal
reducer effects, witnessed/unwitnessed violation asymmetry, exile exclusion rules,
whitelist accept/reject for `meeting.proposal_rephrased`, planner prompt carries the
"Village law" block, and the meeting driver skips cleanly on a failing Submitter.

## 2. Determinism / replay (SC-005)

```sh
go test ./e2e/ -run Determinism -race -count=1
```

Expected: a governed run (meetings held, norms passed, violations landed) replays
from the event log to an identical state hash with zero model calls; snapshots
containing governance fields round-trip; a pre-TASK-13 snapshot loads with zero
values (no meeting yet, no norms).

## 3. Whole-suite gate

```sh
go test ./... -race -count=1
```

Expected: green, including untouched suites (no regression in social/gru/metatron).

## 4. Live acceptance (a real world, spans a game day)

```sh
go build -o promptworld ./cmd/promptworld
./promptworld new /tmp/norms-world --seed 13
./promptworld start /tmp/norms-world
./promptworld speed /tmp/norms-world 32   # a game day ≈ 45 real minutes at 32x
```

Watch (`./promptworld attach /tmp/norms-world`, chronicle pane) across a noon:

- **AC#2 (convening)**: ~11:30 game time, villagers break routines and converge on
  one spot; chronicle narrates the assembly. Verify `meeting.opened` attendance in
  the event log:
  `sqlite3 /tmp/norms-world/world.db "select tick,type,payload from events where type like 'meeting.%' order by seq"`.
- **AC#1 (propose → vote → constrain)**: given day-1 gru contact or a broken debt,
  a proposal is tabled and resolved at a meeting within 2 game days (SC-003); the
  vote tally and voter positions appear in `meeting.proposal_resolved`; the passed
  norm shows up in a villager's planner prompt (soul/thought lines reference it).
- **AC#3 (charter)**: `cat /tmp/norms-world/village_charter.md` lists the rule with
  proposer/day/tally; after a later amend or repeal vote, the file reflects it.
  Metatron's `charter.md` is unchanged.
- **AC#4 (timebox)**: log tick delta between `meeting.opened` and `meeting.closed`
  is ≤ 4500 ticks (3600 + 900 grace); exactly one meeting per game day.
- **Violations**: after a curfew norm passes, a night wanderer with a witness yields
  `norm.violated`, a witness memory ("broke the village's law"), edge movement
  (soul bonds shift), and eventually a rumor retell in a conversation.
- **Degraded mode (SC-007)**: repeat the noon crossing with `llm.json` removed —
  meeting convenes, proposals table with template text, votes resolve, charter
  updates. Only phrasing flavor is missing.

## 5. Cleanup

```sh
./promptworld stop /tmp/norms-world && rm -rf /tmp/norms-world
```

## Results

Recorded in `quickstart-results.md` after implementation (TASK-12 precedent),
including the live-run evidence for each human AC.
