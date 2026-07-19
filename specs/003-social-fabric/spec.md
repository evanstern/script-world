# Feature Specification: Social Fabric

**Feature Branch**: `task-8-social-fabric`

**Created**: 2026-07-19

**Status**: Draft

**Input**: User description: "The conflict engine: relationship graph (trust/affection/debt edges read+written by all social acts); rumor objects (content/source/confidence, mutate on retell via cheap paraphrase, provenance tracked); promises/debts ledger with computed reputation; one seeded secret per persona; agent-to-agent conversations capped at ~5 turns each way. Grounding: grounded-assumptions.md (The world, Agent mind). Spec candidate #4, linked to Backlog TASK-8."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Villagers have opinions of each other (Priority: P1)

Every social act — a chat by the fire, a gift of food to a starving neighbor, a broken
promise, an ugly rumor — moves how one villager feels about another: directed trust
and affection that persist, color future planner decisions, and are visible in each
soul. Relationships are the substrate every other social system writes into.

**Why this priority**: the grounding session named the relationship graph the edge
store "read+written by all social acts" — rumors, debts, and conversations all need
it to matter. Without edges, the rest is decoration.

**Independent Test**: drive deterministic social acts (talks, gives, a broken
promise) and observe directed edge values move per the rules; open a soul.md and see
the bonds section reflect them.

**Acceptance Scenarios**:

1. **Given** two villagers who talk, **Then** both gain a little affection for each
   other, recorded as events and visible in state.
2. **Given** a villager who gives food to a starving neighbor, **Then** the receiver
   gains trust and affection toward the giver (and a debt — Story 2).
3. **Given** a promise broken, **Then** the creditor's trust in the debtor drops
   sharply; edges are bounded (no runaway values) and replayable from the log alone.

---

### User Story 2 - Debts bind and reputations follow (Priority: P1)

Giving creates owing. A villager who accepts food when starving owes one back within
two days; repaying settles the debt (kept), letting it lapse breaks it. The ledger
persists every promise's lifecycle, and a computed reputation — respected, mixed,
poor — follows each villager around, feeding their neighbors' judgments and the
planner's context.

**Why this priority**: promises/debts are the concrete, testable spine of "pressure,
not war" — the first mechanical source of legitimate grievance, and AC#2 verbatim.

**Independent Test**: force a give (starving neighbor), watch the debt open; repay →
settled and reputation intact; let one lapse → broken, reputation drops, trust
penalty lands.

**Acceptance Scenarios**:

1. **Given** a give to a starving villager, **Then** an open debt (food, due +2 game
   days) appears in the ledger.
2. **Given** the debtor gives food back to the creditor while the debt is open,
   **Then** the debt settles as kept and reputation is unharmed.
3. **Given** a debt past due, **Then** it is marked broken permanently (the ledger
   never forgets), the debtor's computed reputation drops, and the creditor's trust
   takes the penalty.

---

### User Story 3 - Words travel and warp (Priority: P2)

