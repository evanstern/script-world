# Quickstart Validation: Hail Protocol

## Prerequisites

- Go 1.26 toolchain (`export PATH="/opt/homebrew/bin:$PATH"` on this machine)
- A local-tier LLM config for the live measurement (gemma4:12b-mlx per project
  defaults) — the unit/replay validation needs no model at all

## Build & unit validation (no LLM)

```sh
go build ./... && go vet ./...
go test ./internal/sim/ -run 'Hail|Replay|Determinism' -v
go test ./...
```

Expected: all green. The hail tests must cover, at minimum:

1. **Relaxed landing** — talk_to landing with target at distance 35 (beyond
   presentRadius 16, inside hailRadius): lands with outcome `adapted`,
   `social.hailed` emitted, target `Hail` set (spec US1-1).
2. **Pause behavior** — hailed target does not move across ticks; needs still
   decay; intent/plan fields byte-identical through pause and after
   `social.hail_expired` (US2, SC-003).
3. **Met** — hailer walks adjacent before expiry: `social.hail_met` +
   `agent.talked` founded despite fresh `LastTalk` (cooldown bypass), hail cleared
   (US1-2, FR-006).
4. **Exemptions** — asleep / dead / meeting-pinned / already-hailed / active-hailer
   targets are not hailed; out-of-radius landings against them reject as before
   (US3, FR-009).
5. **Mutual hail** — A hails B, then B's talk_to(A) lands: no mutual freeze, both
   land, pair meets (edge case D6).
6. **Replay determinism** — event log containing hailed/met/expired replays to the
   identical state hash (SC-004); snapshot taken mid-pause round-trips `Hail`
   (FR-010).

## Live validation (SC-001 / SC-002 — before/after measurement)

Baseline is already recorded on TASK-47 (myworld-01 shape: 1 conversation in
~75 min; rejections at distances 35–50). After:

```sh
# run a fresh world on the local tier at 8x for a comparable wall-clock window
promptworld new hail-test && promptworld start hail-test --llm
promptworld speed hail-test 8x
# ... let it run ~60-75 min ...
promptworld tail hail-test   # hails visible: "social.hailed {from:"Hazel", to:"Rowan", ...}"
```

Then count from the event log (world dir SQLite):

```sh
sqlite3 <world>/events.db "SELECT count(*) FROM events WHERE type='agent.intent_rejected' AND payload LIKE '%is gone%';"
sqlite3 <world>/events.db "SELECT count(*) FROM events WHERE type='social.conversation';"
sqlite3 <world>/events.db "SELECT type, count(*) FROM events WHERE type LIKE 'social.hail%' GROUP BY type;"
```

Expected outcomes:

- "is gone" rejections down ≥70% vs the baseline window (SC-001)
- conversations founded up vs 1-per-75-min baseline (SC-002)
- `social.hailed` > 0 with a mix of `hail_met` and `hail_expired` (FR-008)
- zero orchestrator requests attributable to hail events (hails appear with no
  paired `cog.thought` of their own — SC-005)

Record both counts and the window on TASK-47 (`backlog task edit TASK-47
--append-notes ...`).

## References

- Event payloads and reducer semantics: [contracts/events.md](contracts/events.md)
- State shape and lifecycle: [data-model.md](data-model.md)
- Decision log: [research.md](research.md)
