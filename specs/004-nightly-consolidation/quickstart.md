# Quickstart: Nightly Consolidation + Persona Firewall

Validation guide — proves the feature end-to-end. Run from repo root on
`task-9-consolidation`.

## Prerequisites

- Go 1.22+ (`export PATH="/opt/homebrew/bin:$PATH"`)
- For the live scenario: a cloud tier in llm.json — either `ANTHROPIC_API_KEY` exported,
  or the LAN 9router config (`provider: openai_compat`, endpoint
  `http://192.168.1.92:20128/v1`, model `cc/claude-haiku-4-5-20251001`) — requires the
  TASK-15 config support (PR #8) present in the binary.
- Local tier (Ollama + gemma4:12b-mlx) optional but recommended so planners run too.

## Scenario 1 — unit + integration suite (no network)

```sh
go test ./... -race
```

Expected: green. Specifically covers reducer cases + ledger idempotence + replay
determinism (internal/sim/consolidate_test.go), driver-with-scripted-model + atomicity +
validator fixtures incl. a temperament-drifting output rejected 100% (SC-002)
(internal/mind/consolidate_test.go, validate_test.go).

## Scenario 2 — live overnight run (SC-001, SC-003)

```sh
go build -o /tmp/sw ./cmd/scriptworld
/tmp/sw new ~/worlds/consol-test --seed 11
# put a cloud tier in ~/worlds/consol-test/llm.json (see prerequisites)
/tmp/sw start ~/worlds/consol-test
/tmp/sw speed ~/worlds/consol-test 16        # one game day ≈ 90 real min
```

After ≥ 3 game nights (~4.5 real hours at 16x):

```sh
grep "mind: consolidation" ~/worlds/consol-test/daemon.log
sqlite3 -readonly "file:$HOME/worlds/consol-test/world.db?mode=ro" \
  "SELECT json_extract(payload,'$.agent'), json_extract(payload,'$.night'),
          json_extract(payload,'$.outcome') FROM events WHERE type='agent.consolidated';"
cat ~/worlds/consol-test/agents/*/soul.md
```

Expected:
- exactly one `agent.consolidated` per living agent per night index (SC-001)
- souls contain "Who I am becoming" (narrative, agent voice) and "Beliefs" (confidence +
  provenance) referencing ≥ 2 distinct days (SC-003)
- persona.md files byte-identical to genesis (`md5 agents/*/persona.md` unchanged)

## Scenario 3 — degraded night (SC-005)

Stop the router / unset the key mid-run for one game night, restore it, run one more
night. Expected: nights under outage log `mind: consolidation ... deferred`, no marker
events; the first healthy night digests the multi-day backlog in one call per agent;
world tick never stalls (`/tmp/sw status`).

## Scenario 4 — replay determinism (SC-004)

```sh
/tmp/sw stop ~/worlds/consol-test
go test ./internal/sim/ -race -run Replay      # replay suite includes consolidations
```

Also provable live: restart the daemon (recovery replays the log) — recovered state must
match, with the network unplugged if desired: replay never calls a model.

## Record results

Save observed outputs to `specs/004-nightly-consolidation/quickstart-results.md` — the
spec-bridge Done derivation reads artifacts, not chat.
