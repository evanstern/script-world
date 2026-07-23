package sim

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/worldmap"
)

// Norms and votes (TASK-13): the village legislates itself at a daily
// meeting. Everything outcome-bearing here is deterministic — executor beats
// that are pure functions of (state, map, tick), reduced like any event. The
// model may only rephrase a proposal's text (meeting.proposal_rephrased, the
// single injectable governance type); it never decides who proposes, who
// votes which way, or what passes.
//
// The convention that there IS a meeting — when it convenes, when it opens,
// and where — is not baked into the engine (TASK-36). It exists only once a
// source establishes it: per-world config (source "config", injected on
// daemon boot) or an in-world emergence (source "emergent", the sustained-
// gathering detector below). With no convention, governance stays dormant and
// villagers simply follow their needs.

// --- tuning ---

const (
	meetingTurnTicks    = 360  // one speaking turn per 6 game-min
	meetingTimeboxTicks = 3600 // ~1 game hour
	meetingGraceTicks   = 900  // bounded overrun for remaining agenda
	meetingRadius       = 3    // attendance = this close at open
	minVillagersToMeet  = 2

	// A convention emerges when a quorum keeps gathering at one structure
	// through the daytime: sampled each game-minute, unbroken for this long.
	emergentGatherTicks = 1800 // 30 game-min of sustained daytime gathering

	// Vote function weights (contracts/meeting-lifecycle.md).
	selfInterestBonus     = 400 // repeat violators want the norm gone
	meetingAlignAffection = 8   // voted the same way
	meetingOpposeTrust    = 10  // voted against each other

	// Violation consequences (witness → violator).
	normViolationTrust     = 40
	normViolationAffection = 25
	toneViolation          = -40
	toneVotedYea           = 25
	toneVotedNay           = -25

	// Fodder gates.
	gruFodderWindowTicks = 3 * 86400
	repealViolationCount = 2
	exileHostilityGate   = -600
	curfewAmendDelta     = 7200 // one amendment: curfew starts 2h later
	exileShunRadius      = 6
	exileShunLatchTicks  = 3600

	// NormTextMax caps norm/proposal text — shared truth with the mind's
	// phrasing driver, which must cap model output before injecting.
	NormTextMax = 280

	normViolationCap = 16 // bounded ring per norm
)

// Norm kinds are a closed vocabulary: only what the executor can
// deterministically observe can be violated (research R7).
const (
	NormCurfew     = "curfew"
	NormRepayDebts = "repay_debts"
	NormExile      = "exile"
)

// Proposal kinds.
const (
	ProposeCurfew     = "add_curfew"
	ProposeRepayDebts = "add_repay_debts"
	ProposeAmend      = "amend"
	ProposeRepeal     = "repeal"
	ProposeExile      = "exile"
)

// DayIndex is 1-based like NightIndex: which game day a tick falls in.
func DayIndex(tick int64) int64 { return tick/86400 + 1 }

// --- state ---

// Norm is one village law. Repealed norms stay on the list (Active=false) —
// like the debt ledger, the law's history never forgets.
type Norm struct {
	ID          int    `json:"id"`
	Kind        string `json:"kind"`
	Target      int    `json:"target"` // exile: the judged villager; -1 otherwise
	Param       int    `json:"param,omitempty"`
	Text        string `json:"text"`
	Proposer    int    `json:"proposer"`
	DayPassed   int64  `json:"day_passed"`
	Tally       string `json:"tally"`
	Active      bool   `json:"active"`
	DayRepealed int64  `json:"day_repealed,omitempty"`
	Amended     bool   `json:"amended,omitempty"`
	DayAmended  int64  `json:"day_amended,omitempty"`
	// Violations is a bounded ring — fodder for amend/repeal proposals and
	// the once-per-window violation latches.
	Violations []NormViolation `json:"violations,omitempty"`
}

type NormViolation struct {
	Agent int   `json:"agent"`
	Tick  int64 `json:"tick"`
}

// MeetingConvention is the village's standing agreement to meet — when it
// convenes, when it opens, and where (TASK-36). Nil means no convention: the
// lifecycle never convenes. Established once by config or emergence; the
// reducer never overwrites an existing convention (the establish event is
// one-shot and replay-safe).
type MeetingConvention struct {
	ConveneSecond  int    `json:"convene_second"`  // second-of-day the village breaks routine
	OpenSecond     int    `json:"open_second"`     // second-of-day the assembly opens
	Source         string `json:"source"`          // "config" | "emergent"
	EstablishedDay int64  `json:"established_day"` // 1-based game day it took hold
}

