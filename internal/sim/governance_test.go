package sim

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// Tick math (epoch 06:00): 11:30 day 1 = tick 19800, noon day 1 = tick 21600.
// These are the historical convene/open times the lifecycle tests were
// written against — no longer engine constants (TASK-36), so a convention
// must be established to reproduce them (see establishConvention).
const (
	conveneSecondTest = 11*3600 + 1800 // 11:30
	openSecondTest    = 12 * 3600      // noon
	conveneTickDay1   = int64(conveneSecondTest - 6*3600)
	noonTickDay1      = int64(openSecondTest - 6*3600)
)

// conventionEvent builds a config-shaped convention_established event whose
// place is the tile the engine would derive — convene 11:30, open noon.
func conventionEvent(s *State, m *worldmap.Map, tick int64) store.Event {
	p := deriveMeetingPlace(s, m)
	return store.Event{Tick: tick, Type: "meeting.convention_established",
		Payload: mustPayload(MeetingConventionPayload{
			ConveneSecond: conveneSecondTest, OpenSecond: openSecondTest,
			X: p.X, Y: p.Y, Source: "config"})}
}

// establishConvention reduces a convention at s.Tick so the meeting lifecycle
// has when/where to convene (TASK-36 removed the hard-coded 11:30 clock).
func establishConvention(t *testing.T, s *State, m *worldmap.Map) {
	t.Helper()
	if err := s.Apply(conventionEvent(s, m, s.Tick)); err != nil {
		t.Fatalf("establish convention: %v", err)
	}
}

func governanceLog(log []store.Event, types ...string) []store.Event {
	want := map[string]bool{}
	for _, t := range types {
		want[t] = true
	}
	var out []store.Event
	for _, e := range log {
		if want[e.Type] {
			out = append(out, e)
		}
	}
	return out
}

// TestMeetingLifecycleFullDay: the village convenes at 11:30, opens at noon
// with everyone in the square, hears each attendee, closes inside the
// timebox, disperses — and does it again (in the same place) the next day.
func TestMeetingLifecycleFullDay(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	establishConvention(t, s, m)
	log := driveTicks(t, s, m, noonTickDay1+meetingTimeboxTicks+meetingGraceTicks+60, nil)

	// The place rides the convention (TASK-36): no separate designation event.
	if places := governanceLog(log, "meeting.place_designated"); len(places) != 0 {
		t.Fatalf("meeting place designated %d times, want none (place comes from the convention)", len(places))
	}
	if s.MeetingPlace == nil {
		t.Fatal("convention did not set the meeting place")
	}
	convened := governanceLog(log, "meeting.convened")
	if len(convened) != 1 || convened[0].Tick != conveneTickDay1 {
		t.Fatalf("convened %d times (first at %d), want once at %d", len(convened), convened[0].Tick, conveneTickDay1)
	}

	var pins int
	for _, e := range log {
		if e.Type == "agent.intent_set" {
			var p IntentSetPayload
			json.Unmarshal(e.Payload, &p)
			if p.Source == "meeting" {
				pins++
			}
		}
	}
	if pins < agentCount {
		t.Errorf("only %d meeting pins, want at least %d (everyone convened)", pins, agentCount)
	}

	opened := governanceLog(log, "meeting.opened")
	if len(opened) != 1 || opened[0].Tick != noonTickDay1 {
		t.Fatalf("opened %d times, want once at noon (%d)", len(opened), noonTickDay1)
	}
	var op MeetingOpenedPayload
	json.Unmarshal(opened[0].Payload, &op)
	if len(op.Attendees) != agentCount {
		t.Errorf("attendance %d, want the whole living village (%d) — convening failed to gather", len(op.Attendees), agentCount)
	}

	turns := governanceLog(log, "meeting.turn_taken")
	if len(turns) != len(op.Attendees) {
		t.Errorf("%d speaking turns, want one per attendee (%d)", len(turns), len(op.Attendees))
	}

	closed := governanceLog(log, "meeting.closed")
	if len(closed) != 1 {
		t.Fatalf("closed %d times, want once", len(closed))
	}
	if d := closed[0].Tick - opened[0].Tick; d > meetingTimeboxTicks+meetingGraceTicks {
		t.Errorf("meeting ran %d ticks, over the timebox+grace (%d)", d, meetingTimeboxTicks+meetingGraceTicks)
	}
	if s.Meeting.Phase != "" {
		t.Errorf("meeting phase %q after close, want cleared", s.Meeting.Phase)
	}

	// Day 2: same place, second meeting, no second designation.
	log2 := driveTicks(t, s, m, noonTickDay1+86400+meetingTimeboxTicks+meetingGraceTicks+60, nil)
	if n := len(governanceLog(log2, "meeting.place_designated")); n != 0 {
		t.Errorf("place re-designated on day 2 (%d times) — must persist", n)
	}
	conv2 := governanceLog(log2, "meeting.convened")
	if len(conv2) != 1 {
		t.Fatalf("day 2 convened %d times, want once", len(conv2))
	}
	var p1, p2 MeetingPlacePayload
	json.Unmarshal(convened[0].Payload, &p1)
	json.Unmarshal(conv2[0].Payload, &p2)
	if p1 != p2 {
		t.Errorf("meeting place moved between days: %+v vs %+v", p1, p2)
	}
	if len(governanceLog(log2, "meeting.opened")) != 1 {
		t.Error("no second meeting on day 2")
	}
}

