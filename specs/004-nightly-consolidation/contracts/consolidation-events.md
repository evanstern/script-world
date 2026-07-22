# Contract: Consolidation Events & Injection Door

## Trigger

`internal/mind` observes `agent.slept` (executor-emitted). Guard, evaluated on the mind's
replica:

```
night      := sleepTick / 86400
due        := night > agent.LastConsolidatedNight
gap        := sleepTick - agent.LastConsolidateMark >= 43200   // 12 game-hours
nonEmpty   := exists m in agent.Memories with m.Tick > agent.ConsolidatedUpTo
alive      := !agent.Dead
```

- `due && gap && alive && !nonEmpty` → inject marker `outcome: "skipped_empty"` (no call).
- `due && gap && alive && nonEmpty` → enqueue for the (single-flight, FIFO) consolidation
  worker; one `llm.KindConsolidation` call.
- Transport/model-tier failure (ErrTierDown, ErrBudgetExhausted, timeout): **no marker**
  — the attempt never happened as far as the ledger cares; the next `agent.slept`
  (typically next night) retries with the grown buffer. The world never blocks (FR-002).

## Landing

The existing whitelisted-batch loop command (TASK-8's `inject_social` door) is extended
to admit:

```
agent.memory_promoted
agent.memory_faded
agent.belief_revised
agent.narrative_set
agent.consolidated
```

(alongside the existing social/memory types). Door contract unchanged: dry-run the whole
batch on a state copy, re-stamp ticks, apply + append + notify atomically; any invalid
event rejects the whole batch.

**Accepted night** — one batch, in order: `agent.memory_promoted`×N,
`agent.memory_faded`×N, `agent.memory_added` (day gist), `agent.belief_revised`×N,
`agent.narrative_set`, `agent.consolidated{outcome: accepted, up_to, counts, cost_usd}`.

**Rejected night** — one batch: `agent.consolidated{outcome: rejected, reason}` alone.
Buffer untouched (`ConsolidatedUpTo` unchanged) → next night digests the backlog (SC-005).

**Empty night** — one batch: `agent.consolidated{outcome: skipped_empty}`.

## Reducer semantics

See data-model.md. All five types are reducer-total (no-op on vanished targets); replay
of any recorded history reproduces byte-identical state with zero model calls (FR-005,
SC-004).

## Observability (FR-010)

- Every outcome is an `agent.consolidated` event: visible in `promptworld tail`, the TUI
  event feed, and countable from the log.
- daemon.log lines: `mind: consolidation <agent> night <n> accepted (p/f/b counts, $cost)`
  / `rejected (<reason>)` / `skipped (empty)` / `deferred (<transport error>)`.
- Cloud spend rides the existing meter (`cost_usd` also recorded per marker).

## Firewall (structural half)

No new write path to persona files: `internal/persona` keeps its genesis-only writer;
consolidation code has no filesystem access. Test asserts persona.md bytes are identical
before/after a full consolidation cycle and that the persona package exposes no
post-genesis write API.
