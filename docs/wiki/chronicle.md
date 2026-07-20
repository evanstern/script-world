---
name: chronicle
description: The narrated story feed (TASK-11) — cloud narrator turns notable events into chapter entries injected as chronicle.entry; a snapshot-carried ring is the catch-up mechanism
kind: component
sources:
  - internal/sim/chronicle.go
  - internal/mind/narrate.go
  - internal/scribe/scribe.go
verified_against: 8f24c13a5b2eb1c1f37244978055e3f6eb5d42d2
---

# Chronicle

The chronicle is the catch-up mechanism for the ambient world (the grounding's
core posture: days pass unattended; the story must be readable on return). A
cloud-tier narrator compresses the event stream into a few story entries per
game day; entries are events like everything else, and a bounded ring on
`State` carries the readable history to every attaching client for free.

## How it works

**The event and the ring** (`internal/sim/chronicle.go`): `chronicle.entry`
carries `ChronicleEntryPayload{day, from_tick, to_tick, text, thread, agents}`.
The reducer appends it to `State.Chronicle`, a bounded ring (`chronicleCap`
= 256 — weeks of story at the narrator's ~2 chapters/game-day; the
[[event-log]] keeps everything forever). Because the ring rides
`State.Marshal`, the TUI's state-snapshot fetch and daemon recovery both
deliver narrated history with no extra protocol — that IS the catch-up.
`thread` is a stable lowercase slug naming a storyline across chapters;
`agents` are roster indices, the basis of the TUI's agent filter.

**The narrator driver** (`internal/mind/narrate.go`, hosted by the
[[agent-mind]] Mind): `chronicleNote` (absorb goroutine, replica already
current) turns notable events into pre-named factual log lines — deaths,
builds, [[gru]] emergence/sightings/attacks, conversations with gist+topics,
rumors told, gifts, broken promises, musings, and (TASK-13) the whole
[[governance]] arc: assemblies with attendance named, grievances raised,
proposals tabled/passed/voted down with tallies, exiles, and witnessed norm
violations — each stamped with in-world time. `sim.night_started` closes the day chapter, `sim.day_started` closes
the night chapter; a chapter with no lines spends no call. The chapter job
snapshots the lines plus up to 8 recent thread slugs (offered for reuse) to a
single-flight worker: one `llm.KindNarrator` call ([[llm-orchestrator]] cloud
tier, 3-minute cap, MaxTokens 800) asking for strict JSON of 1–3 entries.
`parseNarration` validates (texts trimmed/capped, threads slugified, agent
names resolved against the roster, unknowns dropped) and the batch lands
atomically through `InjectSocial` — `chronicle.entry` is whitelisted in
[[sim-loop]]'s injection door, so narrator output enters the world only as
recorded input, like all model output.

**Failure honesty**: a transport/tier failure carries the chapter's lines
into the next boundary via a 1-slot retry buffer (merged oldest-first, capped
at `narrMaxLines` = 120, oldest dropped); unusable model output is dropped —
a gap in the story, never a stall or a retry loop; a full chapter queue (8)
drops with a log line. No llm.json → no narrator; the world just has no
narrated story.

**Views**: the [[tui-client]] chronicle pane renders the replica's ring with
agent/thread filters and a raw-feed fallback; the [[agent-mind]] scribe
renders `chronicle.md` in the save dir — the offline catch-up artifact,
regenerated from recovered state at every daemon start.

## Connections

[[event-types]] catalogs `chronicle.entry`; [[sim-state-reducer]] holds the
ring; [[sim-loop]] whitelists the injection; [[llm-orchestrator]] routes
`KindNarrator` to the cloud tier; [[tui-client]] and the scribe render it;
[[snapshots]] carry the ring through recovery.

## Operational notes

Live-proven (chronicle-proof world, 32x, gemma local + 9router cloud): chapters
landed at both boundaries, the narrator reused thread slugs across chapters
unprompted beyond the offered list, gru night drama narrated from real events,
and the ring survived a daemon restart (chronicle.md regenerated from recovered
state). Cost: ~2 narrator calls per game day — noise against the $100/month
ceiling. The chapter buffer is in-memory: a daemon restart loses the current
chapter's collected lines (the story resumes at the next boundary).
