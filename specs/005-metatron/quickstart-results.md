# Quickstart validation results — Metatron v1

Run 2026-07-20. Two live worlds, both cloud tier via 9router
(`cc/claude-haiku-4-5-20251001`):

- **reign-test** (fresh, seed 12, 32x, local tier deliberately dead — reflex world;
  scratchpad)
- **chronicle-proof** (~/worlds; 14+ game days old, gemma local + 9router cloud —
  the upgrade + real-story testbed)

## §1 The angel exists and converses — PASS

- `promptworld new` seeded `charter.md` (default persona) and `metatron/soul.md`
  ("The reign begins. The angel has seen nothing yet.").
- Status peek (no model call): `charges ⚡·· (1/3) · default charter · <path>`.
- First turn: charter-voiced self-introduction, correct roster, correct charge
  count, honest about a fresh reign. Round-trip well under 30 s (SC-001).

## §2 Nudges: dream, omen, refusal, charges — PASS

- Dream request → judged, landed: log seq 189–190 show the atomic batch —
  `metatron.nudged{form:"dream", targets:[4], text:<Metatron's rendering>}` +
  `agent.memory_added{agent:4, "You dreamed: …", salience:8, subject:-1}`. Fern's
  `soul.md` carries the 8★ memory. Charges 1→0. The rendering was entirely
  Metatron's ("a field of grain… the early gatherer survives what the idle do
  not") — zero player phrasing.
- Exhaustion (⚡0): follow-up nudge request refused with counsel ("I counsel
  patience… when a charge returns"); no charge, no injection.
- Omen on all-living + dead-target refusal + charge invariants: unit/integration
  suite (`internal/metatron`, `internal/sim`), race-clean.
- Firewall sentinel (SC-002): `TestFirewallSentinel` proves the player's literal
  text reaches only Metatron's prompt — never payloads, villager memories, or
  soul records.

## §3 Charter editability — PASS

- Charter replaced with "You are BRUTUS, a surly angel…" mid-run: the VERY NEXT
  turn answered in character, no restart (SC-003).
- Missing charter (chronicle-proof, a pre-TASK-12 world): first turn replied with
  "(charter.md was missing — the default charter has been restored)" and recreated
  the file. Empty/oversized fallbacks unit-proven.
- **Live finding folded back**: the surly charter invited invented villager
  activity; the no-invention rule had lived only in the replaceable default
  charter. Fix: both invariants (never invent; never pass player words) now sit
  in the fixed frame beneath any charter.

## §4 Watching: digests and moments — PASS (digest live; moments unit-proven)

- Digest landed live at the day-1 12:00 boundary (reign-test): dated entry in
  `metatron/soul.md` summarizing the shelter-building morning; empty windows
  spend nothing; `metatron.charge_regenerated` recorded at the same absolute
  boundary.
- Moments (death / gru attack / broken promise → immediate soul line + surfaced
  at next turn, never autonomous): `TestDigestAndMoments` +
  acts-only-when-told injection audit. No qualifying drama occurred during the
  live window; the invariant that watching can never inject is test-enforced.

## §5 Substrate guarantees — PASS

- Determinism: `internal/sim` replay tests include nudge batches + regen events
  (hash-identical); binary e2e determinism suite green with the executor's regen
  emission.
- Restart: reign-test stopped at ⚡0 and restarted at ⚡0 — the charges field is
  deliberately not `omitempty`, so a spent bank cannot resurrect (bug caught in
  design review). Soul, transcript, charter all survived.
- Full suite `go test ./... -race`: green.

## §6 Live acceptance on chronicle-proof — PASS

- Upgrade: 14-day pre-TASK-12 world recovered with the genesis charge (documented
  compat), regen accrued to ⚡2 during runtime.
- "What do you know of Fern and the voice at the well?" → the angel answered from
  the village chronicle (the day-13 entry the TASK-11 narrator wrote), named the
  Birch/Fern exchange and the smooth stone, and honestly bounded its knowledge
  ("I have not heard the voice myself"). This exercised the chronicle-tail
  grounding added during acceptance.
- Dream against the live storyline: judged (persuadability: "guarded by nature…
  thoughtful, not stubborn"), rendered in-world weaving the stone motif, landed
  atomically, ⚡2→1. Fern's post-dream cognition observed (planner thoughts
  resumed normally; in-persona interpretation accrues with world time).

## Operational note

macOS kernel kills (`exit 137`, silent) traced to a stale-signature binary copy in
`~/worlds` — resolved by building directly to the destination
(`go build -o ~/worlds/promptworld-task12 ./cmd/promptworld`). Not a promptworld
defect; recorded for future binary distribution.