// MeetingState is the assembly lifecycle; zero value = no meeting.
type MeetingState struct {
	Phase           string `json:"phase,omitempty"` // "" | "convening" | "open"
	OpenedTick      int64  `json:"opened_tick,omitempty"`
	Attendees       []int  `json:"attendees,omitempty"`
	NextSpeaker     int    `json:"next_speaker,omitempty"`
	ProposalsTabled int    `json:"proposals_tabled,omitempty"`
	LastMeetingDay  int64  `json:"last_meeting_day,omitempty"`
	// Emergent-convention watch (TASK-36): while no convention exists, the
	// detector tracks a sustained daytime gathering at one structure.
	// GatherStart is the tick the current gathering began (0 = not watching);
	// GatherX/Y is that structure's tile. All reducer-advanced off
	// sim.gathering_observed so replay reconstructs the watch exactly.
	GatherStart int64 `json:"gather_start,omitempty"`
	GatherX     int   `json:"gather_x,omitempty"`
	GatherY     int   `json:"gather_y,omitempty"`
}

// --- computed views ---

// ActiveNorms filters the law in force, enactment order.
func ActiveNorms(s *State) []Norm {
	var out []Norm
	for _, n := range s.Norms {
		if n.Active {
			out = append(out, n)
		}
	}
	return out
}

// IsExiled reports whether an active exile judgment names the agent.
func IsExiled(s *State, agent int) bool {
	for _, n := range s.Norms {
		if n.Active && n.Kind == NormExile && n.Target == agent {
			return true
		}
	}
	return false
}

// ViolationCount counts an agent's recorded breaches of one norm.
func ViolationCount(n *Norm, agent int) int {
	c := 0
	for _, v := range n.Violations {
		if v.Agent == agent {
			c++
		}
	}
	return c
}

// NormByID looks a norm up by id (nil when unknown) — shared with the mind's
// narrator/phrasing driver and the scribe's charter render.
func NormByID(s *State, id int) *Norm {
	for i := range s.Norms {
		if s.Norms[i].ID == id {
			return &s.Norms[i]
		}
	}
	return nil
}

func normByID(s *State, id int) *Norm { return NormByID(s, id) }

// AtMeeting reports whether an agent is (or is being) convened — the mind
// suppresses planner/musing traffic for attendees while the assembly runs.
func AtMeeting(s *State, agent int) bool {
	return meetingActive(s) && agent >= 0 && agent < len(s.Agents) && attendCandidate(s, agent)
}

func activeNormOfKind(s *State, kind string) *Norm {
	for i := range s.Norms {
		if s.Norms[i].Active && s.Norms[i].Kind == kind {
			return &s.Norms[i]
		}
	}
	return nil
}

// --- payloads ---

type (
	MeetingPlacePayload struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	// MeetingConventionPayload establishes the convention (TASK-36). X/Y is
	// the meeting place, resolved by the source (config coords or the derived
	// tile; the emergent detector's structure). Source is "config" | "emergent".
	MeetingConventionPayload struct {
		ConveneSecond int    `json:"convene_second"`
		OpenSecond    int    `json:"open_second"`
		X             int    `json:"x"`
		Y             int    `json:"y"`
		Source        string `json:"source"`
	}
	// GatheringObservedPayload advances the emergent-gathering watch (the
	// sim.gathering_observed event): the structure tile and the tick the
	// current gathering began (all zero = no gathering, the watch resets).
	// Denormalized so the reducer never looks back at a prior event. It sits
	// in the sim.* namespace, not meeting.* — a gathering is village
	// fabric being watched, not a meeting; no meeting exists until a
	// convention is established.
	GatheringObservedPayload struct {
		X     int   `json:"x"`
		Y     int   `json:"y"`
		Start int64 `json:"start"`
	}
	MeetingOpenedPayload struct {
		Attendees []int `json:"attendees"`
	}
	TurnTakenPayload struct {
		Agent  int    `json:"agent"`
		Raised string `json:"raised,omitempty"`
	}
	// ProposalPayload is shared by tabled and resolved: resolved denormalizes
	// the whole proposal so the reducer never looks back at a prior event.
	ProposalPayload struct {
		ProposalID int    `json:"proposal_id"`
		Kind       string `json:"kind"`
		NormID     int    `json:"norm_id,omitempty"` // amend/repeal target
		Target     int    `json:"target"`            // exile: villager; -1 otherwise
		Param      int    `json:"param,omitempty"`
		Proposer   int    `json:"proposer"`
		Text       string `json:"text"`
	}
	ProposalResolvedPayload struct {
		ProposalPayload
		Yeas   []int `json:"yeas"`
		Nays   []int `json:"nays"`
		Passed bool  `json:"passed"`
	}
	ProposalRephrasedPayload struct {
		ProposalID int    `json:"proposal_id"`
		NormID     int    `json:"norm_id,omitempty"` // 0: proposal failed, log-only
		Text       string `json:"text"`
	}
	MeetingClosedPayload struct {
		Proposals int  `json:"proposals"`
		Graced    bool `json:"graced,omitempty"`
	}
	NormViolatedPayload struct {
		NormID    int   `json:"norm_id"`
		Violator  int   `json:"violator"`
		Witnesses []int `json:"witnesses"`
	}
)

