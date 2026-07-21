package mind

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
)

// The chronicle narrator (TASK-11): absorb collects notable events as
// pre-named factual log lines; each day/night boundary closes a chapter and
// hands it to a single-flight cloud worker (llm.KindNarrator). The model
// returns 1–3 story entries which land as one atomic chronicle.entry batch
// through the same injection door as everything else. Quiet chapters spend
// no call; a failed call carries its lines into the next chapter; bad model
// output is a gap in the story, never a stall.

const (
	// narrateCallTimeout bounds one cloud call; chapters are hours apart, so
	// generous beats interactive.
	narrateCallTimeout = 3 * time.Minute
	// narrMaxLines caps a chapter's log (and the carry from failed calls);
	// overflow drops oldest — the recent story matters most.
	narrMaxLines   = 120
	narrMaxEntries = 3
	narrMaxText    = 600
	narrMaxTokens  = 800
)

// narrJob is the immutable chapter a narration runs against.
type narrJob struct {
	day      int64
	label    string
	fromTick int64
	toTick   int64
	lines    []string
	threads  []string // recent slugs, offered for reuse
}

// narrCarry is a failed chapter's log, carried into the next one.
type narrCarry struct {
	fromTick int64
	lines    []string
}

// chronicleNote turns one notable event into a chronicle log line and closes
// chapters at day/night boundaries. Runs in the absorb goroutine (owns the
// replica, which has already applied e).
func (md *Mind) chronicleNote(e store.Event) {
	if md.social == nil {
		return
	}
	name := func(i int) string {
		if i >= 0 && i < len(md.replica.Agents) {
			return md.replica.Agents[i].Name
		}
		return "someone"
	}
	var line string
	switch e.Type {
	case "agent.died":
		var p sim.DiedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			line = fmt.Sprintf("%s died of %s.", name(p.Agent), p.Cause)
		}
	case "agent.built":
		var p sim.BuiltPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			line = fmt.Sprintf("%s built a %s.", name(p.Agent), p.Kind)
		}
	case "gru.emerged":
		line = "The gru emerged from its den."
	case "gru.sighted":
		var p sim.GruSightedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			line = fmt.Sprintf("%s sighted the gru.", name(p.Agent))
		}
	case "gru.attacked":
		var p sim.GruAttackedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			line = fmt.Sprintf("The gru attacked %s and left them wounded.", name(p.Agent))
		}
	case "gru.withdrew":
		line = "The gru withdrew to its den."
	case "social.conversation":
		var p sim.ConversationPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			parts := p.Participants
			if len(parts) == 0 {
				parts = []int{p.A, p.B}
			}
			names := make([]string, len(parts))
			for i, a := range parts {
				names[i] = name(a)
			}
			line = fmt.Sprintf("%s talked", strings.Join(names, ", "))
			if len(p.Topics) > 0 {
				line += " about " + strings.Join(p.Topics, ", ")
			}
			if p.Gist != "" {
				line += fmt.Sprintf(" — %q", p.Gist)
			}
			line += "."
		}
	case "social.rumor_told":
		var p sim.RumorToldPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.To >= 0 {
			line = fmt.Sprintf("%s told %s a rumor: %q.", name(p.From), name(p.To), p.Text)
		}
	case "social.gave":
		var p sim.GavePayload
		if json.Unmarshal(e.Payload, &p) == nil {
			line = fmt.Sprintf("%s gave %s %s.", name(p.From), name(p.To), p.Kind)
		}
	case "social.promise_broken":
		var p sim.PromiseBrokenPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			for _, d := range md.replica.Debts {
				if d.ID == p.ID {
					line = fmt.Sprintf("%s broke a promise to %s (an owed %s).",
						name(d.Debtor), name(d.Creditor), d.Kind)
					break
				}
			}
		}
	case "agent.thought":
		var p sim.ThoughtPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Source == "musing" {
			line = fmt.Sprintf("%s mused: %q.", name(p.Agent), p.Text)
		}
	case "meeting.convention_established":
		var p sim.MeetingConventionPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Source == "emergent" {
			line = "The villagers took to gathering at the same spot — a daily assembly was born."
		}
	case "meeting.opened":
		var p sim.MeetingOpenedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			at := "the meeting"
			if c := md.replica.MeetingConvention; c != nil {
				at = "the " + clock.FormatTOD(c.OpenSecond) + " meeting"
			}
			if len(p.Attendees) == 0 {
				line = fmt.Sprintf("The hour came for %s, but nobody gathered.", at)
			} else {
				names := make([]string, len(p.Attendees))
				for i, a := range p.Attendees {
					names[i] = name(a)
				}
				line = fmt.Sprintf("The village assembled for %s: %s.", at, strings.Join(names, ", "))
			}
		}
	case "meeting.turn_taken":
		var p sim.TurnTakenPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Raised != "" {
			line = fmt.Sprintf("%s raised a grievance at the meeting: %q.", name(p.Agent), p.Raised)
		}
	case "meeting.proposal_tabled":
		var p sim.ProposalPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			line = fmt.Sprintf("%s put a proposal to the assembly: %q.", name(p.Proposer), p.Text)
		}
	case "meeting.proposal_resolved":
		var p sim.ProposalResolvedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			tally := fmt.Sprintf("%d-%d", len(p.Yeas), len(p.Nays))
			switch {
			case p.Passed && p.Kind == sim.ProposeExile:
				line = fmt.Sprintf("The village voted %s to exile %s.", tally, name(p.Target))
			case p.Passed:
				line = fmt.Sprintf("The village passed %s's proposal %s: %q.", name(p.Proposer), tally, p.Text)
			default:
				line = fmt.Sprintf("The village voted down %s's proposal %s.", name(p.Proposer), tally)
			}
		}
	case "meeting.closed":
		var p sim.MeetingClosedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			line = "The village meeting ended and everyone went back to their day."
		}
	case "norm.violated":
		var p sim.NormViolatedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			if n := sim.NormByID(md.replica, p.NormID); n != nil {
				verb := "was seen breaking the village's law"
				if n.Kind == sim.NormExile {
					verb = "was seen defying their exile"
				}
				line = fmt.Sprintf("%s %s: %q.", name(p.Violator), verb, n.Text)
			}
		}
	case "sim.night_started":
		var p sim.DayPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			md.closeChapter(p.Day, fmt.Sprintf("day %d, dawn to nightfall", p.Day), e.Tick)
		}
		return
	case "sim.day_started":
		var p sim.DayPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Day > 1 {
			md.closeChapter(p.Day-1, fmt.Sprintf("the night after day %d", p.Day-1), e.Tick)
		}
		return
	}
	if line == "" {
		return
	}
	if md.narrFrom == 0 {
		md.narrFrom = e.Tick
	}
	md.narrLines = append(md.narrLines, fmt.Sprintf("[%s] %s", clock.Format(e.Tick), line))
	if len(md.narrLines) > narrMaxLines {
		md.narrLines = append(md.narrLines[:0], md.narrLines[len(md.narrLines)-narrMaxLines:]...)
	}
}

