package tui

// Per-family digest unit tests (T013) and the catalog sweep test (T014,
// contracts/digest-grammar.md §7, SC-001). catalogFixture is the sweep's
// single source (research.md R3): one representative sample payload per
// cataloged event type, plus the plain-text summary contract §3's template
// renders for it — used both to assert per-type output and to gate
// registry coverage in both directions.

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
)

// digestFixture is one catalog sweep entry.
type digestFixture struct {
	payload string
	want    string // expected plainSegs(digest) output — contract §3's template made concrete
}

// catalogFixture: every registry key gets exactly one row here (both
// directions are asserted by TestCatalogSweep) — adding a digest without a
// fixture row, or a fixture row without a digest, fails the sweep.
var catalogFixture = map[string]digestFixture{
	// --- world / clock / daemon ---
	"world.created":   {`{"name":"Ashgrove","seed":42}`, `world "Ashgrove" created · seed 42`},
	"world.migrated":  {`{"from_format":2,"source_events":100,"source_tick":500,"state":{}}`, `migrated from format v2 · 100 events @ tick 500`},
	"clock.paused":    {`{}`, `paused`},
	"clock.resumed":   {`{}`, `resumed`},
	"clock.speed_set": {`{"speed":"4x"}`, `speed=4x`},
	"clock.degraded":  {`{"effective_rate":3.5}`, `degraded rate=3.50`},
	"clock.recovered": {`{}`, `recovered`},
	"clock.governor_shed": {
		`{"requested":"32x","from":"32x","to":"16x","debt":1.4,"jobs":3}`,
		`governor shed 32x→16x debt=140% jobs=3`,
	},
	"clock.governor_recovered": {
		`{"requested":"32x","from":"8x","to":"16x","debt":0.3,"jobs":1}`,
		`governor recovered 8x→16x debt=30% jobs=1`,
	},
	"daemon.started": {`{"tick":100,"recovery_ms":250}`, `tick=100 recovery_ms=250`},
	"daemon.stopped":  {`{"tick":100}`, `tick=100`},

	// --- sim ---
	"sim.day_started":        {`{"day":3}`, `day 3 begins`},
	"sim.night_started":      {`{"day":3}`, `night falls on day 3`},
	"sim.forage_regrown":     {`{"x":2,"y":3}`, `forage regrew at (2,3)`},
	"sim.fire_burned_out":    {`{"x":4,"y":5}`, `the fire at (4,5) burned out`},
	"sim.food_rotted":        {`{"x":6,"y":6,"kind":"food_raw","n":4}`, `4 food_raw rotted at (6,6)`},
	"sim.gathering_observed": {`{"x":1,"y":1,"start":500}`, `gathering at (1,1) since tick 500`},

	// --- agent: acts & needs ---
	"agent.intent_set": {
		`{"agent":0,"goal":"forage","target_x":3,"target_y":4,"res_x":0,"res_y":0,"source":"reflex"}`,
		`Ash intends forage (reflex) → (3,4)`,
	},
	"agent.work_started":    {`{"agent":1,"tick":100}`, `Birch set to work`},
	"agent.intent_done":     {`{"agent":2}`, `Cedar finished`},
	"agent.intent_rejected": {`{"agent":3,"goal":"forage","reason":"blocked","staleness_ticks":5}`, `Rowan's forage refused: blocked (5t stale)`},
	"agent.moved":           {`{"agent":0,"x":1,"y":1}`, `Ash → (1,1)`},
	"agent.foraged":         {`{"agent":0,"x":1,"y":1}`, `Ash foraged at (1,1)`},
	"agent.chopped":         {`{"agent":0,"x":1,"y":1}`, `Ash chopped wood at (1,1)`},
	"agent.hunted":          {`{"agent":0,"x":1,"y":1}`, `Ash hunted at (1,1)`},
	"agent.quarried":        {`{"agent":0,"x":1,"y":1}`, `Ash quarried stone at (1,1)`},
	"agent.collected_water": {`{"agent":0,"x":1,"y":1}`, `Ash drew water at (1,1)`},
	"agent.crafted":         {`{"agent":0,"kind":"planks"}`, `Ash crafted planks`},
	"agent.built":           {`{"agent":0,"kind":"fire","x":1,"y":1}`, `Ash built a fire at (1,1)`},
	"agent.dropped":         {`{"agent":0,"x":3,"y":4,"kind":"wood","n":2}`, `Ash dropped 2 wood at (3,4)`},
	"agent.picked_up":       {`{"agent":1,"x":3,"y":4,"kind":"wood","n":2}`, `Birch picked up 2 wood at (3,4)`},
	"agent.deposited":       {`{"agent":2,"x":5,"y":5,"kind":"planks","n":6}`, `Cedar stored 6 planks in the chest at (5,5)`},
	"agent.withdrew":        {`{"agent":3,"x":5,"y":5,"kind":"planks","n":1,"owner":0}`, `Rowan took 1 planks from Ash's chest`},
	"agent.cooked":          {`{"agent":0,"station":"fire","consumed":2,"produced":1,"kind":"food_cooked"}`, `Ash cooked 1 food_cooked at the fire`},
	"agent.bathed":          {`{"agent":0,"morale_after":80,"warmth_after":90}`, `Ash bathed · morale 80 warmth 90`},
	"agent.refueled":        {`{"agent":0,"x":1,"y":1,"fuel_until":500}`, `Ash refueled the fire at (1,1)`},
	"agent.spear_broke":     {`{"agent":0}`, `Ash's spear broke`},
	"agent.ate":             {`{"agent":0,"meals":1,"cooked":0,"raw":0,"food_after":80}`, `Ash ate 1 meals → food 80`},
	"agent.slept":           {`{"agent":0}`, `Ash fell asleep`},
	"agent.woke":            {`{"agent":0}`, `Ash woke`},
	"agent.needs_changed":   {`{"agent":0,"health":90,"food":50,"rest":60,"warmth":70,"morale":80}`, `Ash health=90 food=50 rest=60 warmth=70 morale=80`},
	"agent.died":            {`{"agent":0,"cause":"starvation"}`, `Ash died: starvation`},
	"agent.talked":          {`{"a":0,"b":1}`, `Ash chatted with Birch`},

	// --- agent: mind & plans ---
	"agent.memory_added":      {`{"agent":0,"text":"the fire needs tending","salience":5,"subject":1,"tone":0}`, `Ash remembers: "the fire needs tending" · about Birch`},
	"agent.thought":           {`{"agent":0,"text":"I should forage","source":"planner"}`, `Ash thought: "I should forage" (planner)`},
	"agent.memory_promoted":   {`{"agent":0,"mem_tick":100,"text_hash":"abc","boost":2}`, `Ash's memory (t100) reinforced`},
	"agent.memory_faded":      {`{"agent":0,"mem_tick":100,"text_hash":"abc"}`, `Ash forgot a memory (t100)`},
	"agent.belief_revised":    {`{"agent":0,"belief_id":0,"statement":"the fire needs tending","confidence":80,"provenance":"observed","source":0,"subject":0}`, `Ash now believes: "the fire needs tending"`},
	"agent.narrative_set":     {`{"agent":0,"text":"a long night"}`, `Ash's story: "a long night"`},
	"agent.consolidated":      {`{"agent":0,"night":1,"up_to":100,"outcome":"accepted"}`, `Ash consolidated the night's memories`},
	"agent.plan_set":          {`{"agent":0,"job":"forage_run","steps":[{"job":"forage_run","goal":"forage"},{"job":"forage_run","goal":"deposit"}]}`, `Ash planned 2 steps: forage, deposit`},
	"agent.plan_step_started": {`{"agent":0,"job":"forage_run","step":"forage"}`, `Ash began step forage`},
	"agent.plan_expired":      {`{"agent":0,"job":"forage_run","step":"forage","reason":"window closed"}`, `Ash's plan lapsed (window closed)`},

	// --- social ---
	"social.conversation_turn": {`{"conv":100,"speaker":3,"listener":0,"text":"hello"}`, `Rowan→Ash "hello"`},
	"social.rumor_told":        {`{"from":1,"to":2,"rumor_id":0,"subject":0,"tone":0,"text":"gossip","confidence":50}`, `Birch→Cedar rumor: "gossip"`},
	"social.conversation":      {`{"conv":1,"a":0,"b":1,"gist":"argued about firewood","turns":6}`, `"argued about firewood" · 6 turns`},
	"social.relation_changed":  {`{"a":0,"b":1,"trust_delta":2,"affection_delta":-1,"reason":"gift"}`, `Ash→Birch trust+2/affection-1 (gift)`},
	"social.gave":              {`{"from":0,"to":1,"kind":"food"}`, `Ash gave Birch food`},
	"social.promise_broken":    {`{"id":7}`, `a promise was broken (#7)`},
	"social.secret_seeded":     {`{"agent":0,"text":"a secret","tone":0}`, `a secret took root with Ash`},
	"social.chest_taken":       {`{"owner":0,"taker":3,"x":5,"y":5}`, `Rowan raided Ash's chest at (5,5)`},
	"social.hailed":            {`{"from":1,"to":3,"until":12345}`, `Birch hailed Rowan (until t12345)`},
	"social.hail_met":          {`{"from":1,"to":3}`, `Birch met Rowan`},
	"social.hail_expired":      {`{"from":0,"to":2}`, `Ash's hail to Cedar lapsed`},

	// --- governance (meeting.* / norm.*) — all 9 meeting.* rows + norm.violated ---
	"meeting.convened":               {`{"x":1,"y":1}`, `meeting convened at (1,1)`},
	"meeting.opened":                 {`{"attendees":[0,1]}`, `meeting opened`},
	"meeting.turn_taken":             {`{"agent":0,"raised":""}`, `Ash spoke at the meeting`},
	"meeting.proposal_tabled":        {`{"proposal_id":1,"kind":"amend","target":-1,"proposer":0,"text":"no stealing"}`, `Ash proposed: "no stealing"`},
	"meeting.proposal_resolved":      {`{"proposal_id":1,"kind":"amend","target":-1,"proposer":0,"text":"no stealing","yeas":[0,1],"nays":[2],"passed":true}`, `proposal passed: "no stealing" (2-1)`},
	"meeting.proposal_rephrased":     {`{"proposal_id":1,"norm_id":1,"text":"no stealing from chests"}`, `norm rephrased: "no stealing from chests"`},
	"meeting.closed":                 {`{"proposals":2}`, `meeting closed`},
	"meeting.place_designated":       {`{"x":2,"y":2}`, `meeting place set at (2,2)`},
	"meeting.convention_established": {`{"convene_second":72000,"open_second":75600,"x":2,"y":2,"source":"config"}`, `meeting convention: 21:00 at (2,2) (config)`},
	"norm.violated":                  {`{"norm_id":3,"violator":0,"witnesses":[1,2]}`, `Ash violated a norm (#3)`},

	// --- gru / chronicle / metatron ---
	"gru.emerged":                 {`{"night":1,"x":5,"y":5}`, `the gru emerged at (5,5)`},
	"gru.moved":                   {`{"x":6,"y":6}`, `the gru prowls to (6,6)`},
	"gru.sighted":                 {`{"agent":0,"x":5,"y":5}`, `Ash sighted the gru`},
	"gru.attacked":                {`{"agent":0,"health":40}`, `the gru attacked Ash · health → 40`},
	"gru.withdrew":                {`{"day":2}`, `the gru withdrew`},
	"chronicle.entry":             {`{"day":3,"from_tick":100,"to_tick":200,"text":"Ash lit the first fire.","thread":"cold-start","agents":[0]}`, `day 3 · cold-start: Ash lit the first fire.`},
	"metatron.charge_regenerated": {`{}`, `a charge regenerated`},
	"metatron.nudged":             {`{"form":"dream","targets":[0],"text":"beware the cold"}`, `Metatron dream → Ash: "beware the cold"`},
	"metatron.time_snapped":       {`{"to_tick":106200,"gratis":false}`, `Metatron snapped time forward to day 2 11:30`},
	"metatron.item_granted":       {`{"agent":0,"kind":"food_raw","qty":2,"gratis":false}`, `Metatron granted Ash 2 food_raw`},
	"metatron.entity_moved":       {`{"class":"pile","x":3,"y":4,"to_x":6,"to_y":7,"gratis":false}`, `Metatron moved the pile at (3,4) to (6,7)`},
	"metatron.entity_removed":     {`{"class":"structure","x":12,"y":8,"gratis":false}`, `Metatron removed the structure at (12,8)`},

	// --- cog (labeled) ---
	"cog.thought": {
		`{"job":"j1","class":"reflex","agent":0,"snapshot_tick":100,"generation":1,"trigger_seq":0,"points":5,"predicted_wall_ms":200,"predicted_land_tick":300}`,
		`job=j1 class=reflex agent=Ash pts=5 pred=200ms`,
	},
	"cog.outcome": {
		`{"job":"j1","class":"reflex","agent":0,"outcome":"landed","snapshot_tick":100,"landing_tick":150,"staleness_ticks":10,"predicted_wall_ms":200,"actual_wall_ms":220}`,
		`job=j1 landed agent=Ash stale=10t wall=220ms`,
	},
	"cog.recalibration_recommended": {
		`{"tier":"cheap","estimate_s_per_pt":0.5,"spike_rate":0.2,"window":50}`,
		`tier=cheap est=0.50s/pt spikes=0.20 window=50`,
	},
	"cog.tool_call": {
		`{"job":"j1","ordinal":1,"tool":"inject_intent","args":{"agent":0},"verdict":"rejected_gate","reason":"stale snapshot","tier":"cheap","snapshot_tick":100}`,
		`job=j1 ord=1 tool=inject_intent rejected_gate tier=cheap reason=stale snapshot`,
	},
}

