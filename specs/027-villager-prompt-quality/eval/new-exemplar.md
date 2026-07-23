# Eval record — variant `new-exemplar`

Produced by `scripts/eval-prompt-73.sh new-exemplar 0310442` (research D1–D3).

| field | value |
|-------|-------|
| variant | `new-exemplar` |
| git_ref | `0310442` |
| git_sha | `0310442d6dda4ca12bb0a06aa387e1f37e9eb5d0` |
| seed | 4242 |
| game_hours | 8 (target tick 28800; final tick 28898) |
| provider/model | ollama `cogito:3b` @ `http://localhost:11434/v1` |
| speed | 16x |
| planner_tool_calls (denominator) | 982 |
| rejected_malformed | 147 (14.97%) |
| rejected_cardinality | 0 (0.00%) |

## Verdict tally (villager planner jobs only)

| verdict | count |
|---------|-------|
| landed | 380 |
| read_error | 20 |
| read_ok | 160 |
| rejected_gate | 271 |
| rejected_malformed | 147 |
| unlanded | 4 |

## Acting-tool selection distribution (landed calls)

Landed acting calls: 380

| tool | count | share |
|------|-------|-------|
| muse | 74 | 19.47% |
| write_journal_entry | 68 | 17.89% |
| talk_to | 46 | 12.1% |
| quarry | 30 | 7.89% |
| forage | 28 | 7.36% |
| wander | 26 | 6.84% |
| cook | 21 | 5.52% |
| chop | 15 | 3.94% |
| eat | 15 | 3.94% |
| goto_warmth | 15 | 3.94% |
| sleep | 10 | 2.63% |
| craft_planks | 8 | 2.1% |
| craft_stone | 5 | 1.31% |
| set_plan | 5 | 1.31% |
| drop | 4 | 1.05% |
| hunt | 3 | 0.78% |
| build_shelter | 2 | 0.52% |
| refuel_fire | 2 | 0.52% |
| build_chest | 1 | 0.26% |
| collect_water | 1 | 0.26% |
| craft_spear | 1 | 0.26% |

## Token counts (research D4)

Rendered `systemPrompt` for the fixed sample agent (name "Ash" + its authored
persona) at ref `0310442`; approx tokens = `len(bytes)/4` (research D4).

| metric | value |
|--------|-------|
| prompt_bytes | 1005 |
| prompt_words | 174 |
| prompt_tokens_approx | 251 |