// TestAsleepVillagersMissMeeting: sleep is missing the meeting; votes
// resolve among attendees only.
func TestAsleepVillagersMissMeeting(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	establishConvention(t, s, m)
	driveTicks(t, s, m, conveneTickDay1-1, nil)
	s.Agents[0].Asleep = true
	s.Agents[0].Intent = nil
	s.Agents[0].Needs.Rest = 100 // too tired for wakeReason
	s.Agents[0].Needs.Food = 900 // no hunger emergency either
	log := driveTicks(t, s, m, noonTickDay1+120, nil)

	opened := governanceLog(log, "meeting.opened")
	if len(opened) != 1 {
		t.Fatalf("opened %d times, want once", len(opened))
	}
	var op MeetingOpenedPayload
	json.Unmarshal(opened[0].Payload, &op)
	if containsInt(op.Attendees, 0) {
		t.Error("sleeping villager counted as an attendee")
	}
	if len(op.Attendees) != agentCount-1 {
		t.Errorf("attendance %d, want %d", len(op.Attendees), agentCount-1)
	}
}

// TestEmptyMeetingOpensAndCloses: a village that sleeps through noon gets a
// one-beat meeting, not a stall.
func TestEmptyMeetingOpensAndCloses(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	establishConvention(t, s, m)
	driveTicks(t, s, m, conveneTickDay1+1, nil) // convened
	for i := range s.Agents {
		s.Agents[i].Asleep = true
		s.Agents[i].Intent = nil
		s.Agents[i].Needs.Rest = 100
		s.Agents[i].Needs.Food = 900
	}
	log := driveTicks(t, s, m, noonTickDay1+120, nil)
	opened := governanceLog(log, "meeting.opened")
	closed := governanceLog(log, "meeting.closed")
	if len(opened) != 1 || len(closed) != 1 {
		t.Fatalf("opened=%d closed=%d, want 1/1", len(opened), len(closed))
	}
	var op MeetingOpenedPayload
	json.Unmarshal(opened[0].Payload, &op)
	if len(op.Attendees) != 0 {
		t.Errorf("attendees %v, want none", op.Attendees)
	}
	if closed[0].Tick-opened[0].Tick > 1 {
		t.Error("empty meeting should close on the next beat")
	}
}

// openMeeting hand-sets an open meeting (reducer-shaped state) for direct
// turn-beat tests.
func openMeeting(s *State, tick int64, attendees []int) {
	p := Point{X: s.Agents[0].X, Y: s.Agents[0].Y}
	s.MeetingPlace = &p
	s.Meeting = MeetingState{Phase: "open", OpenedTick: tick, Attendees: attendees,
		LastMeetingDay: DayIndex(tick)}
	s.Tick = tick
}

func applyAll(t *testing.T, s *State, events []store.Event) {
	t.Helper()
	for _, e := range events {
		if err := s.Apply(e); err != nil {
			t.Fatalf("apply %s: %v", e.Type, err)
		}
	}
}

// TestProposeVotePass: a gru-bitten villager tables the curfew; friends pass
// it; the norm lands with tally and provenance; votes move edges.
func TestProposeVotePass(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s := NewState(seed, m)
	openMeeting(s, noonTickDay1, []int{0, 1, 2, 3})
	s.Agents[0].Memories = append(s.Agents[0].Memories,
		Memory{Text: "The gru came out of the dark and tore into me.", Salience: 9, Tick: noonTickDay1 - 3600, Subject: -1})
	// 1 and 2 like the proposer; 3 despises them.
	s.relation(1, 0).Trust = 200
	s.relation(2, 0).Affection = 50
	s.relation(3, 0).Trust = -300
	s.relation(3, 0).Affection = -200

	events := speakingTurn(s, noonTickDay1+meetingTurnTicks)
	tabled := governanceLog(events, "meeting.proposal_tabled")
	resolved := governanceLog(events, "meeting.proposal_resolved")
	if len(tabled) != 1 || len(resolved) != 1 {
		t.Fatalf("tabled=%d resolved=%d, want 1/1 (same beat)", len(tabled), len(resolved))
	}
	var res ProposalResolvedPayload
	json.Unmarshal(resolved[0].Payload, &res)
	if res.Kind != ProposeCurfew || res.Proposer != 0 {
		t.Fatalf("resolved %+v, want add_curfew by 0", res.ProposalPayload)
	}
	wantYeas, wantNays := []int{0, 1, 2}, []int{3}
	if !equalInts(res.Yeas, wantYeas) || !equalInts(res.Nays, wantNays) {
		t.Errorf("votes yeas=%v nays=%v, want %v / %v", res.Yeas, res.Nays, wantYeas, wantNays)
	}
	if !res.Passed {
		t.Error("3-1 should pass")
	}

	trustBefore := s.RelationBetween(3, 0).Trust
	applyAll(t, s, events)
	norms := ActiveNorms(s)
	if len(norms) != 1 || norms[0].Kind != NormCurfew || norms[0].Tally != "3-1" || norms[0].Proposer != 0 {
		t.Fatalf("enacted norms %+v, want one curfew 3-1 by agent 0", norms)
	}
	if s.RelationBetween(3, 0).Trust >= trustBefore {
		t.Error("opposed voters should lose trust in the proposer's camp")
	}
	if s.RelationBetween(1, 2).Affection <= 0 {
		t.Error("aligned voters should warm to each other")
	}
	var outcomeMemories int
	for _, e := range events {
		if e.Type == "agent.memory_added" {
			var p MemoryAddedPayload
			json.Unmarshal(e.Payload, &p)
			if p.Subject == 0 && strings.Contains(p.Text, "proposal") {
				outcomeMemories++
			}
		}
	}
	if outcomeMemories != 3 {
		t.Errorf("%d outcome memories about the proposer, want 3 (every other voter)", outcomeMemories)
	}
}

