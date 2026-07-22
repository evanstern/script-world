package mind

import (
	"fmt"
	"strings"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/sim"
)

// Prompt construction: a stable per-agent system prefix (persona + tool-choice
// instruction — prompt-cache friendly) and a variable user suffix (situation +
// the bounded working-memory window).
//
// In the tool-use era (spec 017) the goal vocabulary and per-verb gloss are no
// longer hand-rendered into the prompt: each tool carries its own name, gloss
// (Description), and argument schema, declared to the model as callable tools
// (tool.LoopRosterVillager -> the loop's Request.Tools). The system prompt now
// only frames the choice; the tools carry the vocabulary. The spec-014 golden
// prompt test that pinned the free-text contract retires with this change
// (contracts/loop-api.md).

func systemPrompt(name, personaText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, a villager in a small settlement.\n\n", name)
	if personaText != "" {
		b.WriteString(personaText)
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "You decide what %s does next by calling exactly one acting tool. "+
		"Each tool is an action %s can take; its description says what it does and its "+
		"arguments what it needs. You may first call read-only tools to look something "+
		"up, then finish by calling one acting tool: a world action, a short plan "+
		"(set_plan), or a passing thought (muse). Musing is an action too — a beat "+
		"spent thinking is a beat not spent doing. Choose the single action that best "+
		"fits %s's situation and needs right now.\n", name, name, name)
	return b.String()
}

// futureDated tells the model when its decision will land (FR-016): thought
// is not instant, and the prompt stops pretending it is. Empty when there is
// no meaningful prediction (uncapped test speeds).
func futureDated(now, landing int64) string {
	if landing <= now {
		return ""
	}
	return fmt.Sprintf("It is now %s. Your decision will take effect around %s — plan for then, not for this instant.\n",
		clock.Format(now), clock.Format(landing))
}

