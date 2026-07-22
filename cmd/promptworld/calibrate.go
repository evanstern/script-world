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
}

func refShapes(tier llm.Tier) []refShape {
	const situation = "You are a villager in a small settlement. It is mid-morning on a clear day. " +
		"You are near the village square. You can see the well, the woodpile, and two neighbors " +
		"talking by the fire pit. You are somewhat hungry and your firewood is low."
	if tier == llm.TierCloud {
		return []refShape{{
			name: "consolidation-5pt", kind: llm.KindConsolidation, points: 5,
			system:    "You distill a villager's day into a short reflective summary.",
			prompt:    situation + " Today you foraged berries, spoke with a neighbor about the coming cold, and repaired the fence. Summarize the day in three sentences.",
			maxTokens: 256,
		}}
	}
	return []refShape{
		{
			name: "planner-3pt", kind: llm.KindPlanner, points: 3,
			system:    "You decide a villager's next goal. Reply with a JSON object {\"goal\": \"...\", \"reason\": \"...\"}.",
			prompt:    situation + " Recent memories: gathered wood yesterday; shared food with Rowan; slept poorly. What do you do next?",
			maxTokens: 256,
		},
	}
}

// submitter is the calibrate-side seam (satisfied by *llm.Orchestrator).
type submitter interface {
	Submit(ctx context.Context, req llm.Request) (llm.Response, error)
}

// calibrateTier runs the reference workload for one tier and returns its
// profile. Returns an error only if every sample failed (unusable tier).
func calibrateTier(orch submitter, tier llm.Tier, samples int) (cognition.TierProfile, error) {
	tp := cognition.TierProfile{}
	perPoint := []float64{}
	for _, sh := range refShapes(tier) {
		ss := cognition.ShapeSamples{Shape: sh.name, Points: sh.points}
		for i := 0; i < samples; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			resp, err := orch.Submit(ctx, llm.Request{
				Kind: sh.kind, System: sh.system, Prompt: sh.prompt, MaxTokens: sh.maxTokens,
			})
			cancel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "calibrate: %s %s sample %d failed: %v\n", tier, sh.name, i+1, err)
				continue
			}
			ss.WallMs = append(ss.WallMs, resp.Millis)
			perPoint = append(perPoint, float64(resp.Millis)/1000/float64(sh.points))
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

	failed := 0
	for _, t := range tiers {
		fmt.Printf("calibrating %s tier (%d samples per shape)...\n", t, *samples)
		tp, err := calibrateTier(orch, t, *samples)
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
