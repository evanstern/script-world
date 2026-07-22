package mind

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// meetMind builds a bare Mind for phrasing-driver unit tests (no goroutines).
func meetMind(t *testing.T) (*Mind, *fakeSocial, *mockModel) {
	t.Helper()
	m := worldmap.Generate(42, 64, 64)
	social := &fakeSocial{}
	model := &mockModel{}
	md := &Mind{
		orch:    model,
		social:  social,
		replica: sim.NewState(42, m),
		m:       m,
		meetQ:   make(chan meetingJob, 4),
	}
	return md, social, model
}

// enactedResolution applies an enacting resolution to the replica and returns
// the event (as absorb would see it).
func enactedResolution(t *testing.T, md *Mind, passed bool) store.Event {
	t.Helper()
	p := sim.ProposalResolvedPayload{
		ProposalPayload: sim.ProposalPayload{ProposalID: 1, Kind: sim.ProposeCurfew,
			Target: -1, Param: 22 * 3600, Proposer: 2,
			Text: "No one out alone after nightfall — the night hunts us."},
		Yeas: []int{0, 1, 2}, Nays: []int{3}, Passed: passed,
	}
	e := mustEvent(t, 21600, "meeting.proposal_resolved", p)
	if err := md.replica.Apply(e); err != nil {
		t.Fatal(err)
	}
	return e
}

// TestPhraseEnactedProposal: a passed enactment queues one phrasing job; the
// worker call injects a rephrase for the right norm, capped and voiced.
func TestPhraseEnactedProposal(t *testing.T) {
	md, social, model := meetMind(t)
	model.reply = `"The dark belongs to the gru — stay by the fire after nightfall."`

	md.maybePhraseProposal(enactedResolution(t, md, true))
	var job meetingJob
	select {
	case job = <-md.meetQ:
	default:
		t.Fatal("enacted proposal did not queue a phrasing job")
	}
	if job.name != "Cedar" || job.normID != md.replica.NextNormID {
		t.Fatalf("job %+v, want Cedar phrasing norm %d", job, md.replica.NextNormID)
	}

	md.runPhrasing(job)
	if len(social.batches) != 1 || len(social.batches[0]) != 1 {
		t.Fatalf("batches %v, want one single-event injection", social.batches)
	}
	e := social.batches[0][0]
	if e.Type != "meeting.proposal_rephrased" {
		t.Fatalf("injected %s, want meeting.proposal_rephrased", e.Type)
	}
	var p sim.ProposalRephrasedPayload
	json.Unmarshal(e.Payload, &p)
	if p.NormID != job.normID || !strings.Contains(p.Text, "stay by the fire") {
		t.Errorf("rephrase payload %+v", p)
	}
	if kinds := model.kinds; len(kinds) != 1 || kinds[0] != llm.KindMeeting {
		t.Errorf("call kinds %v, want one KindMeeting", kinds)
	}
	// The injected event applies cleanly and swaps only the text.
	if err := md.replica.Apply(store.Event{Tick: 21601, Type: e.Type, Payload: e.Payload}); err != nil {
		t.Fatal(err)
	}
	n := sim.NormByID(md.replica, p.NormID)
	if n.Text != p.Text || n.Kind != sim.NormCurfew || !n.Active {
		t.Errorf("norm after rephrase: %+v", n)
	}
}

// TestPhraseSkipsFailuresAndFlavorlessKinds: failed proposals and amend/
// repeal resolutions never queue.
func TestPhraseSkipsFailuresAndFlavorlessKinds(t *testing.T) {
	md, _, _ := meetMind(t)

	md.maybePhraseProposal(enactedResolution(t, md, false))
	select {
	case <-md.meetQ:
		t.Fatal("failed proposal must not phrase")
	default:
	}

	// A repeal (no text change) never phrases.
	md.replica.Norms = nil
	enactedResolution(t, md, true) // enact the curfew (consumes queue check below)
	drainMeetQ(md)
	repeal := sim.ProposalResolvedPayload{
		ProposalPayload: sim.ProposalPayload{ProposalID: 2, Kind: sim.ProposeRepeal,
			NormID: md.replica.NextNormID, Target: -1, Proposer: 0, Text: "strike it"},
		Yeas: []int{0, 1}, Passed: true,
	}
	e := mustEvent(t, 22000, "meeting.proposal_resolved", repeal)
	if err := md.replica.Apply(e); err != nil {
		t.Fatal(err)
	}
	md.maybePhraseProposal(e)
	select {
	case <-md.meetQ:
		t.Fatal("repeal must not phrase")
	default:
	}
}

