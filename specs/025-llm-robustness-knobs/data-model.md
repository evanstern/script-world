# Phase 1 Data Model: llm.json robustness knobs (TASK-72)

## 1. `llm.Config` — new field

```
Config
└── MaxTokens TokenBudgets   json:"max_tokens,omitempty"     (new)

TokenBudgets
├── Planner       int64      json:"planner,omitempty"        // villager tool-loop round budget
├── MetatronTurn  int64      json:"metatron_turn,omitempty"  // metatron console-turn round budget
└── Consolidation int64      json:"consolidation,omitempty"  // nightly consolidation budget
```

### Normalization (per field, mirrors `Config.Rounds()`)

Each field normalizes independently to `(effective int64, warn string)`:

| Raw value      | Effective            | Warning |
|----------------|----------------------|---------|
| absent / 0     | kind default         | none    |
| 1 … 4096       | as given             | none    |
| < 0            | kind default         | `llm.json max_tokens.<key> <v> out of range (min 1) — using <default>` |
| > 4096         | 4096                 | `llm.json max_tokens.<key> <v> out of range (max 4096) — clamped to 4096` |

Kind defaults (must equal today's hardcodes, spec FR-007/FR-010):

| Key             | Default | Replaces |
|-----------------|---------|----------|
| `planner`       | 512     | `loopMaxTokens` const, `internal/mind/mind.go:378` |
| `metatron_turn` | 1024    | `turnMaxTokens` const, `internal/metatron/turn.go:41` |
| `consolidation` | 1024    | inline `MaxTokens: 1024`, `internal/mind/consolidate.go:133` |

Shared bound constant: `maxTokenBudget = 4096` (one constant, three knobs).
Boot never fails on these fields (warn-not-error doctrine); warnings surface via
the daemon's existing `"daemon: %s"` boot channel next to the `Workers()` /
`Rounds()` / `ToolModeResolved()` warnings (`internal/daemon/daemon.go:158-175`).

### Explicitly NOT governed (spec FR-009)

Conversation utterance 128 (`convo.go:424`), conversation outcome 224
(`convo.go:466`), meeting 72 (`meeting.go:21`), narrator 800 (`narrate.go:34`),
metatron digest 400 (`digest.go:26`) — unchanged hardcodes.

## 2. `toolloop.Result` — new fields

```
Result
├── Final       string          (existing)
├── Landed      *llm.ToolCall   (existing)
├── Rounds      int             (existing)
├── TotalMillis int64           (existing)
├── Term        Termination     (existing)
├── Retried     bool            (new) — the run consumed its one transport retry
└── RetryReason string          (new) — first failure's error text (empty unless Retried)
```

Invariants:
- `Retried` is set at most once per run (the retry budget is one per cognition run).
- `RetryReason` non-empty ⇔ `Retried` true.
- A retried-and-recovered run terminates in the success family with `Retried: true`;
  a twice-failed run terminates `provider_error` with `Retried: true` and the
  SECOND error propagated exactly as today's single failure is.

## 3. Retry state machine (inside `run()`, per cognition)

```
                     Submit error
                          │
          terminationForSubmitErr(serr)
          ┌───────────────┼──────────────────┐
   ctx_done        admission_refused   provider_error
      │                   │                  │
 terminate           terminate        retried already?
 (as today)          (as today)      ┌───────┴───────┐
                                     no              yes
                                     │                │
                            Retried=true         terminate
                            RetryReason=err      provider_error
                            re-Submit same       (as today,
                            transcript           2nd error)
                            (no round consumed)
```

Not in scope of this machine: tool-handler infra failures (`loop.go:269/287`) —
the model call succeeded; they terminate `provider_error` as today, never retried.

## 4. Trail visibility (consumers)

| Consumer | Trigger | Emission |
|----------|---------|----------|
| `mind.runPlan` | `res.Retried` | non-terminal `cog.outcome`, outcome `sim.OutcomeRetried` (`"retried"`), reason = `RetryReason`, via existing `cogOutcomeEvent` family + `emitCog` |
| `metatron.Turn` | `res.Retried` | same event shape through metatron's InjectSocial door (the `cog.tool_call` batch channel, `toolcalls.go`) |

No new event type; `cog.outcome`/`retried` is already cataloged (TASK-42 precedent,
`sim/cognition.go:24-28`) — digest catalog (`TestCatalogSweep`) stays green. The
non-terminal retried marker precedes whatever terminal outcome the run earns, same
ordering discipline as the conversation scene's retried marker.

## 5. Constructor plumbing

```
daemon boot (daemon.go)
  budgets := resolve cfg.MaxTokens (3× normalize, print warnings)
  mind.New(…, loopRounds, plannerBudget, consolidationBudget)   // signature grows
  metatron.New(…, loopRounds, turnBudget)                        // signature grows

mind.Mind:      + plannerTokens, consolidationTokens int64 fields
metatron.Metatron: + turnTokens int64 field

call sites read the injected fields:
  mind.go:405        MaxTokens: md.plannerTokens
  consolidate.go:133 MaxTokens: md.consolidationTokens
  turn.go:157        MaxTokens: mt.turnTokens
```

`cmd/promptworld/calibrate.go` follows the signature change mechanically (it
already resolves `Rounds()` at `calibrate.go:269`).
