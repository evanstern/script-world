# Research: Metatron v1 — the editable angel

Decisions resolving every open point in the Technical Context. Ground truth: the
existing codebase (pinned notes in `docs/wiki/`), grounded-assumptions.md, and the
TASK-11 live run (chronicle-proof, 14+ game days).

## R1 — Where Metatron lives

**Decision**: a new daemon component `internal/metatron`, a notify-fan-out consumer with
its own `sim.State` replica (the scribe/mind pattern), started by `daemon.Run` only when
an LLM config exists.

**Rationale**: Metatron is not villager cognition (doesn't belong in `internal/mind`)
and not a derived view (doesn't belong in `internal/scribe`). The replica pattern is
proven three times over; fan-out consumers are non-blocking by contract, so Metatron can
never stall the loop.

**Alternatives considered**: extending `internal/mind` (rejected: tangles gatekeeper
and villager concerns; the mind's queues/cadences are villager-shaped); a separate
process (rejected: v1 needs no isolation the daemon doesn't already give).

## R2 — LLM routing

**Decision**: add `llm.KindMetatron` → `TierCloud` for all Metatron cognition (console
turns, judgment+rendering, digests). `KindDrama` stays reserved for post-v1 villager
escalation.

**Rationale**: the grounding routes Metatron-class work (judgment, digests) to the cloud
tier; a distinct kind keeps the spend meter and any future routing changes legible.
Volume is tiny: ~4 digests/game-day + player-initiated turns.

**Alternatives considered**: reusing `KindDrama` (rejected: conflates two future-facing
concepts); local tier (rejected: judgment quality is the product; local tier is
throughput-bound by villager cognition).

## R3 — Charge economy determinism

**Decision**: `State.MetatronCharges` (int, genesis = 1), fully event-sourced:

- **Regeneration**: the executor emits `metatron.charge_regenerated{tick}` whenever the
  6-game-hour boundary passes and charges < 3 — a pure function of (state, tick), same
  determinism class as day/night events. Reducer: `charges = min(3, charges+1)`.
- **Spend**: the injected `metatron.nudged` event's reducer decrements (floor 0); the
  `InjectSocial` dry-run rejects a spend at 0 charges before anything lands.

**Rationale**: replay reproduces charges exactly from the log (SC-004/SC-005); no
wall-clock, no hidden counters. Genesis 1 lets a new reign act on day 1 (a reign begins
with one favor) instead of waiting 6 game hours.

**Alternatives considered**: computing charges lazily from last-spend tick (rejected:
cap-3 banking makes the closed form fiddly and un-inspectable in status); storing charge
state only in Metatron's files (rejected: world-visible economy must be world state).

**Compatibility**: pre-TASK-12 snapshots unmarshal without the field; genesis default
applies (an upgraded world gains its first charge) — harmless, and old logs replay
unchanged since unknown event types are reducer no-ops in old binaries and regen events
only appear in logs written by new binaries.

## R4 — Nudge landing shape

**Decision**: one atomic `InjectSocial` batch per landed nudge:
`metatron.nudged{form, targets, text, tick}` (reducer: charge decrement + nudge record)
followed by one `agent.memory_added` per target with new salience `salDream = 8` and
`Subject: -1` (personal, not gossip-seeded; villagers may still gossip about it in
conversation organically). Dream = 1 target; omen = every living villager, same text.

**Rationale**: composes the two established mechanisms (whitelisted event family +
memory events) instead of inventing a parallel memory path; salience 8 sits between
shelter (6) and near-death (9), high enough to reliably enter the top-K window promptly
(spec assumption "dream salience"), low enough that real trauma still outranks the
divine. Memory text is prefixed by form ("You dreamed: …" / "You witnessed an omen: …")
so villagers interpret provenance-unknown experiences in persona.

**Alternatives considered**: a single event whose reducer writes memories itself
(rejected: duplicates memory semantics already owned by `agent.memory_added`); belief
injection (rejected: beliefs carry provenance — dreams must be provenance-unknown
experiences, and belief revision is consolidation's job).

## R5 — The prompt-injection firewall (structural)

**Decision**: player text has exactly one sink: the user-turn content of Metatron's own
prompt. The villager-facing path carries only fields parsed from Metatron's model output
(`text`, `form`, `targets`), validated (length caps, roster resolution, alive check) and
landed through the whitelist door. No code path exists from console input to any villager
prompt builder, memory, or conversation — enforced by construction and asserted by a test
that runs a nudge end-to-end and greps every villager-facing prompt and landed memory for
a sentinel string planted in the player's message.

**Rationale**: FR-005/SC-002 demand a guarantee, not a policy. Structure beats
instruction: the charter can say anything and still cannot open a path that doesn't
exist.

**Alternatives considered**: content filtering of player text (rejected: behavioral, not
structural — and the spec explicitly allows Metatron to honor the *intent* in its own
words).

## R6 — Console transport

**Decision**: new IPC request `metatron_chat{text}` → response `{reply, nudge?, charges,
moments_surfaced}` — synchronous over the existing JSON-lines protocol (the `llm_call`
request already established the long-request pattern; the client's read deadline extends
for it). One-shot CLI: `promptworld metatron <dir> <message…>`. TUI pane 3 becomes the
console: transcript viewport + input line; while the pane is active, printable keys go to
the input, Enter sends, Esc returns to the map pane (documented in the pane footer).
Turns are serialized by a single-flight guard in the component; a second concurrent
request gets a clean "the angel is attending another matter" error.

**Rationale**: reuses proven transport; the CLI one-shot gives tests and scripts the
same door the TUI uses (cli-as-proof-path precedent from TASK-6).

**Alternatives considered**: streaming/push-based console (rejected for v1: one prompt =
one mediated turn is the contract; no streaming need); a separate socket (rejected:
needless surface).

## R7 — Charter file

**Decision**: `charter.md` at the save-dir root, seeded by `promptworld new` from an
authored default (faithful, competent, professional-almost-robotic; documents the
4,000-char cap in its header comment). Loaded fresh at the start of every Metatron turn
and digest. Missing → recreated from default (and the next reply says so); empty/
oversized → default used / truncated with an in-reply notice. Never overwritten once it
exists.

**Rationale**: FR-010's "next turn, no restart" is trivially true when the file is read
per turn (~4k chars is noise next to a cloud call). Root-level placement makes "the one
file you edit" discoverable.

**Alternatives considered**: hot-reload watching (rejected: pointless given per-turn
reads); storing the charter in the event log (rejected: it is player input to Metatron,
not world state — and versioning player prose in the log bloats replay for zero
determinism benefit).

## R8 — Metatron's soul, notes, and transcript

**Decision**: a `metatron/` subdirectory in the save dir: `soul.md` (accreting notes —
dated digest entries, flagged moments, nudge records with judgment one-liners; starts
empty at world creation) and `transcript.md` (append-only console history). Both written
by the component; the prompt carries the charter + a bounded tail of soul.md + the last
few transcript turns + live status (charges, roster, clock).

**Rationale**: these are Metatron's private memory and the player's record — files bound
to the run (the persona/soul precedent), not world state. Restart-survival (SC-006)
comes free with files; the world's determinism never depends on them.

**Alternatives considered**: event-sourcing the notes (rejected: model-authored prose in
the world log couples replay to Metatron's musings; the world-visible effects are already
events); SQLite tables (rejected: flat files are the established, player-readable
posture).

## R9 — Digests and the drama rule

**Decision**: the component collects notable-event lines (reusing the chronicle's
notable-line vocabulary where it fits) per 6-game-hour window; at each boundary with a
non-empty buffer, one `KindMetatron` call summarizes the window into a dated soul.md
digest entry. **Moments** (drama rule v1): `agent.died`, `gru.attacked`,
`social.promise_broken` are appended to soul.md immediately (no model call) and queued;
the next console reply is instructed to surface queued moments first. Digest failures
carry the window forward (the TASK-11 carry pattern); moments never trigger autonomous
action.

**Rationale**: FR-004/FR-013 with the cheapest honest machinery; the trigger list is
deterministic and auditable; "surface at next exchange" implements "reports and
counsels, never acts" exactly.

**Alternatives considered**: per-event model calls (rejected: the grounding explicitly
chose periodic digests); routing villager cognition to cloud on drama (parked, per spec
Assumptions).

## R10 — Turn output contract

**Decision**: one cloud call per console turn. The model must return strict JSON:
`{"say": "...", "nudge": {"form": "dream"|"omen", "target": "<name>"|null, "text": "..."} | null}`.
`nudge` non-null means Metatron judged the request actionable AND charges ≥ 1 (banked
count is in the prompt; the component re-checks before landing and downgrades to a
refusal reply if the model ignored an empty bank). Unparseable output → safe apology
reply, nothing lands, no charge (FR-015). The judgment rubric (persuadability, impact,
method) lives in the fixed system frame; the charter shapes voice and policy on top.

**Rationale**: single-call turns keep the 30 s budget (SC-001) and the one-prompt-one-
turn contract; parse-or-refuse mirrors the consolidation validator posture (reject bad
output, never partially apply).

**Alternatives considered**: two-phase judge-then-render calls (rejected: doubles
latency and spend for marginal quality at v1 scale); function-calling/tool APIs
(rejected: the orchestrator's provider-agnostic chat surface is the established
transport).
