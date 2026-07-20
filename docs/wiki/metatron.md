---
name: metatron
description: The gatekeeper angel (TASK-12) — console conversation, dream/omen nudge mediation behind a structural prompt firewall, event-sourced charge economy, digests + drama moments, the one player-editable charter
kind: component
sources:
  - internal/metatron/metatron.go
  - internal/metatron/turn.go
  - internal/metatron/charter.go
  - internal/metatron/digest.go
  - internal/sim/metatron.go
  - internal/persona/charter.go
verified_against: 8e7ef408d9a9866f621cb0f40a1d930e42cd0b77
---

# Metatron

Metatron is the player's sole verb: a daemon-hosted gatekeeper (`internal/metatron`,
the mind/scribe notify-consumer pattern) that converses in the console, watches the
world, and mediates all influence. Raw player text has exactly one sink — Metatron's
own prompt; villagers can only ever receive Metatron's validated rendering, landed
through [[sim-loop]]'s injection door as recorded events. The meta-game is
prompt-engineering your angel: `charter.md` is the game's ONLY player-editable
prompt.

## How it works

**Console turns** (`turn.go`): one player message = one `llm.KindMetatron` cloud
call ([[llm-orchestrator]]) = at most one mediated turn, serialized single-flight.
The prompt stacks the charter (re-read every turn — edits are live by construction,
with restore/empty/truncate fallbacks and in-reply notices, `charter.go`), a fixed
frame that pins two invariants beneath ANY charter (never invent unobserved events;
never pass the player's words to a villager), live status (clock, ⚡ bank, roster),
queued moments, the [[chronicle]] tail (the angel reads its village's story — this
grounds fresh reigns and upgraded worlds), its soul tail, and recent transcript.
Output contract: strict JSON `{say, nudge|null}`; unusable output → safe apology,
nothing lands, nothing spent.

**Nudges**: `dream` (one living villager) or `omen` (all living, recorded
explicitly). Validation (form, living target, ≤400 chars, charges ≥ 1) downgrades
failures to refusal-with-counsel — refusals are free. A landed nudge is ONE atomic
`InjectSocial` batch: `metatron.nudged{form, targets, text}` (validating reducer
spends the charge; the dry-run enforces it at the door) + one prefixed
(`"You dreamed: "` / `"You witnessed an omen: "`) `agent.memory_added` per target at
`SalDream` (8) — provenance-unknown memories the villager interprets in persona.
The firewall is structural, not behavioral: no code path exists from console input
to any villager surface (sentinel-audited in `metatron_test.go`).

**Charge economy** (`internal/sim/metatron.go`): `State.MetatronCharges` — genesis
1, cap 3, +1 per absolute 6-game-hour boundary emitted by the [[executor]]
(`metatron.charge_regenerated`, a pure function of state + tick), −1 per landed
nudge. Fully event-sourced: replay reproduces the bank exactly; the field is
deliberately not `omitempty` so a spent-to-zero bank survives snapshots; pre-TASK-12
snapshots gain the genesis charge on upgrade.

**Watching** (`digest.go`): notable events collect per 6-game-hour window; each
non-empty window costs one summarization call appended to `metatron/soul.md`
(skip-empty is free; failures carry lines into the next window). The drama rule v1:
`agent.died`, `gru.attacked`, `social.promise_broken` append model-free **moment**
lines immediately and queue for the console — the next reply leads with them.
V1 contract: the angel acts only when told; digests and moments can never construct
a nudge.

**Files** (bound to the run, not event-sourced): `charter.md` at the save-dir root
(seeded by `persona.Genesis`, never overwritten); `metatron/soul.md` (accreting
notes, starts empty) and `metatron/transcript.md` (console history) — restart
survival comes free with files, and world determinism never depends on them.

**Surfaces**: IPC `metatron_chat`/`metatron_status` ([[ipc-protocol]]), CLI
`scriptworld metatron <dir> [message…]` ([[cli-scriptworld]]), and the
[[tui-client]] metatron pane (the console). Protocol status carries the ⚡ bank.

## Connections

[[sim-loop]] whitelists `metatron.nudged`; [[sim-state-reducer]] holds the bank;
[[executor]] regenerates it; [[event-types]] catalogs the family;
[[llm-orchestrator]] routes `KindMetatron` to the cloud tier; [[chronicle]] feeds
the angel's grounding; [[agent-mind]] is how villagers interpret what lands;
[[daemon-lifecycle]] wires the component behind the LLM-config gate. Spec:
`specs/005-metatron/`.

## Operational notes

Live-proven on a fresh world (reign-test: judged dream landed atomically, exhaustion
refused with counsel, BRUTUS charter edit live next turn, digest + regen at the 12:00
boundary) and on the 14-day chronicle-proof world (upgrade granted the genesis
charge; the angel answered "what do you know of Fern and the voice at the well?"
from the chronicle ring, honestly bounded its knowledge, then landed an in-world
dream that wove in the smooth stone from the story). Live finding folded back: the
no-invention rule originally lived in the (replaceable) default charter — a surly
custom charter invited fabricated villager activity; both invariants now sit in the
fixed frame. Cost: ~4 digests/game-day + player turns, noise against the ceiling.
Parked for post-v1: world-tools, standing orders/regency, autonomous action,
drama-based cloud escalation of villager minds.
