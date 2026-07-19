package mind

import (
	"encoding/json"
	"fmt"
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
