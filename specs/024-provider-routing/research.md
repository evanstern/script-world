# Research: Multi-Provider Routing — settled unknowns

All decisions below resolve Technical Context unknowns or contested mechanism choices.
Doctrine-level choices (chain order = quality statement; one wallet; no runtime scoring;
pin-per-scene; advisory leases) were settled in decision-5 and are not re-opened here.

## R1 — Cross-process lease mechanism: `syscall.Flock` on slot files

**Decision**: a lease pool per normalized endpoint is a directory
`~/.promptworld/endpoint-leases/<sha256[:16] of normalized endpoint>/` containing
`capacity` slot files `slot-00 … slot-(C-1)`. Acquiring = trying
`flock(LOCK_EX|LOCK_NB)` on each slot file in order; holding the fd holds the slot;
releasing = closing the fd. A goroutine that finds no free slot retries on a short jitter
ticker until its call context (worker cap, 2 min) expires.

**Rationale**: flock is advisory, crash-reclaimable (kernel drops the lock when the
process dies — SC "slots reclaimed without operator action" comes free), portable
macOS/Linux, and needs no daemon-to-daemon protocol, matching decision-5's "coordinate
through the OS". Slot files (vs one counter file) make capacity N a set of N independent
mutexes — no read-modify-write races, no fsync ordering concerns.

**Alternatives considered**: single counter file with fcntl record locks (read-modify-
write complexity, stale-count risk on crash); Unix socket arbiter daemon (a new process
to own, violates "no protocol"); documenting one-world-per-model only (TASK-24's status-
hint-only option — subsumed, kept as the undeclared-capacity default behavior).

**Normalization**: lowercase scheme+host, strip default ports (:80/:443), strip trailing
`/`, keep path (`/v1`). Two providers with cosmetically different spellings of one
endpoint hash identically; genuinely different paths stay distinct.

**Capacity mismatch across worlds**: each world creates slot files up to ITS declared
capacity and only locks slots < its capacity; the pool directory may hold more files than
a smaller-capacity world uses. Declaring different capacities for one endpoint in two
worlds is an operator inconsistency surfaced in docs (lowest declared wins in practice
for the world that declared it); no runtime reconciliation.

## R2 — Where lease acquisition happens: in the worker, after dequeue, before `caller.call`

**Decision**: the worker acquires the lease inside the per-job function, after the
stale-skip check, bounded by the same `callCtx` (2-min worker cap); lease wait does not
strike the breaker (it precedes the call) and does not count toward the estimator
(estimator samples remain call-duration only... measured from post-acquisition start).

**Rationale**: admission (`Submit`) must stay fail-fast per FR — blocking there would
stall callers that today get instant refusals. The worker is already the blocking layer,
and its call cap bounds the wait exactly as required by FR-015.

**Contended threshold**: waiting > 2 s sets the provider's `contended` atomic flag; the
flag clears on the next acquisition that waits < 2 s (hysteresis by observation, no timer
bookkeeping). 2 s ≈ two orders of magnitude above healthy acquisition, far below the
2-min cap.

## R3 — Scene pinning transport: `Request.Provider` (optional pin field)

**Decision**: `Request.Provider string` — empty means "route by chain" (all existing
callers unchanged); non-empty names a declared provider and bypasses chain-walking while
honoring ALL of that provider's admission checks (breaker, wallet if priced, queue).
Unknown name → `ErrUnknownProvider` (config drift guard, cannot happen with boot-time
config). The conversation layer resolves the pin once per scene via a new orchestrator
method `ResolveProvider(kind) (string, error)` (a dry chain-walk that returns the current
admissible head) and stamps it on every turn's request; on `convo.go`'s two Submit sites
the scene struct carries the pin.

**Rationale**: keeps the pin an explicit, replayable-irrelevant transport field rather
than hidden orchestrator session state; the orchestrator stays stateless per call. The
same field lets tests and the CLI one-shot force a provider.

**Alternative considered**: orchestrator-held per-scene sessions (stateful, lifecycle
questions — scene end cleanup — for no benefit).

## R4 — Meter attribution keys: total stays authoritative, per-provider keys additive

**Decision**: keep `llm_spend_YYYY-MM` (total) exactly as today — it remains what
`Allow()` reads, so legacy worlds' persisted spend carries forward. Add
`llm_spend_YYYY-MM:<provider>` per-provider keys written in the same `Add` call.
Attribution snapshot sums per-provider keys; an assertion test proves Σ(providers) ==
total for every test path. Legacy months (total without breakdown) display the total
under a synthetic `(unattributed)` row — never invented backfill.

**Rationale**: FR-010 restart-safety with zero migration; the total key's meaning is
unchanged so `Allow()` and existing tests hold.

## R5 — Estimator bootstrap classes: pricing class replaces tier for seeds

**Decision**: `cognition.SeedFor(profile, name)` keys the calibration profile by provider
name. Fallback when a provider has no profile entry: zero-priced providers seed from the
existing local bootstrap constant, priced providers from the cloud constant (pricing
class is decision-5's surviving local/cloud distinction). Legacy configs derive providers
named `local`/`cloud`, so existing tier-keyed calibration profiles keep matching by name
with no translation table.

**Rationale**: preserves TASK-32 semantics byte-for-byte for legacy worlds (P1 gate) and
gives v2 providers sane cold-start estimates.

## R6 — Config shapes: `providers` map + `routes` map; bare-array route shorthand

**Decision**: `"providers": {"<name>": {…}}` (map — names are keys, uniqueness free) and
`"routes": {"<kind>": ["a","b"]}` with object form
`{"chain": ["a","b"], "no_fallback": true}` accepted via custom `UnmarshalJSON`. Presence
of `providers` selects v2 parsing; presence of `local`/`cloud` without `providers`
selects legacy derivation; both present → load error (ambiguous). Kind names in routes
are validated against `llm.Kinds()` — unknown kind is a boot error (typo guard), missing
kind is a boot error (completeness, FR-003).

**Rationale**: map-shaped registry makes duplicate names unrepresentable; the shorthand
keeps the common case as terse as the doctrine ("the chain IS the statement").

**Transport field**: `"transport": "openai_compat" | "anthropic"` (named `transport`, not
`provider`, to avoid "provider.provider"); legacy `cloud.provider` maps onto it.

## R7 — Status shape: replace, don't dual-publish

**Decision**: `Status{Providers []ProviderStatus, Month, Spent, Budget}` with
`ProviderStatus{Name, Model, Endpoint, Up, Queue, Inflight, Slots, Contended, SpentUSD}`,
ordered by name for deterministic marshal. The fixed `Local`/`Cloud` fields are removed;
daemon, IPC, TUI, and `scriptworld`-style consumers ship together in this repo (spec
assumption). TUI renders the table in the pane that today shows tier health/spend.

**Rationale**: one shape for legacy and v2 worlds (legacy shows rows `local`, `cloud`);
dual-publishing would be permanent compat surface for zero external consumers.

## R8 — What does NOT change (verified against current sources)

- `health.go` breaker logic: untouched; instantiated per provider.
- Queue cap 32, priority-lane drain order, best-effort slot rule, stale-skip, worker call
  cap, successes-only estimator feeding, `SkipObserve`/`ObserveCognition` contract: all
  per-provider now, logic identical.
- `toolloop`, `metatron`, `consolidate`, `meeting`, `narrate` call sites: no changes
  beyond what the seam interfaces already carry (they never name tiers).
- Determinism/replay: no routing artifact enters recorded events' meaning (FR-018);
  telemetry additions are observational fields only.