// TestCatalogSweep is the SC-001 gate (contract §7): every fixture type
// must have a registry entry and digest without a raw-JSON fallback; every
// registry key must appear in the fixture (no unlisted digests,
// data-model.md invariant 3); and every backticked concrete event type in
// docs/wiki/event-types.md must be covered by the fixture, so the doc and
// the digest catalog cannot drift silently (R3).
func TestCatalogSweep(t *testing.T) {
	names := []string{"Ash", "Birch", "Cedar", "Rowan"}

	for typ, fx := range catalogFixture {
		fn, ok := digestRegistry[typ]
		if !ok {
			t.Errorf("fixture type %q has no registry entry", typ)
			continue
		}
		e := store.Event{Seq: 1, Tick: 1, Type: typ, Payload: json.RawMessage(fx.payload)}
		segs, ok := fn(e, names)
		if !ok {
			t.Errorf("%s: digest fell back (ok=false) on its own sample payload", typ)
			continue
		}
		if got := plainSegs(segs); got != fx.want {
			t.Errorf("%s: plain summary = %q, want %q", typ, got, fx.want)
		}
	}

	for typ := range digestRegistry {
		if _, ok := catalogFixture[typ]; !ok {
			t.Errorf("registry key %q has no catalog fixture row (unlisted digest)", typ)
		}
	}

	doc, err := os.ReadFile("../../docs/wiki/event-types.md")
	if err != nil {
		t.Fatalf("reading docs/wiki/event-types.md: %v", err)
	}
	for _, typ := range backtickedEventTypes(string(doc)) {
		if _, ok := catalogFixture[typ]; !ok {
			t.Errorf("docs/wiki/event-types.md backticks %q but the catalog fixture doesn't cover it", typ)
		}
	}
}

