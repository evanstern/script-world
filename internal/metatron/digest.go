package metatron

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
)

// The watching layer (US4): absolute 6-game-hour digest windows summarized
// into soul.md by one cloud call each (skip-empty, carry-on-failure), and
// model-free moment flags (the drama rule v1) recorded immediately and
// queued for the next console exchange. Neither path can construct a nudge
// — only a console turn builds injection batches.

const (
	digestWindowTicks = 6 * 3600
	digestMaxLines    = 120
	digestMaxTokens   = 400
	digestCallTimeout = 3 * time.Minute
)

type digJob struct {
	label string
	lines []string
}

// observeMoment applies the drama trigger list (absorb goroutine; replica
// already reflects e).
func (mt *Metatron) observeMoment(e store.Event) {
	name := func(i int) string {
		if i >= 0 && i < len(mt.replica.Agents) {
			return mt.replica.Agents[i].Name
		}
		return "someone"
	}
	var line string
	switch e.Type {
	case "agent.died":
		var p sim.DiedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			line = fmt.Sprintf("%s — %s died of %s", clock.Format(e.Tick), name(p.Agent), p.Cause)
		}
	case "gru.attacked":
		var p sim.GruAttackedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			line = fmt.Sprintf("%s — the gru attacked %s in the night", clock.Format(e.Tick), name(p.Agent))
		}
	case "social.promise_broken":
		var p sim.PromiseBrokenPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			for _, d := range mt.replica.Debts {
				if d.ID == p.ID {
					line = fmt.Sprintf("%s — %s broke a promise to %s", clock.Format(e.Tick), name(d.Debtor), name(d.Creditor))
					break
				}
			}
		}
	case "metatron.order_expired":
		// A standing order lapsed with no trigger (spec 029 US2, FR-007): the
		// executor emitted this as a pure function of state + tick, so it lands the
		// same model-free moment here as the other drama triggers. The replica has
		// already applied it (status → expired); look the order up for its
		// condition so the next reply mentions the specific lapsed watch.
		var p sim.OrderIDPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			cond := p.ID
			for i := range mt.replica.MetatronOrders {
				if mt.replica.MetatronOrders[i].ID == p.ID {
					if c := mt.replica.MetatronOrders[i].Condition; c != "" {
						cond = c
					}
					break
				}
			}
			line = fmt.Sprintf("%s — a watch lapsed unfulfilled: %q", clock.Format(e.Tick), cond)
		}
	}
	if line == "" {
		return
	}
	mt.appendFile(mt.soulPath(), "\n**MOMENT** "+line+"\n")
	mt.stateMu.Lock()
	mt.moments = append(mt.moments, line)
	mt.stateMu.Unlock()
}

// digestNote collects notable lines and closes windows at absolute
// 6-game-hour boundaries (absorb goroutine).
func (mt *Metatron) digestNote(e store.Event) {
	if window := e.Tick / digestWindowTicks; window > mt.digFrom {
		mt.closeDigest(e.Tick)
		mt.digFrom = window
	}
	name := func(i int) string {
		if i >= 0 && i < len(mt.replica.Agents) {
			return mt.replica.Agents[i].Name
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
		line = "The gru emerged."
	case "gru.attacked":
		var p sim.GruAttackedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			line = fmt.Sprintf("The gru attacked %s.", name(p.Agent))
		}
	case "social.conversation":
		var p sim.ConversationPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Gist != "" {
			line = fmt.Sprintf("Talk among villagers: %q.", p.Gist)
		}
	case "social.rumor_told":
		var p sim.RumorToldPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.To >= 0 {
			line = fmt.Sprintf("%s told %s a rumor: %q.", name(p.From), name(p.To), p.Text)
		}
	case "social.promise_broken":
		line = "A promise was broken."
	case "metatron.nudged":
		var p sim.MetatronNudgedPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			line = fmt.Sprintf("(my own hand) a %s went out: %q.", p.Form, p.Text)
		}
	}
	if line == "" {
		return
	}
	mt.digLines = append(mt.digLines, fmt.Sprintf("[%s] %s", clock.Format(e.Tick), line))
	if len(mt.digLines) > digestMaxLines {
		mt.digLines = append(mt.digLines[:0], mt.digLines[len(mt.digLines)-digestMaxLines:]...)
	}
}

// closeDigest snapshots the window's lines (plus carry) into a job.
func (mt *Metatron) closeDigest(atTick int64) {
	lines := mt.digLines
	mt.digLines = nil
	select {
	case carry := <-mt.digCarry:
		lines = append(carry, lines...)
		if len(lines) > digestMaxLines {
			lines = lines[len(lines)-digestMaxLines:]
		}
	default:
	}
	if len(lines) == 0 {
		return // empty window: zero cost
	}
	job := digJob{label: clock.Format(atTick), lines: lines}
	select {
	case mt.digQ <- job:
	default:
		log.Printf("metatron: digest queue full, window before %s dropped", job.label)
	}
}

func (mt *Metatron) digestWorker() {
	for {
		select {
		case <-mt.done:
			return
		case job := <-mt.digQ:
			mt.runDigest(job)
		}
	}
}

func (mt *Metatron) runDigest(job digJob) {
	ctx, cancel := context.WithTimeout(context.Background(), digestCallTimeout)
	resp, err := mt.orch.Submit(ctx, llm.Request{
		Kind: llm.KindMetatron,
		System: "You are Metatron, keeper of the village's record. Compress the " +
			"log below into 2-4 terse note lines for your own future reference: " +
			"facts, names, tensions worth watching. No preamble, no JSON — just the lines.",
		Prompt:    strings.Join(job.lines, "\n"),
		MaxTokens: digestMaxTokens,
	})
	cancel()
	if err != nil {
		log.Printf("metatron: digest deferred: %v", err)
		select {
		case old := <-mt.digCarry:
			merged := append(old, job.lines...)
			if len(merged) > digestMaxLines {
				merged = merged[len(merged)-digestMaxLines:]
			}
			mt.digCarry <- merged
		default:
			mt.digCarry <- job.lines
		}
		return
	}
	text := strings.TrimSpace(resp.Text)
	if text == "" {
		return // unusable output: a gap, never a stall
	}
	mt.appendFile(mt.soulPath(), fmt.Sprintf("\n## Digest — up to %s\n\n%s\n", job.label, text))
	log.Printf("metatron: digest landed (up to %s, $%.4f)", job.label, resp.CostUSD)
}
