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

// invField reads one inventory count by its recipes-table item key. "spear"
// has no matching int field (durability lives in Inventory.Spears) — callers
// that touch spears handle that kind directly, never through this helper.
func invField(inv Inventory, kind string) int {
	switch kind {
	case "wood":
		return inv.Wood
	case "stone":
		return inv.Stone
	case "water":
		return inv.Water
	case "planks":
		return inv.Planks
	case "refined_stone":
		return inv.RefinedStone
	case "food_raw":
		return inv.FoodRaw
	case "food_cooked":
		return inv.FoodCooked
	case "meals":
		return inv.Meals
	}
	return 0
}

// hasItems reports whether inv carries at least each item's count — the
// completion-time input re-validation predicate shared by every hand-craft
// and build recipe (contested-resource pattern, spec 012 FR-014).
func hasItems(inv Inventory, items []Item) bool {
	for _, it := range items {
		if invField(inv, it.Kind) < it.N {
			return false
		}
	}
	return true
}

// addItems applies a signed delta to inv for every item kind except "spear"
// (sign -1 spends recipe inputs, +1 adds recipe outputs); durability for a
// crafted spear is appended by the caller directly via Inventory.Spears.
// Every field is clamped at 0 (maxInt), matching every other reducer
// decrement in this package (agent.ate, agent.cooked, agent.built) — inputs
// are re-validated before this ever runs, so the clamp is a defensive floor,
// not expected behavior.
func addItems(inv *Inventory, items []Item, sign int) {
	for _, it := range items {
		n := sign * it.N
		switch it.Kind {
		case "wood":
			inv.Wood = maxInt(0, inv.Wood+n)
		case "stone":
			inv.Stone = maxInt(0, inv.Stone+n)
		case "water":
			inv.Water = maxInt(0, inv.Water+n)
		case "planks":
			inv.Planks = maxInt(0, inv.Planks+n)
		case "refined_stone":
			inv.RefinedStone = maxInt(0, inv.RefinedStone+n)
		case "food_raw":
			inv.FoodRaw = maxInt(0, inv.FoodRaw+n)
		case "food_cooked":
			inv.FoodCooked = maxInt(0, inv.FoodCooked+n)
		case "meals":
			inv.Meals = maxInt(0, inv.Meals+n)
		}
	}
}

// craftKindFor maps a hand-craft goal to its CraftedPayload.Kind, and
// craftGoalFor is its inverse (the reducer only sees the kind, and re-derives
// the recipe by goal — recipes.go stays the single source).
func craftKindFor(goal string) string {
	switch goal {
	case "craft_planks":
		return "planks"
	case "craft_stone":
		return "refined_stone"
	case "craft_spear":
		return "spear"
	}
	return ""
}

func craftGoalFor(kind string) string {
	switch kind {
	case "planks":
		return "craft_planks"
	case "refined_stone":
		return "craft_stone"
	case "spear":
		return "craft_spear"
	}
	return ""
}
