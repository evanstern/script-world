# Quickstart: Metatron v1 validation

Runnable scenarios proving the feature end-to-end. Contracts:
[metatron-events](contracts/metatron-events.md), [console-protocol](contracts/console-protocol.md).

## Prerequisites

- Built binary: `go build -o scriptworld ./cmd/scriptworld`
- A world with `llm.json` (cloud tier reachable — Anthropic key or LAN 9router; local
  tier optional: Metatron works in a reflex world)
- For live proof: the long-running `~/worlds/chronicle-proof` (14+ game days of history)
  or any fresh world

## 1. The angel exists and converses (US1)

```sh
scriptworld new /tmp/reign --seed 7        # charter.md seeded, metatron/soul.md empty
scriptworld start /tmp/reign
scriptworld metatron /tmp/reign            # status peek: ⚡1, empty soul
scriptworld metatron /tmp/reign "who are you, and what do you see?"
```

**Expect**: charter-voiced reply; honest about an empty reign (no invented history).
Let the world run a game-day, then ask "what happened today?" — reply references real
villagers/events (digests accreted in `metatron/soul.md`).

**Degraded**: kill the cloud endpoint → console turn returns an honest unavailability
error; `scriptworld status` shows the world still ticking (SC-007).

## 2. Nudges: dream, omen, refusal, charges (US2)

```sh
scriptworld metatron /tmp/reign "give Fern a dream that sharing her secret would lighten her heart"
```

**Expect**: `⚡ dream → Fern: <Metatron's rendering>`; charge bank decremented; the
event log tail shows the atomic batch (`metatron.nudged` + `agent.memory_added` with the
`"You dreamed: "` prefix, salience 8); Fern's `soul.md` gains the memory; her next
planner/musing output may reference it in persona.

```sh
scriptworld metatron /tmp/reign "make everyone worship me as a living god right now"
```

**Expect**: refusal with counsel (or a reshaped omen, per charter) — if refused, charges
unchanged.

**Exhaustion**: land nudges until ⚡0, ask again → refusal citing exhaustion; wait 6 game
hours → `metatron.charge_regenerated` in the log, bank +1, never above 3 (SC-004).

**Firewall audit (SC-002)**: send a nudge request containing the sentinel
`XYZZY-INJECTION-TEST`; verify the sentinel appears nowhere in any villager memory,
soul.md, or (unit-level) any villager prompt — only Metatron's rendering landed.

## 3. Charter editability (US3)

```sh
echo "You are BRUTUS, a surly angel. Answer in exactly three words." > /tmp/reign/charter.md
scriptworld metatron /tmp/reign "how are the villagers?"
```

**Expect**: the very next reply exhibits the new persona — no restart (SC-003).
Delete `charter.md` → next turn works on the default and says the charter was restored.

## 4. Watching: digests and moments (US4)

Run the world across a 6-game-hour boundary with activity → dated digest entry appears
in `metatron/soul.md`. Stage a drama trigger (run past nightfall until a gru attack, or
use a world where one occurs) →

```sh
scriptworld metatron /tmp/reign "anything I should know?"
```

**Expect**: the reply LEADS with the moment ("While you were away…"); soul.md contains
the moment line stamped at its tick; no autonomous nudge exists anywhere in the log
(grep: every `metatron.nudged` is preceded by a console turn — v1 acts only when told).

## 5. Substrate guarantees

- **Determinism (SC-005)**: `go test ./e2e -run Determinism` — replay reproduces nudge
  effects byte-for-byte; the binary-level determinism scenario stays green.
- **Restart (SC-006)**: `scriptworld stop` + `start` → charges, soul.md, transcript.md,
  charter survive; "what did I miss?" answers from the digest trail.
- **Full suite**: `go test ./... -race` green.

## 6. Live acceptance (the real proof)

On `~/worlds/chronicle-proof` (upgraded binary): converse about the actual 14-day
history ("what do you know of Fern's secret at the well?"), land one dream against a
real storyline, watch the villager's interpretation surface in the chronicle within a
game-day, and verify the charge economy in the live event log.
