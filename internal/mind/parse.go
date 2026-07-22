package mind

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/evanstern/promptworld/internal/tool"
)

// planReply is the goal JSON contract; a reply is either one goal or a
// short guarded plan (TASK-32 US4), never both.
type planReply struct {
	Goal   string          `json:"goal"`
	Target string          `json:"target,omitempty"`
	// Kind/Qty (spec 013 T022) argue drop/pick_up: Kind is an inventory item
	// key ("" = all kinds, pick_up only), Qty a per-kind cap (0 = all of
	// kind / as much as fits) — mirrors sim.Intent.Kind/Qty (data-model.md).
	Kind   string          `json:"kind,omitempty"`
	Qty    int             `json:"qty,omitempty"`
	Reason string          `json:"reason"`
	Plan   []planStepReply `json:"plan,omitempty"`
}

// planStepReply is one model-expressed step: timed guards only in v1 —
// after_min becomes an after_tick guard, for_min bounds the window.
type planStepReply struct {
	Goal     string  `json:"goal"`
	Target   string  `json:"target,omitempty"`
	Kind     string  `json:"kind,omitempty"`
	Qty      int     `json:"qty,omitempty"`
	AfterMin float64 `json:"after_min,omitempty"`
	ForMin   float64 `json:"for_min,omitempty"`
}

// planStepCap mirrors sim.PlanStepCap for the prompt and the parser.
const planStepCap = 3

// worldGoals is the parser's accept set for goal names, DERIVED from the tool
// registry (spec 014, FR-005): the set of World-class villager-roster names.
// It replaces the old hand-maintained worldGoals map — the accept set is
// identical, but it can no longer drift from the prompt vocabulary or the
// sim-door plan-step set (all three are one walk of the registry). Cached once
// at package init; the registry is immutable after startup.
var worldGoals = tool.WorldGoals()

// Expressive text caps are read from the tool registry (spec 014 T020/R7) so
// the parser and the registry never carry divergent literals. Values are
// byte-identical to the old local literals (say 300 bytes, gist 200 bytes,
// muse 200 runes); the registry is now their single source.
var (
	sayCapBytes  = capBytes("say")
	gistCapBytes = capBytes("gist")
	museCapRunes = capRunes("muse")
)

func capBytes(name string) int {
	t, _ := tool.Lookup(name)
	return t.Cost.TextCapBytes
}

func capRunes(name string) int {
	t, _ := tool.Lookup(name)
	return t.Cost.TextCapRunes
}

// validKinds are the inventory item keys drop/pick_up/deposit/withdraw
// accept as Kind — exactly internal/sim's canonicalKinds
// (internal/sim/agents.go), the set the executor actually reads counts by
// ("spears" plural: durability lives in the slice, there is no singular
// "spear" field). "" is valid too: for pick_up/withdraw it means every kind
// (canonical order); for drop/deposit the executor resolves an empty Kind
// to a no-op (agent.intent_done, no pile/chest touched) — not a parse-time
// error either way (the deposit-needs-a-kind rule is carried as prompt
// guidance, not parser rejection — see prompt.go).
var validKinds = map[string]bool{
	"": true,
	"wood": true, "stone": true, "water": true, "planks": true, "refined_stone": true,
	"food_raw": true, "food_cooked": true, "meals": true, "spears": true,
}

