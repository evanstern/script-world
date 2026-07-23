# Quickstart Validation: Player Docs

Prerequisites: repo checkout, Node ≥ 18 (`/opt/homebrew/bin` on PATH), a browser.

## V1 — Pages render standalone (SC-002)

```sh
open docs/player/index.html    # or double-click from Finder
```

Expect: index renders with links to all six topic pages; each opens and renders with no
network access; flip OS appearance light↔dark — both legible. View-source on any topic
page shows `promptworld-docs:source` meta tags with 40-hex pins.

## V2 — All-fresh check is clean and read-only (SC-003)

```sh
node .claude/skills/player-docs/scripts/check-freshness.mjs --check; echo "exit=$?"
git status --porcelain docs/player .claude/skills/player-docs
```

Expect: every page `fresh`, summary `7 fresh, 0 stale, 0 missing, 0 broken-ref`,
`exit=0`, and `git status` empty (nothing written).

## V3 — Staleness detection is exact (SC-004)

Simulate one source moving: edit ONE page's recorded pin for one wiki source to a
different (fake) 40-hex value, then:

```sh
node .claude/skills/player-docs/scripts/check-freshness.mjs; echo "exit=$?"
```

Expect: exactly that page `stale` (naming the moved source), all others `fresh`,
`exit=1`, no files modified. Revert the edit (`git checkout -- docs/player/<page>`).

## V4 — Broken-ref and missing are surfaced

```sh
mv docs/player/time-and-speed.html /tmp/  # or the scratchpad
node .claude/skills/player-docs/scripts/check-freshness.mjs; echo "exit=$?"
mv /tmp/time-and-speed.html docs/player/
```

Expect: `missing time-and-speed.html`, `exit=1`. (Broken-ref path: point one source
meta at a nonexistent file and observe `broken-ref`.)

## V5 — Regeneration no-op (FR-007, SC-004)

With V2 green, invoke the skill (`/player-docs`) and expect it to stop at "all fresh";
`git status --porcelain docs/player` stays empty — byte-identical no-op.

## V6 — Grounding audit (SC-005, AC #4 on TASK-82)

For each topic page, spot-check ≥2 player-facing claims against a declared source at
its recorded pin (`git show <pin>:<path>`). Record the audit (pages, claims, verdicts)
on TASK-82 via `backlog task edit 82 --append-notes`.
