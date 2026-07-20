package mind

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
)

// Governance phrasing (TASK-13): the ONLY model involvement in the norms
// system. When a proposal passes, one best-effort local call rewrites the
// executor's template text in the proposer's voice and injects it through
// meeting.proposal_rephrased — flavor, never outcome. Any failure means the
// template stands; governance never waits on inference.

const meetMaxTokens = 72

// meetingJob is the immutable snapshot the worker phrases against.
type meetingJob struct {
	proposalID int
	normID     int
	name       string
	persona    string
	template   string
}

// maybePhraseProposal (absorb goroutine, replica already updated) queues an
// enacted proposal for rephrasing. Amend/repeal don't change norm text and
// failed proposals have nothing to re-text, so only fresh enactments phrase.
func (md *Mind) maybePhraseProposal(e store.Event) {
	if md.social == nil {
		return
	}
	var p sim.ProposalResolvedPayload
	if json.Unmarshal(e.Payload, &p) != nil || !p.Passed {
		return
	}
	switch p.Kind {
	case sim.ProposeCurfew, sim.ProposeRepayDebts, sim.ProposeExile:
	default:
		return
	}
	// appendNorm assigned the enacted norm the current NextNormID.
	n := sim.NormByID(md.replica, md.replica.NextNormID)
	if n == nil || n.Text != p.Text {
		return // enactment was a defensive no-op; nothing to phrase
	}
	if p.Proposer < 0 || p.Proposer >= len(md.replica.Agents) {
		return
	}
	// Router gate (FR-007): the degrade action is the template itself —
	// enacted law never waits on a model.
	if v := md.routeVerdict("meeting", llm.KindMeeting); !v.Allow {
		md.emitSuppressed("meeting", p.Proposer, e.Tick, v)
		return
	}
	job := meetingJob{
		proposalID: p.ProposalID,
		normID:     n.ID,
		name:       md.replica.Agents[p.Proposer].Name,
		persona:    md.personas[p.Proposer],
		template:   p.Text,
	}
	select {
	case md.meetQ <- job:
	default: // queue full: the template stands, nothing owed
	}
}

// meetingWorker drains phrasing jobs one best-effort call at a time.
func (md *Mind) meetingWorker() {
	for {
		select {
		case <-md.done:
			return
		case job := <-md.meetQ:
			md.runPhrasing(job)
		}
	}
}

func (md *Mind) runPhrasing(job meetingJob) {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, a villager in a small settlement.\n\n", job.name)
	if job.persona != "" {
		b.WriteString(job.persona)
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "The village just voted your proposal into law. Restate the rule in ONE short sentence, in your own voice, keeping its meaning exactly. No JSON, no quotes, no explanation.\n")

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	resp, err := md.orch.Submit(ctx, llm.Request{
		Kind:       llm.KindMeeting,
		System:     b.String(),
		Prompt:     fmt.Sprintf("The rule as passed: %s", job.template),
		MaxTokens:  meetMaxTokens,
		BestEffort: true,
	})
	cancel()
	if err != nil {
		return // best effort: the template stands
	}
	text, err := parseMusing(resp.Text) // same shape: one plain line
	if err != nil {
		return
	}
	for len(text) > sim.NormTextMax { // byte cap, rune-safe truncation
		r := []rune(text)
		text = string(r[:len(r)-1])
	}
	payload, err := json.Marshal(sim.ProposalRephrasedPayload{
		ProposalID: job.proposalID, NormID: job.normID, Text: text,
	})
	if err != nil {
		return
	}
	if err := md.social.InjectSocial([]store.Event{{Type: "meeting.proposal_rephrased", Payload: payload}}); err != nil {
		log.Printf("mind: %s rephrase rejected: %v", job.name, err)
	}
}
