# Phase 1 Data Model: Player Docs

## Player page (`docs/player/<slug>.html`)

| Field | Where | Rules |
|-------|-------|-------|
| slug | filename | kebab-case; stable across regenerations (staleness identity) |
| title | `<title>` + `<h1>` | player-facing, plain language |
| provenance | `<meta name="promptworld-docs:source">` (0..n) | one tag per source, `content="<repo-relative-path>@<40-hex-commit>"`; required on every topic page; index carries none |
| generator mark | `<meta name="promptworld-docs:generated-by">` | `player-docs` — lets the check enumerate pages it owns |
| body | HTML | self-contained; inlined shared CSS; links back to index.html; no external assets; no JS required |

## Provenance record (parsed form)

| Field | Type | Rules |
|-------|------|-------|
| path | repo-relative path | must exist at check time, else **broken-ref** |
| pin | 40-hex commit | compared for equality against the source's current pin |

## Source (two kinds)

| Kind | Identity | Current pin |
|------|----------|-------------|
| wiki note | `docs/wiki/*.md` | `verified_against:` frontmatter value |
| plain file | any other repo path (README.md, docs/llm-providers.md) | `git log -1 --format=%H -- <path>` |

## Freshness verdict (per page) — state machine

```
missing     page listed in expected set but file absent
broken-ref  any provenance path nonexistent, or wiki note lacks verified_against,
            or meta tag malformed
stale       any recorded pin ≠ source's current pin
fresh       none of the above
```

Precedence: missing > broken-ref > stale > fresh. Any non-fresh page ⇒ script exit 1.

## Expected page set

Declared in the check script as a constant list (the seven slugs from plan.md §Project
Structure) so "missing" is detectable; adding a page means adding its slug there — a
one-line, reviewed change.