// --- reducer ---

// applyGovernance handles meeting.* and norm.* events. Executor-emitted
// events degrade to no-ops rather than erroring (they must always re-apply
// in replay); only the injected rephrase validates hard — the InjectSocial
// dry-run runs it on a copy, so bad injections are rejected at the door.
func (s *State) applyGovernance(e store.Event) error {
	switch e.Type {
	case "meeting.convention_established":
		var p MeetingConventionPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		if s.MeetingConvention == nil { // one-shot: first source wins
			s.MeetingConvention = &MeetingConvention{
				ConveneSecond: p.ConveneSecond, OpenSecond: p.OpenSecond,
				Source: p.Source, EstablishedDay: DayIndex(e.Tick)}
		}
		if s.MeetingPlace == nil {
			s.MeetingPlace = &Point{X: p.X, Y: p.Y}
		}
		// The watch is over once a convention exists.
		s.Meeting.GatherStart, s.Meeting.GatherX, s.Meeting.GatherY = 0, 0, 0

	case "sim.gathering_observed":
		var p GatheringObservedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		s.Meeting.GatherStart, s.Meeting.GatherX, s.Meeting.GatherY = p.Start, p.X, p.Y

	case "meeting.place_designated":
		var p MeetingPlacePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		if s.MeetingPlace == nil {
			s.MeetingPlace = &Point{X: p.X, Y: p.Y}
		}

	case "meeting.convened":
		s.Meeting.Phase = "convening"

	case "meeting.opened":
		var p MeetingOpenedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		s.Meeting.Phase = "open"
		s.Meeting.OpenedTick = e.Tick
		s.Meeting.Attendees = p.Attendees
		s.Meeting.NextSpeaker = 0
		s.Meeting.ProposalsTabled = 0
		s.Meeting.LastMeetingDay = DayIndex(e.Tick)

	case "meeting.turn_taken":
		s.Meeting.NextSpeaker++

	case "meeting.proposal_tabled":
		s.NextProposalID++
		s.Meeting.ProposalsTabled++

	case "meeting.proposal_resolved":
		var p ProposalResolvedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		s.resolveProposal(p, e.Tick)

	case "meeting.proposal_rephrased":
		var p ProposalRephrasedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		if p.Text == "" || len(p.Text) > NormTextMax {
			return fmt.Errorf("apply %s: text length %d outside 1..%d", e.Type, len(p.Text), NormTextMax)
		}
		if p.NormID == 0 {
			return nil // failed proposal: flavor for the log/chronicle only
		}
		n := normByID(s, p.NormID)
		if n == nil {
			return fmt.Errorf("apply %s: no norm %d", e.Type, p.NormID)
		}
		n.Text = p.Text

	case "meeting.closed":
		s.Meeting.Phase = ""
		s.Meeting.OpenedTick = 0
		s.Meeting.Attendees = nil
		s.Meeting.NextSpeaker = 0
		s.Meeting.ProposalsTabled = 0

	case "norm.violated":
		var p NormViolatedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		n := normByID(s, p.NormID)
		if n == nil || p.Violator < 0 || p.Violator >= len(s.Agents) {
			return nil // vanished subject: recorded history, no-op on state
		}
		n.Violations = append(n.Violations, NormViolation{Agent: p.Violator, Tick: e.Tick})
		if len(n.Violations) > normViolationCap {
			n.Violations = append(n.Violations[:0], n.Violations[len(n.Violations)-normViolationCap:]...)
		}
		for _, w := range p.Witnesses {
			if w == p.Violator || w < 0 || w >= len(s.Agents) {
				continue
			}
			r := s.relation(w, p.Violator)
			r.Trust = clampInt(r.Trust-normViolationTrust, relMin, relMax)
			r.Affection = clampInt(r.Affection-normViolationAffection, relMin, relMax)
		}
	}
	return nil
}

