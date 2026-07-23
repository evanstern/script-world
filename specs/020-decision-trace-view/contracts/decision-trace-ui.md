# UI Contract: Decision-Trace View

The externally observable behavior the implementation must satisfy. References:
[data-model.md](../data-model.md), research decisions D1–D7.

## §1 Projection semantics

- R1. Every subscribed `cog.thought`, `cog.tool_call`, `cog.outcome` event that passes
  the existing seq-skip guard is ingested exactly once; no other event type mutates
  the projection.
- R2. Chains join on job ID. Calls order by ordinal regardless of arrival order.
- R3. Attribution precedence: thought payload agent → outcome payload agent → villager
  job-ID parse (`^<class>-<idx>-<tick>$`). `turn-metatron-*` → Metatron sentinel.
  `conversation-*` → never ingested. Unattributable jobs are retained fragment-free of
  an agent list only if attributable later (a later thought/outcome may attribute
  them); an unattributed chain is invisible in the villager surface.
- R4. Per-agent retention is exactly `decisionChainCap` (20) chains, oldest-evicted;
  eviction removes the chain entirely (both indexes).
- R5. The projection resets to empty on reconnect (replica swap).
- R6. Stimulus text is fixed at thought-ingest time: ring hit → digest text; trigger 0
  → cadence phrase; miss → neutral reference naming the seq. It never changes after.

## §2 Decisions sub-view (villager detail)

- R7. Key grammar (layered on `handleVillagersKey`, gated on villagers visibility):
  `d` toggles the decisions sub-view while the detail view is open; `j`/`k` scroll
  while decisions is open; `esc` unwinds decisions → detail → roster → (existing
  global chain). No keypress is a silent no-op contrary to the focus contract.
- R8. The detail view advertises the sub-view (a `d decisions` hint) so it is
  discoverable without documentation.
- R9. Chains render most-recent-first. Each chain shows, in order: a when/class
  header, the stimulus line, each call as tool + glossary phrase (+ reason when
  present), and the terminal line (outcome phrase + reason) — or a visible
  in-progress marker when no outcome has arrived (FR-008). Suppression-only chains
  render as "didn't think" entries with the router reason.
- R10. The body clips to the pane's row budget (exact-height invariant preserved at
  all sizes); scrolling reveals clipped chains; scroll state clamps defensively at
  render time and resets on villager change, detail close, and reconnect.
- R11. A villager with no chains renders an explicit empty-state line, not a blank
  pane. Dead villagers keep their chains.

## §3 Metatron inline verdicts

- R12. Every ingested `turn-metatron-*` `cog.tool_call` appends exactly one transcript
  row at arrival: tool name + glossary phrase (+ reason when present), styled as
  telemetry (distinct from `you:`/`angel:` rows), wrapped like other transcript rows.
- R13. Rows appear in call-emission order, before that turn's `angel:` reply row (the
  reply lands only after the turn completes). The existing 200-row transcript cap and
  `⚡` miracle rows are unchanged.
- R14. A prose-only turn (no tool calls) adds no verdict rows.

## §4 Verdict glossary

- R15. One table maps every verdict in the toolloop taxonomy and every outcome in the
  sim vocabulary to a plain-language phrase; both surfaces render through it; the raw
  enum string never reaches either surface.
- R16. A sweep test enumerates the toolloop verdict constants and sim outcome
  constants and fails on any missing glossary entry.
- R17. An unknown future verdict string falls back to a safe generic phrase (it must
  not render the raw enum, panic, or drop the row).

## §5 Non-interference

- R18. No change to daemon, sim, store, event types, payloads, or the existing digest
  registry; `digest_test.go`'s catalog sweep passes unchanged.
- R19. No render-path regression: projection ingest is O(1) amortized per event;
  rendering derives per-frame views only from already-projected data.
