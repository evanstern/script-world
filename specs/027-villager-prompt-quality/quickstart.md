# Quickstart: Villager Prompt Quality — running the eval gate

**Feature**: 027-villager-prompt-quality | **Plan**: [plan.md](plan.md)

How to prove the rewrite end-to-end. Details: [research.md](research.md) D1–D4,
[data-model.md](data-model.md) §2 for the record shape.

## Prerequisites

- Ollama running locally with the model the v2 provider registry declares for the
  local planner tier (currently `cogito:3b`) pulled and warm.
- A quiet machine (soaks run serially and are latency-sensitive).
- The task branch checked out in `.worktrees/task-73` with the variant commits in
  place (research D2): `old` = `origin/main`, `new` = rewrite commit,
  `new+exemplar` = exemplar commit.

## 1. Unit gate (fast, deterministic)

```sh
go test ./internal/mind/ ./internal/toolloop/   # frame contract + stub suites
go test ./...                                    # full suite
```

Expected: all pass; `prompt_test.go` covers contract C1–C5
([contracts/system-prompt.md](contracts/system-prompt.md)).

## 2. Token counts (per variant)

```sh
go test ./internal/mind/ -run TestPromptFrameReport -v
```

Expected: prints bytes / words / approx tokens for the sample agent render; the
eval driver captures this per variant into `eval/<variant>.md`.

## 3. Live soak (per variant, serially)

```sh
scripts/eval-prompt-73.sh <variant> <git-ref>
# e.g. scripts/eval-prompt-73.sh old origin/main
```

The driver (research D2/D3): builds `promptworld` from `<git-ref>` into a temp
dir → `new eval73-<variant> --seed 4242` → `start` → `speed` to the highest
sustainable multiplier → waits until world clock passes the 6-game-hour window →
`stop` → `tail --since 0` → tallies villager-planner `cog.tool_call` verdicts and
tool distribution → writes `specs/027-villager-prompt-quality/eval/<variant>.md`.

Expected per variant: ≥200 villager acting decisions (else extend the window for
ALL variants equally and rerun); counts + rates for `rejected_malformed` and
`rejected_cardinality`; per-tool selection shares.

## 4. The gate (SC-001…SC-004)

Compare `eval/old.md` vs `eval/new.md` vs `eval/new-exemplar.md`:

- **Ship** iff both rejection rates for the shipped variant ≤ old's.
- **Exemplar** ships only if it beats (or ties at lower token cost) plain `new`;
  otherwise record the measured reason it was rejected (FR-004).
- **Distribution screen** (SC-003): every acting tool ≥5% share under `old`
  keeps a nonzero share; no tool's share more than doubles — deviations need a
  written acceptance in the eval notes.

## 5. Record and close the loop

- Append the numbers table + exemplar decision to TASK-73
  (`backlog task edit 73 --append-notes ...`), tick ACs as proven.
- Re-pin the wiki: `/grounding-wiki:wiki-update` (`docs/wiki/agent-mind.md`
  sources `internal/mind/prompt.go`).
- One PR from `.worktrees/task-73` (constitution II).
