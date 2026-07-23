# Eval record — variant `new`

Produced by `scripts/eval-prompt-73.sh new dba7868` (research D1–D3).

| field | value |
|-------|-------|
| variant | `new` |
| git_ref | `dba7868` |
| git_sha | `dba7868bf119f2f85f16c7bab1925ef0be04031a` |
| seed | 4242 |
| game_hours | 8 (target tick 28800; final tick 28899) |
| provider/model | ollama `cogito:3b` @ `http://localhost:11434/v1` |
| speed | 16x |
| planner_tool_calls (denominator) | 896 |
| rejected_malformed | 103 (11.50%) |
| rejected_cardinality | 0 (0.00%) |

## Verdict tally (villager planner jobs only)

| verdict | count |
|---------|-------|
| landed | 347 |
| read_error | 9 |
| read_ok | 195 |
| rejected_gate | 240 |
| rejected_malformed | 103 |
| unlanded | 2 |

## Acting-tool selection distribution (landed calls)

Landed acting calls: 347

| tool | count | share |
|------|-------|-------|
| muse | 65 | 18.73% |
| write_journal_entry | 62 | 17.86% |
| wander | 54 | 15.56% |
| forage | 41 | 11.81% |
| talk_to | 34 | 9.79% |
| eat | 15 | 4.32% |
| goto_warmth | 15 | 4.32% |
| sleep | 14 | 4.03% |
| cook | 12 | 3.45% |
| quarry | 12 | 3.45% |
| chop | 7 | 2.01% |
| craft_planks | 5 | 1.44% |
| drop | 3 | 0.86% |
| set_plan | 3 | 0.86% |
| refuel_fire | 2 | 0.57% |
| build_fire | 1 | 0.28% |
| craft_stone | 1 | 0.28% |
| hunt | 1 | 0.28% |

## Token counts (research D4)

Rendered `systemPrompt` for the fixed sample agent (name "Ash" + its authored
persona) at ref `dba7868`; approx tokens = `len(bytes)/4` (research D4).

| metric | value |
|--------|-------|
| prompt_bytes | 772 |
| prompt_words | 134 |
| prompt_tokens_approx | 193 |

