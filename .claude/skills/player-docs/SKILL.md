---
name: "player-docs"
description: "Generates and refreshes docs/player/ — seven self-contained HTML pages that project the code-grounded wiki (docs/wiki/), README.md, and docs/llm-providers.md into plain-language documentation for players (non-engineers). Runs standalone; the recommended follow-on after /grounding-wiki:wiki-update re-pins wiki notes. Check freshness first: node .claude/skills/player-docs/scripts/check-freshness.mjs --check"
metadata:
  author: "promptworld"
user-invocable: true
disable-model-invocation: false
---

# player-docs

Regenerates `docs/player/` — the player-facing (non-engineer) HTML documentation set —
from the grounded corpus: `docs/wiki/` notes, `README.md`, and `docs/llm-providers.md`.
Distinct from `docs/wiki/` (developer-grounded) and `docs/course/` (interactive
codebase course). This skill is standalone: nothing else invokes it, and it invokes
nothing else — the human loop is "after `wiki-update` re-pins notes, run `player-docs`".

## When to run this

Run after `/grounding-wiki:wiki-update` re-pins any wiki note. More generally, run it
any time you want to know whether the player docs are current, or to bring them
current when they aren't.

## Procedure (check-first, mandatory order)

This is a **check-first regeneration** procedure. Never open a page for writing before
the check has run and named it stale or missing.

1. **Run the check.**

   ```sh
   node .claude/skills/player-docs/scripts/check-freshness.mjs --check
   ```

   - **Exit 0** → every page is fresh. **Stop here.** Do not open, touch, or rewrite
     any file under `docs/player/`. This is the no-op path — regenerating when nothing
     is stale must change nothing (byte-identical no-op).
   - **Exit 1** → at least one page is stale, missing, or broken-ref. Continue to step 2.
   - **Exit 2** → usage/environment error (not a git repo, `docs/player` unreadable, bad
     flag). Fix the environment, not the pages, and re-run step 1.

2. **For each stale or missing page ONLY** (leave every fresh page untouched — do not
   open it, do not re-save it even byte-identically):
   a. Re-read the page's declared sources (see the mapping table below) at their
      **current** pins — wiki notes at their current `verified_against:` frontmatter
      value, `README.md`/`docs/llm-providers.md` at `git log -1 --format=%H -- <path>`.
   b. Rewrite the page's prose as a plain-language projection of those sources — no
      independently asserted facts, nothing a player would need engineering vocabulary
      to parse. Preserve the page's slug, its links (to `index.html` and, for
      `index.html`, to every topic page), and the shared CSS block below.
   c. Update every `promptworld-docs:source` meta tag to the source's current pin. Add
      a tag for any newly-drawn-on source; remove one for a source no longer used.
      `index.html` never carries `source` tags — nav only, no factual claims.

3. **Re-run the check.** It must exit 0. If it doesn't, the regeneration is incomplete
   — go back to step 2 for whatever it still reports. Fresh pages from before this run
   must be byte-identical to what they were (nothing legitimately touched them).

## The expected page set

`index.html` (nav hub, no sources) plus six topic pages:

- `getting-started.html`
- `playing-via-metatron.html`
- `time-and-speed.html`
- `reading-the-story.html`
- `the-ai-behind-the-village.html`
- `llm-setup-basics.html`

## Page → source mapping

The starting, editorial contract (research.md D5). If a page genuinely draws on a
source not listed here, add it — and add the matching meta tag in the same change.

| Page | Sources |
|------|---------|
| `index.html` | none (nav only) |
| `getting-started.html` | `README.md`, `docs/wiki/cli-promptworld.md`, `docs/wiki/daemon-lifecycle.md`, `docs/wiki/tui-client.md` |
| `playing-via-metatron.html` | `docs/wiki/metatron.md`, `docs/wiki/metatron-miracles.md`, `docs/wiki/governance.md` |
| `time-and-speed.html` | `docs/wiki/game-clock.md`, `docs/wiki/sim-loop.md`, `docs/wiki/cli-promptworld.md` |
| `reading-the-story.html` | `docs/wiki/chronicle.md`, `docs/wiki/tui-client.md`, `docs/wiki/event-log.md` |
| `the-ai-behind-the-village.html` | `docs/wiki/agent-mind.md`, `docs/wiki/cognition.md`, `docs/wiki/llm-orchestrator.md`, `docs/wiki/nightly-consolidation.md`, `docs/wiki/social-fabric.md` |
| `llm-setup-basics.html` | `docs/llm-providers.md`, `docs/wiki/llm-orchestrator.md`, `README.md` |

