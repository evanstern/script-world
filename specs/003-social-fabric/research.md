# Research: Social Fabric

**Phase 0 — decisions with rationale.**

## R1. Edges: directed, lazy, event-bounded

- **Decision**: `Relation{From, To, Trust, Affection}` (ints −1000..1000, clamp in
  reducer), stored as a flat slice created on first touch. One event type
  (`social.relation_changed {a, b, trust_delta, affection_delta, reason}`) moves
  them; every source (talk, give, broken promise, rumor tone, conversation tone)
  emits that one type with a reason string for the chronicle.
- **Rationale**: 8 agents = ≤56 edges; a flat slice keeps canonical JSON
  deterministic (map iteration would not be). Directed matters (Oak loves Fern more
  than Fern loves Oak — that's story). One event type keeps the reducer and replay
  simple; `reason` makes the log legible.
- **Alternatives**: symmetric edges (loses asymmetry drama); computed-only relations
  (edges must persist and accrete — they ARE the memory of the fabric).

## R2. Debts as the v1 promise shape

- **Decision**: `Debt{ID, Debtor, Creditor, Kind, Due, Status}` — born from a give
  to a starving neighbor (`social.gave` + `social.debt_incurred`), settled by a
  matching give-back while open (`social.debt_settled`), broken by the hourly
  due-check (`social.promise_broken`). Reputation = pure function: 500 + 100·kept −
  200·broken, clamped 0..1000. IDs from `State.NextDebtID` assigned in the reducer.
- **Rationale**: gives the ledger a complete, deterministic lifecycle that AC#2 can
  verify end-to-end without any model. Reputation-as-function keeps state honest
  (never drifts from its own ledger).
- **Alternatives**: LLM-negotiated promises (unverifiable v1; revisit once the
  chronicle narrates); stored reputation (drift risk, double bookkeeping).

## R3. Rumors: registry identity + per-holder variants

- **Decision**: `Rumor{ID, Subject, Tone, Secret, OriginAgent, OriginTick}` in a
  global registry; each holder carries `KnownRumor{RumorID, Text, Confidence,
  From, Tick}`. First telling births the rumor (reducer assigns `NextRumorID`);
  every telling is `social.rumor_told {from, to, rumor_id(0=new), subject, tone,
  text, confidence}`. Confidence decays ×0.8 per hop (integer math: ×4/5); floor 25
  kills tellability. Provenance = the per-holder `From` chain. Hearing shifts
  listener→subject affection by tone/4.
- **Rationale**: identity-plus-variants is the minimal shape that satisfies
  "mutate on retell, provenance tracked" — the registry answers "same rumor?",
  variants answer "whose version?". Deterministic birth (from salient memories about
  others) keeps model-free worlds gossiping verbatim.
- **Alternatives**: variants as first-class rumors (provenance becomes a forest,
  "same rumor" unanswerable); global single text (loses mutation entirely).

## R4. Secrets: authored self-rumors behind a trust gate

- **Decision**: one authored secret per persona (`persona.Secrets`), seeded by
  `scriptworld new` as tick-0 `social.secret_seeded` events → registry rumor
  (`Secret: true`, subject=owner, strongly negative tone) known only to the owner.
  The conversation driver may pass it only when owner→listener trust ≥ 700 (then a
  seeded 1-in-3 chance per eligible conversation); after that it spreads as an
  ordinary rumor.
- **Rationale**: "seeded secrets" from the grounding is exactly this: authored fuel
  that the fabric can detonate later. The trust gate makes leaks earned; the chance
  keeps them rare enough to be events.
- **Alternatives**: LLM decides to confess (uncontrollable rarity); secrets outside
  the rumor system (would need a parallel spread mechanic for no gain).

## R5. Conversations: bounded exchange, one at a time, injected wholesale

- **Decision**: on an `agent.talked` event (adjacency already proven, cooldown
  already applied), the convo driver — if the slot is free — snapshots an immutable
  context (names, personas, relation summary, memory windows, the teller's best
  tellable) and runs in its own goroutine: alternating utterance calls (≤3 per
  side, `KindConversation`, MaxTokens 128, strict JSON `{"say": "..."}`), then one
  outcome call returning `{gist, tone_a, tone_b, retold}`. Effects are injected as
  ONE `inject_social` batch: turn events, summary, two gist memories, tone edge
  deltas, and (if a tellable existed) the rumor_told with the paraphrased text.
  Any failure anywhere → inject nothing (the primitive talk already happened).
- **Rationale**: all-or-nothing injection keeps partial conversations out of the
  log; the snapshot decouples a ~20-second dialogue from replica freshness; slot=1
  respects local-tier throughput; riding the existing `agent.talked` trigger means
  zero new adjacency logic.
- **Alternatives**: per-turn injection (partial conversations on failure); pausing
  the agents during dialogue (couples wall-time I/O to game time — violates the
  loop's design).

## R6. inject_social: a whitelisted batch door

- **Decision**: new loop command carrying `[]store.Event` restricted to
  {social.relation_changed, social.rumor_told, social.conversation_turn,
  social.conversation, agent.memory_added}; the loop re-stamps ticks to the
  boundary, applies through the reducer, appends, notifies. Anything outside the
  whitelist rejects the whole batch.
- **Rationale**: same recorded-input contract as `inject_intent`, widened for
  multi-event outcomes but fenced so the mind can never smuggle sim-mutating types
  (no injected `agent.died`).
- **Alternatives**: one command per event (loses batch atomicity); reusing
  inject_intent semantics (conversations aren't intents).
