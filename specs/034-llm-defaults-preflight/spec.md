# Feature Specification: Working fresh-world LLM defaults + loud dead-tier surfacing

**Feature Branch**: `034-llm-defaults-preflight`

**Created**: 2026-07-24

**Status**: Draft

**Input**: User description: "Fresh-world default LLM config can leave villagers silently brain-dead (TASK-84). Two problems to solve: (1) Silent failure — a fresh world whose configured local-tier model is absent from the endpoint (or whose tool_mode mismatches the model's capabilities) runs with zero successful planner tool calls and nothing fails loudly: minds plan, every call returns empty, villagers sit at the reflex floor forever. Add a loud surface: a startup/daemon preflight that checks the declared local model exists on the endpoint, and/or a repeated warning event when planner calls consistently return zero tool calls, so a dead tier is visible in status/attach instead of silent. (2) Default alignment — decide whether the shipped DefaultConfig local model should match the documented working guidance (cogito:3b + tool_mode json, per docs/llm-providers.md and TASK-73 eval evidence) or keep gemma4:12b-mlx and document the pull requirement prominently at `promptworld new` time; align DefaultConfig, docs/llm-providers.md, and README to the decision. Evidence: TASK-73 eval driver (specs/027-villager-prompt-quality/eval/decision.md) had to set model cogito:3b + tool_mode json before ANY planner call succeeded."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A dead local tier is loud, not silent (Priority: P1)

An operator starts (or creates and starts) a world whose configured local-tier
model is not actually available at the configured endpoint — the most common
fresh-machine case: the daemon boots, the sim runs, but every planner call
against that provider fails or returns nothing, and villagers never rise above
the reflex floor. Today nothing tells the operator; the world just looks
strangely inert. After this feature, the operator is told loudly and
persistently: at daemon start the world checks that each declared
locally-served model actually exists on its endpoint, and a missing model
produces an unmissable operator warning — in the boot/daemon log, in
`promptworld status`, and in the attach client — naming the provider, the
model, the endpoint, and the remedy (e.g. the exact pull command). The warning
persists while the condition holds and clears on its own once the model
becomes available.

**Why this priority**: this is the reported failure — a brand-new user's first
world is silently brain-dead and there is no signal anywhere. Every other part
of the feature is secondary to making the failure visible.

**Independent Test**: on a machine whose model server lacks the configured
model, run `promptworld new` + `promptworld start` and observe the warning in
the daemon log, `promptworld status`, and attach; then pull the model and
observe the warning clear without a restart being required for the clear.

**Acceptance Scenarios**:

1. **Given** a fresh world whose local provider names a model absent from the
   endpoint, **When** the daemon starts, **Then** an operator-facing warning
   naming the provider, model, endpoint, and remedy appears in the daemon's
   startup output/log, and the world still boots (this is never a boot error).
2. **Given** that same world running, **When** the operator runs
   `promptworld status` or attaches, **Then** the dead-provider condition is
   visible there — not only in scrollback from boot time.
3. **Given** the warning is active, **When** the operator pulls the model so
   the endpoint now serves it and traffic starts succeeding, **Then** the
   warning clears without requiring a world restart.
4. **Given** a world whose endpoint is entirely unreachable at boot, **When**
   the daemon starts, **Then** the operator gets an equally loud "endpoint
   unreachable" warning (distinct wording from "model missing"), and the world
   still boots.

---

### User Story 2 - Consistently tool-silent planner calls are loud (Priority: P2)

