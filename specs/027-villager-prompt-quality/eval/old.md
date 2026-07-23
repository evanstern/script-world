# Eval record — variant `old`

Produced by `scripts/eval-prompt-73.sh old origin/main` (research D1–D3).

| field | value |
|-------|-------|
| variant | `old` |
| git_ref | `origin/main` |
| git_sha | `b96c028796c2e4e3264cc8e7c3f6680917b9baca` |
| seed | 4242 |
| game_hours | 8 (target tick 28800; final tick 28893) |
| provider/model | ollama `cogito:3b` @ `http://localhost:11434/v1` |
| speed | 16x |
| planner_tool_calls (denominator) | 789 |
| rejected_malformed | 121 (15.34%) |
| rejected_cardinality | 0 (0.00%) |

## Verdict tally (villager planner jobs only)

| verdict | count |
|---------|-------|
| landed | 320 |
| read_error | 26 |
| read_ok | 141 |
| rejected_gate | 180 |
| rejected_malformed | 121 |
| unlanded | 1 |

## Acting-tool selection distribution (landed calls)

Landed acting calls: 320

| tool | count | share |
|------|-------|-------|
| muse | 110 | 34.37% |
| write_journal_entry | 43 | 13.43% |
| forage | 35 | 10.93% |
| wander | 35 | 10.93% |
| quarry | 21 | 6.56% |
| sleep | 18 | 5.62% |
| talk_to | 18 | 5.62% |
| goto_warmth | 9 | 2.81% |
| cook | 8 | 2.5% |
| eat | 5 | 1.56% |
| craft_stone | 4 | 1.25% |
| set_plan | 4 | 1.25% |
| drop | 3 | 0.93% |
| chop | 2 | 0.62% |
| craft_planks | 2 | 0.62% |
| collect_water | 1 | 0.31% |
| delete_from_journal | 1 | 0.31% |
| hunt | 1 | 0.31% |

## Token counts (research D4)

Rendered `systemPrompt` for the fixed sample agent (name "Ash" + its authored
persona) at ref `origin/main`; approx tokens = `len(bytes)/4` (research D4).

| metric | value |
|--------|-------|
| prompt_bytes | 759 |
| prompt_words | 132 |
| prompt_tokens_approx | 189 |