// resolveProposal enacts a passed proposal and lands the pairwise voter edge
// movement (reducer-internal, like social.gave's ledger transition).
// Defensive no-ops (duplicate norm, dead exile target) keep executor-emitted
// history always re-applicable.
func (s *State) resolveProposal(p ProposalResolvedPayload, tick int64) {
	if p.Passed {
		day := DayIndex(tick)
		tally := fmt.Sprintf("%d-%d", len(p.Yeas), len(p.Nays))
		switch p.Kind {
		case ProposeCurfew:
			if activeNormOfKind(s, NormCurfew) == nil {
				s.appendNorm(Norm{Kind: NormCurfew, Target: -1, Param: p.Param,
					Text: p.Text, Proposer: p.Proposer, DayPassed: day, Tally: tally})
			}
		case ProposeRepayDebts:
			if activeNormOfKind(s, NormRepayDebts) == nil {
				s.appendNorm(Norm{Kind: NormRepayDebts, Target: -1,
					Text: p.Text, Proposer: p.Proposer, DayPassed: day, Tally: tally})
			}
		case ProposeAmend:
			if n := normByID(s, p.NormID); n != nil && n.Active && !n.Amended {
				n.Param = p.Param
				n.Amended = true
				n.DayAmended = day
			}
		case ProposeRepeal:
			if n := normByID(s, p.NormID); n != nil && n.Active {
				n.Active = false
				n.DayRepealed = day
			}
		case ProposeExile:
			if p.Target >= 0 && p.Target < len(s.Agents) && !s.Agents[p.Target].Dead && !IsExiled(s, p.Target) {
				s.appendNorm(Norm{Kind: NormExile, Target: p.Target,
					Text: p.Text, Proposer: p.Proposer, DayPassed: day, Tally: tally})
			}
		}
	}

	// Votes are socially visible: aligned voters warm, opposed voters cool
	// (FR-010) — both directions, whatever the outcome.
	deltas := func(group []int, other []int) {
		for _, a := range group {
			for _, b := range group {
				if a == b || a < 0 || b < 0 || a >= len(s.Agents) || b >= len(s.Agents) {
					continue
				}
				r := s.relation(a, b)
				r.Affection = clampInt(r.Affection+meetingAlignAffection, relMin, relMax)
			}
			for _, b := range other {
				if a == b || a < 0 || b < 0 || a >= len(s.Agents) || b >= len(s.Agents) {
					continue
				}
				r := s.relation(a, b)
				r.Trust = clampInt(r.Trust-meetingOpposeTrust, relMin, relMax)
			}
		}
	}
	deltas(p.Yeas, p.Nays)
	deltas(p.Nays, p.Yeas)
}

func (s *State) appendNorm(n Norm) {
	s.NextNormID++
	n.ID = s.NextNormID
	n.Active = true
	s.Norms = append(s.Norms, n)
}

// --- executor beats (pure functions of state/map/tick) ---

func meetingActive(s *State) bool { return s.Meeting.Phase != "" }

// attendCandidate: who gets pinned to the meeting and counted at the door.
func attendCandidate(s *State, i int) bool {
	a := &s.Agents[i]
	return !a.Dead && !a.Asleep && !IsExiled(s, i)
}

