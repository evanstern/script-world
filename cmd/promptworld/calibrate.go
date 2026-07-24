package main

// promptworld calibrate — the cognition horizon's setup stage
// (specs/007-cognition-horizon/contracts/cli.md, generalized to declared
// providers by spec 024 T020): benchmark the configured host+model per
// DECLARED PROVIDER against a uniform reference workload, write
// calibration.json, and show the operator the horizon their hardware buys.
// A legacy llm.json's two derived providers ("local"/"cloud") calibrate
// exactly as before this generalization (calibrateLegacy, unchanged); a v2
// registry calibrates every declared provider, each reference call pinned to
// its provider via Request.Provider so the sample measures the NAMED
// provider regardless of what its kind's chain currently resolves to.

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

// refShapes is the legacy tier-keyed entry point, preserved for the
// unchanged calibrateTier/calibrateLegacy path and its tests: TierCloud maps
// to the priced shape set, everything else to the zero-priced set.
func refShapes(tier llm.Tier) []refShape {
	return refShapesFor(tier == llm.TierCloud)
}

// refShapesFor returns the reference-workload shapes for a pricing class
// (spec 024 T020 generalizes the old tier==TierCloud branch to any priced
// declared provider, local vs cloud no longer being the only two names in
// play).
func refShapesFor(priced bool) []refShape {
	const situation = "You are a villager in a small settlement. It is mid-morning on a clear day. " +
		"You are near the village square. You can see the well, the woodpile, and two neighbors " +
		"talking by the fire pit. You are somewhat hungry and your firewood is low."
	if priced {
		// Consolidation is a single-shot kind (it did not adopt the loop —
		// spec 017 FR-014), so it stays a plain Submit probe and its per-point
		// timing remains a valid sample for the provider's estimator. Metatron
		// IS a loop cognition, but calibrating it would drive extra metered
		// calls per sample — a new spend path the spec 017 contract does not
		// invite (contracts/loop-api.md scopes the calibrate change to the
		// planner probe). Its live whole-loop observations converge the
		// estimator at run time; the tiny console-turn volume tolerates a
		// single-shot seed until then.
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

// calibrateTier runs the reference workload for one legacy tier and returns
// its profile. Returns an error only if every sample failed (unusable tier).
// Preserved unchanged (T020: calibrateLegacy's output stays byte-identical
// to pre-spec-024 promptworld calibrate) — calibrateProvider below is the
// declared-provider generalization every non-legacy config runs instead.
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

// calibrateProvider runs the reference workload for one DECLARED PROVIDER
// (spec 024 T020) and returns its profile — calibrateTier's generalization
// from the fixed local/cloud tiers to an arbitrary provider name, keyed by
// pricing class (priced) rather than Tier. Returns an error only if every
// sample failed (unusable provider).
func calibrateProvider(sample sampleWallMs, name string, priced bool, samples int) (cognition.TierProfile, error) {
	tp := cognition.TierProfile{}
	perPoint := []float64{}
	for _, sh := range refShapesFor(priced) {
		ss := cognition.ShapeSamples{Shape: sh.name, Points: sh.points}
		for i := 0; i < samples; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			millis, err := sample(ctx, sh)
			cancel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "calibrate: provider %q %s sample %d failed: %v\n", name, sh.name, i+1, err)
				continue
			}
			ss.WallMs = append(ss.WallMs, millis)
			perPoint = append(perPoint, float64(millis)/1000/float64(sh.points))
		}
		tp.Samples = append(tp.Samples, ss)
	}
	if len(perPoint) == 0 {
		return tp, fmt.Errorf("provider %q unusable: every sample failed", name)
	}
	sort.Float64s(perPoint)
	tp.SecondsPerPoint = perPoint[len(perPoint)/2] // median of normalized samples
	return tp, nil
}

// orchSampler is the production sampleWallMs for the legacy tier path
// (unpinned — a bare tier name IS its own provider, so the chain head always
// resolves to it). single-shot shapes Submit once; loop shapes drive
// toolloop.Run against the tier's real model with the cognition's registry
// roster and no-op probe handlers (calibration measures latency, not
// landings — every acting call "lands" instantly, ending the loop on the
// first action the model takes, exactly as a live villager loop with no read
// tools yet does). rounds is the operator's loop_max_rounds cap.
func orchSampler(orch *llm.Orchestrator, rounds int) sampleWallMs {
	return orchSamplerFor(orch, rounds, "")
}