func drainMeetQ(md *Mind) {
	for {
		select {
		case <-md.meetQ:
		default:
			return
		}
	}
}

// TestPhraseDegradedModeSkips: a down tier costs nothing — no injection, the
// template text stands (SC-007 flavor half).
func TestPhraseDegradedModeSkips(t *testing.T) {
	md, social, model := meetMind(t)
	model.err = llm.ErrTierDown

	md.maybePhraseProposal(enactedResolution(t, md, true))
	md.runPhrasing(<-md.meetQ)
	if len(social.batches) != 0 {
		t.Fatal("failed call must inject nothing")
	}
	n := sim.NormByID(md.replica, md.replica.NextNormID)
	if !strings.Contains(n.Text, "the night hunts us") {
		t.Errorf("template text should stand, got %q", n.Text)
	}
}

// TestPhraseCapsOversizedReply: model rambling truncates to the reducer's cap
// instead of being rejected at the door.
func TestPhraseCapsOversizedReply(t *testing.T) {
	md, social, model := meetMind(t)
	model.reply = strings.Repeat("night ", 100)

	md.maybePhraseProposal(enactedResolution(t, md, true))
	md.runPhrasing(<-md.meetQ)
	if len(social.batches) != 1 {
		t.Fatal("capped reply should still inject")
	}
	var p sim.ProposalRephrasedPayload
	json.Unmarshal(social.batches[0][0].Payload, &p)
	if len(p.Text) > sim.NormTextMax || p.Text == "" {
		t.Errorf("text length %d, want 1..%d", len(p.Text), sim.NormTextMax)
	}
}

// TestPlannerSuppressedAtMeeting: convened villagers don't burn planner
// calls; the armed trigger fires once the meeting closes.
func TestPlannerSuppressedAtMeeting(t *testing.T) {
	md, _, _ := meetMind(t)
	md.planQ = make(chan planJob, sim.AgentCount)

	md.replica.MeetingPlace = &sim.Point{X: 5, Y: 5}
	md.replica.Meeting = sim.MeetingState{Phase: "open", OpenedTick: 21600,
		Attendees: []int{0, 1, 2, 3, 4, 5, 6, 7}, LastMeetingDay: 1}
	md.replica.Tick = 22000
	for i := range md.replica.Agents {
		md.pending[i] = true
	}
	md.plan()
	select {
	case job := <-md.planQ:
		t.Fatalf("agent %d planned mid-meeting", job.agent)
	default:
	}

	// Close the meeting: pending triggers fire again.
	if err := md.replica.Apply(mustEvent(t, 25000, "meeting.closed", sim.MeetingClosedPayload{})); err != nil {
		t.Fatal(err)
	}
	md.replica.Tick = 25000
	md.plan()
	select {
	case <-md.planQ:
	default:
		t.Fatal("planning did not resume after the meeting closed")
	}
}

