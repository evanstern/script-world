# Quickstart Validation: Conversation Robustness

Prerequisites: repo checkout, Go toolchain, `export PATH="/opt/homebrew/bin:$PATH"`.

## 1. Unit / fault-injection suite (primary gate)

```sh
go test ./internal/mind/ -run 'Convo|Conversation|Parse' -v
```

Expected new/updated tests, all green:
- outcome retry: first summary reply malformed → retry succeeds → scene lands whole,
  terminal `landed` has `retried:true`; a `retried` marker with `raw` was emitted.
- outcome double failure → scene abandons, terminal `unusable` carries retry's `raw`,
  no partial state injected (extends `TestConversationFailureInjectsNothing`).
- utterance retry: one bad say → same speaker retried → scene completes; two
  consecutive bad says → abandon.
- transport error at either site → immediate abandon, zero retries (Submit call count
  asserted on the fake).
- lenient parse: table test feeding the four observed malformed shapes
  (`{"gist": Hazel and Rowan ...}` etc.) → parsed without a model call.
- golden happy path: scripted scene with all-valid replies → emitted event batch
  identical to pre-change expectations (SC-004).
- truncation: >2048-byte raw reply → rune-boundary truncation with marker.

## 2. Whole-package regression

```sh
go vet ./... && go test ./...
```

## 3. Live-world smoke (optional but recommended)

```sh
go build -o promptworld ./cmd/promptworld
./promptworld new /tmp/robustness-smoke --name robustness-smoke
./promptworld start /tmp/robustness-smoke && ./promptworld speed /tmp/robustness-smoke 8x
# let conversations run, then:
sqlite3 "file:/tmp/robustness-smoke/world.db?mode=ro" \
  "SELECT json_extract(payload,'$.outcome'), COUNT(*) FROM events
   WHERE type='cog.outcome' AND json_extract(payload,'$.class')='conversation'
   GROUP BY 1;"
```

Expected: `landed` dominates; any `retried` markers correlate 1:1 with either a
subsequent `landed{retried:true}` or `unusable`; `SELECT ... '$.raw'` returns the
verbatim reply for every parse failure (SC-003, one query).

## 4. MLX probe (FR-008, findings → board)

```sh
sh specs/011-conversation-robustness/probe-mlx-reasoning.sh   # N=10 per config
backlog task edit TASK-42 --append-notes "MLX probe findings: ..."
```

Expected: a recorded answer (honored / ignored; empty-reply rates per setting) on
TASK-42 — AC #5.

## 5. Post-merge

- `spec-bridge:sync` — board catches up to artifacts.
- `/grounding-wiki:wiki-update` — re-pin `agent-mind`, `social-fabric` (+ any note
  listing convo.go/parse.go/telemetry.go as sources).
