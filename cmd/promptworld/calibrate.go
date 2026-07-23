package main

// promptworld calibrate — the cognition horizon's setup stage
// (specs/007-cognition-horizon/contracts/cli.md): benchmark the configured
// host+model per tier against a uniform reference workload, write
// calibration.json, and show the operator the horizon their hardware buys.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evanstern/promptworld/internal/cognition"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/tool"
	"github.com/evanstern/promptworld/internal/toolloop"
	"github.com/evanstern/promptworld/internal/world"
)

// refShape is one reference-workload prompt shape: fixed content, so runs
// are comparable across hosts and models.
type refShape struct {
	name      string
	kind      llm.Kind
	points    int
	system    string
	prompt    string
	maxTokens int64
	// loop marks a shape whose unit of work is a whole tool-use loop, not a
	// single model call (spec 017, FR-011). A loop shape is measured by driving
	// toolloop.Run with the cognition's registry roster, so the seeded
	// seconds-per-point is in the SAME unit the live estimator observes
	// (Orchestrator.ObserveCognition reports whole-loop wall time). The villager
	// planner is the loop cognition on the local tier.
	loop bool
}

func refShapes(tier llm.Tier) []refShape {
	const situation = "You are a villager in a small settlement. It is mid-morning on a clear day. " +
		"You are near the village square. You can see the well, the woodpile, and two neighbors " +
		"talking by the fire pit. You are somewhat hungry and your firewood is low."
	if tier == llm.TierCloud {
		// Consolidation is a single-shot cloud kind (it did not adopt the loop —
		// spec 017 FR-014), so it stays a plain Submit probe and its per-point
		// timing remains a valid sample for the shared cloud estimator. Metatron
		// IS the cloud loop cognition, but calibrating it would drive extra
		// metered cloud calls per sample — a new cloud spend path the spec 017
		// contract does not invite (contracts/loop-api.md scopes the calibrate
		// change to the planner probe). Its live whole-loop observations converge
		// the cloud estimator at run time; the tiny console-turn volume tolerates
		// a single-shot seed until then.
		return []refShape{{
			name: "consolidation-5pt", kind: llm.KindConsolidation, points: 5,
			system:    "You distill a villager's day into a short reflective summary.",
			prompt:    situation + " Today you foraged berries, spoke with a neighbor about the coming cold, and repaired the fence. Summarize the day in three sentences.",
			maxTokens: 256,
		}}
	}
	return []refShape{
		{
			// The villager planner runs the tool-use loop (spec 017): the probe
			// declares the villager registry roster and drives toolloop.Run, so it
			// measures the real tool-call round-trip (native tools or the json
			// fallback envelope) end to end — the whole-loop unit the governor
			// budgets for — instead of a bare free-text completion.
			name: "planner-3pt", kind: llm.KindPlanner, points: 3, loop: true,
			system:    "You decide a villager's next action by calling exactly one tool.",
			prompt:    situation + " Recent memories: gathered wood yesterday; shared food with Rowan; slept poorly. What do you do next?",
			maxTokens: 256,
		},
	}
}

// sampleWallMs measures ONE sample's wall time (ms) for a shape. Single-shot
// shapes issue one Submit; loop shapes drive toolloop.Run and report the whole
// loop's wall time (spec 017 FR-011). Injected so tests can measure a stub
// server (or a scripted probe) without a real model.
type sampleWallMs func(ctx context.Context, sh refShape) (int64, error)

// calibrateTier runs the reference workload for one tier and returns its
// profile. Returns an error only if every sample failed (unusable tier).
func calibrateTier(sample sampleWallMs, tier llm.Tier, samples int) (cognition.TierProfile, error) {
	tp := cognition.TierProfile{}
	perPoint := []float64{}
	for _, sh := range refShapes(tier) {
		ss := cognition.ShapeSamples{Shape: sh.name, Points: sh.points}
		for i := 0; i < samples; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			millis, err := sample(ctx, sh)
			cancel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "calibrate: %s %s sample %d failed: %v\n", tier, sh.name, i+1, err)
				continue
			}
			ss.WallMs = append(ss.WallMs, millis)
			perPoint = append(perPoint, float64(millis)/1000/float64(sh.points))
		}
		tp.Samples = append(tp.Samples, ss)
	}
	if len(perPoint) == 0 {
		return tp, fmt.Errorf("%s tier unusable: every sample failed", tier)
	}
	sort.Float64s(perPoint)
	tp.SecondsPerPoint = perPoint[len(perPoint)/2] // median of normalized samples
	return tp, nil
}

