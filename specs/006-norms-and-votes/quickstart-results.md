# Quickstart results — Norms and Votes (TASK-13)

**Run**: 2026-07-20, seed 13, model-free (llm.json removed — the SC-007
degraded-mode posture), `speed max` (~109k ticks/s), ~19 game days in under a
minute of wall clock. Binary built from this branch; world driven via the
`scriptworld` CLI; evidence queried from `world.db` with sqlite3.

## §1–3 Test suites

- `go test ./... -race -count=1` — green across every package (sim 15+
  governance tests, mind driver/prompt/narration, scribe charter render +
  reconstruction, llm routing, e2e).
- Determinism e2e extended past tick 25000: two same-seed daemons produce
  byte-identical histories through the full day-1 governance cycle.

## §4 Live acceptance

**AC#2 — convening.** 18/18 meetings opened with `attendees: [0..7]` — full
attendance every day. Exactly 144 `agent.intent_set` events with source
`"meeting"` (8 villagers × 18 days): everyone broke routine and walked to the
meeting place, (24, 27), designated once on day 1 and never moved.

**AC#4 — once per day, noon, timeboxed.** 18 `meeting.opened` events on 18
distinct game days, every one at second-of-day 43200 (noon exactly). Every
meeting closed at opened+2881 ticks — each attendee spoke, then the agenda-done
early close fired, well inside the 3600+900 timebox+grace.

**AC#1 — propose, vote, constrain.** 7 organic proposals, zero authored input:

| Day | Proposer | Kind | Tally | Story |
|---|---|---|---|---|
| 4 | Ash | add_curfew | 8-0 | gru contact on an earlier night → "the night hunts us" |
| 6 | Fern | amend | 8-0 | Fern, caught twice, pushes the curfew 2h later |
| 7 | Fern | repeal | 8-0 | still getting caught → strike it entirely |
| 7 | Oak | add_curfew | **7-1** | re-tabled the same meeting; **Fern casts the run's only nay** |
| 13 | Hazel | amend | 8-0 | the cycle repeats on norm 2 |
| 14 | Hazel | repeal | 8-0 | |
| 17 | Sage | add_curfew | 8-0 | third enactment |

The self-interested-legislator loop (violate → amend → repeal → someone else
re-enacts → violator votes nay) emerged exactly as the fodder rules and vote
function intend — the lone nay in 60 recorded votes is the repeat violator
voting against the law that keeps catching them.

Constraint teeth: 14 `norm.violated` events, all witnessed (empty-witness
breaches emit nothing by construction); each landed subject-tagged, toned
witness memories ("Cedar broke the village's law: …", salience 6, tone −40 —
`TellableFor`-eligible gossip seeds) plus witness→violator edge penalties.

**AC#3 — the charter remembers.** `village_charter.md` after the run:

- Rules in force: Sage's day-17 curfew (8-0) with live violation count.
- Repealed: Ash's day-4 curfew (repealed day 7) and Oak's day-7 curfew
  (repealed day 14), struck through with full provenance.
- Metatron's `charter.md` untouched beside it.

**Restart survival (FR-007).** Deleted `village_charter.md`, restarted the
daemon: state recovered from snapshot+log at day 18, the charter re-rendered
on scribe start, and the world kept legislating — the day-19 meeting amended
Sage's curfew post-restart ("amended day 19" in the re-rendered file).

**SC-007 — degraded mode.** The entire run above happened with no LLM
configured: convening, turns, proposals (template text), votes, enactment,
amendment, repeal, violations, charter — governance never stalls on
inference. Model-on phrasing flavor is covered by the mocked driver tests
(`internal/mind/meeting_test.go`); the injected `meeting.proposal_rephrased`
path applies cleanly against real reducer state there.

## Verdict

All four human ACs demonstrated live on a real world; all success criteria
with test-level proofs green. Exile (US5) did not trigger organically in 19
days — the −600 mean-hostility gate is deliberately the valve of last resort —
and is proven by `TestExileVoteAndShun` / `TestExileVoteScoreInversion`
end-to-end against real reducer state.