// TestVillageLawPrompt: norms in force, exile judgments, and the assembly
// call all reach planner context; a lawless village renders nothing.
func TestVillageLawPrompt(t *testing.T) {
	m := worldmap.Generate(42, 64, 64)
	s := sim.NewState(42, m)

	if law := villageLaw(s, 0); law != "" {
		t.Fatalf("lawless village rendered %q", law)
	}
	if strings.Contains(userPrompt(s, 0, sim.WindowK), "Village law") {
		t.Fatal("lawless prompt must not carry a law section")
	}

	// A convention (open at noon) fixes the meeting-time phrasing in the law header.
	if err := s.Apply(mustEvent(t, 0, "meeting.convention_established",
		sim.MeetingConventionPayload{ConveneSecond: 11*3600 + 1800, OpenSecond: 12 * 3600, X: 5, Y: 5, Source: "config"})); err != nil {
		t.Fatal(err)
	}

	enact := sim.ProposalResolvedPayload{
		ProposalPayload: sim.ProposalPayload{ProposalID: 1, Kind: sim.ProposeCurfew,
			Target: -1, Param: 22 * 3600, Proposer: 1, Text: "No one out after nightfall."},
		Yeas: []int{0, 1, 2}, Passed: true,
	}
	exile := sim.ProposalResolvedPayload{
		ProposalPayload: sim.ProposalPayload{ProposalID: 2, Kind: sim.ProposeExile,
			Target: 3, Proposer: 1, Text: "Rowan is a danger to us all — cast them out."},
		Yeas: []int{0, 1, 2}, Passed: true,
	}
	for i, p := range []sim.ProposalResolvedPayload{enact, exile} {
		if err := s.Apply(mustEvent(t, int64(90000+i), "meeting.proposal_resolved", p)); err != nil {
			t.Fatal(err)
		}
	}

	prompt := userPrompt(s, 0, sim.WindowK)
	for _, want := range []string{
		"Village law (decided at the daily meeting, 12:00):",
		"No one out after nightfall. (passed day 2, Birch's proposal, 3-0)",
		"Rowan is exiled from the village (day 2).",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}

	// The exile reads their own judgment in second person.
	if !strings.Contains(userPrompt(s, 3, sim.WindowK), "You are exiled from the village") {
		t.Error("the exile's own prompt must carry the judgment personally")
	}

	// The assembly call appears for convened villagers.
	s.MeetingPlace = &sim.Point{X: 5, Y: 5}
	s.Meeting.Phase = "convening"
	if !strings.Contains(userPrompt(s, 0, sim.WindowK), "gathering at the meeting place") {
		t.Error("convening prompt must announce the assembly")
	}
	if strings.Contains(userPrompt(s, 3, sim.WindowK), "gathering at the meeting place") {
		t.Error("an exile is not called to the assembly")
	}
}

// TestGovernanceChronicleNotes: governance events become named, narratable
// log lines (TASK-17 convention: participants named).
func TestGovernanceChronicleNotes(t *testing.T) {
	md, _, _ := narrMind(t)

	md.chronicleNote(mustEvent(t, 21600, "meeting.opened", sim.MeetingOpenedPayload{Attendees: []int{0, 1}}))
	md.chronicleNote(mustEvent(t, 21960, "meeting.turn_taken", sim.TurnTakenPayload{Agent: 1, Raised: "Cedar never repaid me."}))
	md.chronicleNote(mustEvent(t, 22320, "meeting.proposal_tabled", sim.ProposalPayload{
		ProposalID: 1, Kind: sim.ProposeCurfew, Target: -1, Proposer: 0, Text: "No one out after nightfall."}))
	resolved := sim.ProposalResolvedPayload{
		ProposalPayload: sim.ProposalPayload{ProposalID: 1, Kind: sim.ProposeCurfew,
			Target: -1, Param: 22 * 3600, Proposer: 0, Text: "No one out after nightfall."},
		Yeas: []int{0, 1}, Passed: true,
	}
	md.replica.Apply(mustEvent(t, 22320, "meeting.proposal_resolved", resolved))
	md.chronicleNote(mustEvent(t, 22320, "meeting.proposal_resolved", resolved))
	md.chronicleNote(mustEvent(t, 22320, "norm.violated", sim.NormViolatedPayload{
		NormID: md.replica.NextNormID, Violator: 2, Witnesses: []int{0}}))
	exiled := sim.ProposalResolvedPayload{
		ProposalPayload: sim.ProposalPayload{ProposalID: 2, Kind: sim.ProposeExile,
			Target: 3, Proposer: 0, Text: "out"},
		Yeas: []int{0, 1}, Passed: true,
	}
	md.chronicleNote(mustEvent(t, 22680, "meeting.proposal_resolved", exiled))
	md.chronicleNote(mustEvent(t, 25000, "meeting.closed", sim.MeetingClosedPayload{Proposals: 2}))

	joined := strings.Join(md.narrLines, "\n")
	for _, want := range []string{
		"The village assembled for the meeting: Ash, Birch.",
		`Birch raised a grievance at the meeting: "Cedar never repaid me."`,
		`Ash put a proposal to the assembly: "No one out after nightfall."`,
		`The village passed Ash's proposal 2-0: "No one out after nightfall."`,
		`Cedar was seen breaking the village's law: "No one out after nightfall."`,
		"The village voted 2-0 to exile Rowan.",
		"The village meeting ended",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("chronicle missing %q in:\n%s", want, joined)
		}
	}
}