An operator runs a world whose local model exists and answers, but never emits
tool calls for planner work — the measured cogito-class failure: the call
succeeds at the transport level, returns prose or an empty result, and the
planner acts on nothing. A startup existence check cannot catch this (the
model is present; its tool-calling is what's broken). After this feature, the
world notices the pattern at runtime: when a provider's tool-expecting calls
keep coming back with zero tool calls, the operator gets a repeated warning
naming the provider and suggesting the known remedy (switching that provider's
tool-call strategy to the fallback envelope). The warning clears once tool
calls start landing.

**Why this priority**: second-most-common silent death, proven live during
TASK-73 bring-up (cogito:3b in native mode never function-calls). Runtime
detection is the only net that catches it — but the aligned defaults from
Story 3 make it rare on fresh worlds, so it ranks below Story 1.

**Independent Test**: configure a provider with a model known to never emit
native tool calls (tool-call strategy left native), run the world, and observe
the repeated warning after the detection threshold; flip the provider's
strategy to the fallback envelope, restart, and observe no warning.

**Acceptance Scenarios**:

1. **Given** a world whose planner provider consistently returns completions
   containing zero tool calls, **When** the pattern crosses the detection
   threshold, **Then** an operator-facing warning appears (log + status/attach)
   naming the provider and the suggested remedy.
2. **Given** the warning has fired, **When** the condition persists, **Then**
   the warning repeats/persists rather than firing once and scrolling away.
3. **Given** a healthy provider that emits tool calls normally, **When** the
   world runs a long session including occasional legitimately tool-free
   completions, **Then** no zero-tool-call warning ever fires (no false
   positives from isolated empty results).
4. **Given** a provider that serves only non-tool call kinds (conversation,
   meeting), **When** those calls return no tool calls (as expected), **Then**
   they never count toward the detection threshold.

---

### User Story 3 - Fresh-world defaults work out of the box (Priority: P2)

A new user on a stock machine (model server installed, one documented pull)
runs `promptworld new` and `promptworld start` and gets living villagers —
planner tool calls succeed with zero config editing. The shipped fresh-world
default local model + tool-call strategy is the configuration proven to work
(cogito:3b with the JSON fallback envelope, per the TASK-73 eval record), the
`promptworld new` output tells the operator exactly which model to pull, and
the operator guide (docs/llm-providers.md) and README describe the same
default — one consistent story across code and docs.

**Why this priority**: prevention beats detection — with a working default the
Story 1/2 warnings become the safety net instead of the first-run experience.
It ranks with Story 2 because both stem from the same default misalignment.

**Independent Test**: on a machine with only the documented default model
pulled, create and start a fresh world with no config edits and verify planner
tool calls succeed; grep the shipped default, operator guide, and README and
verify they name the same model + strategy.

**Acceptance Scenarios**:

1. **Given** a machine whose model server has only the documented default
   model pulled, **When** the operator creates and starts a fresh world with
   no config edits, **Then** villager planner tool calls succeed.
2. **Given** a fresh world is created, **When** `promptworld new` completes,
   **Then** its output states the local model the world expects and how to
   pull it.
3. **Given** the shipped default changes, **When** one reads
   docs/llm-providers.md and README, **Then** both name the same default model
   and tool-call strategy as the code writes into a fresh world.
4. **Given** an existing world created before this change, **When** it is
   restarted after upgrading, **Then** its behavior and configured models are
   unchanged (defaults only affect newly created worlds).

---

### Edge Cases

- Endpoint serves no model-listing capability (non-standard router): the
  existence check degrades gracefully — skipped with a low-key note, never a
  false "model missing" warning, and Story 2's runtime net still applies.
- Endpoint unreachable at boot (server not started yet): distinct
  "unreachable" warning (US1 scenario 4); when the server comes up later, the
  world recovers on its own (existing breaker behavior) and the warning
  clears.
- Model pulled while the world is running: the warning must clear without a
  daemon restart (US1 scenario 3).
- Providers on the managed-SDK transport (cloud tier): existence preflight
  does not apply (no local model registry to check); such providers are
  skipped without warning.
- Several providers share one endpoint: each provider's model is checked
  independently; one missing model must not mark sibling providers dead.
- Legitimately tool-free planner completions (model answers but declines to
  act): isolated instances never fire the Story 2 warning — only a sustained
  consecutive pattern does.
- A world whose operator deliberately routes planner traffic to a dead
  provider with a live fallback in the chain: warnings must name the specific
  dead provider, not declare the whole kind dead, since the chain still
  serves calls.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: At daemon start, the system MUST verify, for each declared
  provider served from a local-style (OpenAI-compatible) endpoint, that the
  configured model exists on that endpoint, wherever the endpoint exposes a
  model listing; providers on the managed cloud transport are exempt.
- **FR-002**: A failed existence check (model missing, or endpoint
  unreachable, with distinct wording for each) MUST produce an operator-facing
  warning naming the provider, model, endpoint, and concrete remedy — and MUST
  NOT fail the boot; the world always starts.
- **FR-003**: An active dead-provider condition MUST be visible in
  `promptworld status` output and in the attach client for as long as it
  holds — a persistent condition, not a one-time log line.
- **FR-004**: The dead-provider condition MUST clear automatically once the
  provider serves traffic successfully (or a re-check finds the model), with
  no world restart required to clear it.
- **FR-005**: The system MUST detect at runtime when a provider's
  tool-expecting calls consistently return zero tool calls (a sustained
  consecutive pattern, not isolated instances), and raise a repeated
  operator-facing warning naming the provider and suggesting the tool-call
  strategy remedy; calls of kinds that don't expect tool calls MUST NOT count.
- **FR-006**: The zero-tool-call warning MUST clear once the provider's
  tool-expecting calls produce tool calls again.
- **FR-007**: Fresh worlds MUST be created with a local-tier default
  configuration proven to produce planner tool calls out of the box:
  model `cogito:3b` with the JSON fallback tool-call strategy (decision
  grounded in the TASK-73 eval record; see Assumptions).
- **FR-008**: `promptworld new` MUST tell the operator which local model the
  new world expects and how to obtain it (the pull command).
- **FR-009**: docs/llm-providers.md and README MUST name the same fresh-world
  default model and tool-call strategy that world creation writes, presented
  as the default (other models, e.g. gemma-class, remain documented as
  upgrade options).
- **FR-010**: Existing worlds' configurations and behavior MUST be unaffected;
  the default change applies only to newly created worlds.

### Key Entities

- **Provider health condition**: an operator-visible state attached to a named
  provider — kind (model-missing / endpoint-unreachable / tool-silent), the
  evidence (model, endpoint, count), the remedy text, and whether it is
  currently active; surfaces in daemon log, status, and attach; clears
  automatically.
- **Fresh-world default config**: the LLM configuration written at
  `promptworld new` — the artifact that must stay aligned with the operator
  guide and README.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On a machine missing the configured local model, an operator
  creating and starting a fresh world sees the dead-tier warning within 30
  seconds of daemon start, in all three surfaces (daemon log, status, attach).
- **SC-002**: On a machine with only the documented default model pulled, a
  fresh world produces successful villager planner tool calls with zero
  configuration edits.
- **SC-003**: A healthy long-running world (existing soak setup) produces zero
  false dead-tier or tool-silent warnings over a full soak window.
- **SC-004**: The default model + strategy named by world creation, the
  operator guide, and the README are identical (verifiable by inspection);
  no document still presents the old default as what fresh worlds get.
- **SC-005**: A tool-silent provider (present model, wrong strategy) is
  flagged within the first minutes of sim activity rather than never —
  operator can identify the affected provider and remedy from the warning text
  alone, without reading source code.

## Assumptions

- **Default decision (FR-007) — derived from artifacts, not preference**: the
  TASK-73 eval record (specs/027-villager-prompt-quality/eval/decision.md) is
  the only live-proven fresh-world planner evidence: cogito:3b +
  JSON-envelope strategy sustained three 8-game-hour soaks (789–982 planner
  decisions each); the current default (gemma4:12b-mlx, native) is a
  machine-local MLX build unlikely to exist on a stock install, and the
  operator guide already documents cogito's need for the JSON envelope. A
  silently dead high-quality default loses to a working modest default;
  gemma-class models remain the documented upgrade path for operators who
  pull them.
- The existence check relies on the endpoint exposing its model list the way
  stock local servers (Ollama-class) do; endpoints that don't are skipped
  gracefully (edge case above) and covered by the runtime net.
- "Loud" means operator-facing surfaces (daemon log/stderr, status, attach) —
  not in-world narrative surfaces (chronicle/story feed); villagers and the
  narrator never see infrastructure health.
- The existing per-provider circuit breaker (transport-failure based) stays
  as-is; this feature adds visibility and a semantic (tool-silence) net on
  top, not a redesign of failure handling.
- Tuning values (detection threshold for consecutive tool-silent calls,
  re-check cadence for a missing model) are implementation-phase decisions;
  the spec constrains behavior (no false positives on isolated empties;
  warning within minutes, clears automatically).
- No config file migration: existing `llm.json` files load and behave
  unchanged (matches the established no-migration guarantee).