// musingSystemPrompt frames the same situation window as pure interiority
// (TASK-21): one plain sentence, no goal vocabulary, no JSON.
func musingSystemPrompt(name, personaText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, a villager in a small settlement.\n\n", name)
	if personaText != "" {
		b.WriteString(personaText)
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Reply with ONE short sentence: an idle thought passing through %s's mind right now — first person, present tense, in your own voice. No JSON, no quotes, no explanation.\n", name)
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
	// Carried inventory: the full resource/item set (spec 012, T025/T029/T035)
	// so the planner can reason about cooking/eating AND the crafting chain
	// (planks/refined stone/spear) and the oven's water/wood consumers.
	fmt.Fprintf(&b, "Carrying: %d wood, %d stone, %d water, %d planks, %d refined stone, food (%d raw, %d cooked, %d meals)",
		a.Inv.Wood, a.Inv.Stone, a.Inv.Water, a.Inv.Planks, a.Inv.RefinedStone,
		a.Inv.FoodRaw, a.Inv.FoodCooked, a.Inv.Meals)
	if n := len(a.Inv.Spears); n > 0 {
		fmt.Fprintf(&b, ", %d spear(s) (%d uses left on the most-worn)", n, a.Inv.Spears[0])
	}
	b.WriteString(".\n")

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

	// Social context (TASK-8): bonds, debts, reputation, the loudest rumor.
	if social := socialContext(s, idx); social != "" {
		b.WriteString(social)
	}

	// Village law (TASK-13): the rules in force are standing knowledge —
	// obeying, skirting, or defying them is an informed, in-persona choice.
	if law := villageLaw(s, idx); law != "" {
		b.WriteString(law)
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

// socialContext renders a compact bonds/debts/reputation/rumor block.
func socialContext(s *sim.State, idx int) string {
	var b strings.Builder
	var bonds []string
	for _, r := range s.Relations {
		if r.From != idx {
			continue
		}
		switch {
		case r.Affection >= 100:
			bonds = append(bonds, fmt.Sprintf("you like %s", s.Agents[r.To].Name))
		case r.Affection <= -100:
			bonds = append(bonds, fmt.Sprintf("you resent %s", s.Agents[r.To].Name))
		case r.Trust <= -100:
			bonds = append(bonds, fmt.Sprintf("you distrust %s", s.Agents[r.To].Name))
		}
	}
	if len(bonds) > 4 {
		bonds = bonds[:4]
	}
	if len(bonds) > 0 {
		fmt.Fprintf(&b, "People: %s.\n", strings.Join(bonds, "; "))
	}
	// Last-conversation callback (TASK-22): the durable record ring gives
	// prompts continuity across encounters.
	if r, ok := sim.LastConversationInvolving(s, idx); ok {
		var others []string
		for _, p := range r.Participants {
			if p != idx && p >= 0 && p < len(s.Agents) {
				others = append(others, s.Agents[p].Name)
			}
		}
		if len(others) > 0 && r.Gist != "" {
			fmt.Fprintf(&b, "Last conversation, with %s: %s\n", strings.Join(others, " and "), r.Gist)
		}
	}
	for _, d := range s.Debts {
		if d.Status != "open" {
			continue
		}
		if d.Debtor == idx {
			fmt.Fprintf(&b, "You owe %s one %s (due %s).\n", s.Agents[d.Creditor].Name, d.Kind, clock.Format(d.Due))
		} else if d.Creditor == idx {
			fmt.Fprintf(&b, "%s owes you one %s.\n", s.Agents[d.Debtor].Name, d.Kind)
		}
	}
	rep := sim.Reputation(s, idx)
	switch {
	case rep >= 700:
		b.WriteString("Your word is respected in the village.\n")
	case rep <= 300:
		b.WriteString("People say you don't keep your word.\n")
	}
	best := sim.KnownRumor{Confidence: -1}
	for _, kr := range s.Agents[idx].Known {
		if kr.Confidence > best.Confidence && kr.From >= 0 { // heard, not own secret
			best = kr
		}
	}
	if best.Confidence > 0 {
		fmt.Fprintf(&b, "You have heard: %q\n", best.Text)
	}
	return b.String()
}

// villageLaw renders the norms in force, standing judgments, and (while the
// village convenes) the meeting call. Empty for a lawless village.
func villageLaw(s *sim.State, idx int) string {
	var b strings.Builder
	var rules []string
	var judgments []string
	for _, n := range s.Norms {
		if !n.Active {
			continue
		}
		if n.Kind == sim.NormExile {
			if n.Target == idx {
				judgments = append(judgments, fmt.Sprintf("You are exiled from the village (day %d) — the village shuns you.", n.DayPassed))
			} else if n.Target >= 0 && n.Target < len(s.Agents) {
				judgments = append(judgments, fmt.Sprintf("%s is exiled from the village (day %d).", s.Agents[n.Target].Name, n.DayPassed))
			}
			continue
		}
		proposer := "someone"
		if n.Proposer >= 0 && n.Proposer < len(s.Agents) {
			proposer = s.Agents[n.Proposer].Name
		}
		rules = append(rules, fmt.Sprintf("- %s (passed day %d, %s's proposal, %s)", n.Text, n.DayPassed, proposer, n.Tally))
	}
	if len(rules) > 0 {
		header := "Village law:"
		if c := s.MeetingConvention; c != nil {
			header = fmt.Sprintf("Village law (decided at the daily meeting, %s):", clock.FormatTOD(c.OpenSecond))
		}
		b.WriteString(header + "\n")
		b.WriteString(strings.Join(rules, "\n"))
		b.WriteString("\n")
	}
	for _, j := range judgments {
		b.WriteString(j)
		b.WriteString("\n")
	}
	if sim.AtMeeting(s, idx) {
		when := "the assembly"
		if c := s.MeetingConvention; c != nil {
			when = fmt.Sprintf("the %s assembly", clock.FormatTOD(c.OpenSecond))
		}
		fmt.Fprintf(&b, "The village is gathering at the meeting place for %s — you can raise grievances and vote there.\n", when)
	}
	return b.String()
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