// TestTieFails: 1-1 is not a majority; the status quo wins.
func TestTieFails(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s := NewState(seed, m)
	openMeeting(s, noonTickDay1, []int{0, 1})
	s.Agents[0].Memories = append(s.Agents[0].Memories,
		Memory{Text: "Saw the gru prowling in the dark.", Salience: 6, Tick: noonTickDay1 - 100, Subject: -1})
	s.relation(1, 0).Trust = -100

	events := speakingTurn(s, noonTickDay1+meetingTurnTicks)
	var res ProposalResolvedPayload
	json.Unmarshal(governanceLog(events, "meeting.proposal_resolved")[0].Payload, &res)
	if res.Passed || len(res.Yeas) != 1 || len(res.Nays) != 1 {
		t.Fatalf("1-1 tie must fail, got %+v", res)
	}
	applyAll(t, s, events)
	if len(ActiveNorms(s)) != 0 {
		t.Error("failed proposal must not enact")
	}
}

// TestFodderRules: each rule fires on its grievance and never duplicates an
// active norm.
func TestFodderRules(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s := NewState(seed, m)
	tick := noonTickDay1

	if p := proposalFor(s, 0, tick); p != nil {
		t.Fatalf("no fodder should table nothing, got %+v", p)
	}

	// Rule 2: a stiffed creditor wants the debt law.
	s.Debts = append(s.Debts, Debt{ID: 1, Debtor: 3, Creditor: 0, Kind: "food", Due: 100, Status: "broken"})
	if p := proposalFor(s, 0, tick); p == nil || p.Kind != ProposeRepayDebts {
		t.Fatalf("broken debt should table add_repay_debts, got %+v", p)
	}

	// Rule 1 outranks rule 2: fresh gru terror.
	s.Agents[0].Memories = append(s.Agents[0].Memories,
		Memory{Text: "Saw the gru attack Rowan in the dark.", Salience: 7, Tick: tick - 86400, Subject: 3, Tone: -60})
	if p := proposalFor(s, 0, tick); p == nil || p.Kind != ProposeCurfew {
		t.Fatalf("gru memory should table add_curfew first, got %+v", p)
	}
	// A stale gru memory (outside the window) does not.
	s.Agents[0].Memories[len(s.Agents[0].Memories)-1].Tick = tick - gruFodderWindowTicks - 1
	if p := proposalFor(s, 0, tick); p == nil || p.Kind != ProposeRepayDebts {
		t.Fatalf("stale gru memory should fall through to repay_debts, got %+v", p)
	}

	// Duplicates never table: with both norms active, neither rule fires.
	s.appendNorm(Norm{Kind: NormCurfew, Target: -1, Param: nightStartSecond, Text: "curfew", Proposer: 1, DayPassed: 1, Tally: "3-0"})
	s.appendNorm(Norm{Kind: NormRepayDebts, Target: -1, Text: "repay", Proposer: 1, DayPassed: 1, Tally: "3-0"})
	s.Agents[0].Memories[len(s.Agents[0].Memories)-1].Tick = tick - 100
	if p := proposalFor(s, 0, tick); p != nil {
		t.Fatalf("active norms must not be re-proposed, got %+v", p)
	}

	// Rule 3: a repeat violator legislates in self-interest — amend when they
	// like the proposer, repeal when they don't.
	curfew := &s.Norms[0]
	curfew.Violations = []NormViolation{{Agent: 0, Tick: tick - 200}, {Agent: 0, Tick: tick - 100}}
	if p := proposalFor(s, 0, tick); p == nil || p.Kind != ProposeAmend || p.NormID != curfew.ID {
		t.Fatalf("friendly repeat violator should amend, got %+v", p)
	}
	s.relation(0, 1).Affection = -200
	if p := proposalFor(s, 0, tick); p == nil || p.Kind != ProposeRepeal || p.NormID != curfew.ID {
		t.Fatalf("hostile repeat violator should repeal, got %+v", p)
	}
	// An already-amended curfew only repeals.
	s.relation(0, 1).Affection = 200
	curfew.Amended = true
	if p := proposalFor(s, 0, tick); p == nil || p.Kind != ProposeRepeal {
		t.Fatalf("amended curfew cannot re-amend, got %+v", p)
	}
}

