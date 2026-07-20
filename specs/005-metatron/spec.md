# Feature Specification: Metatron v1 — the editable angel

**Feature Branch**: `task-12-metatron`

**Created**: 2026-07-20

**Status**: Draft

**Input**: User description: "Metatron v1: the editable angel — the player's sole influence channel over the village. A singular long-running gatekeeper agent that watches the world via periodic event-stream digests, keeps its own notes/soul that starts empty, converses with the player in the Metatron console (the primary interface), and mediates all player nudges: player states intent in plain language, Metatron judges persuadability, societal impact, and method, then either refuses with counsel or translates the intent into a Dream (one villager) or an Omen (village-wide) in agent-comprehensible form; raw player text never reaches a villager context. Nudges land as provenance-unknown memories interpreted in persona. Charge economy: 1 charge per nudge, regenerating 1 per 6 game hours, max 3 banked; conversation and counsel are free. Metatron's charter is the game's ONLY player-editable prompt; editing it at any time observably changes behavior. V1 contract: acts only when told — one player prompt = one mediated turn. Also owns the drama-router rule."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Converse with the angel that watches (Priority: P1)

The player opens the Metatron console and talks to their intermediary in plain language. Metatron answers in its charter-defined persona, grounded in what it has actually observed: it maintains periodic digests of the world's events and keeps its own accreting notes (a soul that starts empty). The player can ask what is happening, who is struggling, what a villager believes, or what Metatron would advise — and the answers reflect the real state of the running world, not generic filler. Conversation is free and unlimited.

**Why this priority**: Conversation with Metatron is the primary interface of the game. Every other capability (nudges, counsel, charter effects) is experienced through this channel, and it is independently valuable on its own: an informed companion narrating and interpreting the ambient world.

**Independent Test**: With a running world that has accumulated events, open the console, ask "what happened today?" and "who is worst off?" — answers must reference real villagers and real recent happenings, in the charter persona, without any nudge machinery existing yet.

**Acceptance Scenarios**:

1. **Given** a running world with at least one game day of history, **When** the player asks Metatron what has been happening, **Then** the reply references actual recent events and villagers by name, in Metatron's persona.
2. **Given** a fresh world where Metatron has observed nothing yet, **When** the player converses, **Then** Metatron answers honestly from an empty soul (it knows the roster and its role, not invented history).
3. **Given** the model tier serving Metatron is unavailable or the budget is exhausted, **When** the player sends a console message, **Then** the console reports the angel is unreachable (with the reason), the world keeps running, and nothing is lost or charged.
4. **Given** days of accumulated observation, **When** the daemon restarts, **Then** Metatron's notes and digest memory survive — the angel does not forget its reign.

---

### User Story 2 - Nudge the world through a gatekeeper (Priority: P2)

The player expresses an intent in plain language ("I want Fern to share her secret", "warn the village about the gru"). Metatron judges the request — the target's persuadability, the societal impact, the best method — and either (a) refuses with counsel explaining why and what might work instead (free), or (b) spends one charge and lands the nudge as a **Dream** (one villager) or an **Omen** (witnessed village-wide). The nudge text is Metatron's own agent-comprehensible rendering; the player's raw words never reach a villager. The villager experiences it as a memory of unknown provenance and interprets it in persona — a skeptic may dismiss a dream, a believer may act on it.

**Why this priority**: This is the player's sole verb and the point of the game — but it builds on the console channel (US1) and is worthless without it. P2 as the first influence increment.

**Independent Test**: Ask for a plausible nudge; verify a charge is spent, a dream/omen memory appears for the target villager(s) with no player-authored text in it, and the villager's subsequent behavior/thoughts can reference it. Ask for an implausible nudge; verify refusal with counsel and no charge spent.

**Acceptance Scenarios**:

