# Quickstart: validating the social fabric

## Automated
```sh
go test -race ./...   # edges/ledger/reputation/rumor-chain units, convo driver vs
                      # scripted mocks, determinism + replay with social timelines
```

## Scenario A — edges and debts (US1+US2)
```sh
promptworld new /tmp/s1 --seed 42 && promptworld start /tmp/s1
promptworld speed /tmp/s1 16x
promptworld tail /tmp/s1 --follow | grep -E "social\.(gave|debt|relation|promise)"
# gives to starving neighbors open debts; repayments settle; check soul.md Bonds
cat /tmp/s1/agents/ash/soul.md
```

## Scenario B — conversations and rumors (US3+US4, needs Ollama)
```sh
# llm.json local.model: gemma4:12b-mlx (always-on)
promptworld tail /tmp/s1 --follow | grep -E "conversation|rumor_told"
# expect multi-turn exchanges (≤3/side), gists, and paraphrased retellings
```

## Scenario C — model-free world stays whole
Delete llm.json, restart: talks still bump affection, gives still bind debts,
verbatim rumor fallback still spreads; zero conversation events.

## What "done" looks like
AC#1 ← edge tests + rumor provenance chain; AC#2 ← ledger lifecycle tests;
AC#3 ← convo cap + dual gist memories (mock + live).
