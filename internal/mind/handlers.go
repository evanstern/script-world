package mind

// Villager tool-use loop handlers (spec 017, TASK-52 T011). Every acting tool
// a villager may call wraps an existing landing door — the loop never mutates
// the world, it REQUESTS through the door and translates the door's verdict:
//
//   - world verbs + set_plan  → Loop.InjectIntent (the intent/plan ladder)
//   - muse                    → Loop.InjectSocial (the whitelisted social door)
//
// Doctrine (spec 017): a tool call is a REQUEST; the door decides; the executor
// grounds. Read-effect tools are supported generically (dispatched by effect)
// so test-fixture read tools exercise the loop's read path, but the production
// villager roster ships none this task (TASK-16 brings the journal tools).

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/tool"
	"github.com/evanstern/promptworld/internal/toolloop"
)

// villagerDispatch carries one cognition's loop state: the job it runs against,
// the wall-clock start (whole-loop elapsed is the ActualWallMs a landing
// records, matching the governor's whole-loop observation unit), the buffered
// call records (T018 sink), and the door-outcome flag runPlan reads to decide
// the terminal cog.outcome and the rearm (see runPlan / mind.go).
type villagerDispatch struct {
	md    *Mind
	job   planJob
	start time.Time

	// records buffers every CallRecord the driver emits for this cognition.
	// TODO(T018): land these as cog.tool_call events through the telemetry
	// door (internal/mind/telemetry.go). For now they are held only for the
	// duration of the loop and dropped — no durable artifact yet.
	records []toolloop.CallRecord

	// doorOutcome is true once any acting handler drove a door that emitted a
	// cog.outcome — a landed intent/plan/muse, or a gate rejection (both the
	// intent ladder and the muse social batch record their verdict atomically).
	// It is NOT set for a rejection the handler raises before touching a door
	// (unknown talk_to target, unparseable plan): those carry no door record,
	// so runPlan must still emit the terminal FR-015 outcome. runPlan reads it
	// to avoid double-emitting an outcome the door already recorded, and to
	// mirror today's rearm-on-rejection (mind.go runPlan).
	doorOutcome bool
}

// villagerHandlers builds the handler map the tool-use loop dispatches against
// for one villager cognition. Read-effect tools (none in production this task)
// are wired generically so a test roster's read tool is dispatched by effect;
// acting tools wrap their landing door.
func (md *Mind) villagerHandlers(d *villagerDispatch) map[string]toolloop.Handler {
	handlers := map[string]toolloop.Handler{}
	for _, t := range tool.LoopRosterVillager() {
		switch {
		case t.Name == "set_plan":
			handlers[t.Name] = d.handleSetPlan
		case t.Name == "muse":
			handlers[t.Name] = d.handleMuse
		case t.Effect == tool.World:
			handlers[t.Name] = d.handleWorldVerb(t.Name)
		case t.Effect == tool.Read:
			// Generic read dispatch: return data, ground nothing. No production
			// read tool ships this task; the seam keeps test rosters honest.
			handlers[t.Name] = d.handleRead(t.Name)
		}
	}
	return handlers
}

// record is the Job.Record sink — one CallRecord per model tool call.
func (d *villagerDispatch) record(r toolloop.CallRecord) {
	d.records = append(d.records, r)
}

// handleWorldVerb wraps Loop.InjectIntent for one world verb, mirroring exactly
// the InjectArgs fields runPlan set today (minus the free-text reason, which the
// tool era carries via the muse tool rather than a per-action field — the world
// verbs declare no reason param). talk_to keeps its mind-side alive/present
// guards, built from the job's snapshot worldview. Door accept → landed; door
// reject → rejected_gate carrying the door's queryable reason.
func (d *villagerDispatch) handleWorldVerb(name string) toolloop.Handler {
	return func(_ context.Context, call llm.ToolCall) toolloop.Outcome {
		target := -1
		var guards []sim.Guard
		if name == "talk_to" {
			tname := argString(call.Args, "target")
			if target = d.md.agentIndexByName(tname); target < 0 {
				// The door would reject an unknown target; surface it as a
				// gate rejection the model can repair, without touching the
				// door (so no cog.outcome is recorded for this attempt).
				return toolloop.Outcome{
					Verdict:        toolloop.VerdictRejectedGate,
					ResultForModel: "unknown target " + strings.TrimSpace(tname),
				}
			}
			guards = d.md.buildTalkToGuards(d.job, target)
		}
		kind, qty := argKindQty(call.Args)
		err := d.md.loop.InjectIntent(sim.InjectArgs{
			Agent: d.job.agent, Goal: name, TargetAgent: target,
			Kind: kind, Qty: qty,
			Class: d.job.meta.class.Class, JobID: d.job.meta.job,
			SnapshotTick: d.job.meta.snapshotTick, Generation: d.job.meta.generation,
			PredictedWallMs: d.job.meta.predictedWallMs,
			ActualWallMs:    time.Since(d.start).Milliseconds(),
			Guards:          guards,
		})
		// The door recorded its verdict (landed/adapted, or a rejection) as a
		// cog.outcome atomically — either way an outcome now exists for this job.
		d.doorOutcome = true
		if err != nil {
			return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: err.Error()}
		}
		return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: name + " landed"}
	}
}

