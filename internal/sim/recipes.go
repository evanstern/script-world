package sim

// Recipe table — the authoritative machine mirror of
// specs/012-resources-food-crafting/contracts/recipes.md (spec 012). The
// human-readable contract table and this table must agree; recipes_test.go
// asserts the mirror against the contract's literal numbers.
//
// This is compile-time data, never part of serialized State, so a keyed lookup
// is determinism-safe (the state hash never sees it). Behavior wiring (input
// re-validation, delta application) lands in later phases (T026/T030–T037);
// Phase 2 only establishes the table.

// Site is where a recipe may be executed.
type Site string

const (
	SiteAnywhere Site = "anywhere" // hand-craft on the agent's current tile
	SiteOnSite   Site = "on_site"  // build on open buildable ground
	SiteStation  Site = "station"  // act at a fire or oven
)

// Item is an inventory count: Kind is the inventory JSON field key
// (wood/stone/water/planks/refined_stone/food_raw/food_cooked/meals/spear).
type Item struct {
	Kind string
	N    int
}

// Recipe is one transformation. Inputs are consumed at completion (after
// re-validation); Outputs are inventory items produced; Structure (non-empty
// only for build_*) is the structure Kind placed on the site. For the two cook
// recipes, the FoodRaw input N is the batch cap ("up to N") and Outputs mirror
// that cap — the executor consumes min(cap, carried). Bathe and refuel produce
// no inventory items (their effect is on needs / fire fuel).
type Recipe struct {
	Goal      string
	Inputs    []Item
	Outputs   []Item
	Structure string
	Duration  int64
	Site      Site
}

// recipes is the full v2 table. Order mirrors the contract (hand-crafts, then
// builds, then station actions). build_fire keeps its legacy wood cost; the
// v2 change is the FuelUntil window, not the cost.
var recipes = []Recipe{
	// Hand-crafts (anywhere, planner-only).
	{Goal: "craft_planks", Inputs: []Item{{"wood", 1}}, Outputs: []Item{{"planks", plankYield}}, Duration: craftPlanksTicks, Site: SiteAnywhere},
	{Goal: "craft_stone", Inputs: []Item{{"stone", 1}}, Outputs: []Item{{"refined_stone", 1}}, Duration: craftStoneTicks, Site: SiteAnywhere},
	{Goal: "craft_spear", Inputs: []Item{{"wood", 1}, {"refined_stone", 1}}, Outputs: []Item{{"spear", 1}}, Duration: craftSpearTicks, Site: SiteAnywhere},

	// Builds (on-site). Fire is also reflex-buildable; the rest are planner-only.
	{Goal: "build_fire", Inputs: []Item{{"wood", fireWoodCost}}, Structure: "fire", Duration: buildFireTicks, Site: SiteOnSite},
	{Goal: "build_shelter", Inputs: []Item{{"planks", shelterPlankCost}}, Structure: "shelter", Duration: buildShelterTicks, Site: SiteOnSite},
	{Goal: "build_oven", Inputs: []Item{{"refined_stone", 4}, {"planks", 2}}, Structure: "oven", Duration: buildOvenTicks, Site: SiteOnSite},

	// Station actions. cook_fire is fuel-free (the fire's own fuel); cook_oven
	// and bathe each burn 1 wood from the worker's inventory.
	{Goal: "cook_fire", Inputs: []Item{{"food_raw", ovenBatchSize}}, Outputs: []Item{{"food_cooked", ovenBatchSize}}, Duration: cookFireTicks, Site: SiteStation},
	{Goal: "cook_oven", Inputs: []Item{{"wood", 1}, {"food_raw", ovenBatchSize}}, Outputs: []Item{{"meals", ovenBatchSize}}, Duration: cookOvenTicks, Site: SiteStation},
	{Goal: "bathe", Inputs: []Item{{"water", 1}, {"wood", 1}}, Duration: batheTicks, Site: SiteStation},
	{Goal: "refuel_fire", Inputs: []Item{{"wood", 1}}, Duration: 0, Site: SiteStation},
}

// recipeFor returns the recipe for a goal and whether one exists.
func recipeFor(goal string) (Recipe, bool) {
	for _, r := range recipes {
		if r.Goal == goal {
			return r, true
		}
	}
	return Recipe{}, false
}
