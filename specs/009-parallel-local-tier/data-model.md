# Data Model: Parallel Local Tier

Entities from the spec, mapped to concrete Go types in `internal/llm`.

## LocalConfig (extended) — `internal/llm/config.go`

| Field | Type | JSON | Semantics |
|---|---|---|---|
| `Parallel` | `int` | `parallel,omitempty` | Requested local-tier concurrency. Absent/0 ⇒ 1. Normalized by `Workers()`. |

**Validation / normalization** (`Workers() (n int, warn string)`):

| Raw value | Effective | Warning |
|---|---|---|
| absent / 0 | 1 | none (compat default, FR-001) |
| 1–16 | as given | none |
| < 0 | 1 | yes (FR-007) |
| > 16 | 16 | yes (FR-007) |

Normalization never errors; `LoadConfig` accepts any integer (a world never fails
to boot over this field). `WriteDefault`/`DefaultConfig` omit the field (default 1).

## Tier slot — `internal/llm/llm.go` (`tier` struct, extended)

| Field | Type | Semantics |
|---|---|---|
| `slots` | `int` | Number of worker goroutines = concurrent capacity. Local: `cfg.Local.Workers()`. Cloud: always 1 (FR-008). |
| `inflight` | `atomic.Int32` | Jobs dequeued by a worker and not yet replied. Incremented at dequeue, decremented on every reply path (stale-skip, error, success). |

Invariant: `0 ≤ inflight ≤ slots` at all times; `len(queue)+len(prio) > 0` implies
`inflight == slots` (modulo the instant a worker is between dequeue operations).

## Admission verdict — `Orchestrator.Submit`

Decision order (unchanged except the best-effort clause):

1. Unknown kind → `ErrUnknownKind`
2. Cloud + budget exhausted → `ErrBudgetExhausted`
3. Circuit open → `ErrTierDown`
4. **Best-effort with no free slot** → `ErrTierBusy` where "no free slot" ≡
   `len(queue) > 0 ∨ len(prio) > 0 ∨ inflight ≥ slots` (FR-003)
5. Queue full → `ErrQueueFull`
6. Otherwise → enqueued (prio for `KindConversation`, queue for the rest), FIFO
   within its channel; competing free workers receive in FIFO order and prefer
   `prio` when both channels have work (FR-002)

## Latency estimate — `internal/cognition.Estimator` (unchanged)

Fed per completed call by whichever worker ran it:
`Sample(wallSeconds / points)`. Mutex-protected; concurrent completions serialize
on the estimator's lock. Under concurrency the sampled wall time includes
server-side contention — the estimate converges on true concurrent-rate
seconds-per-point (FR-004). Breach verdict (`recalibrate == true`) is returned to
exactly one caller per breach episode under the same lock.

## Spend meter — `internal/llm.Meter` (unchanged)

`Add(cost)` is mutex-protected and persists under the lock; N concurrent cloud
completions serialize, final total = exact sum of per-call costs (FR-006). (Cloud
stays 1-wide in this feature; the property is proven by test anyway, SC-005.)

## State transitions

```
Submit(best-effort): free slot ─────────────▶ enqueued ─▶ worker dequeues (inflight++)
                     no free slot ──────────▶ ErrTierBusy (dropped, no retry)

worker: dequeue ─▶ ctx already dead ─▶ reply ctx.Err (inflight--)   [no breaker strike]
                └▶ call provider ─▶ ok   ─▶ health.succeed, est.Sample, meter (cloud), reply (inflight--)
                                 └▶ err  ─▶ health.fail (unless caller ctx died), reply (inflight--)
```
