# Quickstart: Validating Metatron Agency (spec 029)

Prerequisites: a built daemon (`go build ./...`), a throwaway world
(`promptworld new /tmp/w29 && promptworld start /tmp/w29`), cloud tier configured
(metatron needs it), console via `promptworld metatron /tmp/w29`.

## Gate zero — the suites

```sh
go test ./...            # reducer replay/determinism, toolloop equivalence,
                         # registry Validate/coverage, metatron sentinel audit
```

## Scenario 1 — taxonomy (US1)

1. During game day: ask the angel for a vision to one villager → lands, bank −1,
   `metatron.nudged{form:"vision"}` in the log (`promptworld log /tmp/w29 | tail`).
2. Ask for an omen during day → reply promises nightfall; `metatron.order_placed`
   (origin system) in the log; NO nudged event, bank unchanged.
3. At night (or `time_snap` to 22:30): the deferred omen lands —
   `order_triggered` + `nudged{form:"omen"}`, bank −1, next console reply leads
   with the moment.
4. `promptworld metatron-status /tmp/w29` (IPC status): `granted_tools` shows the
   new roster; no `nudge_dream`/`nudge_omen`.

## Scenario 2 — standing orders (US2/US3)

1. "When Rowan next falls asleep, send her a comforting vision." → reply confirms;
   `order_placed` (origin player, event_types ["agent.slept"], agent Rowan's
   index) in the log; status lists the order.
2. Let the world run until Rowan sleeps → `order_triggered`, then the system
   turn's `cog.tool_call` chain (jobID `watch-metatron-<tick>`), then
   `nudged{form:"vision"}`; transcript shows the `[watch]` exchange; next console
   reply leads with the moment.
3. Restart the daemon mid-watch (`promptworld stop` / `start`) → status still
   lists the order; replay check (`promptworld verify /tmp/w29` or the replay
   test) reproduces state.
4. Place 3 more orders → the 4th is refused with counsel, no `order_placed`.
5. "Cancel the Rowan watch" → `order_cancelled`; slot frees.
6. TTL: place an order with ttl_days 1, `time_snap` forward a day →
   `order_expired` in the log (executor-emitted), moment mentions the lapsed
   watch.

## Scenario 3 — zero-cost watching & fuzzy confirms (US6, SC-001)

1. With one structural order active, let the world run a game-day: the LLM meter
   (`promptworld llm-status /tmp/w29` or meter dump) shows ZERO `metatron_watch`
   calls (structural orders never confirm) and no metatron calls beyond digests.
2. Place a fuzzy order ("when Rowan seems heartbroken…") → `order_placed` has
   `confirm: true` + coarse filter.
3. On filter hits: at most one `metatron_watch` meter entry per 30 game minutes
   per order; a `no` verdict leaves the order active.

## Scenario 4 — meta tools (US5)

1. "Pause the world." → clock stops (`promptworld status`), `clock.paused` in the
   log, bank unchanged.
2. "Start it again at fast speed." → resumes at fast.
3. Write `capabilities.json` with `{"tools": ["send_vision"]}` → next turn/status:
   meta tools absent from `granted_tools`; asking the angel to pause yields
   counsel, no clock change, and no rejected-unknown crash.

## Scenario 5 — honesty (US3, SC-005)

1. Spend the bank to 0; place (or leave) a vision-bearing deferral order; trigger
   it → NO model turn for known-act orders (empty-bank precheck), no spend, one
   queued "strength was spent" moment.
2. Set a tiny daily budget in `llm.json`, exhaust it, trigger an order → one
   honest moment, no retry loop (check the meter: no repeated calls).

## Expected outcomes

Every scenario's proof is durable: the event log, the meter, the transcript, and
the status surface — never chat memory. Contracts: [tools](contracts/tools.md),
[events](contracts/events.md), [routing](contracts/routing.md); entities:
[data-model.md](data-model.md).
