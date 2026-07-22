package mind

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/evanstern/promptworld/internal/tool"
)

// The villager planner reply contract (planReply / planStepReply / the goal
// accept set / the sampler schema / parseReply) retired with spec 017: the
// planner now calls tools instead of emitting free-text JSON, so the model's
// output is parsed by the transport (native tool_calls or the fallback
// envelope), validated by the loop driver against the registry's derived
// schemas, and dispatched to handlers. What remains here is the conversation
// and consolidation parsing, which keep their pre-loop mechanics (spec 017
// FR-014). parseMusing survives only for the scheduled musing channel until
// T013 deletes it; the muse TOOL receives structured args, not free text.

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