// sortedKeys returns a map-set's keys in deterministic (sorted) order — so a
// schema built from worldGoals/validKinds is stable across runs rather than
// reflecting Go's map iteration order.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// plannerReplySchema is the OpenAI/Ollama structured-output JSON Schema for a
// planner reply, generated from worldGoals + planStepCap (TASK-58) so the
// sampler-level constraint and parseReply's gate share one source of truth —
// the goal vocabulary and step cap are never hand-copied. kind is constrained
// to validKinds ("" included, a legal value). The cloud tier ignores this;
// parseReply stays the final gate.
//
// "goal" and "reason" are required at the top level, and each plan step
// requires "goal". Requiring goal does NOT break the plan form: plan is an
// optional property, the model still emits it, and parseReply prefers a
// present plan (discarding the top-level goal) — so a plan reply parses as a
// plan. This deviates from the original TASK-58 brief (which specified
// required: ["reason"] only, to keep the plan form legal). Live probing of
// cogito:3b showed that shape leaves ~1/3 of replies as reason-only objects
// ({"reason": "..."} with neither goal nor plan) — schema-valid but unusable,
// failing AC#5's "0 unusable". The clean goal-xor-plan encoding (anyOf) was
// rejected empirically: llama.cpp's schema-to-grammar converter bails out on
// anyOf and applies NO constraint at all. Requiring goal is the only shape
// that both stays enforced and drives unusable replies to zero.
//
// The free-text fields (reason, target) carry maxLength bounds — also honored
// by llama.cpp's grammar. Without them a rambling reason overruns the 256-token
// planner budget mid-string and the reply arrives as truncated, unterminated
// JSON (another unusable class seen live). The bounds are generous for the
// "one short sentence" the prompt asks for; parseReply does not itself cap
// reason, so the ceiling only exists to keep the reply inside the token budget.
func plannerReplySchema() json.RawMessage {
	goals := sortedKeys(worldGoals)
	kinds := sortedKeys(validKinds)
	// reasonMaxLen/targetMaxLen bound the model's free-text fields so a reply
	// can't blow the planner's max_tokens budget through prose (see above).
	const reasonMaxLen, targetMaxLen = 200, 80
	step := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"goal":      map[string]any{"type": "string", "enum": goals},
			"target":    map[string]any{"type": "string", "maxLength": targetMaxLen},
			"kind":      map[string]any{"type": "string", "enum": kinds},
			"qty":       map[string]any{"type": "integer"},
			"after_min": map[string]any{"type": "number"},
			"for_min":   map[string]any{"type": "number"},
		},
		"required": []string{"goal"},
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"goal":   map[string]any{"type": "string", "enum": goals},
			"target": map[string]any{"type": "string", "maxLength": targetMaxLen},
			"kind":   map[string]any{"type": "string", "enum": kinds},
			"qty":    map[string]any{"type": "integer"},
			"reason": map[string]any{"type": "string", "maxLength": reasonMaxLen},
			"plan": map[string]any{
				"type":     "array",
				"maxItems": planStepCap,
				"items":    step,
			},
		},
		"required": []string{"goal", "reason"},
	}
	b, err := json.Marshal(schema)
	if err != nil {
		// The schema is built from literal maps; marshaling cannot fail. Panic
		// is the honest response to an impossible state at package init.
		panic("mind: planner reply schema marshal: " + err.Error())
	}
	return b
}

// plannerSchema is built once — worldGoals derives from the immutable tool
// registry at init, and validKinds/planStepCap are compile-time constants, so
// the schema never changes at runtime.
var plannerSchema = plannerReplySchema()

// validateKindQty normalizes and validates a drop/pick_up/deposit/withdraw
// step's Kind/Qty against what the sim executor actually accepts
// (canonicalKinds) — the same "reject unknown at the door" discipline
// worldGoals applies to goal strings, so a malformed kind never reaches
// InjectIntent. build_chest takes no Kind/Qty (like every other goal not
// listed here): its Kind/Qty are ignored (zero-value from a model that
// didn't emit them).
func validateKindQty(goal, kind string, qty int) (string, error) {
	switch goal {
	case "drop", "pick_up", "deposit", "withdraw":
	default:
		return kind, nil
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if !validKinds[kind] {
		return "", fmt.Errorf("goal %q: unknown kind %q", goal, kind)
	}
	if qty < 0 {
		return "", fmt.Errorf("goal %q: negative qty %d", goal, qty)
	}
	return kind, nil
}

// parseMusing accepts one plain line of interiority (TASK-21): first line,
// quotes and whitespace stripped, rune-capped. Empty or JSON-shaped replies
// are model failures, not thoughts.
func parseMusing(text string) (string, error) {
	t := strings.TrimSpace(text)
	if i := strings.IndexByte(t, '\n'); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	t = strings.Trim(t, "\"' ")
	if t == "" || strings.HasPrefix(t, "{") {
		return "", fmt.Errorf("unusable musing %q", text)
	}
	if r := []rune(t); len(r) > museCapRunes {
		t = string(r[:museCapRunes])
	}
	return t, nil
}

// firstJSON extracts the first balanced JSON object from model output.
func firstJSON(text string) (string, error) {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return "", fmt.Errorf("no JSON object in reply")
	}
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("unterminated JSON object")
}

