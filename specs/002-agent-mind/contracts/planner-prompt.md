# Contract: planner prompt & goal JSON

One planner call = one `llm.Request{Kind: planner}` through the orchestrator's local
tier. MaxTokens 256.

## System (stable per agent — cache-friendly prefix)

```text
You are <Name>, a villager in a small settlement.

<persona.md content>

You decide what <Name> does next. Reply with ONLY a JSON object:
{"goal": "<goal>", "target": "<agent name, only for talk_to>", "reason": "<one short sentence in your voice>"}
Goals: forage, chop, hunt, build_fire, build_shelter, eat, sleep, wander, goto_warmth, talk_to.
```

## User (variable suffix)

```text
It is day 3, 14:20 (daytime). You are at (12, 41).
Needs (0-100): health 74, food 31, rest 62, warmth 80, morale 55.
Carrying: 2 wood, 0 meals. Village: fire at (11,42); shelter at (11,40).
Nearby: Birch (2 tiles away), forage to the east.

You remember:
- day 3 09:12 (3★) Talked with Birch.
- day 2 22:00 (5★) Survived a freezing night by the fire.
… (at most K lines — the working-memory window, never the whole soul)

What do you do next?
```

## Response contract

The first JSON object found in the reply is parsed; everything else is ignored.

| Field | Required | Semantics |
|---|---|---|
| `goal` | ✅ | one of the vocabulary above; anything else → rejected, no event |
| `target` | only for `talk_to` | agent name, case-insensitive |
| `reason` | ✅ | recorded verbatim as `agent.thought` (chronicle material) |

The loop resolves the goal to concrete coordinates deterministically at the tick
boundary (nearest resource / build site / the target agent's tile). Parse failures
and rejected goals produce **no** sim event; the mind logs and waits for the next
trigger, with the reflex grace as the floor.