// backtickedTypeRe matches a backticked `namespace.verb` token; the caller
// filters to known event-family namespaces so incidental matches like
// “ `social.go` “ or “ `world.json` “ (source-file/config references in
// prose) don't get mistaken for event types.
var backtickedTypeRe = regexp.MustCompile("`([a-z]+)\\.([a-z_]+)`")

func backtickedEventTypes(doc string) []string {
	var types []string
	seen := map[string]bool{}
	for _, m := range backtickedTypeRe.FindAllStringSubmatch(doc, -1) {
		ns, verb := m[1], m[2]
		if _, ok := familyByNamespace[ns]; !ok {
			continue // not one of our namespaces — e.g. a stray "foo.bar" in prose
		}
		if verb == "go" || verb == "json" || verb == "md" {
			continue // source-file / config references, not event types
		}
		t := ns + "." + verb
		if !seen[t] {
			seen[t] = true
			types = append(types, t)
		}
	}
	return types
}

// --- T013: per-family cases the flat fixture table doesn't reach —
// role-span assertions and conditional branches (subject absent, all-zero
// gathering, alert types). The fixture table above already exercises plain
// text for every type; these add the "and role spans" half of T013.

var digestTestNames = []string{"Ash", "Birch", "Cedar", "Rowan"}

func digestOf(t *testing.T, typ, payload string) []seg {
	t.Helper()
	fn, ok := digestRegistry[typ]
	if !ok {
		t.Fatalf("%s: no registry entry", typ)
	}
	e := store.Event{Seq: 1, Tick: 1, Type: typ, Payload: json.RawMessage(payload)}
	segs, ok := fn(e, digestTestNames)
	if !ok {
		t.Fatalf("%s: digest returned ok=false", typ)
	}
	return segs
}