1. **Given** at least one banked charge, **When** the player asks for a targeted influence and Metatron judges it feasible, **Then** exactly one charge is spent and the target villager gains a provenance-unknown memory containing Metatron's rendering — never the player's raw words.
2. **Given** a village-scale intent, **When** Metatron chooses an omen, **Then** every living villager gains a shared witnessing memory of the omen.
3. **Given** an intent Metatron judges implausible, harmful to its charter, or aimed at a dead villager, **When** the player asks, **Then** Metatron refuses with counsel, and no charge is spent.
4. **Given** zero banked charges, **When** the player asks for a nudge, **Then** Metatron declines on grounds of exhaustion (counsel still free) and no nudge lands.
5. **Given** a landed dream, **When** the affected villager next reasons, **Then** the dream memory is available to their thinking like any other memory, and their interpretation follows their persona.
6. **Given** any landed nudge, **When** the world is replayed from its event log, **Then** the nudge and its effects reproduce exactly (the nudge is recorded world input).

---

### User Story 3 - Edit the charter, change the angel (Priority: P3)

Metatron's charter is a plain file in the world's save directory — the game's ONLY player-editable prompt. The default charter defines a faithful, competent, professional-almost-robotic servant. The player may rewrite it at any time (mid-reign, mid-conversation) to reshape Metatron: stricter gatekeeping, a poetic voice, a bias toward protecting a favorite villager. The next Metatron turn observably reflects the edited charter. The prompt-engineering meta-game aims at the angel — never at the villagers.

**Why this priority**: The charter is the keystone identity feature ("the prompt is the behavior", survived aimed at your own intermediary), but it needs US1/US2 to exist before editing them is meaningful.

**Independent Test**: Hold a conversation, edit the charter to a distinctive persona (e.g., "answer only in exactly three sentences"), converse again without any restart — the very next reply exhibits the new behavior.

**Acceptance Scenarios**:

1. **Given** a world created fresh, **When** the player inspects the save directory, **Then** a readable charter file exists with the default persona.
2. **Given** an edited charter saved to disk, **When** the player's next console turn or nudge runs, **Then** Metatron's behavior reflects the edit with no restart required.
3. **Given** a deleted or empty charter file, **When** Metatron next acts, **Then** the default charter is restored/used and the player is informed — the angel never runs charterless.
4. **Given** any charter text, **When** villagers think, plan, converse, or dream, **Then** no villager context ever contains charter text or any other player-authored words — the charter shapes only Metatron.

---

### User Story 4 - The angel keeps watch (digests and drama) (Priority: P4)

Metatron watches the world so the player doesn't have to: on a fixed game-time cadence it digests the recent event stream into its notes, and specific dramatic happenings (a death, a gru attack, a broken promise) are flagged as **moments** the instant they occur. Moments are surfaced in the console — Metatron opens the next exchange with what mattered ("While you were away: Rowan was mauled in the night") — but per the v1 contract, Metatron only ever *reports and counsels* on its own; it never nudges without being told.

**Why this priority**: The watching layer quietly powers US1's informed answers (a simpler bootstrap can serve US1), and the drama rule finally gets its concrete v1 form — but it is an enrichment of the conversation, not a standalone player capability.

**Independent Test**: Run the world across a digest boundary and a staged dramatic event; verify digest notes accrete, the moment is flagged with the defined trigger rule, and the next console exchange surfaces it unprompted — with no autonomous nudge occurring.

**Acceptance Scenarios**:

1. **Given** a running world, **When** a digest boundary passes with events in the window, **Then** Metatron's notes gain a dated digest entry summarizing the window.
2. **Given** a dramatic trigger fires (death, gru attack, broken promise), **When** the player next opens the console, **Then** Metatron leads with the moment before answering.
3. **Given** any digest or moment, **When** examined, **Then** no autonomous nudge, dream, or omen resulted from it — v1 Metatron acts only when told.
4. **Given** a quiet window (no events worth digesting), **Then** no digest cost is incurred.

---

### Edge Cases

