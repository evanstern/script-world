# Data Model: Multi-Provider Routing

## Configuration entities (llm.json)

### Config (v2)

| Field | Type | Rules |
|-------|------|-------|
| `monthly_budget_usd` | float | > 0 (unchanged) |
| `providers` | map[name]ProviderConfig | v2 marker; ≥ 1 entry; names non-empty, unique by construction |
| `routes` | map[kind]RouteConfig | exactly the kinds `llm.Kinds()` accepts — no more, no fewer |
| `loop_max_rounds` | int | unchanged (top-level, clamp doctrine) |
| `local` / `cloud` | LocalConfig / CloudConfig | legacy shape; mutually exclusive with `providers` (both present → load error) |

### ProviderConfig

| Field | Type | Rules |
|-------|------|-------|
| `transport` | string | `"openai_compat"` \| `"anthropic"`; required |
| `endpoint` | string | required for `openai_compat`; optional override for `anthropic` |
| `model` | string | required |
| `input_usd_per_mtok` / `output_usd_per_mtok` | float | ≥ 0; both 0 ⇒ zero-priced (never budget-refused, local-class seed) |
| `api_key_env` | string | env var NAME (never a key) |
| `api_key` | string | inline, LAN routers only; wins over env when both set (unchanged rule) |
| `parallel` | int | clamp 1–16 warn-not-error (unchanged `Workers()` doctrine, per provider) |
| `reasoning_effort` | *string | nil/"" convention unchanged; nil default: `"none"` for zero-priced, omit for priced (mirrors today's local/cloud defaults) |
| `tool_mode` | string | `""`/`native`/`json`, warn-clamp (unchanged; `anthropic` transport ignores) |
| `endpoint_capacity` | int | optional; > 0 enables the advisory lease pool for this provider's normalized endpoint; absent/0 = leases off (today's behavior) |

### RouteConfig

JSON forms: `["a","b"]` (shorthand) or `{"chain": ["a","b"], "no_fallback": true}`.

| Field | Type | Rules |
|-------|------|-------|
| `chain` | []string | non-empty; every name declared in `providers`; no duplicates within a chain |
| `no_fallback` | bool | true ⇒ only chain[0] is ever considered (chain len > 1 with no_fallback → load error: contradiction) |

### Legacy derivation (loading `local`/`cloud` shape)

- provider `"local"` ← LocalConfig (transport `openai_compat`, pricing 0/0, parallel, reasoning_effort, tool_mode)
- provider `"cloud"` ← CloudConfig (transport ← `cloud.provider` mapping: ""/`anthropic` → `anthropic`; `openai_compat` → `openai_compat`; pricing, endpoint, keys, reasoning_effort, tool_mode; parallel = 1)
- routes ← today's table: planner/conversation/meeting → `["local"]`; consolidation/narrator/drama/metatron → `["cloud"]`; no `no_fallback` flags (single-entry chains are already fallback-free)
- `endpoint_capacity` absent (leases off) — legacy worlds behave byte-identically

## Runtime entities (internal/llm)

### provider (replaces `tier`)

name, cfg (ProviderConfig), caller, health (breaker), queue+prio chans (cap 32 each),
slots, inflight (atomic), est (\*cognition.Estimator), leases (\*leasePool, nil when
capacity undeclared), contended (atomic bool), spent accessor via meter.

Invariants (all carried over per-provider): 0 ≤ inflight ≤ slots; breaker counts only
genuine provider failures; estimator fed on success only, whole-cognition observations
via `ObserveCognition(kind, provider, ms)` route to the serving provider's estimator.

### route

`{chain []*provider, noFallback bool}` — resolved once at `New()` from validated config;
immutable thereafter.

### Chain-walk admission (Submit)

For each candidate in chain (or chain[:1] if noFallback / req.Provider pin):
1. priced && !meter.Allow() → skip `wallet-exhausted`
2. breaker open → skip `circuit-open`
3. best-effort && (queued work || inflight ≥ slots) → skip `busy`
4. enqueue attempt; queue full → skip `queue-full`
5. accepted → dispatch; record skipped list

All skipped → return chain-head's refusal error (`ErrBudgetExhausted` / `ErrTierDown` /
`ErrTierBusy` / `ErrQueueFull` per its reason). `Request.Provider` unknown →
`ErrUnknownProvider`.

### Request / Response additions

- `Request.Provider string` (optional pin; empty = chain routing)
- `Response.Provider string` (serving provider name; always set)
- `Response.Skipped []RouteSkip` where `RouteSkip{Provider, Reason string}` — empty on
  head dispatch; reasons: `circuit-open` | `wallet-exhausted` | `queue-full` | `busy`

### Meter

- `Allow()` unchanged (reads total vs ceiling)
- `Add(provider string, costUSD float64)` — writes total key `llm_spend_YYYY-MM` AND
  `llm_spend_YYYY-MM:<provider>` atomically under mu
- `Snapshot()` → (month, spent, budget, perProvider map[string]float64); Σ(perProvider) +
  unattributed == spent

### leasePool

- dir: `~/.promptworld/endpoint-leases/<sha256[:16](normalizedEndpoint)>/`
- `acquire(ctx) (release func(), waited time.Duration, err)` — non-blocking flock sweep
  over `slot-00…slot-(C-1)`, jittered retry until ctx done
- contended flag: waited > 2 s sets; waited < 2 s clears. The flag lives on the POOL
  (providers sharing a normalized endpoint share one pool and one flag — endpoint
  congestion is one truth); `StatusSnapshot` reads it per provider row

### Status

`Status{Providers []ProviderStatus, Month, Spent, Budget}` —
`ProviderStatus{Name, Model, Endpoint, Up, Queue, Inflight, Slots, Contended, SpentUSD}`,
sorted by Name.

## Cognition seam

- `cognition.SeedFor(profile, name)`: profile keyed by provider name; miss → bootstrap
  by pricing class (zero-priced → local constant, priced → cloud constant). Signature
  gains the class: `SeedFor(p *Profile, name string, zeroPriced bool)` (or equivalent).
- Orchestrator exports: `ProviderNames() []string`, `EstimateForKind(kind) (name string,
  secPerPoint float64, ok bool)` (deterministic: current admissible head, falling back to
  chain head when none admissible), `ResolveProvider(kind) (string, error)`.
  `TierFor`/`SecondsPerPoint(tier)` retire; `internal/mind/telemetry.go` consumes the new
  seam.

## State transitions

- Provider health: closed → open (3 consecutive genuine failures) → half-open (backoff
  15 s→5 min) → closed (probe success). Unchanged, per provider.
- Contended: false → true (lease wait > 2 s) → false (next wait < 2 s).
- Scene pin: unresolved → pinned(name) at scene start → (never changes; scene ends).
- Month rollover: total and all per-provider keys roll together (same `rollover()`).
