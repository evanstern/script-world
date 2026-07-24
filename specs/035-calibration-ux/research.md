# Research: Calibration UX (spec 035)

No NEEDS CLARIFICATION markers survived specify; this file records the design decisions and the
alternatives weighed, grounded in the existing artifacts (spec 007 doctrine, spec 031, TASK-39/40
field evidence).

## R1 — Where the suppression arithmetic lives

**Decision**: lift `horizonSummary` (cmd/promptworld/calibrate.go:241) into
`internal/cognition/horizon.go` as exported, stdlib-only helpers: a per-class suppression check at
one (speed, sec/pt) point and the ladder-wide summary string. Daemon boot, ipc set_speed, and
calibrate all call the same code.

**Rationale**: spec FR-006 demands the warning never disagree with the router. The router is
`cognition.Route` (mind/telemetry.go:70); putting the warning arithmetic next to `Route` in the
same leaf package makes agreement structural rather than tested-for. `internal/cognition` is
stdlib-only and imported by llm, mind, daemon and cmd already — no dependency direction problems.

**Alternatives considered**: (a) keep it in cmd and re-implement in ipc — two arithmetics that can
drift, rejected; (b) a new `internal/horizon` package — needless package for ~40 lines that belong
with `Route`/`ClassFor`, rejected.

## R2 — Warning gate vs warning arithmetic (seed state vs live estimates)

**Decision**: the *gate* is seed state — only providers whose estimator was seeded from bootstrap
(no usable profile entry) can trigger boot/speed warnings. The *arithmetic* is the router's own:
`cognition.Route` with the serving provider's **current** estimate via
`Orchestrator.EstimateForKind` (llm.go:574). At boot these coincide (live estimate == seed).

**Rationale**: FR-006 (never disagree with the router at the moment of warning) plus the edge case
where an uncalibrated world's estimator has already converged out of suppression — warning there
would be false and would teach operators to ignore the warning. Gating on seed keeps calibrated
worlds warning-free (SC-002) regardless of live drift (drift signals belong to spec 031/033).

**Alternatives considered**: evaluating at frozen bootstrap constants — deterministic but can emit
false warnings after live convergence, rejected; gating on live estimates only (no seed check) —
would warn on calibrated-but-loaded worlds, violating SC-002, rejected.

## R3 — How "calibrated" is known per provider

**Decision**: `Orchestrator.SeedCalibration` (llm.go:504) already receives the profile and decides
per provider whether a named entry exists (via the same presence check `cognition.SeedFor` uses);
it additionally records `calibratedAt` (the profile's `calibrated_at`) on the provider when the
entry exists, empty when bootstrapped. `StatusSnapshot` copies it into `ProviderStatus` as an
additive `calibrated_at,omitempty` field; a small read (provider name → calibratedAt) serves the
ipc warning gate. Absence of the field IS the bootstrap marker — one field carries FR-004.

**Rationale**: SeedCalibration is the single place seed provenance is decided today; recording the
outcome there is one assignment, not new machinery. Empty-means-bootstrap avoids a second
enum-like field and keeps no-LLM/omitempty byte-identity (FR-008).

**Alternatives considered**: daemon-level bookkeeping outside the orchestrator — but ipc only
holds the orchestrator handle, so state would need new plumbing through Server, rejected; a
separate `seed_source` string field — redundant with the timestamp's presence, rejected.

## R4 — Where the set_speed warning rides

**Decision**: additive `Warning string json:"warning,omitempty"` on `StatusData`
(ipc/protocol.go:108), populated only on the `set_speed` command path (server.go:285 area) after
validation succeeds; the speed change applies unconditionally. CLI `set-speed` rendering prints it
to stderr-style prominence; other commands sharing StatusData never set it.

**Rationale**: StatusData is already the set_speed reply shape; an omitempty field keeps pause/
resume/status byte-identical and old clients simply ignore it (FR-008, edge case "clients that
ignore the new field behave exactly as today"). Precedent: the additive governor fields on
ClockStatus (spec 028) used exactly this pattern.

**Alternatives considered**: a separate warnings array `[]string` — nothing today produces more
than one message; a single string composed of the class list keeps rendering trivial (YAGNI, can
widen additively later); an event (`cog.*`) into the chronicle — operator-facing plumbing, not
world history, and the reply is the surface the acting operator actually sees, rejected as the
primary channel.

## R5 — Boot warning content

**Decision**: both no-profile and unreadable-profile branches (daemon.go:153-162) print a warning
block: uncalibrated statement + `cognition.HorizonSummary(BootstrapLocalSecPerPt)` per-class
ladder summary + `run: promptworld calibrate <world-name>` with the actual world name. The
existing seeded-profile line is untouched (SC-002).

**Rationale**: US2 scenarios; the malformed-profile path already falls back to bootstrap so it is
"uncalibrated" in every sense that matters (spec edge case). Local/zero-priced bootstrap is the
summary's input because every watchable class routes local-first today and the local constant is
the pessimistic one — same choice horizonSummary already makes in calibrate.

**Alternatives considered**: per-provider summaries at boot — the classes are what operators watch
and the calibrate horizon summary is already class-keyed; per-provider truth lives in status
(FR-004), rejected for boot verbosity.

## R6 — Calibrate output disclosure vs spec 024's byte-identical guarantee

**Decision**: add the sequential-floor disclosure line(s) to both the legacy and v2 calibrate
paths, adjacent to the horizon summary (and once per run, not per provider). Spec 024 T020's
"legacy output byte-identical" guarantee was a **transition invariant for the 024 refactor**
(proving the provider generalization changed nothing), not a permanent freeze; spec 035 FR-005
deliberately supersedes it for this one addition. Recorded here so the 024 contract's history
stays coherent.

**Rationale**: US4 applies to every operator running calibrate regardless of config generation;
excluding legacy configs would hide the disclosure from exactly the older setups most likely to
be uncalibrated.

**Alternatives considered**: v2-only disclosure — splits the UX by config age for no doctrine
reason, rejected.

## R7 — Doctrine review: bootstrap default

**Decision**: `BootstrapLocalSecPerPt`/`BootstrapCloudSecPerPt` unchanged (spec Doctrine Review
section). Closes TASK-40's "revisit bootstrap default".

**Rationale**: decision-4 — fail toward reflex, never stale action; lowering the seed converts a
visible impoverishment into invisible corruption. Spec 031 already gives worlds a live path out of
the pessimistic seed once samples flow; the residual gap is visibility, which this feature is.

**Alternatives considered**: lowering to ~2 s/pt (modern-rig median) — rejected per doctrine; a
per-rig heuristic probe at daemon start — that IS calibration, just unsolicited and slower to
boot, rejected.

## R8 — Warning fatigue / statefulness

**Decision**: stateless — every set_speed reply landing in suppressing territory on a
bootstrap-seeded world carries the warning; no dedup, no per-session memory.

**Rationale**: simplicity first (spec assumption); the reply-field channel means the warning costs
one line, and repeated sightings are arguably the point for an ignored calibrate suggestion.
Revisit only on operator complaint.