func TestDigestRoleSpans(t *testing.T) {
	// Speech privilege (contract §2): conversation_turn/rumor_told/scene/
	// meeting speech all carry a segSpeech span on the quoted text.
	for _, typ := range []string{"social.conversation_turn", "social.rumor_told", "social.conversation",
		"meeting.proposal_tabled", "meeting.proposal_resolved", "meeting.proposal_rephrased",
		"agent.thought", "agent.narrative_set", "agent.belief_revised", "agent.memory_added"} {
		fx := catalogFixture[typ]
		if !anyRole(digestOf(t, typ, fx.payload), segSpeech) {
			t.Errorf("%s: expected a segSpeech span", typ)
		}
	}

	// Every resolved agent name carries segName (contract §2 "name").
	for _, typ := range []string{"agent.moved", "agent.died", "agent.talked", "gru.sighted", "social.hailed"} {
		fx := catalogFixture[typ]
		if !anyRole(digestOf(t, typ, fx.payload), segName) {
			t.Errorf("%s: expected a segName span", typ)
		}
	}

	// Labeled voice (contract §2): cog/clock/daemon render key=value spans.
	for _, typ := range []string{"cog.thought", "cog.outcome", "cog.recalibration_recommended", "cog.tool_call",
		"clock.speed_set", "clock.degraded", "daemon.started", "daemon.stopped", "agent.needs_changed"} {
		fx := catalogFixture[typ]
		if !anyRole(digestOf(t, typ, fx.payload), segLabel) {
			t.Errorf("%s: expected a segLabel span", typ)
		}
	}
}

