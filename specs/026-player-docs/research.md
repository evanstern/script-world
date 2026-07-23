# Phase 0 Research: Player Docs

No NEEDS CLARIFICATION markers survived the spec; research consolidates the decisions
already latent in the corpus and records page→source mappings.

## D1 — Provenance carrier: HTML meta tags

- **Decision**: each page records provenance as `<meta>` tags in `<head>`: one
  `promptworld-docs:source` tag per source (`<path>@<commit>`), and the check script
  parses pages with a line-oriented regex (no HTML parser dependency).
- **Rationale**: meta tags are machine-readable, invisible to readers, survive any
  static hosting, and need no sidecar file that could drift from the page.
- **Alternatives considered**: sidecar JSON manifest (rejected: two files can drift;
  page stops being self-describing); HTML comments (rejected: less conventional, same
  parsing cost).

## D2 — Staleness semantics: pin inequality, two source kinds

- **Decision**: a page is **stale** iff any recorded source pin differs from the
  source's current pin. For wiki notes the current pin is the note's
  `verified_against:` frontmatter value. For non-wiki sources (`README.md`,
  `docs/llm-providers.md`) the current pin is `git log -1 --format=%H -- <path>` (last
  commit touching the file). Inequality, not ancestry: pins only move forward on main,
  and inequality needs no git graph walk.
- **Rationale**: mirrors the grounding-wiki gate exactly (a source moving past the
  recorded pin invalidates the derivative); works identically in worktrees and CI.
- **Alternatives considered**: mtime comparison (rejected: not commit-anchored, breaks
  on fresh clones); content hashing (rejected: wiki notes already carry an
  authoritative pin — reuse it).

## D3 — Script home and shape: skill-owned Node ESM, exit-code gate

- **Decision**: `.claude/skills/player-docs/scripts/check-freshness.mjs`, Node ≥ 18,
  zero dependencies. Default (and only) mode is read-only reporting; exit 0 = all
  fresh, 1 = any stale/missing/broken-ref, 2 = usage/environment error. Regeneration is
  performed by the skill (model-authored prose), never by the script.
- **Rationale**: matches the educate `progress.mjs` precedent (skill-owned script as
  the machine gate; the model does the judgment work); exit codes make the check
  composable into hooks/CI later without changes.
- **Alternatives considered**: shell script (rejected: frontmatter + meta parsing gets
  fragile); making the script also write/regenerate (rejected: prose is model-authored
  by design — a script cannot regenerate it, so "generate" belongs to the skill).

## D4 — Idempotence: regeneration touches only stale pages

- **Decision**: the skill's procedure is check-first: run the script, rewrite only
  pages reported stale (or missing), re-record their provenance at the sources' current
  pins, and never open fresh pages for editing. "Run twice → no-op" follows
  structurally (second check reports all fresh → nothing rewritten).
- **Rationale**: byte-identical no-op cannot be guaranteed by re-generating prose with
  a model; it *is* guaranteed by not touching fresh files. This is the same trick the
  wiki-update flow uses (computed re-pins vs review work).

## D5 — Page inventory and source mapping (initial; editorial at regen time)

| Page | Sources (wiki notes unless noted) |
|------|-----------------------------------|
| index.html | none (nav only — no factual claims, so no provenance needed; carries a generated-by tag only) |
| getting-started.html | `README.md`, cli-promptworld, daemon-lifecycle, tui-client |
| playing-via-metatron.html | metatron, metatron-miracles, governance |
| time-and-speed.html | game-clock, sim-loop, cli-promptworld |
| reading-the-story.html | chronicle, tui-client, event-log |
| the-ai-behind-the-village.html | agent-mind, cognition, llm-orchestrator, nightly-consolidation, social-fabric |
| llm-setup-basics.html | `docs/llm-providers.md`, llm-orchestrator, `README.md` |

- **Rationale**: covers the six mandated topics (FR-002) with the narrowest source set
  that grounds each page; narrow source lists keep staleness precise (SC-004).
- **Note**: the mapping is the *starting* contract; the implementer may add a source a
  page genuinely draws on (must add it to the meta tags in the same change).

## D6 — Styling: one shared CSS block, inlined per page

- **Decision**: a single minimal CSS block (system font stack, readable measure,
  `prefers-color-scheme: dark` overrides via CSS custom properties) authored once and
  inlined into every page's `<style>`. No JS, no toggle, no external requests.
- **Rationale**: self-containment (FR-004) beats DRY for 7 files; identical inlined
  blocks keep pages copyable/hostable anywhere.
- **Alternatives considered**: shared style.css file (rejected: pages stop being
  single-file self-contained); theme toggle (rejected: spec assumes OS preference
  suffices).

## D7 — Skill invocation and documentation

- **Decision**: skill name `player-docs`; standalone invocation; SKILL.md description
  documents "run after `/grounding-wiki:wiki-update` re-pins notes". CLAUDE.md gets a
  one-line pointer in the PDLC block area naming the skill and when to run it.
- **Rationale**: FR-009 + Principle III (plugins compose through files + gates, never
  calls) — wiki-update must not invoke player-docs; the human loop closes it.