Villagers gossip. A memorable happening about someone becomes tellable; told once, it
becomes a rumor with a subject, a source, and a confidence; retold, it mutates (a
cheap paraphrase in the reteller's voice) and its confidence decays per hop, while
provenance — who heard it from whom, when — stays traceable end to end. Each persona
also carries one seeded secret from genesis: a self-rumor that only spreads if its
owner lets it slip to someone deeply trusted, after which it travels like any rumor.

**Why this priority**: rumors are the signature Rumor-Mill system, but they need
Stories 1–2 (edges to move, acts worth gossiping about) before spreading matters.

**Acceptance Scenarios**:

1. **Given** A tells B a rumor and B tells C, **Then** C's version records B as its
   source and B's records A; confidence decays hop by hop; the origin marks itself.
2. **Given** a retelling with the language model available, **Then** the retold text
   is a paraphrase recorded in the event (and the original survives with the
   originator); without a model, the text passes verbatim (confidence still decays).
3. **Given** genesis, **Then** every persona holds exactly one seeded secret; it
   never spreads below the trust threshold, and above it, it spreads with provenance
   like any rumor.
4. **Given** a villager hears a rumor about a third party, **Then** their affection
   toward the subject shifts with the rumor's tone.

---

### User Story 4 - Real conversations (Priority: P2)

When two villagers meet and chat, the moment can become an actual exchange: a short
model-driven dialogue in both voices — at most a handful of turns each — that ends
with both remembering the gist, their relationship nudged by the conversation's tone,
and at most one rumor passed along inside it. When no model is available, the old
one-beat talk still happens; conversation is an enrichment, never a dependency.

**Why this priority**: conversations are where personas audibly collide — the payoff
layer over Stories 1–3.

**Acceptance Scenarios**:

1. **Given** a conversation, **Then** it runs at most the configured cap (≤5 turns
   each way), each utterance recorded, and ends with a summary event.
2. **Given** a completed conversation, **Then** BOTH participants' souls carry a
   memory of it (with the gist), and their mutual edges move with the model-judged
   tone.
3. **Given** an unusable or absent model, **Then** no conversation events appear —
   the primitive talk (and its small affection bump) still happens.
4. **Given** any replay of the log, **Then** conversations reproduce exactly with no
   model calls (all conversation content entered as recorded events).

---

### Edge Cases

- **Simultaneous conversations**: at most one model-driven conversation runs at a
  time; other encounters that tick fall back to primitive talks.
- **Death mid-anything**: dead villagers hold their edges and ledger rows frozen;
  rumors about the dead keep circulating (that's a feature).
- **Nothing tellable**: a conversation with no eligible rumor simply passes none.
- **Rumors about the listener**: never told to their own subject in v1 (confrontation
  mechanics are future work).
- **Self-dealing**: no self-edges, no self-debts, no telling yourself rumors.
- **Bounded everything**: edges clamp; confidence floors at a minimum where the rumor
  dies (no longer tellable); the ledger only ever appends states, never deletes.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST maintain directed relationship edges (trust, affection,
  both bounded) between villagers, created lazily, persisted via events, and
  replayable from the log alone.
- **FR-002**: Deterministic social acts MUST move edges by fixed rules: talking (small
  mutual affection), giving (receiver trust+affection toward giver), promise broken
  (sharp trust penalty creditor→debtor), rumor heard (listener affection toward
  subject shifts by tone).
- **FR-003**: A villager adjacent to a starving neighbor while carrying spare food
  MUST give one (reflex and planner-eligible act), transferring the item and opening
  a ledger debt (due +2 game days).
- **FR-004**: The ledger MUST record every debt's full lifecycle (open → kept |
  broken) permanently; repayment (a matching give back while open) settles as kept;
  the hourly due-check breaks overdue debts.
- **FR-005**: Reputation MUST be computed from the ledger (never stored): kept raises
  it, broken lowers it harder; exposed to planner prompts and souls.
- **FR-006**: Memorable happenings about other villagers MUST be tellable; a first
  telling births a rumor (subject, source, tone, confidence); every telling records
  teller, listener, text, and confidence in an event.
- **FR-007**: Retellings MUST decay confidence per hop deterministically and SHOULD
  mutate the text via a model paraphrase (recorded in the event); verbatim text is
  the deterministic fallback. Provenance (heard-from chain with ticks) MUST be
  reconstructible per holder.
- **FR-008**: Each persona MUST carry exactly one authored secret, seeded as events at
  world creation; secrets MUST NOT spread below a high trust threshold from the owner,
  and once shared, spread as rumors with provenance.
- **FR-009**: Adjacency encounters MAY escalate to a model-driven conversation
  (local tier, one at a time): alternating utterances capped per side, each recorded
  as an event, closed by a summary event carrying the gist and model-judged tones;
  effects (edge deltas, the gist memory for both, at most one rumor told) enter the
  simulation ONLY as recorded events through the loop's injection door.
- **FR-010**: Without a usable model, the primitive talk behavior MUST remain intact
  and complete (conversation is additive); replay MUST reproduce all social state
  with zero model calls.
- **FR-011**: Planner prompts MUST include a compact social context: strongest bonds
  and grudges, open debts, computed reputation, and the villager's most confident
  rumor.
- **FR-012**: soul.md MUST render a bonds section (top relations, open debts,
  reputation) alongside memories.

### Key Entities

- **Relation edge**: directed (from, to) → trust, affection (bounded ints).
- **Debt**: id, debtor, creditor, kind, due, status (open/kept/broken) — append-only
  lifecycle.
- **Rumor**: registry identity — id, subject, tone, secret flag, origin.
- **Known rumor (variant)**: per-holder — rumor id, their text, confidence,
  heard-from, heard-tick. The provenance chain is the heard-from links.
- **Secret**: an authored self-rumor seeded at genesis, gated by trust.
- **Conversation**: a bounded exchange — turns, gist, tones — recorded wholly as
  events.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After a simulated day with social acts, edges exist and match the fixed
  rules exactly (unit-verified), and replay reproduces them byte-for-byte.
- **SC-002**: A forced give→lapse cycle shows: open debt, broken at due+1 hour check,
  reputation drop, trust penalty — all from the log alone.
- **SC-003**: A three-hop retelling chain shows per-holder provenance, monotonic
  confidence decay, and (with a model) distinct per-hop texts recorded in events.
- **SC-004**: No conversation exceeds the per-side cap; every completed conversation
  leaves exactly one gist memory in each participant's soul; model-free worlds show
  zero conversation events and unchanged talk behavior.
- **SC-005**: Determinism and replay harnesses pass with social timelines (including
  injected conversation outcomes) — state-hash equality, zero model calls.
- **SC-006**: All conversation/paraphrase traffic stays on the local tier.

## Assumptions

- **Norms/votes are TASK-13**; confrontation mechanics, rumor-denial, and alliance
  formation are out of scope.
- **Promises are debt-shaped in v1**: the give→owe→repay/lapse loop. Free-form
  model-negotiated promises arrive after the chronicle can narrate them.
- **Conversation cap defaults to 3 turns each way** (within the grounding's "~5 cap");
  one conversation at a time serializes local-tier load.
- **Rumor birth is deterministic** (salient memories about others); model creativity
  enters only through paraphrase and conversation text — always as recorded events.
- **Secrets are authored in-repo** alongside personas.