// TestDigestMemoryAddedNoSubject: Subject's -1 sentinel (no gossip subject)
// must not append the "· about X" clause (internal/sim/memory.go).
func TestDigestMemoryAddedNoSubject(t *testing.T) {
	segs := digestOf(t, "agent.memory_added", `{"agent":0,"text":"the wind is picking up","salience":3,"subject":-1,"tone":0}`)
	if want := `Ash remembers: "the wind is picking up"`; plainSegs(segs) != want {
		t.Errorf("plain summary = %q, want %q", plainSegs(segs), want)
	}
}

// TestDigestGatheringDispersed: sim.gathering_observed's all-zero payload is
// the watch-reset sentinel, not a real gathering at (0,0) (contract §3).
func TestDigestGatheringDispersed(t *testing.T) {
	segs := digestOf(t, "sim.gathering_observed", `{"x":0,"y":0,"start":0}`)
	if want := "gathering dispersed"; plainSegs(segs) != want {
		t.Errorf("plain summary = %q, want %q", plainSegs(segs), want)
	}
}

// hasSeg reports whether segs contains a seg matching both text and role
// exactly — used where anyRole's role-only check is too loose (e.g.
// distinguishing the gratis marker's own segment from other segEmphasis
// spans already present in the summary).
func hasSeg(segs []seg, text string, role segRole) bool {
	for _, s := range segs {
		if s.Text == text && s.Role == role {
			return true
		}
	}
	return false
}