// TestAmendAndRepealReducer: amendment moves the curfew window in place;
// repeal retires the rule but never erases it.
func TestAmendAndRepealReducer(t *testing.T) {
	const seed = 7
	s := NewState(seed, testMap(seed))
	s.appendNorm(Norm{Kind: NormCurfew, Target: -1, Param: nightStartSecond, Text: "curfew", Proposer: 0, DayPassed: 1, Tally: "3-0"})
	id := s.Norms[0].ID

	amend := ProposalResolvedPayload{
		ProposalPayload: ProposalPayload{ProposalID: 1, Kind: ProposeAmend, NormID: id,
			Target: -1, Param: (nightStartSecond + curfewAmendDelta) % 86400, Proposer: 2, Text: "later curfew"},
		Yeas: []int{0, 1, 2}, Nays: nil, Passed: true,
	}
	applyAll(t, s, []store.Event{{Tick: 90000, Type: "meeting.proposal_resolved", Payload: mustPayload(amend)}})
	n := normByID(s, id)
	if !n.Amended || n.Param != (nightStartSecond+curfewAmendDelta)%86400 || n.DayAmended != DayIndex(90000) {
		t.Fatalf("amend did not land: %+v", n)
	}

	repeal := ProposalResolvedPayload{
		ProposalPayload: ProposalPayload{ProposalID: 2, Kind: ProposeRepeal, NormID: id,
			Target: -1, Proposer: 2, Text: "strike it"},
		Yeas: []int{0, 1, 2}, Nays: nil, Passed: true,
	}
	applyAll(t, s, []store.Event{{Tick: 180000, Type: "meeting.proposal_resolved", Payload: mustPayload(repeal)}})
	n = normByID(s, id)
	if n.Active || n.DayRepealed != DayIndex(180000) {
		t.Fatalf("repeal did not land: %+v", n)
	}
	if len(ActiveNorms(s)) != 0 {
		t.Error("repealed norm still active")
	}
	if len(s.Norms) != 1 {
		t.Error("the law's history must never be erased")
	}

	// Repeal of a repealed norm and amend of a missing norm are no-ops.
	applyAll(t, s, []store.Event{{Tick: 180060, Type: "meeting.proposal_resolved", Payload: mustPayload(repeal)}})
	bogus := amend
	bogus.NormID = 999
	applyAll(t, s, []store.Event{{Tick: 180120, Type: "meeting.proposal_resolved", Payload: mustPayload(bogus)}})
}

// TestCurfewViolationWitnessed: a night wanderer with a witness is caught —
// ring entry, witness memory (gossip-seed shape), edge penalty; the latch
// holds for the rest of the night; the unwitnessed breach costs nothing.
func TestCurfewViolationWitnessed(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s := NewState(seed, m)
	s.appendNorm(Norm{Kind: NormCurfew, Target: -1, Param: nightStartSecond, Text: "No one out after nightfall.", Proposer: 1, DayPassed: 1, Tally: "4-0"})
	nightTick := int64(60000) // 22:40 day 1
	s.Tick = nightTick
	s.Night = true
	// Violator 0 awake in the open; witness 1 adjacent; everyone else asleep.
	for i := range s.Agents {
		s.Agents[i].Needs = Needs{Health: 1000, Food: 900, Rest: 900, Warmth: 900, Morale: 900}
		if i >= 2 {
			s.Agents[i].Asleep = true
			s.Agents[i].Needs.Rest = 100
		}
	}
	s.Agents[1].X, s.Agents[1].Y = s.Agents[0].X+1, s.Agents[0].Y

	log := driveTicks(t, s, m, nightTick+120, nil)
	violations := governanceLog(log, "norm.violated")
	if len(violations) != 2 { // 0 seen by 1, and 1 seen by 0 — both are out
		t.Fatalf("%d violations, want 2 (both night-walkers witnessed each other)", len(violations))
	}
	var v NormViolatedPayload
	json.Unmarshal(violations[0].Payload, &v)
	if len(v.Witnesses) == 0 {
		t.Fatal("violation recorded without witnesses")
	}
	n := normByID(s, 1)
	if ViolationCount(n, 0) != 1 || ViolationCount(n, 1) != 1 {
		t.Errorf("violation ring: agent0=%d agent1=%d, want 1/1", ViolationCount(n, 0), ViolationCount(n, 1))
	}
	if r := s.RelationBetween(1, 0); r.Trust != -normViolationTrust || r.Affection != -normViolationAffection {
		t.Errorf("witness edge = %+v, want -%d/-%d", r, normViolationTrust, normViolationAffection)
	}
	// The witness memory is rumor-seed shaped: subject-tagged, toned, salient.
	var seeded bool
	for _, e := range governanceLog(log, "agent.memory_added") {
		var p MemoryAddedPayload
		json.Unmarshal(e.Payload, &p)
		if p.Agent == 1 && p.Subject == 0 && p.Tone == toneViolation && p.Salience >= rumorMinSalience {
			seeded = true
		}
	}
	if !seeded {
		t.Error("witness memory missing or not gossip-seed shaped")
	}

	// Latch: the rest of the night adds nothing for the same pair.
	tail := driveTicks(t, s, m, nightTick+120+3600, nil)
	if extra := len(governanceLog(tail, "norm.violated")); extra != 0 {
		t.Errorf("latch failed: %d more violations the same night", extra)
	}
}