// governanceEvents runs the meeting lifecycle and the per-minute violation
// detectors. Called from stepEvents.
func governanceEvents(s *State, m *worldmap.Map, nextTick int64) []store.Event {
	var events []store.Event
	emit := func(typ string, payload any) {
		events = append(events, store.Event{Tick: nextTick, Type: typ, Payload: mustPayload(payload)})
	}
	sod := clock.SecondOfDay(nextTick)

	if conv := s.MeetingConvention; conv == nil {
		// No convention → no meeting. Instead, watch for one to emerge
		// in-world: a quorum sustaining a daytime gathering at one structure
		// (TASK-36). Norm enforcement below still runs — an upgraded save can
		// carry a law with no convention yet.
		if nextTick%60 == 0 {
			events = append(events, emergentGatheringEvents(s, nextTick)...)
		}
	} else {
		// Convene: once per day, ahead of the open, when the village exists.
		if sod == int64(conv.ConveneSecond) && s.Meeting.Phase == "" &&
			DayIndex(nextTick) > s.Meeting.LastMeetingDay && livingCount(s) >= minVillagersToMeet {
			place := s.MeetingPlace
			if place == nil {
				p := deriveMeetingPlace(s, m)
				place = &p
				emit("meeting.place_designated", MeetingPlacePayload{X: p.X, Y: p.Y})
			}
			emit("meeting.convened", MeetingPlacePayload{X: place.X, Y: place.Y})
		}

		// Open at the convention hour with whoever made it.
		if sod == int64(conv.OpenSecond) && s.Meeting.Phase == "convening" && s.MeetingPlace != nil {
			var attendees []int
			for i := range s.Agents {
				a := &s.Agents[i]
				if attendCandidate(s, i) &&
					abs(a.X-s.MeetingPlace.X)+abs(a.Y-s.MeetingPlace.Y) <= meetingRadius {
					attendees = append(attendees, i)
				}
			}
			if attendees == nil {
				attendees = []int{} // an empty meeting still opens (and closes)
			}
			emit("meeting.opened", MeetingOpenedPayload{Attendees: attendees})
		}

		// The open meeting: speaking turns on cadence, then close.
		if s.Meeting.Phase == "open" {
			elapsed := nextTick - s.Meeting.OpenedTick
			agendaDone := s.Meeting.NextSpeaker >= len(s.Meeting.Attendees)
			switch {
			case agendaDone:
				emit("meeting.closed", MeetingClosedPayload{
					Proposals: s.Meeting.ProposalsTabled, Graced: elapsed > meetingTimeboxTicks})
			case elapsed >= meetingTimeboxTicks+meetingGraceTicks:
				emit("meeting.closed", MeetingClosedPayload{
					Proposals: s.Meeting.ProposalsTabled, Graced: true})
			case elapsed > 0 && elapsed%meetingTurnTicks == 0:
				events = append(events, speakingTurn(s, nextTick)...)
			}
		}
	}

	// Violation detectors ride the per-game-minute heartbeat.
	if nextTick%60 == 0 {
		events = append(events, curfewViolations(s, nextTick)...)
		events = append(events, exileViolations(s, nextTick)...)
	}

	return events
}

func livingCount(s *State) int {
	n := 0
	for i := range s.Agents {
		if !s.Agents[i].Dead {
			n++
		}
	}
	return n
}

// deriveMeetingPlace: the village gathers where the first fire was lit —
// else the first shelter, else the passable tile nearest map center. Pure
// function of (state, map); persisted forever by the designation event.
func deriveMeetingPlace(s *State, m *worldmap.Map) Point {
	for _, kind := range []string{"fire", "shelter"} {
		for _, st := range s.Structures {
			if st.Kind == kind {
				return Point{X: st.X, Y: st.Y}
			}
		}
	}
	cx, cy := m.W/2, m.H/2
	if p, ok := nearest(m, s, cx, cy, func(x, y int) bool { return passable(m, s, x, y) }); ok {
		return p
	}
	return Point{X: cx, Y: cy}
}

// emergentGatheringEvents advances the emergent-convention watch (TASK-36):
// a pure function of (state, tick), sampled once a game-minute while no
// convention exists. It tracks how long a quorum has kept gathering at one
// structure during daytime; unbroken for emergentGatherTicks, a convention is
// born (source "emergent") — place = that structure, convene = the current
// half-hour, open = a half-hour later. Emits only on a change to the watch
// (start, break) or at establishment, so a quiet village logs nothing.
func emergentGatheringEvents(s *State, nextTick int64) []store.Event {
	gx, gy, present := gatheringStructure(s)
	watching := s.Meeting.GatherStart > 0
	same := watching && s.Meeting.GatherX == gx && s.Meeting.GatherY == gy

	switch {
	case present && same:
		// The gathering has held. Once sustained, the convention takes hold.
		if nextTick-s.Meeting.GatherStart >= emergentGatherTicks {
			sod := clock.SecondOfDay(nextTick)
			convene := int(sod - sod%1800) // round the observed hour down to the half-hour
			return []store.Event{{Tick: nextTick, Type: "meeting.convention_established",
				Payload: mustPayload(MeetingConventionPayload{
					ConveneSecond: convene, OpenSecond: convene + 1800,
					X: gx, Y: gy, Source: "emergent"})}}
		}
		return nil
	case present:
		// A new gathering (nothing watched, or it moved structures): start the clock.
		return []store.Event{{Tick: nextTick, Type: "sim.gathering_observed",
			Payload: mustPayload(GatheringObservedPayload{X: gx, Y: gy, Start: nextTick})}}
	case watching:
		// The gathering broke: reset the watch.
		return []store.Event{{Tick: nextTick, Type: "sim.gathering_observed",
			Payload: mustPayload(GatheringObservedPayload{})}}
	}
	return nil
}