// closeChapter snapshots the buffer (plus any carry from a failed call) into
// a job for the narrator worker. Quiet chapters spend nothing.
func (md *Mind) closeChapter(day int64, label string, toTick int64) {
	lines := md.narrLines
	fromTick := md.narrFrom
	md.narrLines = nil
	md.narrFrom = 0
	select {
	case c := <-md.narrRetry:
		if c.fromTick > 0 && (fromTick == 0 || c.fromTick < fromTick) {
			fromTick = c.fromTick
		}
		lines = append(c.lines, lines...)
		if len(lines) > narrMaxLines {
			lines = lines[len(lines)-narrMaxLines:]
		}
	default:
	}
	if len(lines) == 0 {
		return
	}
	if fromTick == 0 {
		fromTick = toTick
	}
	// Recent thread slugs from the ring, offered so the model reuses them.
	seen := map[string]bool{}
	var threads []string
	for i := len(md.replica.Chronicle) - 1; i >= 0 && len(threads) < 8; i-- {
		if t := md.replica.Chronicle[i].Thread; t != "" && !seen[t] {
			seen[t] = true
			threads = append(threads, t)
		}
	}
	// Router gate (FR-007): a day-scale budget passes at every watchable
	// speed; a suppression (future faster speeds) drops the chapter into
	// the retry carry like any other narrator failure.
	if v := md.routeVerdict("chronicle", llm.KindNarrator); !v.Allow {
		md.emitSuppressed("chronicle", -1, toTick, v)
		return
	}
	job := narrJob{day: day, label: label, fromTick: fromTick, toTick: toTick,
		lines: lines, threads: threads}
	select {
	case md.narrQ <- job:
	default:
		// Queue full means chapters of backlog against a wedged cloud tier;
		// this chapter becomes a gap.
		log.Printf("mind: narrator queue full, chapter %q dropped (%d lines)", label, len(lines))
	}
}

// narrateWorker drains chapters one cloud call at a time.
func (md *Mind) narrateWorker() {
	for {
		select {
		case <-md.done:
			return
		case job := <-md.narrQ:
			md.runNarration(job)
		}
	}
}