// orchSampler is the production sampleWallMs: single-shot shapes Submit once;
// loop shapes drive toolloop.Run against the tier's real model with the
// cognition's registry roster and no-op probe handlers (calibration measures
// latency, not landings — every acting call "lands" instantly, ending the loop
// on the first action the model takes, exactly as a live villager loop with no
// read tools yet does). rounds is the operator's loop_max_rounds cap.
func orchSampler(orch *llm.Orchestrator, rounds int) sampleWallMs {
	return func(ctx context.Context, sh refShape) (int64, error) {
		if !sh.loop {
			resp, err := orch.Submit(ctx, llm.Request{
				Kind: sh.kind, System: sh.system, Prompt: sh.prompt, MaxTokens: sh.maxTokens,
			})
			if err != nil {
				return 0, err
			}
			return resp.Millis, nil
		}
		res, err := toolloop.Run(ctx, orch, villagerProbeJob(sh, rounds))
		if err != nil {
			return 0, err
		}
		return res.TotalMillis, nil
	}
}

// villagerProbeJob builds the whole-loop calibration probe for the villager
// planner: the real villager registry roster (so the model sees the production
// tool declarations and the wire exercises native tool_calls / the json
// fallback), and a no-op handler per tool that reports read_ok for read tools
// and landed for acting tools — the loop measures the model round-trip and ends
// on the first action, mutating no world (there is none in calibrate).
func villagerProbeJob(sh refShape, rounds int) toolloop.Job {
	roster := tool.LoopRosterVillager()
	handlers := make(map[string]toolloop.Handler, len(roster))
	for _, tl := range roster {
		if tl.Effect == tool.Read {
			handlers[tl.Name] = func(context.Context, llm.ToolCall) toolloop.Outcome {
				return toolloop.Outcome{Verdict: toolloop.VerdictReadOK, ResultForModel: "ok"}
			}
			continue
		}
		handlers[tl.Name] = func(context.Context, llm.ToolCall) toolloop.Outcome {
			return toolloop.Outcome{Verdict: toolloop.VerdictLanded, ResultForModel: "done"}
		}
	}
	return toolloop.Job{
		JobID:     "calibrate-planner",
		Kind:      sh.kind,
		System:    sh.system,
		Seed:      sh.prompt,
		Roster:    roster,
		Handlers:  handlers,
		MaxRounds: rounds,
		MaxTokens: sh.maxTokens,
	}
}

// horizonSummary evaluates the registry against a fresh seconds-per-point
// across the watchable speed ladder: the operator sees the cognition horizon
// for their hardware before ever running a world.
func horizonSummary(secPerPt float64) string {
	ladder := []float64{1, 4, 8, 16, 32}
	parts := []string{}
	for _, class := range []string{"planner", "conversation", "meeting"} {
		dc, ok := cognition.ClassFor(class)
		if !ok {
			continue
		}
		maxOK := 0.0
		for _, sp := range ladder {
			if cognition.Route(dc, sp, secPerPt).Allow {
				maxOK = sp
			}
		}
		switch {
		case maxOK == 0:
			parts = append(parts, class+" always suppressed")
		case maxOK >= 32:
			parts = append(parts, class+" OK at 32x")
		default:
			parts = append(parts, fmt.Sprintf("%s suppressed above %gx", class, maxOK))
		}
	}
	return strings.Join(parts, "; ")
}

// memMeter satisfies llm.MeterStore without touching world.db — local-tier
// calibration spends nothing and must not contend with a running daemon's
// store.
type memMeter struct{ m map[string]string }

func (s *memMeter) GetMeta(key string) (string, error) { return s.m[key], nil }
func (s *memMeter) SetMeta(key, value string) error    { s.m[key] = value; return nil }

