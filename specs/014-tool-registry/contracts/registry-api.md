# Contract: Registry API surface (spec 014)

The Go surface `internal/tool` exposes and the exact consumption points. Signatures are
the contract; bodies are implementation. The package imports nothing internal (leaf, R1).

## Package `internal/tool`

```go
type EffectClass int // World, Expressive, Read

type ParamKind int   // AgentName, Text, Enum

type Param struct {
    Name     string
    Kind     ParamKind
    Required bool
    MaxBytes int      // 0 = n/a
    MaxRunes int      // 0 = n/a
    Enum     []string // nil = n/a
}

type GateClass int // Resolvable, Charge, Scene, None

type Cost struct {
    DurationTicks int64
    Charges       int
    TextCapBytes  int
    TextCapRunes  int
}

type Tool struct {
    Name           string
    Effect         EffectClass
    Params         []Param
    Gate           GateClass
    Cost           Cost
    PlanStep       bool
    ReflexEligible bool
    PromptGloss    string
    Events         []string
}

// All returns every registered tool in registration order (stable; the
// prompt-derivation order anchor).
func All() []Tool

// Lookup returns the tool by name; ok=false for unknown names.
func Lookup(name string) (Tool, bool)

// Rosters: ordered tool-name lists per agent kind.
var RosterVillager []string
var RosterMetatron []string

// OnRoster reports membership (the door check).
func OnRoster(roster []string, name string) bool

// Derived surfaces — each is one walk of the registry:

// VocabularyLine returns the comma-joined World-tool names for the villager
// roster in registration order (byte-identical to the old goalVocabulary).
func VocabularyLine() string

// PromptGloss returns the concatenated gloss block (byte-identical to the
// old hand-written prose lines).
func PromptGlossBlock() string

// WorldGoals returns the set of World-class villager-roster names
// (replaces mind's validGoals).
func WorldGoals() map[string]bool

// PlanStepGoals returns names with PlanStep==true (replaces sim's planGoals;
// the FR-012 delta lives in the catalog data, not in code).
func PlanStepGoals() map[string]bool

// Validate enforces R9 (unique names, class/flag coherence, roster
// resolution, expressive Events non-empty). Whitelist-subset and
// resolver/duration coverage checks live sim-side (see below) because the
// whitelist and resolver tables are sim's. Returns all violations, not the first.
func Validate() error
```

## Consumption points (the only permitted ones)

| Consumer | Replaces | Call |
|---|---|---|
| `internal/mind/prompt.go` | `goalVocabulary` const + gloss prose | `tool.VocabularyLine()`, `tool.PromptGlossBlock()` |
| `internal/mind/parse.go` | `validGoals` map; cap literals (300/200/200) | `tool.WorldGoals()`; `tool.Lookup("say"/"gist"/"muse").Cost` |
| `internal/sim/loop.go` (plan-step + goal door) | `planGoals` map | `tool.PlanStepGoals()`, `tool.OnRoster(tool.RosterVillager, goal)` |
| `internal/sim/agents.go` | `intentDuration` switch | duration table built from `tool.All()` at init |
| `internal/sim` startup check | — (new) | asserts every World tool has a resolver-table arm + duration; every expressive `Events` ⊆ `injectSocialWhitelist` |
| `internal/metatron/turn.go` + `internal/sim/metatron.go` | nudge form strings + 400 literal | `tool.RosterMetatron`, `tool.Lookup("nudge_…").Cost` |
| `internal/daemon` startup | — (new) | `tool.Validate()` + sim-side coverage check; error aborts boot |

Unchanged by contract: `injectSocialWhitelist` (loop.go:146 — zero entries
added/removed), the `InjectIntent`/`InjectSocial` signatures and validation ladder
(generation → staleness → guards), `resolveGoal` semantics per verb, `executeAtTarget`,
`workDuration`, all reducer arms, `decideIntent` (reflex), musing/planner/conversation
scheduling, and every prompt except that its vocabulary/gloss text now comes from the
registry byte-identically.

## Test contract

1. **Golden prompt**: fixture captured from pre-refactor main; `systemPrompt` output must
   match byte-for-byte (SC-003, R3).
2. **Single-walk invariant**: `VocabularyLine` names ≡ `WorldGoals` keys ≡
   `PlanStepGoals` keys (TASK-55 AC#2).
3. **Door equivalence**: table-driven — every catalog name accepted at its door; every
   non-roster/unknown name rejected with today's rejection shape (SC-005).
4. **Replay byte-identity**: existing suite (`whole_feature_test.go:32`,
   `sim_test.go:68,99`, per-capability replay tests) passes unmodified (SC-002).
5. **Startup validation**: malformed registry/roster fixtures each produce a boot error
   (FR-003).
6. **Landing ladder**: existing generation/staleness/guard tests pass unmodified (FR-014);
   `cognition_test.go:16`, `governance_test.go:628`, `metatron_test.go:24,58` pin the
   whitelist/dry-run behavior.