// TestDigestMiracleGratisMark: the four miracle types (TASK-59/spec 016)
// append a visible " (forced)" annotation when Gratis waives the charge, and
// nothing when it doesn't (spec 016 SC-004's enumerability story surfaced in
// the digest — gratisMark must never appear on a charge-priced miracle).
func TestDigestMiracleGratisMark(t *testing.T) {
	cases := []struct {
		typ                           string
		chargedPayload, gratisPayload string
		want                          string // base summary, sans the gratis suffix
	}{
		{
			"metatron.time_snapped",
			`{"to_tick":106200,"gratis":false}`, `{"to_tick":106200,"gratis":true}`,
			`Metatron snapped time forward to day 2 11:30`,
		},
		{
			"metatron.item_granted",
			`{"agent":0,"kind":"food_raw","qty":2,"gratis":false}`, `{"agent":0,"kind":"food_raw","qty":2,"gratis":true}`,
			`Metatron granted Ash 2 food_raw`,
		},
		{
			"metatron.entity_moved",
			`{"class":"pile","x":3,"y":4,"to_x":6,"to_y":7,"gratis":false}`, `{"class":"pile","x":3,"y":4,"to_x":6,"to_y":7,"gratis":true}`,
			`Metatron moved the pile at (3,4) to (6,7)`,
		},
		{
			"metatron.entity_removed",
			`{"class":"structure","x":12,"y":8,"gratis":false}`, `{"class":"structure","x":12,"y":8,"gratis":true}`,
			`Metatron removed the structure at (12,8)`,
		},
	}
	for _, tc := range cases {
		if got := plainSegs(digestOf(t, tc.typ, tc.chargedPayload)); got != tc.want {
			t.Errorf("%s (charged): plain summary = %q, want %q", tc.typ, got, tc.want)
		}
		gotGratis := plainSegs(digestOf(t, tc.typ, tc.gratisPayload))
		wantGratis := tc.want + " (forced)"
		if gotGratis != wantGratis {
			t.Errorf("%s (gratis): plain summary = %q, want %q", tc.typ, gotGratis, wantGratis)
		}
		if !hasSeg(digestOf(t, tc.typ, tc.gratisPayload), "forced", segEmphasis) {
			t.Errorf("%s (gratis): expected a styled %q segment", tc.typ, "forced")
		}
		if hasSeg(digestOf(t, tc.typ, tc.chargedPayload), "forced", segEmphasis) {
			t.Errorf("%s (charged): unexpected %q segment on a charge-priced miracle", tc.typ, "forced")
		}
	}
}

// TestDigestEntityMovedRemovedClasses: entity_moved/entity_removed render
// distinctly per Class (internal/sim/miracles.go) — villager/structure/pile
// for a move, structure/pile/terrain for a remove (terrain is overlaid, not
// deleted, so it reads "cleared" rather than "removed").
func TestDigestEntityMovedRemovedClasses(t *testing.T) {
	moveCases := []struct{ payload, want string }{
		{`{"class":"villager","x":1,"y":1,"to_x":2,"to_y":2,"gratis":false}`, `Metatron moved the villager at (1,1) to (2,2)`},
		{`{"class":"structure","x":1,"y":1,"to_x":2,"to_y":2,"gratis":false}`, `Metatron moved the structure at (1,1) to (2,2)`},
		{`{"class":"pile","x":1,"y":1,"to_x":2,"to_y":2,"gratis":false}`, `Metatron moved the pile at (1,1) to (2,2)`},
	}
	for _, tc := range moveCases {
		if got := plainSegs(digestOf(t, "metatron.entity_moved", tc.payload)); got != tc.want {
			t.Errorf("entity_moved %s: plain summary = %q, want %q", tc.payload, got, tc.want)
		}
	}

	removeCases := []struct{ payload, want string }{
		{`{"class":"structure","x":1,"y":1,"gratis":false}`, `Metatron removed the structure at (1,1)`},
		{`{"class":"pile","x":1,"y":1,"gratis":false}`, `Metatron removed the pile at (1,1)`},
		{`{"class":"terrain","x":1,"y":1,"gratis":false}`, `Metatron cleared the terrain at (1,1)`},
	}
	for _, tc := range removeCases {
		if got := plainSegs(digestOf(t, "metatron.entity_removed", tc.payload)); got != tc.want {
			t.Errorf("entity_removed %s: plain summary = %q, want %q", tc.payload, got, tc.want)
		}
	}
}

// TestDigestAlertTypesDigestCleanly: the four alert-flagged types (contract
// §2) still digest through the registry like any other type — the alert
// treatment is a view-layer style decision (Phase 5), not a formatting one.
func TestDigestAlertTypesDigestCleanly(t *testing.T) {
	for _, typ := range []string{"agent.died", "gru.attacked", "social.chest_taken", "norm.violated"} {
		fx, ok := catalogFixture[typ]
		if !ok {
			t.Fatalf("missing fixture for alert type %q", typ)
		}
		digestOf(t, typ, fx.payload) // fails the test via t.Fatalf if it falls back
	}
}