// gatheringStructure: the fire or shelter with the most awake, non-exiled,
// living villagers within meetingRadius during daytime — a candidate meeting
// place forming on its own. ok is false at night, or below quorum. Ties break
// on structure order, so the choice is deterministic.
func gatheringStructure(s *State) (x, y int, ok bool) {
	if s.Night {
		return 0, 0, false
	}
	best := 0
	for _, st := range s.Structures {
		if st.Kind != "fire" && st.Kind != "shelter" {
			continue
		}
		c := 0
		for i := range s.Agents {
			a := &s.Agents[i]
			if a.Dead || a.Asleep || IsExiled(s, i) {
				continue
			}
			if abs(a.X-st.X)+abs(a.Y-st.Y) <= meetingRadius {
				c++
			}
		}
		if c >= minVillagersToMeet && c > best {
			best, x, y, ok = c, st.X, st.Y, true
		}
	}
	return x, y, ok
}

// NewConventionEvent builds the meeting.convention_established event for a
// per-world config-declared convention (TASK-36, source "config"), for the
// daemon to seed on boot. Place defaults to the derived gathering tile when
// the config supplies no coordinates.
func NewConventionEvent(s *State, m *worldmap.Map, tick int64, conveneSecond, openSecond int, x, y *int) store.Event {
	px, py := 0, 0
	if x != nil && y != nil {
		px, py = *x, *y
	} else {
		p := deriveMeetingPlace(s, m)
		px, py = p.X, p.Y
	}
	return store.Event{Tick: tick, Type: "meeting.convention_established",
		Payload: mustPayload(MeetingConventionPayload{
			ConveneSecond: conveneSecond, OpenSecond: openSecond, X: px, Y: py, Source: "config"})}
}

// speakingTurn gives the next attendee the floor: a fodder-rule proposal
// (tabled and resolved in this same beat — no open-proposal state survives
// the tick) or their loudest grievance as a raised note.
func speakingTurn(s *State, nextTick int64) []store.Event {
	var events []store.Event
	emit := func(typ string, payload any) {
		events = append(events, store.Event{Tick: nextTick, Type: typ, Payload: mustPayload(payload)})
	}

	speaker := s.Meeting.Attendees[s.Meeting.NextSpeaker]
	if speaker < 0 || speaker >= len(s.Agents) || s.Agents[speaker].Dead {
		emit("meeting.turn_taken", TurnTakenPayload{Agent: speaker}) // silent skip
		return events
	}

	prop := proposalFor(s, speaker, nextTick)
	raised := ""
	if prop == nil {
		raised = grievanceOf(s, speaker)
	}
	emit("meeting.turn_taken", TurnTakenPayload{Agent: speaker, Raised: raised})
	events = append(events, situatedMemoryEvent(nextTick, speaker, salMeetingSpoke,
		PlaceAt(s, s.Agents[speaker].X, s.Agents[speaker].Y), "", "Spoke at the village meeting."))

	if prop == nil {
		return events
	}
	prop.ProposalID = s.NextProposalID + 1
	emit("meeting.proposal_tabled", *prop)

	// Same-beat resolution: votes are a pure function of the edges.
	yeas, nays := resolveVotes(s, *prop)
	passed := len(yeas)*2 > len(yeas)+len(nays) // strict majority; ties fail
	emit("meeting.proposal_resolved", ProposalResolvedPayload{
		ProposalPayload: *prop, Yeas: yeas, Nays: nays, Passed: passed})

	// Outcomes are memory fodder for everyone in the square.
	proposer := &s.Agents[prop.Proposer]
	if passed {
		events = append(events, situatedMemoryEvent(nextTick, prop.Proposer, salMeetingOutcome,
			PlaceAt(s, proposer.X, proposer.Y), "", "The village passed my proposal: %s", prop.Text))
	} else {
		events = append(events, situatedMemoryEvent(nextTick, prop.Proposer, salMeetingOutcome,
			PlaceAt(s, proposer.X, proposer.Y), "", "The village voted my proposal down: %s", prop.Text))
	}
	for _, v := range append(append([]int{}, yeas...), nays...) {
		if v == prop.Proposer || v < 0 || v >= len(s.Agents) || s.Agents[v].Dead {
			continue
		}
		tone, verb := toneVotedYea, "for"
		if containsInt(nays, v) {
			tone, verb = toneVotedNay, "against"
		}
		outcome := "passed"
		if !passed {
			outcome = "failed"
		}
		events = append(events, situatedMemoryAboutEvent(nextTick, v, prop.Proposer, tone, salMeetingOutcome,
			PlaceAt(s, s.Agents[v].X, s.Agents[v].Y), "Voted %s %s's proposal (%s): %s", verb, proposer.Name, outcome, prop.Text))
	}
	if passed && prop.Kind == ProposeExile && prop.Target >= 0 && prop.Target < len(s.Agents) && !s.Agents[prop.Target].Dead {
		events = append(events, situatedMemoryEvent(nextTick, prop.Target, salExiled,
			PlaceAt(s, s.Agents[prop.Target].X, s.Agents[prop.Target].Y), "", "The village voted to cast me out."))
	}
	return events
}