// TestCurfewViolationUnwitnessed: the village only judges what it can see.
func TestCurfewViolationUnwitnessed(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s := NewState(seed, m)
	s.appendNorm(Norm{Kind: NormCurfew, Target: -1, Param: nightStartSecond, Text: "No one out after nightfall.", Proposer: 1, DayPassed: 1, Tally: "4-0"})
	nightTick := int64(60000)
	s.Tick = nightTick
	s.Night = true
	for i := range s.Agents {
		s.Agents[i].Needs = Needs{Health: 1000, Food: 900, Rest: 900, Warmth: 900, Morale: 900}
		if i != 0 {
			s.Agents[i].Asleep = true
			s.Agents[i].Needs.Rest = 100
		}
	}
	// Park the violator far from every sleeper.
	s.Agents[0].X, s.Agents[0].Y = 0, 0
	for i := 1; i < len(s.Agents); i++ {
		s.Agents[i].X, s.Agents[i].Y = 60, 60
	}

	log := driveTicks(t, s, m, nightTick+120, nil)
	if n := len(governanceLog(log, "norm.violated")); n != 0 {
		t.Fatalf("%d violations with zero witnesses, want none", n)
	}
	if ViolationCount(normByID(s, 1), 0) != 0 {
		t.Error("unwitnessed breach must not enter the ring")
	}
}

// TestRepayNormPiggyback: with the debt law in force, a broken promise is
// also a witnessed crime.
func TestRepayNormPiggyback(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s := NewState(seed, m)
	s.appendNorm(Norm{Kind: NormRepayDebts, Target: -1, Text: "Debts must be repaid.", Proposer: 0, DayPassed: 1, Tally: "4-0"})
	s.Debts = append(s.Debts, Debt{ID: 1, Debtor: 2, Creditor: 0, Kind: "food", Due: 7000, Status: "open"})
	s.NextDebtID = 1
	s.Tick = 7199 // the 7200 hourly beat is past due
	// Witness 3 beside the debtor; keep everyone fed so nothing else fires.
	for i := range s.Agents {
		s.Agents[i].Needs = Needs{Health: 1000, Food: 900, Rest: 900, Warmth: 900, Morale: 900}
	}
	s.Agents[3].X, s.Agents[3].Y = s.Agents[2].X+1, s.Agents[2].Y

	log := driveTicks(t, s, m, 7260, nil)
	if len(governanceLog(log, "social.promise_broken")) != 1 {
		t.Fatal("debt should break on the hourly beat")
	}
	violations := governanceLog(log, "norm.violated")
	if len(violations) != 1 {
		t.Fatalf("%d norm violations, want 1 (the broken promise under the law)", len(violations))
	}
	var v NormViolatedPayload
	json.Unmarshal(violations[0].Payload, &v)
	if v.Violator != 2 || !containsInt(v.Witnesses, 3) {
		t.Errorf("violation %+v, want debtor 2 witnessed by 3", v)
	}
}

// TestExileVoteAndShun: hostility tables exile; the subject doesn't vote;
// the judgment lands; the shun detector catches defiance; forgiveness works.
func TestExileVoteAndShun(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s := NewState(seed, m)
	// Everyone despises 3.
	for o := range s.Agents {
		if o == 3 {
			continue
		}
		s.relation(o, 3).Trust = -400
		s.relation(o, 3).Affection = -400
	}
	openMeeting(s, noonTickDay1, []int{0, 1, 2, 3})

	events := speakingTurn(s, noonTickDay1+meetingTurnTicks)
	var res ProposalResolvedPayload
	resolved := governanceLog(events, "meeting.proposal_resolved")
	if len(resolved) != 1 {
		t.Fatal("exile should table and resolve")
	}
	json.Unmarshal(resolved[0].Payload, &res)
	if res.Kind != ProposeExile || res.Target != 3 {
		t.Fatalf("resolved %+v, want exile of 3", res.ProposalPayload)
	}
	if containsInt(res.Yeas, 3) || containsInt(res.Nays, 3) {
		t.Error("the subject must not vote on their own exile")
	}
	if !res.Passed || len(res.Yeas) != 3 {
		t.Errorf("universal hostility should pass 3-0, got yeas=%v passed=%v", res.Yeas, res.Passed)
	}
	applyAll(t, s, events)
	if !IsExiled(s, 3) {
		t.Fatal("passed exile should mark the target exiled")
	}
	var castOut bool
	for _, e := range governanceLog(events, "agent.memory_added") {
		var p MemoryAddedPayload
		json.Unmarshal(e.Payload, &p)
		if p.Agent == 3 && p.Salience == salExiled {
			castOut = true
		}
	}
	if !castOut {
		t.Error("the exile should carry the formative cast-out memory")
	}

	// The exile is not convened.
	if attendCandidate(s, 3) {
		t.Error("exiled villagers must not be meeting candidates")
	}

	// Shun detector: exile near a structure with an awake witness.
	s.Meeting = MeetingState{LastMeetingDay: s.Meeting.LastMeetingDay}
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: s.Agents[3].X + 1, Y: s.Agents[3].Y})
	s.Agents[0].X, s.Agents[0].Y = s.Agents[3].X+2, s.Agents[3].Y
	start := s.Tick
	for i := range s.Agents {
		s.Agents[i].Needs = Needs{Health: 1000, Food: 900, Rest: 900, Warmth: 900, Morale: 900}
		s.Agents[i].Intent = nil
		s.Agents[i].IdleSince = start // reflex grace keeps everyone still for the first beat
	}
	log := driveTicks(t, s, m, start+120, nil)
	violations := governanceLog(log, "norm.violated")
	if len(violations) != 1 {
		t.Fatalf("%d shun violations, want 1", len(violations))
	}
	var v NormViolatedPayload
	json.Unmarshal(violations[0].Payload, &v)
	if v.Violator != 3 {
		t.Errorf("violator %d, want the exile (3)", v.Violator)
	}

	// The village can forgive: repeal restores the exile's standing.
	exileNorm := ActiveNorms(s)[0]
	repeal := ProposalResolvedPayload{
		ProposalPayload: ProposalPayload{ProposalID: 9, Kind: ProposeRepeal, NormID: exileNorm.ID,
			Target: -1, Proposer: 0, Text: "let them back in"},
		Yeas: []int{0, 1, 2}, Passed: true,
	}
	applyAll(t, s, []store.Event{{Tick: s.Tick, Type: "meeting.proposal_resolved", Payload: mustPayload(repeal)}})
	if IsExiled(s, 3) {
		t.Error("repealed exile should restore standing")
	}
	if !attendCandidate(s, 3) {
		t.Error("a forgiven exile is convened again")
	}
}