func (md *Mind) runNarration(job narrJob) {
	ctx, cancel := context.WithTimeout(context.Background(), narrateCallTimeout)
	resp, err := md.orch.Submit(ctx, llm.Request{
		Kind:      llm.KindNarrator,
		System:    narrateSystemPrompt(),
		Prompt:    narrateUserPrompt(job),
		MaxTokens: narrMaxTokens,
	})
	cancel()
	if err != nil {
		// Transport/tier failure: the lines carry into the next chapter.
		log.Printf("mind: narration %q deferred: %v", job.label, err)
		carry := narrCarry{fromTick: job.fromTick, lines: job.lines}
		select {
		case old := <-md.narrRetry:
			carry.lines = append(old.lines, carry.lines...)
			if len(carry.lines) > narrMaxLines {
				carry.lines = carry.lines[len(carry.lines)-narrMaxLines:]
			}
			if old.fromTick > 0 && old.fromTick < carry.fromTick {
				carry.fromTick = old.fromTick
			}
		default:
		}
		md.narrRetry <- carry
		return
	}

	entries, err := parseNarration(resp.Text)
	if err != nil {
		// Bad output is a gap in the story, never a stall or a retry loop.
		log.Printf("mind: narration %q unusable: %v", job.label, err)
		return
	}
	var batch []store.Event
	for _, en := range entries {
		agents := make([]int, 0, len(en.Agents))
		dedup := map[int]bool{}
		for _, n := range en.Agents {
			if i := md.agentIndexByName(n); i >= 0 && !dedup[i] {
				dedup[i] = true
				agents = append(agents, i)
			}
		}
		b, _ := json.Marshal(sim.ChronicleEntryPayload{
			Day: job.day, FromTick: job.fromTick, ToTick: job.toTick,
			Text: en.Text, Thread: en.Thread, Agents: agents,
		})
		batch = append(batch, store.Event{Type: "chronicle.entry", Payload: b})
	}
	if err := md.social.InjectSocial(batch); err != nil {
		log.Printf("mind: narration %q injection rejected: %v", job.label, err)
		return
	}
	log.Printf("mind: chronicle %q landed (%d entries, $%.4f)", job.label, len(batch), resp.CostUSD)
}

// narrEntry is one validated story entry.
type narrEntry struct {
	Text   string   `json:"text"`
	Thread string   `json:"thread"`
	Agents []string `json:"agents"`
}

// parseNarration extracts and validates the narrator's entries: 1–3 kept,
// texts trimmed and capped, threads slugified. An output with no usable
// entry is a model failure.
func parseNarration(text string) ([]narrEntry, error) {
	raw, err := firstJSON(text)
	if err != nil {
		return nil, err
	}
	var out struct {
		Entries []narrEntry `json:"entries"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("bad narration JSON: %w", err)
	}
	var entries []narrEntry
	for _, en := range out.Entries {
		en.Text = strings.TrimSpace(en.Text)
		if en.Text == "" {
			continue
		}
		if r := []rune(en.Text); len(r) > narrMaxText {
			en.Text = string(r[:narrMaxText])
		}
		en.Thread = slugify(en.Thread)
		entries = append(entries, en)
		if len(entries) == narrMaxEntries {
			break
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no usable entries")
	}
	return entries, nil
}

// slugify normalizes a thread label to a stable lowercase slug.
func slugify(s string) string {
	var b strings.Builder
	lastDash := true // no leading dash
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 24 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "village"
	}
	return out
}

func narrateSystemPrompt() string {
	return fmt.Sprintf(`You are the chronicler of a small village of eight: %s.
You write the village's ongoing story from its true event log — vivid, compact,
past tense, third person. You never invent events, injuries, deaths, or words
that are not in the log; you may connect and interpret what is there.`,
		strings.Join(sim.AgentNames[:], ", "))
}

func narrateUserPrompt(job narrJob) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Chapter: %s. The log:\n", job.label)
	for _, l := range job.lines {
		b.WriteString(l + "\n")
	}
	if len(job.threads) > 0 {
		fmt.Fprintf(&b, "\nOngoing threads (reuse these slugs when an entry continues one): %s\n",
			strings.Join(job.threads, ", "))
	}
	fmt.Fprintf(&b, `
Reply with ONLY this JSON:
{"entries":[{"text":"<the story of one thread this chapter, 1-3 sentences, under 400 characters>",
             "thread":"<short-lowercase-slug for that storyline, stable across chapters>",
             "agents":["<villager names appearing in this entry>"]}]}
1 to %d entries, most important first. Group by storyline, not by hour.`, narrMaxEntries)
	return b.String()
}