// orchSamplerFor is orchSampler generalized to a named declared provider
// (spec 024 T020): provider rides Request.Provider on every reference call
// (single-shot or loop-round), pinning the sample to that provider regardless
// of what its kind's chain currently resolves to (R3) — the reference
// measurement stays the NAMED provider's even when it isn't the chain head.
// provider == "" reproduces orchSampler's unpinned legacy behavior exactly.
func orchSamplerFor(orch *llm.Orchestrator, rounds int, provider string) sampleWallMs {
	return func(ctx context.Context, sh refShape) (int64, error) {
		if !sh.loop {
			resp, err := orch.Submit(ctx, llm.Request{
				Kind: sh.kind, System: sh.system, Prompt: sh.prompt, MaxTokens: sh.maxTokens,
				Provider: provider,
			})
			if err != nil {
				return 0, err
			}
			return resp.Millis, nil
		}
		res, err := toolloop.Run(ctx, orch, villagerProbeJob(sh, rounds, provider))
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
// on the first action, mutating no world (there is none in calibrate). provider
// rides straight through to toolloop.Job.Provider (spec 024 T020); "" leaves
// the loop's kind-routed chain to pick the head, as every non-calibrate caller
// does.
func villagerProbeJob(sh refShape, rounds int, provider string) toolloop.Job {
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
		Provider:  provider,
	}
}

// horizonSummary evaluates the registry against a fresh seconds-per-point
// across the watchable speed ladder: the operator sees the cognition horizon
// for their hardware before ever running a world. Delegates to
// cognition.HorizonSummary (spec 035 R1/T004): the daemon boot warning, the
// set_speed warning, and calibrate all read the one implementation, so the
// warning may never disagree with the router (FR-006).
func horizonSummary(secPerPt float64) string {
	return cognition.HorizonSummary(secPerPt)
}

// sequentialFloorDisclosure is printed once per calibrate run, adjacent to
// the horizon summary (spec 035 US4/FR-005, contracts/warnings.md §4): the
// measurement is one reference call at a time while a live world drives the
// same endpoint concurrently (many agents queue on it), so the measured
// seconds-per-point is a FLOOR — the effective rate under concurrent load
// runs higher, and the live estimator (spec 031) adapts upward at runtime.
// Printed in both calibrateLegacy and calibrateDeclaredProviders — research
// R6 records the deliberate supersession of spec 024 T020's legacy
// byte-identity guarantee for this one addition.
func sequentialFloorDisclosure() string {
	return "note: calibration measures one call at a time; a live world runs N agents concurrently against\n" +
		"      the same endpoint, so measured s/pt is a floor — effective rate under load runs higher\n" +
		"      (the live estimator adapts at runtime).\n"
}

// memMeter satisfies llm.MeterStore without touching world.db — local-tier
// calibration spends nothing and must not contend with a running daemon's
// store.
type memMeter struct{ m map[string]string }

func (s *memMeter) GetMeta(key string) (string, error) { return s.m[key], nil }
func (s *memMeter) SetMeta(key, value string) error    { s.m[key] = value; return nil }

// priced reports whether a declared provider bills for traffic — the same
// pricing-class test llm.ProviderConfig.zeroPriced applies internally
// (unexported there), read here off the exported pricing fields so calibrate
// needs no new llm-package surface beyond ProviderConfig/ProviderNames.
func priced(pc llm.ProviderConfig) bool {
	return pc.InputUSDPerMTok > 0 || pc.OutputUSDPerMTok > 0
}

func cmdCalibrate(args []string) error {
	fs := flag.NewFlagSet("calibrate", flag.ContinueOnError)
	tierFlag := fs.String("tier", "", "deprecated alias for --provider: local|cloud|all. "+
		"Legacy llm.json (no `providers` map): selects its two derived providers exactly as "+
		"before this flag existed — empty means \"local\" alone, the historical default. v2 "+
		"registry: local/cloud select every zero-priced/priced declared provider; prefer "+
		"--provider on a v2 registry.")
	providerFlag := fs.String("provider", "", "calibrate only this declared provider "+
		"(default: every declared provider; a legacy config with neither flag set defaults "+
		"to \"local\" alone, matching --tier's historical default)")
	samples := fs.Int("samples", 5, "calls per reference shape")
	dir, err := parseWorldFlags(fs, args)
	if err != nil {
		return fmt.Errorf("usage: promptworld calibrate <world> [--provider name | --tier local|cloud|all] [--samples N]: %w", err)
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
	if len(cfg.Providers) == 0 && *providerFlag == "" {
		// Legacy two-tier config, no v2-only flag requested: the exact
		// behavior shipped before spec 024 (T020: byte-identical output).
		return calibrateLegacy(w, cfg, *tierFlag, *samples)
	}
	return calibrateDeclaredProviders(w, cfg, *tierFlag, *providerFlag, *samples)
}

// calibrateLegacy runs the pre-spec-024 two-tier calibration path for a
// legacy llm.json — preserved verbatim (T020: "legacy config output is
// byte-identical to today's") down to the print statements' wording.
// tierFlag == "" reproduces the historical default ("local" alone).
func calibrateLegacy(w *world.World, cfg *llm.Config, tierFlag string, samples int) error {
	if tierFlag == "" {
		tierFlag = "local"
	}
	var tiers []llm.Tier
	switch tierFlag {
	case "local":
		tiers = []llm.Tier{llm.TierLocal}
	case "cloud":
		tiers = []llm.Tier{llm.TierCloud}
	case "all":
		tiers = []llm.Tier{llm.TierLocal, llm.TierCloud}
	default:
		return fmt.Errorf("unknown --tier %q (local|cloud|all)", tierFlag)
	}
	for _, t := range tiers {
		if t == llm.TierCloud {
			shapes := len(refShapes(llm.TierCloud))
			fmt.Printf("cloud calibration will make %d metered calls against %s\n", shapes*samples, cfg.Cloud.Model)
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
	disclosed := false
	for _, t := range tiers {
		fmt.Printf("calibrating %s tier (%d samples per shape)...\n", t, samples)
		tp, err := calibrateTier(sample, t, samples)
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
		// The profile is keyed by PROVIDER NAME (spec 024 R5): a legacy config
		// derives providers named exactly "local"/"cloud", so writing the tier
		// string writes the provider-named entry the daemon's SeedFor reads by
		// name with no translation.
		prof.Tiers[string(t)] = tp

		fmt.Printf("tier %s  (%s)\n", t, tp.Model)
		for _, ss := range tp.Samples {
			fmt.Printf("  %-14s %d/%d samples", ss.Shape, len(ss.WallMs), samples)
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
			fmt.Print(sequentialFloorDisclosure())
			disclosed = true
		}
	}
	// US4/FR-005: every run discloses the sequential-measurement floor exactly
	// once, adjacent to the horizon summary when one printed above; a
	// cloud-only run (no local tier, no horizon line) still gets it.
	if !disclosed {
		fmt.Print(sequentialFloorDisclosure())
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

// selectDeclaredProviders resolves --provider/--tier into the provider names
// this run calibrates (spec 024 T020). --provider wins outright and must name
// a declared provider. --tier is the deprecated alias: "local"/"cloud" select
// every zero-priced/priced declared provider by pricing class (a v2 registry
// may have several of each); "all" or no flag at all selects every declared
// provider — "iteration over the declared providers" IS the v2 default
// (tasks.md T020), unlike the legacy path's single-tier default.
func selectDeclaredProviders(orch *llm.Orchestrator, names []string, tierFlag, providerFlag string) ([]string, error) {
	if providerFlag != "" {
		for _, n := range names {
			if n == providerFlag {
				return []string{n}, nil
			}
		}
		return nil, fmt.Errorf("unknown --provider %q (declared providers: %s)", providerFlag, strings.Join(names, ", "))
	}
	switch tierFlag {
	case "", "all":
		return names, nil
	case "local", "cloud":
		class := "zero-priced"
		want := false
		if tierFlag == "cloud" {
			class, want = "priced", true
		}
		fmt.Printf("note: --tier is deprecated on a v2 registry — %q selects every %s provider; use --provider <name> to pin one\n", tierFlag, class)
		var sel []string
		for _, n := range names {
			pc, _ := orch.ProviderConfig(n)
			if priced(pc) == want {
				sel = append(sel, n)
			}
		}
		if len(sel) == 0 {
			return nil, fmt.Errorf("--tier %q matched no declared provider by pricing class", tierFlag)
		}
		return sel, nil
	default:
		return nil, fmt.Errorf("unknown --tier %q (local|cloud|all)", tierFlag)
	}
}

// calibrateDeclaredProviders runs T020's v2-registry calibration: one
// reference-workload pass per DECLARED PROVIDER rather than per fixed tier,
// each reference call pinned via Request.Provider (R3) so the measured
// latency is the named provider's, never whichever candidate its kind's
// chain currently resolves to. Writes one profile entry per provider name —
// the shape cognition.SeedFor already reads by name (llm.go
// Orchestrator.SeedCalibration). Also serves a legacy config when --provider
// narrows to one of its two derived providers (a new path, not covered by
// T020's byte-identical requirement, which applies to the flag-free legacy
// default only).
func calibrateDeclaredProviders(w *world.World, cfg *llm.Config, tierFlag, providerFlag string, samples int) error {
	// The meter lives in memory, mirroring calibrateLegacy: calibration must
	// not contend with a running daemon's store, and a priced provider's spend
	// is the operator's explicit choice via --provider/--tier.
	orch, err := llm.New(*cfg, &memMeter{m: map[string]string{}})
	if err != nil {
		return err
	}
	defer orch.Close()

	names := orch.ProviderNames() // sorted — the wire order, contracts/status.md
	selected, err := selectDeclaredProviders(orch, names, tierFlag, providerFlag)
	if err != nil {
		return err
	}
	for _, name := range selected {
		pc, _ := orch.ProviderConfig(name)
		if priced(pc) {
			shapes := len(refShapesFor(true))
			fmt.Printf("provider %q calibration will make %d metered calls against %s\n", name, shapes*samples, pc.Model)
		}
	}

	// Preserve providers not being recalibrated on rewrite.
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

	failed := 0
	disclosed := false
	for _, name := range selected {
		pc, _ := orch.ProviderConfig(name)
		p := priced(pc)
		sample := orchSamplerFor(orch, rounds, name)

		fmt.Printf("calibrating provider %q (%d samples per shape)...\n", name, samples)
		tp, err := calibrateProvider(sample, name, p, samples)
		if err != nil {
			fmt.Fprintf(os.Stderr, "calibrate: %v — profile for this provider not written\n", err)
			failed++
			continue
		}
		tp.Model, tp.Endpoint = pc.Model, pc.Endpoint
		prof.Tiers[name] = tp

		fmt.Printf("provider %q  (%s)\n", name, tp.Model)
		for _, ss := range tp.Samples {
			fmt.Printf("  %-14s %d/%d samples", ss.Shape, len(ss.WallMs), samples)
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
		if !p {
			fmt.Printf("  cognition at this profile: %s\n", horizonSummary(tp.SecondsPerPoint))
			if !disclosed {
				fmt.Print(sequentialFloorDisclosure())
				disclosed = true
			}
		}
	}
	// US4/FR-005: every run discloses the sequential-measurement floor exactly
	// once, adjacent to the first horizon summary printed above; a run that
	// calibrates only priced providers (no horizon line) still gets it.
	if !disclosed {
		fmt.Print(sequentialFloorDisclosure())
	}
	if len(prof.Tiers) == 0 {
		return fmt.Errorf("no provider produced a usable profile; calibration.json not written")
	}
	prof.CalibratedAt = time.Now().UTC().Format(time.RFC3339)
	if err := prof.Save(w.CalibrationPath()); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", w.CalibrationPath())
	if failed > 0 {
		return fmt.Errorf("%d provider(s) failed calibration", failed)
	}
	return nil
}
