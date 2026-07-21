package sim

// Cognition-horizon telemetry (TASK-32, specs/007-cognition-horizon).
// These event types are recorded observability with zero state effect:
// reducer no-ops whitelisted on the inject_social door (cog.*) or emitted by
// the loop itself alongside the verdict they describe
// (agent.intent_rejected). Payload field order is canonical — histories are
// byte-comparable (contracts/events.md).

// Thought outcomes: every requested thought terminates in exactly one
// (FR-015). Silent failure is eliminated.
const (
	OutcomeLanded        = "landed"
	OutcomeAdapted       = "adapted"
	OutcomeRejectedStale = "rejected-stale"
	OutcomeRejectedGuard = "rejected-guard"
	OutcomeSuperseded    = "superseded"
	OutcomeExpired       = "expired"
	OutcomeUnavailable   = "rejected-unavailable"
	OutcomeUnusable      = "unusable"
	OutcomeSuppressed    = "suppressed"
	// OutcomeRetried is a NON-TERMINAL marker (TASK-42, conversation
	// robustness): one scene reply failed to parse and the scene continued
	// via one retry. It carries the failed reply's raw text; consumers that
	// sum job completions MUST filter it out (contracts/telemetry.md rule 1).
	OutcomeRetried = "retried"
)

// Rejection classification (FR-013): prediction-miss is an infrastructure
// signal (kept out of tuning heuristics as a spike); world-change means the
// world moved on — supersede/guards working as intended.
const (
	RejectKindPredictionMiss = "prediction-miss"
	RejectKindWorldChange    = "world-change"
)

// GenerationBumpSalience: an agent.memory_added at or above this salience
// bumps Agent.Generation (FR-014). The salience table defines "emergency":
// near-death 9, witnessed death 10, exile 9 — dreams (8) do not interrupt.
const GenerationBumpSalience = 9

// PredictionMissFactor: a landing whose actual wall time exceeded its
// prediction by this factor is classified prediction-miss, not world-change
// — infra noise that must stay out of budget-tuning heuristics (FR-013).
const PredictionMissFactor = 3

// CogThoughtPayload — cog.thought: a model call passed the router and was
// enqueued. trigger_seq is the event-log seq of the stimulus that armed the
// trigger (0 = pure cadence): the causality edge stimulus → thought.
type CogThoughtPayload struct {
	Job               string `json:"job"`
	Class             string `json:"class"`
	Agent             int    `json:"agent"`
	SnapshotTick      int64  `json:"snapshot_tick"`
	Generation        int64  `json:"generation"`
	TriggerSeq        int64  `json:"trigger_seq"`
	Points            int    `json:"points"`
	PredictedWallMs   int64  `json:"predicted_wall_ms"`
	PredictedLandTick int64  `json:"predicted_land_tick"`
}

// CogOutcomePayload — cog.outcome: the single terminal record of a thought.
// Router suppressions carry the routing arithmetic in reason and have no
// matching cog.thought (no call was made).
type CogOutcomePayload struct {
	Job             string `json:"job"`
	Class           string `json:"class"`
	Agent           int    `json:"agent"`
	Outcome         string `json:"outcome"`
	SnapshotTick    int64  `json:"snapshot_tick"`
	LandingTick     int64  `json:"landing_tick"`
	StalenessTicks  int64  `json:"staleness_ticks"`
	PredictedWallMs int64  `json:"predicted_wall_ms"`
	ActualWallMs    int64  `json:"actual_wall_ms"`
	Kind            string `json:"kind,omitempty"`
	Reason          string `json:"reason,omitempty"`
	// Raw / Retried (TASK-42): raw is the verbatim model reply on a scene
	// parse failure (bounded, truncated on a rune boundary); retried marks a
	// terminal scene outcome whose run consumed ≥1 retry. Both omitempty, so
	// every pre-TASK-42 emission stays byte-identical (FR-009).
	Raw     string `json:"raw,omitempty"`
	Retried bool   `json:"retried,omitempty"`
}

// IntentRejectedPayload — agent.intent_rejected: the loop refused a landing
// intent. Its own type (not just telemetry) so souls/chronicle can later
// notice refused intentions without parsing cog.* payloads.
type IntentRejectedPayload struct {
	Agent          int    `json:"agent"`
	Goal           string `json:"goal"`
	Reason         string `json:"reason"`
	StalenessTicks int64  `json:"staleness_ticks"`
}

// RecalibrationPayload — cog.recalibration_recommended: the live estimator's
// spike rate breached the drift threshold (once per breach episode).
type RecalibrationPayload struct {
	Tier           string  `json:"tier"`
	EstimateSPerPt float64 `json:"estimate_s_per_pt"`
	SpikeRate      float64 `json:"spike_rate"`
	Window         int     `json:"window"`
}
