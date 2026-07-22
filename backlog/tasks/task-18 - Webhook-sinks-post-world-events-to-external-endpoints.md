---
id: TASK-18
title: 'Webhook sinks: post world events to external endpoints'
status: To Do
assignee: []
created_date: '2026-07-19 19:30'
updated_date: '2026-07-22 04:34'
labels:
  - events
  - observability
dependencies: []
ordinal: 17000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Let a world POST its committed events to configured web hooks so debugging/monitoring can be offloaded to external tools (dashboards, log collectors, alerting). The seam already exists: the daemon fans committed event batches out to a consumers list via the non-blocking notify callback (internal/daemon/daemon.go:67-83) — scribe and mind are consumers today; a webhook emitter is simply another consumer following the same Observe pattern (buffered channel, never blocks the sim loop, drops on overflow like scribe's 256-batch buffer). Config-gated per world dir like the LLM orchestrator (llm.LoadConfig precedent): a webhooks config listing sinks, each with a URL and event-type filters (exact types or globs like 'agent.*', 'social.rumor_told'). Delivery: batched JSON POSTs with timeouts and bounded retry; on persistent failure drop and count, surface delivery health in daemon status — an unreachable sink must never stall or kill the world. Outbound only (no inbound webhook surface). Pairs with TASK-17: named-agent payloads make the outbound events readable without sim knowledge.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A world with a webhooks config POSTs matching committed events as JSON batches to each configured sink
- [ ] #2 Per-sink event-type filters work (exact and glob), and a world with no config sends nothing
- [ ] #3 A slow, failing, or unreachable sink never blocks or slows the sim loop — drops are counted and delivery health is visible in daemon status
- [ ] #4 Sim behavior and the event log are byte-identical with and without webhooks enabled (pure observer, replay unaffected)
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Re-grounding 2026-07-22: notify fan-out moved to daemon.go:89-105 (was 67-83); consumer list has grown — Broadcast, scribe, mind, metatron (daemon.go:93/99/164/175) — which strengthens the just-another-Observe-consumer premise. Scribe 256-batch buffer holds (scribe.go:38). Pairs with TASK-17: external readers cannot do replica lookups, they need named payloads.
<!-- SECTION:NOTES:END -->