`llm-setup-basics.html` stays at "get it working" depth (the minimum `llm.json` a
non-engineer needs) and defers registry-reference/migration depth to
`docs/llm-providers.md` by link/mention rather than duplicating it.

## Provenance meta-tag format

Contract: `specs/026-player-docs/contracts/provenance-and-check.md`. In every page's
`<head>`, each tag on its own line (line-oriented parsing depends on this):

```html
<meta name="promptworld-docs:generated-by" content="player-docs">
<meta name="promptworld-docs:source" content="docs/wiki/metatron.md@8ada1050cc5b108790d0e48640dba0b985632e25">
<meta name="promptworld-docs:source" content="README.md@8fa82e1a4deefb4f7d3923b334ef85f25cf2c298">
```

- `generated-by` is required on every page, including `index.html`.
- `source` is required on every topic page (one or more); forbidden (absent) on
  `index.html`.
- `content` grammar: `<repo-relative-path>@<40-hex-lowercase-commit>`.

## Canonical page skeleton + shared CSS

Every page inlines this exact `<style>` block (system font stack, ~70ch measure, CSS
custom properties, `prefers-color-scheme: dark` override, no external assets, no JS).
Copy it verbatim into every page — do not factor it into a shared file; self-containment
per page is the point (FR-004).

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="promptworld-docs:generated-by" content="player-docs">
<meta name="promptworld-docs:source" content="<repo-relative-path>@<40-hex-commit>">
<!-- one promptworld-docs:source tag per source, each on its own line -->
<title>&lt;Page title&gt; — promptworld player docs</title>
<style>
  :root {
    --bg: #ffffff;
    --fg: #1a1a1a;
    --muted: #55606a;
    --accent: #0b5fff;
    --border: #d8dee4;
    --code-bg: #f3f5f7;
    --card-bg: #f8f9fb;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --bg: #14161a;
      --fg: #e6e9ec;
      --muted: #9aa4ae;
      --accent: #6ea8ff;
      --border: #333a42;
      --code-bg: #1d2024;
      --card-bg: #1a1d21;
    }
  }
  * { box-sizing: border-box; }
  html, body { margin: 0; padding: 0; background: var(--bg); color: var(--fg); }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica,
      Arial, sans-serif;
    line-height: 1.6;
    max-width: 70ch;
    margin: 0 auto;
    padding: 2rem 1.25rem 4rem;
  }
  h1, h2, h3 { line-height: 1.25; }
  h1 { font-size: 1.9rem; margin-top: 0; }
  h2 { font-size: 1.35rem; margin-top: 2.25rem; border-bottom: 1px solid var(--border);
    padding-bottom: .3rem; }
  h3 { font-size: 1.1rem; margin-top: 1.5rem; }
  p, ul, ol { margin: .9rem 0; }
  a { color: var(--accent); }
  code, pre { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
  code { background: var(--code-bg); padding: .15em .4em; border-radius: .3em;
    font-size: .92em; }
  pre { background: var(--code-bg); padding: 1rem; border-radius: .5em;
    overflow-x: auto; }
  pre code { background: none; padding: 0; }
  blockquote { border-left: 3px solid var(--accent); margin: 1rem 0; padding: .2rem 1rem;
    color: var(--muted); }
  .lede { color: var(--muted); font-size: 1.05rem; }
  .card { background: var(--card-bg); border: 1px solid var(--border);
    border-radius: .6rem; padding: 1rem 1.25rem; margin: 1rem 0; }
  .nav-grid { display: block; padding: 0; margin: 0; list-style: none; }
  .nav-grid li { margin: .6rem 0; }
  table { border-collapse: collapse; width: 100%; margin: 1rem 0; }
  th, td { border: 1px solid var(--border); padding: .5rem .6rem; text-align: left; }
  footer { margin-top: 3rem; padding-top: 1rem; border-top: 1px solid var(--border);
    color: var(--muted); font-size: .85rem; }
</style>
</head>
<body>
<h1>&lt;Page title&gt;</h1>
<p class="lede">&lt;one-line framing&gt;</p>

&lt;... content ...&gt;

<footer><a href="index.html">&larr; Back to player docs index</a></footer>
</body>
</html>
```

`index.html` follows the same skeleton minus the `source` meta tags, and its body is a
nav hub linking every topic page with a one-line blurb rather than factual content.
