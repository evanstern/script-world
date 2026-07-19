# Contract: social event family

All social state changes ride these events; replay needs nothing else.

| Type | Payload | Emitted by | Reducer effect |
|---|---|---|---|
| `social.relation_changed` | `{a, b, trust_delta, affection_delta, reason}` | executor rules; convo outcome (injected) | edge (a→b) clamped add; lazy-create |
| `social.gave` | `{from, to, kind}` | executor (reflex + planner goal `give_food`) | inventory transfer; ledger transition is reducer-internal — settles the oldest matching open debt (kept) or opens a new one (id from NextDebtID, due +2 game days) |
| `social.promise_broken` | `{id}` | executor hourly due-check | status broken |
| `social.rumor_told` | `{from, to, rumor_id, subject, tone, text, confidence, secret}` | executor verbatim fallback; convo driver (paraphrase, injected) | rumor_id 0 → registry birth (NextRumorID); add/update listener's KnownRumor; affection shift to subject |
| `social.secret_seeded` | `{agent, text, tone}` | `scriptworld new` at tick 0 | registry rumor (Secret) + owner's Known |
| `social.conversation_turn` | `{conv, speaker, listener, text}` | convo driver (injected) | none (chronicle) |
| `social.conversation` | `{conv, a, b, gist, turns}` | convo driver (injected) | none (chronicle) |

`inject_social` whitelist: relation_changed, rumor_told, conversation_turn,
conversation, agent.memory_added. Everything else in a batch → whole batch rejected.
Ticks are re-stamped to the boundary by the loop.