// TestExileVoteScoreInversion: voters judge an exile by their feelings for
// the TARGET, not the proposer.
func TestExileVoteScoreInversion(t *testing.T) {
	s := NewState(7, testMap(7))
	p := ProposalPayload{Kind: ProposeExile, Target: 3, Proposer: 0}
	// Voter 1 loves the target: nay regardless of liking the proposer.
	s.relation(1, 3).Trust = 500
	s.relation(1, 3).Affection = 400
	s.relation(1, 0).Trust = 300
	if voteScore(s, 1, p) >= 0 {
		t.Error("loving the target should vote against their exile")
	}
	// Voter 2 hates the target: yea.
	s.relation(2, 3).Trust = -500
	if voteScore(s, 2, p) < 0 {
		t.Error("hating the target should vote for their exile")
	}
}

// TestRephrasedTextValidation: the one injectable governance event — text
// swaps in, everything else is immutable; bad injections error (the
// InjectSocial dry-run rejects them at the door).
func TestRephrasedTextValidation(t *testing.T) {
	s := NewState(7, testMap(7))
	s.appendNorm(Norm{Kind: NormCurfew, Target: -1, Param: nightStartSecond, Text: "template text", Proposer: 0, DayPassed: 1, Tally: "3-0"})

	if !injectSocialWhitelist["meeting.proposal_rephrased"] {
		t.Fatal("meeting.proposal_rephrased must be injectable")
	}
	for _, typ := range []string{"meeting.opened", "meeting.proposal_resolved", "norm.violated", "meeting.closed"} {
		if injectSocialWhitelist[typ] {
			t.Errorf("%s must NOT be injectable — outcomes are executor-only", typ)
		}
	}

	ev := func(p ProposalRephrasedPayload) store.Event {
		return store.Event{Tick: 100, Type: "meeting.proposal_rephrased", Payload: mustPayload(p)}
	}
	if err := s.Apply(ev(ProposalRephrasedPayload{ProposalID: 1, NormID: 1, Text: "No wandering once the dark comes down."})); err != nil {
		t.Fatal(err)
	}
	if s.Norms[0].Text != "No wandering once the dark comes down." {
		t.Error("rephrase should replace the norm text")
	}
	if err := s.Apply(ev(ProposalRephrasedPayload{ProposalID: 2, NormID: 999, Text: "x"})); err == nil {
		t.Error("unknown norm must reject")
	}
	if err := s.Apply(ev(ProposalRephrasedPayload{ProposalID: 3, NormID: 1, Text: ""})); err == nil {
		t.Error("empty text must reject")
	}
	if err := s.Apply(ev(ProposalRephrasedPayload{ProposalID: 4, NormID: 1, Text: strings.Repeat("x", NormTextMax+1)})); err == nil {
		t.Error("oversized text must reject")
	}
	if err := s.Apply(ev(ProposalRephrasedPayload{ProposalID: 5, NormID: 0, Text: "failed proposal flavor"})); err != nil {
		t.Errorf("norm_id 0 (failed proposal) is log-only flavor, got %v", err)
	}
}

// TestGovernanceReplay: a governed stretch (meeting, a norm in force,
// violations at night) replays from the log to the identical state.
func TestGovernanceReplay(t *testing.T) {
	const seed, ticks = 99, 65_000 // through day-1 noon and into the night
	m := testMap(seed)
	enact := ProposalResolvedPayload{
		ProposalPayload: ProposalPayload{ProposalID: 1, Kind: ProposeCurfew, NormID: 0,
			Target: -1, Param: nightStartSecond, Proposer: 0, Text: "No one out after nightfall."},
		Yeas: []int{0, 1, 2}, Nays: nil, Passed: true,
	}
	live := NewState(seed, m)
	// The convention rides the log (injected at tick 0), so replay reconstructs
	// it and the day-1 meeting exactly (TASK-36).
	timeline := map[int64][]store.Event{
		0:    {conventionEvent(live, m, 0)},
		1000: {{Tick: 1000, Type: "meeting.proposal_resolved", Payload: mustPayload(enact)}},
	}
	log := driveTicks(t, live, m, ticks, timeline)

	if len(governanceLog(log, "meeting.opened")) != 1 {
		t.Fatal("expected the day-1 meeting in the log")
	}

	replayed := NewState(seed, m)
	for _, e := range log {
		if err := replayed.Apply(e); err != nil {
			t.Fatalf("replay apply %s: %v", e.Type, err)
		}
		replayed.Tick = e.Tick
	}
	driveTicks(t, replayed, m, ticks, nil)

	if live.Hash() != replayed.Hash() {
		t.Fatalf("governed replay diverged:\nlive:     %s\nreplayed: %s",
			string(live.Marshal()), string(replayed.Marshal()))
	}
}

