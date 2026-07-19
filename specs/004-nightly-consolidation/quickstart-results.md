# Quickstart Results: Nightly Consolidation + Persona Firewall

Recorded 2026-07-19. World: `~/worlds/consol-test` (seed 11), local tier gemma4:12b-mlx
(Ollama), cloud tier `cc/claude-haiku-4-5-20251001` via LAN 9router
(`provider: openai_compat`, pricing 0/0). Speeds: 16x for all consolidation windows
(max-speed sprints between nights; `max` is the documented replica-drop regime and was
excluded from claims).

## Scenario 1 — unit + integration suite

`go test ./... -race`: **green** across the repo (sim reducer/ledger/replay, driver
atomicity/dedupe/deferral, validator fixtures incl. all 32 authored drift markers
rejected 100% (SC-002), persona-bytes canary, scribe growth).

## Scenario 2 — live nights (SC-001, SC-003)

Three consecutive observed nights; every living agent attempted exactly once per night
(marker ledger, from the event log):

| night | accepted | rejected | notes |
|---|---|---|---|
| 176 | 1 | 7 | v1 output interface: models mangled hex hash refs, invented belief IDs, overran gist cap |
| 177 | 3 | 5 | after ordinal refs + ID coercion: remaining rejections were cap overruns + anchor typography |
| 178 | **8** | 0 | after truncate-not-reject caps + normalized anchor echo |

- Exactly one `agent.consolidated` marker per agent per night — 8/8/8, no doubles, across
  four daemon restarts (ledger survives recovery; FR-001).
- Souls grew visibly (SC-003): narratives in-voice and changing between nights, beliefs
  with confidence + provenance referencing lived and social events, e.g.
  - Birch: "…maybe I'm not just curious. Maybe I'm hungry in a way that gossip never
    quite fills." + belief "Oak's quiet kindness—feeding someone in need—says more about
    the village than a hundred conversations" *(71%, told)*
  - Hazel: belief "I'm building a web of favors across the village; everyone owes me
    something, or soon will" *(75%, inferred)*
  - Cedar: "I am Cedar, the one who counts his strokes to nine and builds what outlasts
    the frost."
- persona.md untouched throughout: mode `-r--r--r--` (0444), content byte-identical to
  genesis (FR-007). Rejected nights landed markers only; buffers survived to the next
  night and were digested then (the multi-day backlog path, exercised for real).
- Cost: $0.0000 metered (subscription-routed LAN router); ~6 s/agent wall time, whole
  village ≈ 45 s/night.

## Scenario 3 — degraded behavior

Not staged against a dead router this run; the equivalent guarantees were exercised
live anyway: 7+5 rejected nights retained buffers and re-digested later, and unit tests
cover the no-marker/transport-failure path (`TestConsolidationTransportFailureDefers`).
The world's tick never stalled during any consolidation activity.

## Scenario 4 — replay determinism (SC-004)

`TestConsolidationReplayDeterminism` green (timeline with all five event types replays
byte-identical, zero model calls); live corroboration: four daemon restarts recovered
the world losslessly from snapshot+log, consolidation ledger intact.

## Live findings folded back into the implementation

1. Planner calls blocked the mind's absorb loop → event batches (incl. `agent.slept`)
   dropped at 16x; planner moved to its own single-flight worker.
2. Model-facing refs: hex hashes → ordinal labels (`m1..m60`); unknown belief IDs
   coerced to new; cap overruns truncated best-first; anchor echo compared under
   typography normalization. Firewall semantics unchanged — paraphrased natures and
   drift-marker text still reject.
3. Daemon lifecycle: pidfile removal is now ownership-checked and `stop` waits 30s —
   a slow big-world shutdown could previously orphan its successor ("database is
   locked" on the third start).