// parseSay extracts a conversation utterance.
func parseSay(text string) (string, error) {
	raw, err := firstJSON(text)
	if err != nil {
		return "", err
	}
	var r struct {
		Say string `json:"say"`
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return "", fmt.Errorf("bad say JSON: %w", err)
	}
	r.Say = strings.TrimSpace(r.Say)
	if r.Say == "" {
		return "", fmt.Errorf("empty utterance")
	}
	if len(r.Say) > sayCapBytes {
		r.Say = r.Say[:sayCapBytes]
	}
	return r.Say, nil
}

// parseOutcome extracts the conversation summary with clamped tones.
func parseOutcome(text string) (convoOutcome, error) {
	raw, err := firstJSON(text)
	if err != nil {
		return convoOutcome{}, err
	}
	var o convoOutcome
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		// Lenient recovery (TASK-42 R3) for the one shape observed live: the
		// model emits gist/retold as a bare, unquoted prose value. Quote the
		// bare value(s) and retry once — anything past the known shape stays a
		// failure (the retry site covers it). A lenient success is a parse
		// success: no retry is consumed.
		fixed := lenientOutcome(raw)
		if fixed == "" {
			return convoOutcome{}, fmt.Errorf("bad outcome JSON: %w", err)
		}
		if err := json.Unmarshal([]byte(fixed), &o); err != nil {
			return convoOutcome{}, fmt.Errorf("bad outcome JSON: %w", err)
		}
	}
	if o.Gist == "" {
		return convoOutcome{}, fmt.Errorf("empty gist")
	}
	if len(o.Gist) > gistCapBytes {
		o.Gist = o.Gist[:gistCapBytes]
	}
	clamp := func(v float64) int {
		r := int(math.Round(v))
		if r < -2 {
			return -2
		}
		if r > 2 {
			return 2
		}
		return r
	}
	// Per-participant tones (TASK-22); the pre-TASK-22 pair shape
	// (tone_a/tone_b) still parses so older prompts and models degrade
	// gracefully.
	if len(o.RawTones) > 0 {
		for _, v := range o.RawTones {
			o.Tones = append(o.Tones, clamp(v))
		}
	} else {
		o.Tones = []int{clamp(o.RawToneA), clamp(o.RawToneB)}
	}
	if len(o.Topics) > 3 {
		o.Topics = o.Topics[:3]
	}
	for i, t := range o.Topics {
		if len(t) > 40 {
			o.Topics[i] = t[:40]
		}
	}
	if strings.EqualFold(strings.TrimSpace(o.Retold), "null") {
		o.Retold = ""
	}
	return o, nil
}

// lenientOutcome repairs the observed unquoted-value shape (TASK-42 R3):
//
//	{"gist": Hazel talked about the fire, "topics": [...], "retold": null}
//
// It quotes bare gist/retold values and returns JSON encoding/json can read;
// "" means nothing was repaired (leave the original failure to the retry).
// This is deliberately NOT a general tolerant parser.
func lenientOutcome(raw string) string {
	out := quoteBareValue(raw, "gist")
	out = quoteBareValue(out, "retold")
	if out == raw {
		return ""
	}
	return out
}

func isJSONSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

