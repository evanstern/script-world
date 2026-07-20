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
)

// Rejection classification (FR-013): prediction-miss is an infrastructure
// signal (kept out of tuning heuristics as a spike); world-change means the
// world moved on — supersede/guards working as intended.
const (
	RejectKindPredictionMiss = "prediction-miss"
	RejectKindWorldChange    = "world-change"
)

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
