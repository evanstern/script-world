package tool

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// wantWorldOrder is the world-tool registration order (= goalVocabulary order,
// the byte-identity anchor) mirrored from contracts/tool-catalog.md. A drift
// between this and the registry is either a catalog-vs-code mismatch or an
// accidental reordering that would move the prompt bytes.
var wantWorldOrder = []string{
	"forage", "chop", "hunt", "build_fire", "build_shelter", "eat", "sleep",
	"wander", "goto_warmth", "talk_to", "quarry", "collect_water", "cook",
	"refuel_fire", "craft_planks", "craft_stone", "craft_spear", "build_oven",
	"bathe", "drop", "pick_up", "build_chest", "deposit", "withdraw",
}

// wantExpressive is the expressive tools' registration order (catalog table
// order). The villager roster orders its expressive tail differently — say,
// muse, gist (data-model.md / T007) — captured separately below.
var wantExpressive = []string{"say", "gist", "muse"}
var wantMetatron = []string{"converse", "nudge_dream", "nudge_omen"}

// wantMetatronCatalog is the metatron tools' catalog membership (registration
// order): the RosterMetatron three plus work_miracle (spec 017 T019b). It is a
// superset of wantMetatron (the name-only DOOR roster) because work_miracle is a
// registered capability that lands through landMiracle, not through landNudge's
// OnRoster(RosterMetatron) check — so it belongs in the catalog and the LOOP
// roster, but RosterMetatron (the pre-loop door roster) is left unchanged.
var wantMetatronCatalog = []string{"converse", "nudge_dream", "nudge_omen", "work_miracle"}
var wantVillagerExpressiveTail = []string{"say", "muse", "gist"}

// wantJournal is the four villager journal tools (spec 019, US3): two Expressive
// (write/delete), two Read (search/read). Catalog membership mirrors
// contracts/tool-catalog.md; they join LoopRosterVillager only (villager-private).
var wantJournal = []string{"write_journal_entry", "delete_from_journal", "search_journal", "read_journal"}

// TestCatalogCompleteness: every catalog row is present, nothing extra is
// registered, the legacy (free-text-vocabulary) world-tool order is exactly
// goalVocabulary order (R3), and set_plan (spec 017) is registered as the
// sole Effect-World, loop-only (PlanStep false) entry, immediately after the
// legacy world tools.
func TestCatalogCompleteness(t *testing.T) {
	var gotWorld []string
	gotAll := make(map[string]bool)
	for _, tl := range All() {
		gotAll[tl.Name] = true
		if isLegacyWorldTool(tl) {
			gotWorld = append(gotWorld, tl.Name)
		}
	}

	if !reflect.DeepEqual(gotWorld, wantWorldOrder) {
		t.Errorf("legacy world-tool order drifted.\n got: %v\nwant: %v", gotWorld, wantWorldOrder)
	}

	wantAll := make(map[string]bool)
	catalog := append(append([]string{}, wantWorldOrder...), "set_plan")
	catalog = append(catalog, wantExpressive...)
	catalog = append(catalog, wantMetatronCatalog...)
	catalog = append(catalog, wantJournal...)
	for _, n := range catalog {
		wantAll[n] = true
	}
	for n := range gotAll {
		if !wantAll[n] {
			t.Errorf("registry has unexpected tool %q (not in catalog)", n)
		}
	}
	for n := range wantAll {
		if !gotAll[n] {
			t.Errorf("registry is missing catalog tool %q", n)
		}
	}

	setPlan, ok := Lookup("set_plan")
	if !ok {
		t.Fatal("set_plan not registered")
	}
	if setPlan.Effect != World {
		t.Errorf("set_plan.Effect = %v, want World", setPlan.Effect)
	}
	if setPlan.PlanStep {
		t.Error("set_plan.PlanStep = true, want false (it must stay out of the legacy vocabulary)")
	}
	if len(setPlan.InputSchemaJSON) == 0 {
		t.Error("set_plan has no InputSchemaJSON override")
	}
	// Registration order: set_plan sits immediately after the legacy world
	// tools, before the expressive tools, so nothing else's position shifts.
	all := All()
	if all[len(wantWorldOrder)].Name != "set_plan" {
		t.Errorf("registry index %d = %q, want set_plan immediately after the %d legacy world tools", len(wantWorldOrder), all[len(wantWorldOrder)].Name, len(wantWorldOrder))
	}
}

// TestSingleWalkInvariant (TASK-55 AC#2): the prompt vocabulary, the mind parse
// accept set, and the plan-step accept set are the SAME set — divergence is
// impossible by construction because all three are one walk of the registry.
func TestSingleWalkInvariant(t *testing.T) {
	vocab := strings.Split(VocabularyLine(), ", ")
	world := WorldGoals()
	plan := PlanStepGoals()

	if len(vocab) != len(world) || len(world) != len(plan) {
		t.Fatalf("set sizes differ: vocab %d, world %d, plan %d", len(vocab), len(world), len(plan))
	}
	for _, name := range vocab {
		if !world[name] {
			t.Errorf("vocabulary name %q missing from WorldGoals", name)
		}
		if !plan[name] {
			t.Errorf("vocabulary name %q missing from PlanStepGoals", name)
		}
	}
	// And the vocabulary is exactly the world-tool order (no dupes, right count).
	if !reflect.DeepEqual(vocab, wantWorldOrder) {
		t.Errorf("VocabularyLine order = %v, want %v", vocab, wantWorldOrder)
	}
}