// TestPreTask13SnapshotUpgrade: old snapshots (no governance fields) load as
// a lawless village with no meeting history.
func TestPreTask13SnapshotUpgrade(t *testing.T) {
	s := NewState(7, testMap(7))
	b := s.Marshal()
	var m map[string]json.RawMessage
	json.Unmarshal(b, &m)
	for _, k := range []string{"meeting_place", "meeting", "norms", "next_norm_id", "next_proposal_id"} {
		delete(m, k)
	}
	old, _ := json.Marshal(m)

	restored := &State{}
	if err := json.Unmarshal(old, restored); err != nil {
		t.Fatal(err)
	}
	if restored.MeetingPlace != nil || restored.Meeting.Phase != "" ||
		restored.Meeting.LastMeetingDay != 0 || len(restored.Norms) != 0 {
		t.Errorf("pre-TASK-13 snapshot should restore lawless: %+v", restored.Meeting)
	}
}

// TestCurfewWindowWrap: an amended curfew starting at midnight only bites
// after midnight.
func TestCurfewWindowWrap(t *testing.T) {
	cases := []struct {
		param int
		sod   int64
		want  bool
	}{
		{nightStartSecond, 79200, true},  // 22:00 at 22:00
		{nightStartSecond, 3600, true},   // 22:00 curfew at 01:00
		{nightStartSecond, 21600, false}, // dawn
		{0, 79200, false},                // midnight curfew at 22:00 — not yet
		{0, 3600, true},                  // midnight curfew at 01:00
		{0, 21600, false},                // dawn
	}
	for _, c := range cases {
		if got := curfewActiveAt(c.param, c.sod); got != c.want {
			t.Errorf("curfewActiveAt(%d, %d) = %v, want %v", c.param, c.sod, got, c.want)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestDegradedModeGovernanceEndToEnd (SC-007): with no model anywhere, a
// seeded grievance becomes a tabled proposal at the day-1 meeting, resolves,
// and (given a friendly village) enacts with template text — the whole
// pipeline through the executor's turn beats.
func TestDegradedModeGovernanceEndToEnd(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	establishConvention(t, s, m)
	// The night before, the gru got someone; the village likes agent 0.
	s.Agents[0].Memories = append(s.Agents[0].Memories,
		Memory{Text: "The gru came out of the dark and tore into me.", Salience: 9, Tick: 100, Subject: -1})
	for o := 1; o < len(s.Agents); o++ {
		s.relation(o, 0).Trust = 100
	}
	log := driveTicks(t, s, m, noonTickDay1+meetingTimeboxTicks+meetingGraceTicks, nil)

	tabled := governanceLog(log, "meeting.proposal_tabled")
	if len(tabled) == 0 {
		t.Fatal("a gru grievance should table a proposal at the next meeting (SC-003)")
	}
	var tp ProposalPayload
	json.Unmarshal(tabled[0].Payload, &tp)
	if tp.Kind != ProposeCurfew || tp.Proposer != 0 {
		t.Fatalf("tabled %+v, want add_curfew by agent 0", tp)
	}
	norms := ActiveNorms(s)
	if len(norms) != 1 || norms[0].Kind != NormCurfew {
		t.Fatalf("active norms %+v, want the enacted curfew (governance never stalls model-off)", norms)
	}
	if norms[0].Text == "" {
		t.Error("template text must stand when no model rephrases")
	}
}

// TestFreshDefaultDayNoMeeting (TASK-36 AC#1): a fresh default world with no
// convention runs a full game day and never emits a single meeting.* event —
// villagers just follow their needs; no engine clock convenes them.
func TestFreshDefaultDayNoMeeting(t *testing.T) {
	const seed = 42
	m := testMap(seed)
	s := NewState(seed, m)
	log := driveTicks(t, s, m, 90_000, nil) // a full day (86 400) and change

	if s.MeetingConvention != nil {
		t.Fatalf("a fresh default world grew a convention unbidden: %+v", s.MeetingConvention)
	}
	for _, e := range log {
		if strings.HasPrefix(e.Type, "meeting.") {
			t.Errorf("fresh default world emitted %s at tick %d — no meeting without a convention", e.Type, e.Tick)
		}
	}
}

// gatherLog drives governanceEvents alone (no reflex) across minute
// heartbeats, applying and collecting the emitted events — full control over
// who is gathered where.
func gatherLog(t *testing.T, s *State, m *worldmap.Map, minutes int) []store.Event {
	t.Helper()
	var log []store.Event
	for k := 0; k < minutes; k++ {
		next := s.Tick + 60 // one game-minute heartbeat
		for _, e := range governanceEvents(s, m, next) {
			if err := s.Apply(e); err != nil {
				t.Fatalf("apply %s: %v", e.Type, err)
			}
			log = append(log, e)
		}
		s.Tick = next
	}
	return log
}

// TestEmergentConventionFromGathering (TASK-36 AC#3): with no config, a quorum
// that keeps gathering at one fire through the daytime establishes the
// convention in-world (deterministically) — and that convention drives a real
// meeting the next day.
func TestEmergentConventionFromGathering(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s := NewState(seed, m)

	// Noon on day 1: a fire, with the whole village gathered on it, awake.
	s.Tick = noonTickDay1
	fx, fy := s.Agents[0].X, s.Agents[0].Y
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: fx, Y: fy})
	for i := range s.Agents {
		s.Agents[i].X, s.Agents[i].Y = fx, fy
		s.Agents[i].Asleep = false
	}

	// Sustain the gathering just past the window (30 game-min → establishes).
	log := gatherLog(t, s, m, emergentGatherTicks/60+1)

	est := governanceLog(log, "meeting.convention_established")
	if len(est) != 1 {
		t.Fatalf("%d conventions established, want exactly one from the sustained gathering", len(est))
	}
	var p MeetingConventionPayload
	json.Unmarshal(est[0].Payload, &p)
	if p.Source != "emergent" {
		t.Errorf("source %q, want emergent", p.Source)
	}
	if p.X != fx || p.Y != fy {
		t.Errorf("place (%d,%d), want the fire tile (%d,%d)", p.X, p.Y, fx, fy)
	}
	// convene = the observed half-hour; the establish beat lands at 12:30 exactly.
	if p.ConveneSecond != 12*3600+1800 || p.OpenSecond != 13*3600 {
		t.Errorf("times convene=%d open=%d, want 45000/46800 (12:30/13:00)", p.ConveneSecond, p.OpenSecond)
	}
	if s.MeetingConvention == nil || s.MeetingConvention.Source != "emergent" {
		t.Fatalf("state convention = %+v, want the emergent one", s.MeetingConvention)
	}
	if s.Meeting.GatherStart != 0 { // the watch is cleared once it takes hold
		t.Errorf("gather watch not reset after establishment: %+v", s.Meeting)
	}

	// The next day, that convention drives a real meeting (convene → open).
	for i := range s.Agents {
		s.Agents[i].Needs = Needs{Health: 1000, Food: 900, Rest: 900, Warmth: 900, Morale: 900}
	}
	log2 := driveTicks(t, s, m, s.Tick+86400+2000, nil)
	if len(governanceLog(log2, "meeting.convened")) == 0 {
		t.Error("the emergent convention did not convene the village the next day")
	}
	if len(governanceLog(log2, "meeting.opened")) == 0 {
		t.Error("the emergent convention did not open a meeting the next day")
	}
	// No second convention: the detector is off once one exists.
	if n := len(governanceLog(log2, "meeting.convention_established")); n != 0 {
		t.Errorf("%d further conventions established after the first", n)
	}
}

// TestEmergentGatheringResets: a gathering that breaks before the window
// resets the watch and never establishes a convention.
func TestEmergentGatheringResets(t *testing.T) {
	const seed = 7
	m := testMap(seed)
	s := NewState(seed, m)
	s.Tick = noonTickDay1
	fx, fy := s.Agents[0].X, s.Agents[0].Y
	s.Structures = append(s.Structures, Structure{Kind: "fire", X: fx, Y: fy})
	// Everyone but 0,1 out of the picture (asleep); 0,1 gathered at the fire.
	for i := range s.Agents {
		s.Agents[i].Asleep = true
	}
	gather := func(on bool) {
		for _, i := range []int{0, 1} {
			s.Agents[i].X, s.Agents[i].Y = fx, fy
			s.Agents[i].Asleep = !on
		}
	}

	gather(true)
	log := gatherLog(t, s, m, 5) // five minutes gathered — the watch starts
	if s.Meeting.GatherStart == 0 {
		t.Fatal("the watch never started tracking the gathering")
	}
	gather(false) // disperse (0,1 asleep now)
	log = append(log, gatherLog(t, s, m, 40)... /* well past the window */)

	if len(governanceLog(log, "meeting.convention_established")) != 0 {
		t.Error("a broken gathering must not establish a convention")
	}
	if s.MeetingConvention != nil {
		t.Errorf("convention formed from a broken gathering: %+v", s.MeetingConvention)
	}
	if s.Meeting.GatherStart != 0 {
		t.Errorf("watch not reset after the gathering dispersed: %+v", s.Meeting)
	}
}

// TestGovernedDeterminism: same seed, same timeline, governance on — byte-
// identical logs (the sim_test determinism contract extended over meetings).
func TestGovernedDeterminism(t *testing.T) {
	const seed, ticks = 13, 24_000 // through the day-1 meeting
	m := testMap(seed)
	a, b := NewState(seed, m), NewState(seed, m)
	establishConvention(t, a, m)
	establishConvention(t, b, m)
	logA := driveTicks(t, a, m, ticks, nil)
	logB := driveTicks(t, b, m, ticks, nil)
	if !bytes.Equal(canonicalLog(t, logA), canonicalLog(t, logB)) {
		t.Fatal("governed runs diverged under identical inputs")
	}
	if a.Hash() != b.Hash() {
		t.Fatal("state hashes diverged")
	}
}