// handleSetPlan wraps Loop.InjectIntent's Plan path (mirroring injectPlan). The
// set_plan authored schema (spec 017 T004) carries goal/kind/qty per step — no
// target and no timing guards — so steps land with Target -1 and the door's
// default validity window; the ladder and per-step validation apply as for any
// landing.
func (d *villagerDispatch) handleSetPlan(_ context.Context, call llm.ToolCall) toolloop.Outcome {
	steps, reason := d.parsePlanSteps(call.Args)
	if reason != "" {
		return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: reason}
	}
	err := d.md.loop.InjectIntent(sim.InjectArgs{
		Agent: d.job.agent, TargetAgent: -1,
		Class: d.job.meta.class.Class, JobID: d.job.meta.job,
		SnapshotTick: d.job.meta.snapshotTick, Generation: d.job.meta.generation,
		PredictedWallMs: d.job.meta.predictedWallMs,
		ActualWallMs:    time.Since(d.start).Milliseconds(),
		Plan:            steps,
	})
	d.doorOutcome = true
	if err != nil {
		return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: err.Error()}
	}
	return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "plan set"}
}

// handleMuse lands the musing text as an agent.thought through the social door,
// batched atomically with its landed cog.outcome — the exact landing today's
// scheduled musing used (mind.go muse worker), now driven by the muse tool. The
// driver has already enforced the 200-rune cap (muse's Text param), so the
// handler only guards the empty case; an over-cap musing never reaches here
// (the driver records it rejected_malformed and feeds it back).
func (d *villagerDispatch) handleMuse(_ context.Context, call llm.ToolCall) toolloop.Outcome {
	text := strings.TrimSpace(argString(call.Args, "text"))
	if text == "" {
		return toolloop.Outcome{Verdict: toolloop.VerdictRejectedGate, ResultForModel: "musing text is empty"}
	}
	// Defensive rune cap: identical to today's parseMusing truncation, though
	// the driver already rejects an over-cap musing before dispatch.
	if r := []rune(text); len(r) > museCapRunes {
		text = string(r[:museCapRunes])
	}
	payload, err := json.Marshal(sim.ThoughtPayload{Agent: d.job.agent, Text: text, Source: "musing"})
	if err != nil {
		return toolloop.Outcome{Err: err}
	}
	if err := d.md.social.InjectSocial([]store.Event{
		{Type: "agent.thought", Payload: payload},
		d.md.cogOutcomeEvent(d.job.meta, sim.OutcomeLanded, "", time.Since(d.start).Milliseconds()),
	}); err != nil {
		// The whitelisted social door failed atomically — nothing landed and no
		// outcome was recorded. Treat it as infrastructure failure so the loop
		// terminates; runPlan records the terminal FR-015 outcome.
		return toolloop.Outcome{Err: err}
	}
	d.doorOutcome = true
	return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "musing recorded"}
}

// handleRead is the generic read-effect dispatch: no production read tool ships
// this task, so the default returns empty data. Test rosters that register a
// read tool install their own handler in the map after villagerHandlers.
func (d *villagerDispatch) handleRead(name string) toolloop.Handler {
	return func(_ context.Context, _ llm.ToolCall) toolloop.Outcome {
		return toolloop.Outcome{Verdict: toolloop.VerdictReadOK, ResultForModel: "{}"}
	}
}

// buildTalkToGuards reproduces runPlan's mind-side talk_to guards: the target
// was alive and present in the prompt's worldview (FR-011). Unchanged from the
// pre-loop inline construction.
func (md *Mind) buildTalkToGuards(job planJob, target int) []sim.Guard {
	return []sim.Guard{
		{Type: sim.GuardTargetAlive, Target: target},
		{Type: sim.GuardTargetPresent, Target: target,
			X: job.world[target].x, Y: job.world[target].y},
	}
}

// parsePlanSteps converts set_plan's steps argument into []sim.PlanStep,
// carrying the job id on every step (as injectPlan did). Returns a non-empty
// reason string when a step is unusable (fed back as a gate rejection). The
// door re-validates each step's goal against the registry and defaults the
// validity window; this only shapes the steps.
func (d *villagerDispatch) parsePlanSteps(raw json.RawMessage) ([]sim.PlanStep, string) {
	var args struct {
		Steps []struct {
			Goal string `json:"goal"`
			Kind string `json:"kind"`
			Qty  int    `json:"qty"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, "set_plan arguments must be a JSON object with a steps array"
	}
	if len(args.Steps) == 0 {
		return nil, "set_plan needs at least one step"
	}
	steps := make([]sim.PlanStep, 0, len(args.Steps))
	for _, sr := range args.Steps {
		steps = append(steps, sim.PlanStep{
			Job:    d.job.meta.job,
			Goal:   strings.ToLower(strings.TrimSpace(sr.Goal)),
			Target: -1,
			Kind:   strings.ToLower(strings.TrimSpace(sr.Kind)),
			Qty:    sr.Qty,
		})
	}
	return steps, ""
}

// argString reads a string-valued argument from a tool call's raw JSON object;
// "" when absent or the wrong shape (the driver's schema validation already
// gates required/typed args, so this is a lenient reader).
func argString(raw json.RawMessage, key string) string {
	m := map[string]json.RawMessage{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	var s string
	_ = json.Unmarshal(m[key], &s)
	return s
}

// argKindQty reads the storage verbs' optional kind/qty. An absent kind is the
// "all kinds" sentinel (Kind ""), exactly as the pre-loop parser resolved an
// omitted kind; an absent qty is 0 (all of kind / as much as fits).
func argKindQty(raw json.RawMessage) (string, int) {
	m := map[string]json.RawMessage{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	var kind string
	_ = json.Unmarshal(m["kind"], &kind)
	var qty int
	_ = json.Unmarshal(m["qty"], &qty)
	return strings.ToLower(strings.TrimSpace(kind)), qty
}
