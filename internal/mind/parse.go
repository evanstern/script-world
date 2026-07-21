package mind

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// planReply is the goal JSON contract; a reply is either one goal or a
// short guarded plan (TASK-32 US4), never both.
type planReply struct {
	Goal   string          `json:"goal"`
	Target string          `json:"target,omitempty"`
	Reason string          `json:"reason"`
	Plan   []planStepReply `json:"plan,omitempty"`
}

// planStepReply is one model-expressed step: timed guards only in v1 —
// after_min becomes an after_tick guard, for_min bounds the window.
type planStepReply struct {
	Goal     string  `json:"goal"`
	Target   string  `json:"target,omitempty"`
	AfterMin float64 `json:"after_min,omitempty"`
	ForMin   float64 `json:"for_min,omitempty"`
}

// planStepCap mirrors sim.PlanStepCap for the prompt and the parser.
const planStepCap = 3

var validGoals = map[string]bool{
	"forage": true, "chop": true, "hunt": true,
	"build_fire": true, "build_shelter": true,
	"eat": true, "sleep": true, "wander": true,
	"goto_warmth": true, "talk_to": true,
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
	const museMaxRunes = 200
	if r := []rune(t); len(r) > museMaxRunes {
		t = string(r[:museMaxRunes])
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
	if len(r.Say) > 300 {
		r.Say = r.Say[:300]
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
	if len(o.Gist) > 200 {
		o.Gist = o.Gist[:200]
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
			if !validGoals[r.Plan[i].Goal] {
				return planReply{}, fmt.Errorf("plan step %d: unknown goal %q", i, r.Plan[i].Goal)
			}
			if r.Plan[i].AfterMin < 0 || r.Plan[i].ForMin < 0 {
				return planReply{}, fmt.Errorf("plan step %d: negative time", i)
			}
		}
		r.Goal = ""
		return r, nil
	}
	r.Goal = strings.ToLower(strings.TrimSpace(r.Goal))
	if !validGoals[r.Goal] {
		return planReply{}, fmt.Errorf("unknown goal %q", r.Goal)
	}
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
