// Package cognition is the deterministic substrate of the cognition horizon
// (decision-4, specs/007-cognition-horizon): the registry of model-reaching
// decision classes (Fibonacci-point thought costs + game-tick staleness
// budgets), the pure routing function that decides — without ever consulting
// a model — whether a decision may go to the LLM at the current speed, and
// the seconds-per-point calibration machinery that bridges the
// host-independent point scale to this deployment's measured latency.
//
// Purity rule: stdlib-only, and no imports from internal/mind, internal/sim,
// or internal/llm — both the mind (routing before enqueue) and the sim loop
// (budget lookup at the injection doors) depend on this package, never the
// reverse.
package cognition
