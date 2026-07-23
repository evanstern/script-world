# Contract: provenance meta tags + check-freshness CLI

## Meta tag format (in each topic page's `<head>`)

```html
<meta name="promptworld-docs:generated-by" content="player-docs">
<meta name="promptworld-docs:source" content="docs/wiki/metatron.md@8ada1050cc5b108790d0e48640dba0b985632e25">
<meta name="promptworld-docs:source" content="README.md@<commit>">
```

- `generated-by` is required on **every** page including index.html.
- `source` is required on every topic page (≥1), forbidden from being empty-valued;
  index.html carries no `source` tags (nav only, no factual claims).
- `content` grammar: `<repo-relative-path>@<40-hex-lowercase-commit>`.
- Tags must each sit on their own line (line-oriented parsing is part of the contract).

## CLI: `node .claude/skills/player-docs/scripts/check-freshness.mjs [--check] [--json]`

- Read-only always; `--check` is accepted as an explicit alias of the default so
  invocations documented as `--check` work verbatim. `--json` emits a machine report.
- Must be runnable from any cwd inside the repo (resolves repo root via
  `git rev-parse --show-toplevel`).

### Text output (default): one line per page

```
fresh  getting-started.html
stale  time-and-speed.html      docs/wiki/game-clock.md moved 8ada105… → 3f2c9a1…
missing reading-the-story.html
broken-ref llm-setup-basics.html  docs/wiki/llm-orchestrator.md: no verified_against
```

Followed by a summary line: `N fresh, N stale, N missing, N broken-ref`.

### JSON output (`--json`)

```json
{ "pages": [ { "page": "getting-started.html", "verdict": "fresh",
    "sources": [ { "path": "README.md", "recorded": "…", "current": "…", "fresh": true } ] } ],
  "summary": { "fresh": 6, "stale": 1, "missing": 0, "brokenRef": 0 } }
```

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | every expected page exists, parses, and every source pin matches |
| 1 | at least one page stale, missing, or broken-ref |
| 2 | usage/environment error (not a git repo, docs/player unreadable, bad flag) |

## Regeneration contract (skill procedure, not script)

1. Run the check. Exit 0 ⇒ stop (no-op — no file may be opened for writing).
2. For each stale/missing page only: re-read its sources at their current pins, rewrite
   the page's prose as a projection of those sources, set every `source` meta to the
   sources' current pins.
3. Re-run the check; it must exit 0. Fresh pages must be byte-identical to before.
