package mind

import (
	"fmt"
	"strings"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/sim"
)

// Prompt construction per contracts/planner-prompt.md: a stable per-agent
// system prefix (persona + instruction block — prompt-cache friendly) and a
// variable user suffix (situation + the bounded working-memory window).

const goalVocabulary = "forage, chop, hunt, build_fire, build_shelter, eat, sleep, wander, goto_warmth, talk_to"

func systemPrompt(name, personaText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, a villager in a small settlement.\n\n", name)
	if personaText != "" {
		b.WriteString(personaText)
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, `You decide what %s does next. Reply with ONLY a JSON object:
{"goal": "<goal>", "target": "<agent name, only for talk_to>", "reason": "<one short sentence in your voice>"}
Goals: %s.
`, name, goalVocabulary)
	return b.String()
}

// userPrompt renders the situation + memory window. The window is the ONLY
// memory content that ever reaches a prompt (AC#3).
func userPrompt(s *sim.State, idx int, k int) string {
	a := s.Agents[idx]
	var b strings.Builder

	phase := "daytime"
	if s.Night {
		phase = "night"
	}
	fmt.Fprintf(&b, "It is %s (%s). You are at (%d, %d).\n", clock.Format(s.Tick), phase, a.X, a.Y)
	fmt.Fprintf(&b, "Needs (0-100): health %d, food %d, rest %d, warmth %d, morale %d.\n",
		a.Needs.Health/10, a.Needs.Food/10, a.Needs.Rest/10, a.Needs.Warmth/10, a.Needs.Morale/10)
	fmt.Fprintf(&b, "Carrying: %d wood, %d meals.\n", a.Inv.Wood, a.Inv.Food)

	if len(s.Structures) > 0 {
		var parts []string
		for _, st := range s.Structures {
			parts = append(parts, fmt.Sprintf("%s at (%d,%d)", st.Kind, st.X, st.Y))
		}
		if len(parts) > 6 {
			parts = parts[:6]
		}
		fmt.Fprintf(&b, "Village: %s.\n", strings.Join(parts, "; "))
	}

	var nearby []string
	for j := range s.Agents {
		o := s.Agents[j]
		if j == idx || o.Dead {
			continue
		}
		if d := absInt(o.X-a.X) + absInt(o.Y-a.Y); d <= 10 {
			state := ""
			if o.Asleep {
				state = ", asleep"
			}
			nearby = append(nearby, fmt.Sprintf("%s (%d tiles away%s)", o.Name, d, state))
		}
	}
	if len(nearby) > 0 {
		fmt.Fprintf(&b, "Nearby: %s.\n", strings.Join(nearby, ", "))
	}

	window := sim.SelectMemories(&a, s.Seed, idx, s.Tick, k)
	if len(window) > 0 {
		b.WriteString("\nYou remember:\n")
		for _, m := range window {
			fmt.Fprintf(&b, "- %s\n", sim.FormatMemory(m))
		}
	}

	b.WriteString("\nWhat do you do next?")
	return b.String()
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
