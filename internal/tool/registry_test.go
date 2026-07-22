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
var wantVillagerExpressiveTail = []string{"say", "muse", "gist"}

// TestCatalogCompleteness: every catalog row is present, nothing extra is
// registered, and the world-tool order is exactly goalVocabulary order (R3).
func TestCatalogCompleteness(t *testing.T) {
	var gotWorld []string
	gotAll := make(map[string]bool)
	for _, tl := range All() {
		gotAll[tl.Name] = true
		if tl.Effect == World {
			gotWorld = append(gotWorld, tl.Name)
		}
	}

	if !reflect.DeepEqual(gotWorld, wantWorldOrder) {
		t.Errorf("world-tool order drifted.\n got: %v\nwant: %v", gotWorld, wantWorldOrder)
	}

	wantAll := make(map[string]bool)
	for _, n := range append(append(append([]string{}, wantWorldOrder...), wantExpressive...), wantMetatron...) {
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