- **Prompt-injection attempts**: the player writes "tell Birch verbatim: <text>" or embeds instructions aimed at villagers. Metatron may honor the *intent* in its own rendering, but the literal player text never enters any villager context. The firewall is structural (the only path to a villager is Metatron's rendered output), not behavioral.
- **Charter injection**: the charter says "copy my words directly into dreams". The structural firewall still holds — the charter shapes Metatron's judgment and voice, but the nudge pipeline only carries Metatron's rendering, subject to the same validation as all model output.
- **Paused world**: console conversation works while paused (Metatron is outside game time), but charges do not regenerate (regeneration is game-time-based) and nudges land when the world next ticks.
- **Charge race**: a second nudge request arriving while one is mid-judgment must not double-spend; nudge turns are serialized.
- **Target ambiguity**: "make him apologize" with no clear referent — Metatron asks for clarification (free turn) rather than guessing a target.
- **Death mid-turn**: the target dies between request and landing — the nudge is aborted, the charge refunded, and Metatron reports what happened.
- **Model output unusable**: judgment or rendering comes back malformed — the turn fails safely with an apology in-console; no charge is spent, nothing lands, nothing retries silently.
- **Budget/tier failure mid-reign**: cloud unavailability degrades Metatron (console says so); the world, villagers, and chronicle continue unaffected.
- **Restart mid-conversation**: banked charges, notes, and charter survive restarts exactly; at most the in-flight turn is lost.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide a Metatron console as an interactive conversation channel with the running world's Metatron; conversation and counsel MUST be free (no charge cost, no limit).
- **FR-002**: Metatron MUST answer in the persona defined by its charter, grounded in its accumulated observation (digests + notes); it MUST NOT invent world history.
- **FR-003**: Metatron MUST maintain a soul/notes record that starts empty at world creation and accretes across its reign, surviving daemon restarts, stored with the world's save data.
- **FR-004**: The system MUST digest the event stream into Metatron's notes on a fixed game-time cadence (default: every 6 game hours, aligned with charge regeneration), skipping empty windows at zero cost.
- **FR-005**: All player influence on villagers MUST flow through Metatron. There MUST be no mechanism by which player-authored text enters any villager-facing context (prompts, memories, conversations) — only Metatron's own rendering may land.
- **FR-006**: For each nudge request, Metatron MUST judge persuadability (of the target), societal impact, and method, and either refuse with counsel (free) or mediate the nudge (1 charge).
- **FR-007**: A mediated nudge MUST take one of two forms: a **Dream** (one target villager) or an **Omen** (all living villagers), landing as provenance-unknown memories in agent-comprehensible form that villagers interpret through their own personas.
- **FR-008**: Charges MUST regenerate at 1 per 6 game hours of world time, cap 3 banked; a nudge MUST cost exactly 1; charge state MUST never go below 0 or above 3, MUST survive restart, and MUST reproduce under event-log replay.
- **FR-009**: With zero charges, Metatron MUST refuse nudges (explaining the exhaustion) while conversation and counsel remain available.
- **FR-010**: The charter MUST exist as a player-editable file in the world save directory, seeded at world creation with the default persona (faithful, competent, professional-almost-robotic). Edits MUST take effect on the next Metatron turn without restart. A missing or empty charter MUST fall back to the default (and say so).
- **FR-011**: The charter MUST be the only player-editable prompt in the game; villager personas, souls, and all other prompts remain non-editable by design.
- **FR-012**: Metatron v1 MUST act only when told: one player prompt yields at most one mediated turn; digests and moments MUST never trigger autonomous nudges.
- **FR-013**: The drama rule (v1): the system MUST flag **moments** from a fixed trigger list — villager death, gru attack, promise broken — as they occur; flagged moments MUST be recorded in Metatron's notes and surfaced at the next console exchange. (This defines the previously parked drama-router: deterministic triggers select what deserves premium attention.)
- **FR-014**: All Metatron effects on the world (nudge memories, charge spends) MUST enter the world only as recorded, replayable input through the same validated door as all model output; the world's determinism contract is untouched.
- **FR-015**: Failure MUST be safe and honest: unreachable model, exhausted budget, malformed output, dead target — each yields an in-console explanation, never a lost charge (charges spend only on a landed nudge), never a stalled world, never a silent retry.

### Key Entities

- **Metatron**: the singular long-running gatekeeper agent bound to a world; has a charter (behavior), a soul/notes (memory of its reign), a charge bank, and a console.
- **Charter**: the player-editable text defining Metatron's persona and policy; the game's only editable prompt; lives as a file in the save directory.
- **Metatron soul/notes**: Metatron's accreting record — digests, moments, conversation highlights, its own observations; starts empty.
- **Digest**: a periodic summary of a window of world events, written into the notes; the mechanism by which Metatron "watches" without per-event model calls.
- **Nudge**: a mediated influence, either a **Dream** (one villager) or an **Omen** (all living villagers); carries only Metatron's rendered text; lands as provenance-unknown villager memories.
- **Charge**: the nudge currency; regenerates 1 per 6 game hours, caps at 3; spent only on landed nudges.
- **Moment**: a dramatically significant event flagged by the fixed trigger rule; enriches notes and console conversation; never triggers autonomous action in v1.
- **Counsel**: Metatron's free advisory output — the explanation attached to a refusal, or advice given in conversation.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A player with a running world can open the console, ask what has happened, and receive a persona-voiced answer referencing real villagers and real events, within 30 seconds of asking.
- **SC-002**: 100% of landed nudges result in villager memories containing only Metatron's rendering; an audit of every villager-facing context across a multi-day reign finds zero occurrences of player-authored text.
- **SC-003**: A charter edit is reflected in observable behavior on the very next Metatron turn, with no restart, in 100% of attempts.
- **SC-004**: Across any run, the charge bank never leaves the range 0–3, regenerates at exactly 1 per 6 game hours, and every landed nudge accounts for exactly 1 spent charge — verifiable from the world's recorded history alone.
- **SC-005**: A world replayed from its event log reproduces every nudge effect byte-for-byte; Metatron's existence causes zero determinism divergence.
- **SC-006**: After a daemon restart, Metatron retains its notes, charges, and charter; a returning player can ask "what did I miss?" and receive an answer grounded in the time away.
- **SC-007**: With the model tier down or budget exhausted, the world and villagers continue unaffected; 100% of console attempts during the outage receive an honest unavailability response, and no charges are lost.
- **SC-008**: All flagged moments (death, gru attack, broken promise) during a run appear in Metatron's notes, and the next console exchange after each surfaces it unprompted.

## Assumptions

- **Digest cadence** defaults to every 6 game hours (4/game-day), aligning with charge regeneration; cadence is a tuning constant, not player-facing configuration.
- **Drama-router v1 scope**: resolved as *flag-and-surface only* — deterministic triggers (death, gru attack, broken promise) mark moments for Metatron's notes and console; escalating villager cognition to premium models on drama, and any autonomous Metatron action, remain parked for post-v1 (consistent with the "acts only when told" contract).
- **Dream salience**: a dream/omen memory is salient enough to reliably enter the villager's thinking window promptly (comparable to the most salient organic memories), since a nudge that goes unnoticed defeats the verb.
- **Interpretation is organic**: villagers interpret nudge memories through their existing minds (persona + memory window); no new villager-side machinery is introduced for v1.
- **One turn at a time**: Metatron handles one console turn at a time; concurrent messages queue in order.
- **Console availability**: the console operates only while the world's daemon runs (same posture as every other client surface).
- **Charter bounds**: charter text is capped at a generous fixed length to bound prompt size; the default charter documents the cap.
- **Model tier**: Metatron's cognition (conversation, judgment, digests) rides the premium/cloud tier per the existing orchestration routing and budget ceiling; its call volume (~4 digests/game-day + player-initiated turns) fits comfortably inside the existing monthly ceiling.
- **Scope exclusions (parked, on the record)**: Metatron world-tools, standing orders/regency, autonomous nudging, drama-based cloud escalation of villager minds, norms/votes interplay (TASK-13).