// TestPromptGlossBlockStructure: the gloss block is the six per-verb lines in
// registration order, each newline-terminated (the byte-exactness against the
// live prompt is pinned by TestGoldenPrompt in internal/mind).
func TestPromptGlossBlockStructure(t *testing.T) {
	block := PromptGlossBlock()
	if !strings.HasSuffix(block, "\n") {
		t.Fatalf("gloss block must end in a newline")
	}
	lines := strings.Split(strings.TrimSuffix(block, "\n"), "\n")
	if len(lines) != 6 {
		t.Fatalf("gloss block has %d lines, want 6:\n%s", len(lines), block)
	}
	leads := []string{"quarry", "cook", "craft_planks", "build_oven", "drop", "build_chest"}
	for i, want := range leads {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("gloss line %d starts %q, want prefix %q", i, lines[i], want)
		}
	}
}

// TestTestOnlyToolFlows (T017, US1 acceptance scenario 4 / SC-001): a tool
// registered only in this test appears in all three derived surfaces — the
// prompt vocabulary, the parse accept set, and the plan-step door set (what the
// sim door consults) — with no other edit; removing it removes it everywhere.
func TestTestOnlyToolFlows(t *testing.T) {
	const probe = "probe_verb"
	restore := withExtraTool(Tool{Name: probe, Effect: World, Gate: Resolvable, PlanStep: true})
	defer restore()

	if !strings.Contains(VocabularyLine(), probe) {
		t.Errorf("probe tool absent from VocabularyLine: %s", VocabularyLine())
	}
	if !WorldGoals()[probe] {
		t.Errorf("probe tool absent from WorldGoals")
	}
	if !PlanStepGoals()[probe] {
		t.Errorf("probe tool absent from PlanStepGoals (the door would reject it)")
	}
	if _, ok := Lookup(probe); !ok {
		t.Errorf("probe tool not found via Lookup")
	}

	restore()
	if strings.Contains(VocabularyLine(), probe) || WorldGoals()[probe] || PlanStepGoals()[probe] {
		t.Errorf("probe tool survived removal in a derived surface")
	}
	if _, ok := Lookup(probe); ok {
		t.Errorf("probe tool survived removal in Lookup")
	}
}

// TestStorageVerbsCarryQty (spec 017 R12/T002): qty is a declared optional
// Number param (Min 1, unbounded Max) on exactly the four storage verbs —
// build_chest takes neither kind nor qty.
func TestStorageVerbsCarryQty(t *testing.T) {
	// The four storage verbs (spec 014 debt, R12) plus work_miracle's give_item
	// (spec 017 T019b) carry an optional, Min-1, unbounded qty.
	wantQty := map[string]bool{"drop": true, "pick_up": true, "deposit": true, "withdraw": true, "work_miracle": true}

	for _, tl := range All() {
		var qty *Param
		for i := range tl.Params {
			if tl.Params[i].Name == "qty" {
				qty = &tl.Params[i]
			}
		}
		if wantQty[tl.Name] {
			if qty == nil {
				t.Errorf("tool %q: expected a qty param, found none", tl.Name)
				continue
			}
			if qty.Kind != Number {
				t.Errorf("tool %q: qty param Kind = %v, want Number", tl.Name, qty.Kind)
			}
			if qty.Required {
				t.Errorf("tool %q: qty param must be optional", tl.Name)
			}
			if qty.Min != 1 {
				t.Errorf("tool %q: qty param Min = %d, want 1", tl.Name, qty.Min)
			}
			if qty.Max != 0 {
				t.Errorf("tool %q: qty param Max = %d, want 0 (unbounded)", tl.Name, qty.Max)
			}
		} else if qty != nil {
			t.Errorf("tool %q: unexpected qty param (only drop/pick_up/deposit/withdraw take qty)", tl.Name)
		}
	}
}

// TestValidateRealRegistry: the shipped registry + rosters validate clean.
func TestValidateRealRegistry(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatalf("shipped registry failed Validate: %v", err)
	}
}

// TestRostersResolve: every roster name resolves and rosters are non-empty;
// the villager roster carries every world tool plus the three expressive tools.
func TestRostersResolve(t *testing.T) {
	for _, n := range append(append([]string{}, RosterVillager...), RosterMetatron...) {
		if _, ok := Lookup(n); !ok {
			t.Errorf("roster name %q does not resolve", n)
		}
	}
	wantVillager := append(append([]string{}, wantWorldOrder...), wantVillagerExpressiveTail...)
	if !reflect.DeepEqual(RosterVillager, wantVillager) {
		t.Errorf("RosterVillager = %v, want %v", RosterVillager, wantVillager)
	}
	got := append([]string{}, RosterMetatron...)
	sort.Strings(got)
	want := append([]string{}, wantMetatron...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RosterMetatron = %v, want %v", RosterMetatron, wantMetatron)
	}
}
