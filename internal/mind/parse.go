package mind

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// planReply is the goal JSON contract.
type planReply struct {
	Goal   string `json:"goal"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason"`
}

var validGoals = map[string]bool{
	"forage": true, "chop": true, "hunt": true,
	"build_fire": true, "build_shelter": true,
	"eat": true, "sleep": true, "wander": true,
	"goto_warmth": true, "talk_to": true,
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
		return convoOutcome{}, fmt.Errorf("bad outcome JSON: %w", err)
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
	o.ToneA, o.ToneB = clamp(o.RawToneA), clamp(o.RawToneB)
	if strings.EqualFold(strings.TrimSpace(o.Retold), "null") {
		o.Retold = ""
	}
	return o, nil
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
