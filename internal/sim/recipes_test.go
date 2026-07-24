package sim

import (
	"reflect"
	"testing"
)

// TestRecipeTableMirror pins recipes.go to the numbers in
// specs/012-resources-food-crafting/contracts/recipes.md. The want-values here
// are the contract's literals, transcribed independently of the constants the
// table is built from — so a drift in either the table or a tuning constant
// trips this test.
func TestRecipeTableMirror(t *testing.T) {
	want := map[string]Recipe{
		"craft_planks":  {Goal: "craft_planks", Inputs: []Item{{"wood", 1}}, Outputs: []Item{{"planks", 4}}, Duration: 180, Site: SiteAnywhere},
		"craft_stone":   {Goal: "craft_stone", Inputs: []Item{{"stone", 1}}, Outputs: []Item{{"refined_stone", 1}}, Duration: 180, Site: SiteAnywhere},
		"craft_spear":   {Goal: "craft_spear", Inputs: []Item{{"wood", 1}, {"refined_stone", 1}}, Outputs: []Item{{"spear", 1}}, Duration: 240, Site: SiteAnywhere},
		"build_fire":    {Goal: "build_fire", Inputs: []Item{{"wood", 2}}, Structure: "fire", Duration: 600, Site: SiteOnSite},
		"build_shelter": {Goal: "build_shelter", Inputs: []Item{{"planks", 8}}, Structure: "shelter", Duration: 1200, Site: SiteOnSite},
		"build_oven":    {Goal: "build_oven", Inputs: []Item{{"refined_stone", 4}, {"planks", 2}}, Structure: "oven", Duration: 900, Site: SiteOnSite},
		"build_chest":   {Goal: "build_chest", Inputs: []Item{{"planks", 6}}, Structure: "chest", Duration: 600, Site: SiteOnSite},
		// Spec 032 (walls) — contracts/recipes.md literals.
		"build_wall_plank": {Goal: "build_wall_plank", Inputs: []Item{{"planks", 2}}, Structure: "wall_plank", Duration: 600, Site: SiteOnSite},
		"build_wall_stone": {Goal: "build_wall_stone", Inputs: []Item{{"refined_stone", 2}}, Structure: "wall_stone", Duration: 600, Site: SiteOnSite},
		"cook_fire":     {Goal: "cook_fire", Inputs: []Item{{"food_raw", 8}}, Outputs: []Item{{"food_cooked", 8}}, Duration: 240, Site: SiteStation},
		"cook_oven":     {Goal: "cook_oven", Inputs: []Item{{"wood", 1}, {"food_raw", 8}}, Outputs: []Item{{"meals", 8}}, Duration: 360, Site: SiteStation},
		"bathe":         {Goal: "bathe", Inputs: []Item{{"water", 1}, {"wood", 1}}, Duration: 240, Site: SiteStation},
		"refuel_fire":   {Goal: "refuel_fire", Inputs: []Item{{"wood", 1}}, Duration: 0, Site: SiteStation},
	}

	if len(recipes) != len(want) {
		t.Fatalf("recipe count = %d, want %d (contract has %d rows)", len(recipes), len(want), len(want))
	}
	for goal, w := range want {
		got, ok := recipeFor(goal)
		if !ok {
			t.Errorf("recipe %q missing from table", goal)
			continue
		}
		if !reflect.DeepEqual(got, w) {
			t.Errorf("recipe %q:\n got  %+v\n want %+v", goal, got, w)
		}
	}
	// No stray rows beyond the contract.
	for _, r := range recipes {
		if _, ok := want[r.Goal]; !ok {
			t.Errorf("recipe table has unexpected goal %q", r.Goal)
		}
	}
}

// TestGatherTuningMirror pins the gather-side numbers (recipes.md §Gathering
// and the v2 food yields) that live as tuning constants rather than recipe
// rows.
func TestGatherTuningMirror(t *testing.T) {
	checks := []struct {
		name string
		got  int
		want int
	}{
		{"quarryYield", quarryYield, 2},
		{"quarryTicks", quarryTicks, 400},
		{"collectWaterYield", collectWaterYield, 1},
		{"collectWaterTicks", collectWaterTicks, 60},
		{"forageYieldV2", forageYieldV2, 2},
		{"huntYieldBare", huntYieldBare, 8},
		{"huntYieldSpear", huntYieldSpear, 12},
		{"huntTicksSpear", huntTicksSpear, 600},
		{"foodRawRestore", foodRawRestore, 40},
		{"foodCookedRestore", foodCookedRestore, 80},
		{"mealRestore", mealRestore, 100},
		{"satietyAt", satietyAt, 900},
		{"fireBurnPerWood", fireBurnPerWood, 4 * 3600},
		{"fireFuelCap", fireFuelCap, 12 * 3600},
		{"spearDurability", spearDurability, 3},
		{"restRegenShelter", restRegenShelter, 6},
		{"bathMorale", bathMorale, 150},
		{"bathWarmth", bathWarmth, 300},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// TestWallTuningMirror pins the spec-032 wall tuning constants (research R8 /
// contracts/recipes.md) and the derived-max-HP / repair-material helpers.
func TestWallTuningMirror(t *testing.T) {
	checks := []struct {
		name string
		got  int
		want int
	}{
		{"wallPlankHP", wallPlankHP, 200},
		{"wallStoneHP", wallStoneHP, 600},
		{"buildWallTicks", buildWallTicks, 600},
		{"demolishChipHP", demolishChipHP, 100},
		{"demolishTicks", demolishTicks, 300},
		{"repairHPPerUnit", repairHPPerUnit, 100},
		{"repairTicks", repairTicks, 240},
		{"wallMaxHP(wall_plank)", wallMaxHP("wall_plank"), 200},
		{"wallMaxHP(wall_stone)", wallMaxHP("wall_stone"), 600},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
	// Stone endures strictly more damage than plank (spec FR-003: ≥2x).
	if wallStoneHP < 2*wallPlankHP {
		t.Errorf("stone wall HP (%d) must be at least twice plank HP (%d)", wallStoneHP, wallPlankHP)
	}
	if got := wallRepairMaterial("wall_plank"); got != "planks" {
		t.Errorf("wallRepairMaterial(wall_plank) = %q, want planks", got)
	}
	if got := wallRepairMaterial("wall_stone"); got != "refined_stone" {
		t.Errorf("wallRepairMaterial(wall_stone) = %q, want refined_stone", got)
	}
}