// grievanceOf: the speaker's loudest sore point — most salient negative-tone
// memory; ties to the newest. Empty when they have nothing to raise.
func grievanceOf(s *State, agent int) string {
	var best *Memory
	for i := range s.Agents[agent].Memories {
		m := &s.Agents[agent].Memories[i]
		if m.Tone >= 0 {
			continue
		}
		if best == nil || m.Salience > best.Salience ||
			(m.Salience == best.Salience && m.Tick > best.Tick) {
			best = m
		}
	}
	if best == nil {
		return ""
	}
	return best.Text
}

// proposalFor runs the fodder rules in order; first match tables
// (contracts/meeting-lifecycle.md). Nil means nothing to table.
func proposalFor(s *State, proposer int, nextTick int64) *ProposalPayload {
	// 1) Curfew: the night hunts us.
	if activeNormOfKind(s, NormCurfew) == nil {
		for _, m := range s.Agents[proposer].Memories {
			if strings.Contains(m.Text, "gru") && nextTick-m.Tick <= gruFodderWindowTicks {
				return &ProposalPayload{Kind: ProposeCurfew, Target: -1,
					Param: nightStartSecond, Proposer: proposer,
					Text: "No one out alone after nightfall — the night hunts us."}
			}
		}
	}
	// 2) Repay debts: a promise is a promise.
	if activeNormOfKind(s, NormRepayDebts) == nil {
		for _, d := range s.Debts {
			if d.Status == "broken" && d.Creditor == proposer {
				return &ProposalPayload{Kind: ProposeRepayDebts, Target: -1,
					Proposer: proposer,
					Text:     "Debts must be repaid — a promise is a promise."}
			}
		}
	}
	// 3) Amend/repeal: repeat violators legislate in self-interest.
	for i := range s.Norms {
		n := &s.Norms[i]
		if !n.Active || n.Kind == NormExile || ViolationCount(n, proposer) < repealViolationCount {
			continue
		}
		rel := s.RelationBetween(proposer, n.Proposer)
		if n.Kind == NormCurfew && !n.Amended && rel.Affection >= 0 {
			return &ProposalPayload{Kind: ProposeAmend, NormID: n.ID, Target: -1,
				Param: (n.Param + curfewAmendDelta) % 86400, Proposer: proposer,
				Text: "The curfew starts too early — give us two more hours of dusk."}
		}
		return &ProposalPayload{Kind: ProposeRepeal, NormID: n.ID, Target: -1,
			Proposer: proposer,
			Text:     fmt.Sprintf("Strike the rule down — it serves nobody: %s", n.Text)}
	}
	// 4) Exile: the valve of last resort.
	for t := range s.Agents {
		if t == proposer || s.Agents[t].Dead || IsExiled(s, t) {
			continue
		}
		sum, others := 0, 0
		for o := range s.Agents {
			if o == t || s.Agents[o].Dead {
				continue
			}
			r := s.RelationBetween(o, t)
			sum += r.Trust + r.Affection
			others++
		}
		pr := s.RelationBetween(proposer, t)
		if others > 0 && sum/others < exileHostilityGate && pr.Trust+pr.Affection < 0 {
			return &ProposalPayload{Kind: ProposeExile, Target: t, Proposer: proposer,
				Text: fmt.Sprintf("%s is a danger to us all — cast them out.", s.Agents[t].Name)}
		}
	}
	return nil
}

// resolveVotes: each eligible attendee's vote is a pure integer function of
// their relation edges (contracts/meeting-lifecycle.md). The proposer always
// votes yea; an exile's subject does not vote on their own exile.
func resolveVotes(s *State, p ProposalPayload) (yeas, nays []int) {
	for _, v := range s.Meeting.Attendees {
		if v < 0 || v >= len(s.Agents) || s.Agents[v].Dead {
			continue
		}
		if p.Kind == ProposeExile && v == p.Target {
			continue
		}
		if v == p.Proposer || voteScore(s, v, p) >= 0 {
			yeas = append(yeas, v)
		} else {
			nays = append(nays, v)
		}
	}
	return yeas, nays
}