// quoteBareValue quotes an unquoted string value for key in a flat JSON
// object. The value runs to the next `, "` (the following key) or the object
// close; if that span is already valid JSON it is left untouched.
func quoteBareValue(s, key string) string {
	marker := `"` + key + `"`
	ki := strings.Index(s, marker)
	if ki < 0 {
		return s
	}
	rest := ki + len(marker)
	ci := strings.IndexByte(s[rest:], ':')
	if ci < 0 {
		return s
	}
	vstart := rest + ci + 1
	for vstart < len(s) && isJSONSpace(s[vstart]) {
		vstart++
	}
	if vstart >= len(s) {
		return s
	}
	vend := -1
	for j := vstart; j < len(s); j++ {
		if s[j] == '}' {
			vend = j
			break
		}
		if s[j] == ',' {
			k := j + 1
			for k < len(s) && isJSONSpace(s[k]) {
				k++
			}
			if k < len(s) && s[k] == '"' {
				vend = j
				break
			}
		}
	}
	if vend < 0 {
		return s
	}
	region := strings.TrimRight(s[vstart:vend], " \t\n\r")
	if region == "" || json.Valid([]byte(region)) {
		return s // already a valid JSON value — nothing to repair
	}
	enc, err := json.Marshal(region) // correct quoting/escaping/UTF-8
	if err != nil {
		return s
	}
	return s[:vstart] + string(enc) + s[vend:]
}

// parseReply extracts the first JSON object from model output and validates
// the goal. Small local models pad JSON with prose (and sometimes
// chain-of-thought tags); everything outside the braces is ignored.
func parseReply(text string) (planReply, error) {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return planReply{}, fmt.Errorf("no JSON object in reply")
	}
	depth := 0
	end := -1
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
		if end > 0 {
			break
		}
	}
	if end < 0 {
		return planReply{}, fmt.Errorf("unterminated JSON object")
	}
	var r planReply
	if err := json.Unmarshal([]byte(text[start:end]), &r); err != nil {
		return planReply{}, fmt.Errorf("bad JSON: %w", err)
	}
	if len(r.Plan) > 0 {
		// The plan form: every step's goal must be in vocabulary, the cap
		// is hard (an over-long plan is a model failure, not a trim).
		if len(r.Plan) > planStepCap {
			return planReply{}, fmt.Errorf("plan has %d steps (cap %d)", len(r.Plan), planStepCap)
		}
		for i := range r.Plan {
			r.Plan[i].Goal = strings.ToLower(strings.TrimSpace(r.Plan[i].Goal))
			if !worldGoals[r.Plan[i].Goal] {
				return planReply{}, fmt.Errorf("plan step %d: unknown goal %q", i, r.Plan[i].Goal)
			}
			if r.Plan[i].AfterMin < 0 || r.Plan[i].ForMin < 0 {
				return planReply{}, fmt.Errorf("plan step %d: negative time", i)
			}
			kind, kerr := validateKindQty(r.Plan[i].Goal, r.Plan[i].Kind, r.Plan[i].Qty)
			if kerr != nil {
				return planReply{}, fmt.Errorf("plan step %d: %w", i, kerr)
			}
			r.Plan[i].Kind = kind
		}
		r.Goal = ""
		return r, nil
	}
	r.Goal = strings.ToLower(strings.TrimSpace(r.Goal))
	if !worldGoals[r.Goal] {
		return planReply{}, fmt.Errorf("unknown goal %q", r.Goal)
	}
	kind, kerr := validateKindQty(r.Goal, r.Kind, r.Qty)
	if kerr != nil {
		return planReply{}, kerr
	}
	r.Kind = kind
	return r, nil
}

// parseConsolidation extracts the nightly consolidation output (TASK-9).
// Parse failures are consolidation rejections ("unparseable"), never
// partial landings.
func parseConsolidation(text string) (consolidationOutput, error) {
	raw, err := firstJSON(text)
	if err != nil {
		return consolidationOutput{}, err
	}
	var out consolidationOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return consolidationOutput{}, err
	}
	return out, nil
}
