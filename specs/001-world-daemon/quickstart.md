# Quickstart: validating the world daemon end-to-end

Runnable proof that the substrate meets the spec. Prerequisites: Go 1.22+, macOS or
Linux, repo root.

## Build & unit/integration tests

```sh
go build ./...
go test ./...        # unit + integration (includes determinism harness)
go test ./e2e/...    # builds the binary; full lifecycle scenarios
```

Expected: all green. The e2e suite is the automated form of the scenarios below.

## Scenario A — always-on world (US1, SC-001/SC-002)

```sh
promptworld new /tmp/w1 --name demo --seed 42
promptworld start /tmp/w1            # detached daemon
promptworld status /tmp/w1           # tick advancing, speed 4x
sleep 60
promptworld status /tmp/w1           # ~4 game-minutes later (4x: 60s real ≈ 240 game-s)
promptworld attach /tmp/w1           # header + live event stream; Ctrl-C to detach
promptworld status /tmp/w1           # still running — detach ≠ pause
```

## Scenario B — time is a dial (US2, SC-004/SC-005)

```sh
promptworld pause /tmp/w1            # paused:true
promptworld status /tmp/w1; sleep 30; promptworld status /tmp/w1   # tick unchanged
promptworld resume /tmp/w1
promptworld speed /tmp/w1 1x         # 1 game-s per real-s
promptworld speed /tmp/w1 max        # as fast as sustainable; watch effective_rate
promptworld speed /tmp/w1 4x
```

## Scenario C — kill and lossless resume (US3, SC-003)

```sh
promptworld tail /tmp/w1 --since 0 | tail -3     # note last seq
kill -9 "$(cat /tmp/w1/daemon.pid)"              # crash, no goodbye
promptworld start /tmp/w1                        # recovers snapshot+log, < 10 s
promptworld status /tmp/w1                       # clock continuous, same speed/pause state
promptworld tail /tmp/w1 --since 0 | tail -3     # history intact, seq contiguous
```

## Scenario D — determinism (SC-006)

```sh
promptworld new /tmp/da --seed 7 && promptworld new /tmp/db --seed 7
# e2e harness drives both at max speed for N ticks with an identical command timeline,
# then compares histories:
sqlite3 /tmp/da/world.db "SELECT seq,tick,type,payload FROM events" > /tmp/a.txt
sqlite3 /tmp/db/world.db "SELECT seq,tick,type,payload FROM events" > /tmp/b.txt
diff /tmp/a.txt /tmp/b.txt && echo DETERMINISTIC   # byte-identical
```

(`wall_time` is deliberately excluded — it is observability metadata, not sim state.)

## Scenario E — separable runs (FR-009)

```sh
promptworld stop /tmp/w1
cp -R /tmp/w1 /tmp/w1-archive
promptworld start /tmp/w1-archive    # the copy is a complete, runnable world
```

## What "done" looks like

Every scenario passes; `go test ./...` and `./e2e/...` green; Backlog TASK-2 ACs
check off against Scenarios A (AC#1), B (AC#2), and C (AC#3).