func voteScore(s *State, voter int, p ProposalPayload) int {
	rp := s.RelationBetween(voter, p.Proposer)
	base := rp.Trust + rp.Affection
	switch p.Kind {
	case ProposeAmend, ProposeRepeal:
		if n := normByID(s, p.NormID); n != nil && ViolationCount(n, voter) > 0 {
			base += selfInterestBonus
		}
	case ProposeExile:
		rt := s.RelationBetween(voter, p.Target)
		return -(rt.Trust + rt.Affection) + base/4
	}
	return base
}

// --- violation detectors ---

// witnessesOf: living, awake villagers close enough to see (and judge).
func witnessesOf(s *State, violator int) []int {
	var out []int
	v := &s.Agents[violator]
	for w := range s.Agents {
		wa := &s.Agents[w]
		if w == violator || wa.Dead || wa.Asleep {
			continue
		}
		if abs(wa.X-v.X)+abs(wa.Y-v.Y) <= witnessRadius {
			out = append(out, w)
		}
	}
	return out
}

// violatedSince: the latch — has this agent already been caught breaking
// this norm since the given tick?
func violatedSince(n *Norm, agent int, tick int64) bool {
	for _, v := range n.Violations {
		if v.Agent == agent && v.Tick >= tick {
			return true
		}
	}
	return false
}

// curfewActiveAt handles the wrap past midnight: the window runs from the
// curfew's start second to dawn.
func curfewActiveAt(param int, sod int64) bool {
	if int64(param) >= dayStartSecond {
		return sod >= int64(param) || sod < dayStartSecond
	}
	return sod >= int64(param) && sod < dayStartSecond
}

func curfewViolations(s *State, nextTick int64) []store.Event {
	n := activeNormOfKind(s, NormCurfew)
	if n == nil || !s.Night {
		return nil
	}
	sod := clock.SecondOfDay(nextTick)
	if !curfewActiveAt(n.Param, sod) {
		return nil
	}
	// The current night began at the most recent 22:00 boundary.
	nightStart := nextTick - ((sod - nightStartSecond + 86400) % 86400)
	var events []store.Event
	for i := range s.Agents {
		a := &s.Agents[i]
		if a.Dead || a.Asleep || IsExiled(s, i) || warmAt(s, a.X, a.Y, nextTick) {
			continue
		}
		if violatedSince(n, i, nightStart) {
			continue
		}
		events = append(events, violationEvents(s, n, i, nextTick)...)
	}
	return events
}

func exileViolations(s *State, nextTick int64) []store.Event {
	var events []store.Event
	for i := range s.Norms {
		n := &s.Norms[i]
		if !n.Active || n.Kind != NormExile {
			continue
		}
		t := n.Target
		if t < 0 || t >= len(s.Agents) || s.Agents[t].Dead {
			continue
		}
		if violatedSince(n, t, nextTick-exileShunLatchTicks) {
			continue
		}
		a := &s.Agents[t]
		near := s.MeetingPlace != nil &&
			abs(a.X-s.MeetingPlace.X)+abs(a.Y-s.MeetingPlace.Y) <= exileShunRadius
		if !near {
			for _, st := range s.Structures {
				if abs(a.X-st.X)+abs(a.Y-st.Y) <= exileShunRadius {
					near = true
					break
				}
			}
		}
		if near {
			events = append(events, violationEvents(s, n, t, nextTick)...)
		}
	}
	return events
}

// violationEvents: the norm.violated event plus its witness-memory
// companions — the village only judges what it can see, so no witnesses
// means nothing happens (FR-009).
func violationEvents(s *State, n *Norm, violator int, nextTick int64) []store.Event {
	witnesses := witnessesOf(s, violator)
	if len(witnesses) == 0 {
		return nil
	}
	events := []store.Event{{Tick: nextTick, Type: "norm.violated",
		Payload: mustPayload(NormViolatedPayload{NormID: n.ID, Violator: violator, Witnesses: witnesses})}}
	verb := "broke the village's law"
	if n.Kind == NormExile {
		verb = "defied their exile"
	}
	for _, w := range witnesses {
		events = append(events, situatedMemoryAboutEvent(nextTick, w, violator, toneViolation, salNormViolation,
			PlaceAt(s, s.Agents[w].X, s.Agents[w].Y), "%s %s: %s", s.Agents[violator].Name, verb, n.Text))
	}
	return events
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
