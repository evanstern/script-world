# Contract: Calibration-UX surfaces (spec 035)

Four operator-facing surfaces. All additive; nothing blocks, fails, or re-routes (spec FR-007).

## 1. Daemon boot warning (stdout, LLM worlds only)

Fires when `cognition.LoadProfile` yields no usable profile — both the absent-file branch and the
unreadable-file branch (which keeps its existing error note first).

Shape (wording may be polished; MUST contain the three bracketed elements):

```
daemon: WARNING — world is UNCALIBRATED: latency estimates are pessimistic bootstrap defaults
        (local 20 s/pt, cloud 10 s/pt)                                     [statement]
daemon: at these estimates: planner suppressed above 16x; conversation suppressed above 16x;
        meeting OK at 32x                                                   [horizon summary]
daemon: run `promptworld calibrate <world>` to measure this rig             [remedy, real world name]
```

- Horizon summary line = `cognition.HorizonSummary(BootstrapLocalSecPerPt)` — the identical
  string calibrate prints today (moved, not reimplemented).
- Profile-seeded boot output: byte-identical to today (the `calibration seeded (...)` line).

## 2. `set_speed` reply — additive warning field

`ipc.StatusData` gains:

```json
{ "warning": "uncalibrated world at 32x: planner, conversation suppressed at current estimates — run `promptworld calibrate <world>`" }
```

- `omitempty`; set ONLY on the `set_speed` command path. `status`/`pause`/`resume` replies never
  carry it (byte-identical).
- Populated when: world has an orchestrator AND at least one cognition class's serving provider
  (via `EstimateForKind`) is bootstrap-seeded AND `cognition.Route` at the requested speed with
  that provider's current estimate does not Allow that class. Class names listed in registry
  order.
- The speed change ALWAYS applies when validation passed — a warning-augmented success. The
  existing `max`-gate error path is untouched and takes precedence (an error reply has no
  warning).
- CLI `set-speed` rendering prints the warning after the normal status line. Clients ignoring
  the field behave exactly as today.

## 3. Status — per-provider calibration state

`llm.ProviderStatus` gains:

```json
{ "name": "local", "model": "...", "calibrated_at": "2026-07-20T21:40:00Z", ... }
```

- Present iff the provider's estimator was seeded from a usable profile entry; **absent means
  bootstrap-seeded (uncalibrated)** — no separate marker field.
- Carried verbatim from the profile's `calibrated_at`; never mutated by live adoption (spec 031)
  or estimator drift; restart re-derives from disk.
- CLI `status` rendering: calibrated providers show the timestamp; bootstrap providers show an
  explicit `uncalibrated (bootstrap)` marker in the human rendering (the wire stays omitempty).
- No-LLM worlds: no `llm` section at all, as today.

## 4. `promptworld calibrate` — sequential-floor disclosure

Once per run, adjacent to the horizon summary (both legacy and v2 paths — see research R6 for
the spec 024 byte-identity supersession):

```
note: calibration measures one call at a time; a live world runs N agents concurrently against
      the same endpoint, so measured s/pt is a floor — effective rate under load runs higher
      (the live estimator adapts at runtime).
```

MUST state: (a) measurement is sequential, (b) live load is concurrent so the number is a floor,
(c) the live estimator adapts at runtime. MUST be visible without scrolling away from the horizon
summary (SC-005).

## Test obligations

- ipc: set_speed warning present (uncalibrated + suppressing speed), absent (calibrated; or
  uncalibrated + non-suppressing speed; or no-LLM), speed applied in all cases; max-gate error
  unchanged. (US1 scenarios 1–4.)
- llm: SeedCalibration records `calibratedAt` per profile-entry presence incl. partial profiles;
  StatusSnapshot carries it; omitempty marshals verified.
- cognition: horizon helpers agree with `Route` across the ladder at bootstrap and calibrated
  values (property: summary says suppressed ⇔ Route disallows).
- daemon boot: warning block on absent AND unreadable profile; seeded line byte-identical.
- cmd: calibrate disclosure present in both paths; status/set-speed rendering of the new fields.