func cmdCalibrate(args []string) error {
	fs := flag.NewFlagSet("calibrate", flag.ContinueOnError)
	tierFlag := fs.String("tier", "local", "tier(s) to calibrate: local|cloud|all (cloud spends real money)")
	samples := fs.Int("samples", 5, "calls per reference shape")
	dir, err := parseWorldFlags(fs, args)
	if err != nil {
		return fmt.Errorf("usage: promptworld calibrate <world> [--tier local|cloud|all] [--samples N]: %w", err)
	}
	w, err := world.Open(dir)
	if err != nil {
		return err
	}
	cfg, err := llm.LoadConfig(w.LLMConfigPath())
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("no llm.json in %s — nothing to calibrate (create the world with `promptworld new`, or restore the config)", w.Dir)
	}
	var tiers []llm.Tier
	switch *tierFlag {
	case "local":
		tiers = []llm.Tier{llm.TierLocal}
	case "cloud":
		tiers = []llm.Tier{llm.TierCloud}
	case "all":
		tiers = []llm.Tier{llm.TierLocal, llm.TierCloud}
	default:
		return fmt.Errorf("unknown --tier %q (local|cloud|all)", *tierFlag)
	}
	for _, t := range tiers {
		if t == llm.TierCloud {
			shapes := len(refShapes(llm.TierCloud))
			fmt.Printf("cloud calibration will make %d metered calls against %s\n", shapes**samples, cfg.Cloud.Model)
		}
	}

	// The meter lives in memory: calibration must not contend with a running
	// daemon's store, and local calls are free. Cloud spend from calibrate
	// is the operator's explicit choice via --tier.
	orch, err := llm.New(*cfg, &memMeter{m: map[string]string{}})
	if err != nil {
		return err
	}
	defer orch.Close()

	// Preserve tiers not being recalibrated on rewrite.
	prof, err := cognition.LoadProfile(w.CalibrationPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "calibrate: existing profile unreadable (%v) — starting fresh\n", err)
		prof = nil
	}
	if prof == nil {
		prof = &cognition.Profile{Tiers: map[string]cognition.TierProfile{}}
	}
	if prof.Tiers == nil {
		prof.Tiers = map[string]cognition.TierProfile{}
	}

	// The loop cap the villager probe runs under, from the same knob the daemon
	// uses (llm.json loop_max_rounds, normalized) so the probe's whole-loop unit
	// matches the live cognition's.
	rounds, _ := cfg.Rounds()
	sample := orchSampler(orch, rounds)

	failed := 0
	for _, t := range tiers {
		fmt.Printf("calibrating %s tier (%d samples per shape)...\n", t, *samples)
		tp, err := calibrateTier(sample, t, *samples)
		if err != nil {
			fmt.Fprintf(os.Stderr, "calibrate: %v — profile for this tier not written\n", err)
			failed++
			continue
		}
		if t == llm.TierLocal {
			tp.Model, tp.Endpoint = cfg.Local.Model, cfg.Local.Endpoint
		} else {
			tp.Model, tp.Endpoint = cfg.Cloud.Model, cfg.Cloud.Endpoint
		}
		prof.Tiers[string(t)] = tp

		fmt.Printf("tier %s  (%s)\n", t, tp.Model)
		for _, ss := range tp.Samples {
			fmt.Printf("  %-14s %d/%d samples", ss.Shape, len(ss.WallMs), *samples)
			if len(ss.WallMs) > 0 {
				strs := make([]string, len(ss.WallMs))
				for i, ms := range ss.WallMs {
					strs[i] = strconv.FormatFloat(float64(ms)/1000, 'f', 1, 64)
				}
				fmt.Printf("   [%ss]", strings.Join(strs, " "))
			}
			fmt.Println()
		}
		fmt.Printf("  seconds_per_point: %.1f\n", tp.SecondsPerPoint)
		if t == llm.TierLocal {
			fmt.Printf("  cognition at this profile: %s\n", horizonSummary(tp.SecondsPerPoint))
		}
	}
	if len(prof.Tiers) == 0 {
		return fmt.Errorf("no tier produced a usable profile; calibration.json not written")
	}
	prof.CalibratedAt = time.Now().UTC().Format(time.RFC3339)
	if err := prof.Save(w.CalibrationPath()); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", w.CalibrationPath())
	if failed > 0 {
		return fmt.Errorf("%d tier(s) failed calibration", failed)
	}
	return nil
}
