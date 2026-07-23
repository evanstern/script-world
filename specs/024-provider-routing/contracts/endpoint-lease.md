# Contract: advisory endpoint lease

Cross-world concurrency bound for a shared model endpoint (TASK-24). Opt-in per provider
via `endpoint_capacity`; absent = feature off, today's behavior.

## Layout

```
~/.promptworld/endpoint-leases/
└── <sha256[:16] of normalized endpoint>/
    ├── endpoint        # plaintext normalized endpoint, for operator inspection
    ├── slot-00
    ├── slot-01
    └── …               # up to slot-(C-1) for the largest capacity any world declared
```

- Normalization: lowercase scheme+host, strip `:80`/`:443` default ports, strip trailing
  slash, keep path. (`HTTP://LocalHost:11434/v1/` ≡ `http://localhost:11434/v1`.)
- Slot files are created lazily (0o644, empty); the `endpoint` file is written
  best-effort on pool creation.

## Protocol

1. Worker dequeues a job for a lease-enabled provider.
2. Sweep `slot-00 … slot-(C-1)` with `flock(LOCK_EX | LOCK_NB)`; first success holds the
   slot for the duration of the provider call; release = close(fd).
3. No free slot: retry the sweep on a jittered ~100 ms ticker until the job's call
   context (worker 2-min cap) expires — expiry surfaces as the call's context error,
   which does NOT strike the breaker when the caller is gone (existing worker rules).
4. Wait > 2 s → provider `contended` flag set; a subsequent wait < 2 s clears it.

## Guarantees and non-guarantees

- Crash-safe: the kernel releases flocks of a dead process; survivors acquire freed slots
  with no operator action (SC of US5).
- Advisory: only worlds declaring a capacity participate; a world without the declaration
  (or any other process) is invisible to the pool.
- Same-process pools contend correctly too (flock is per open-file-description), so two
  providers in one world sharing one endpoint (e.g. two models on one Ollama) share the
  bound if both declare it.
- Lease wait is never a health signal: it precedes the provider call; breaker and
  estimator are untouched by waiting (estimator measures from post-acquisition call
  start).
