# Quickstart: validating the agent mind

## Automated

```sh
go test -race ./...        # sim units (window, memories, determinism, replay),
                           # mind integration vs mock model, persona immutability, e2e
```

## Scenario A — minds steer bodies (US1, SC-001)

```sh
scriptworld new /tmp/m1 --seed 42          # writes personas + llm.json
# point llm.json local.model at a pulled Ollama model, then:
scriptworld start /tmp/m1
scriptworld tail /tmp/m1 --follow | grep -E "thought|intent_set"
# expect agent.thought lines with model reasons and intent_set with "source":"planner"
```

## Scenario B — natures fixed, souls grow (US2, SC-002)

```sh
shasum /tmp/m1/agents/*/persona.md > /tmp/before
ls -l /tmp/m1/agents/ash/persona.md        # -r--r--r--
scriptworld speed /tmp/m1 max; sleep 5; scriptworld speed /tmp/m1 4x
cat /tmp/m1/agents/ash/soul.md             # dated, starred memories
shasum -c /tmp/before                      # all OK after days of sim
```

## Scenario C — degraded mode (US1 case 3, SC-004)

```sh
# stop Ollama (or set llm.json local.endpoint to a dead port and restart)
scriptworld tail /tmp/m1 --follow | grep intent_set
# expect "source":"reflex" intents resuming within ~2 game-minutes of idleness
```

## Scenario D — replay is model-free (SC-005)

Covered by the sim replay harness (state-hash equality over logs containing
planner-injected commands) — no network in the test environment proves no model call.

## What "done" looks like

All suites green; Scenario A shows planner-sourced behavior on cadence and triggers;
B shows the firewall + accretion; C shows the reflex floor; TASK-7 ACs map A→#1,
B→#2, window unit tests→#3.
