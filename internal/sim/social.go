package sim

import (
	"encoding/json"
	"fmt"

	"github.com/evanstern/script-world/internal/store"
)

// The social fabric (TASK-8): directed relation edges, an append-only debt
// ledger with computed reputation, and rumors with per-holder variants and
// provenance. Everything here is event-sourced and deterministic; model
// creativity (conversation text, paraphrases) only ever arrives as recorded
// events through the loop's inject_social door.

// --- types ---

type Relation struct {
	From      int `json:"from"`
	To        int `json:"to"`
	Trust     int `json:"trust"`     // −1000..1000
	Affection int `json:"affection"` // −1000..1000
}

type Debt struct {
	ID       int    `json:"id"`
	Debtor   int    `json:"debtor"`
	Creditor int    `json:"creditor"`
	Kind     string `json:"kind"` // "food"
	Due      int64  `json:"due"`
	Status   string `json:"status"` // open | kept | broken
}

type Rumor struct {
	ID          int   `json:"id"`
	Subject     int   `json:"subject"`
	Tone        int   `json:"tone"` // −100..100
	Secret      bool  `json:"secret,omitempty"`
	OriginAgent int   `json:"origin_agent"`
	OriginTick  int64 `json:"origin_tick"`
}

// KnownRumor is one holder's variant of a rumor; From is the provenance link
// (−1 = originator). The chain of From links IS the provenance trail.
type KnownRumor struct {
	RumorID    int    `json:"rumor_id"`
	Text       string `json:"text"`
	Confidence int    `json:"confidence"` // 0..100; ≤ rumorFloor = no longer tellable
	From       int    `json:"from"`
	Tick       int64  `json:"tick"`
}

// --- tuning ---

const (
	relMin, relMax = -1000, 1000

	// Deterministic edge rules (data-model.md).
	talkAffection        = 5
	giveTrustToGiver     = 30
	giveAffectionToGiver = 20
	giveAffectionToRecv  = 10
	brokenTrustPenalty   = -150
	brokenAffectPenalty  = -50

	debtDueTicks     = 2 * 24 * 3600 // 2 game days
	giveCooldownSec  = 3600
	giveNeedBelow    = 150 // receiver Food need
	giveKeepsAtLeast = 2   // giver keeps one meal for themselves

	rumorFloor       = 25 // confidence at or below: rumor dies
	rumorDecayNum    = 4  // ×4/5 per hop
	rumorDecayDen    = 5
	rumorMinSalience = 4 // memories this salient (about others) become tellable
	// SecretTrustGate is the owner→listener trust needed before a secret can
	// slip; SecretShareRoll adds a seeded 1-in-3 chance per eligible convo.
	SecretTrustGate   = 700
	secretShareChance = 3

	// Conversation tuning (used by the mind's driver; here so sim tests and
	// the driver share one truth).
	// 2 per side (within the grounding's "~5 cap"): local 12B inference runs
	// ~45s/utterance under village load, and a conversation must finish
	// inside its deadline at that pace.
	ConvoTurnsPerSide = 2
	ConvoToneAffect   = 25
	ConvoToneTrust    = 12

	salGaveHelp  = 5
	salWasSaved  = 5
	salNeverPaid = 6
	// SalConvoGist weights conversation-gist memories (driver-injected).
	SalConvoGist  = 4
	toneSaved     = 40
	toneNeverPaid = -60
)

// SecretShareRoll: seeded chance gate for a secret slipping in an eligible
// conversation — deterministic per (seed, cadence bucket, owner).
func SecretShareRoll(seed uint64, tick int64, agent int) bool {
	r := rngAt(seed, "secret-share", tick/PlannerCadenceTicks, agent)
	return r.Uint64N(secretShareChance) == 0
}

// --- payloads ---

type (
	RelationChangedPayload struct {
		A              int    `json:"a"`
		B              int    `json:"b"`
		TrustDelta     int    `json:"trust_delta"`
		AffectionDelta int    `json:"affection_delta"`
		Reason         string `json:"reason"`
	}
	GavePayload struct {
		From int    `json:"from"`
		To   int    `json:"to"`
		Kind string `json:"kind"`
	}
	PromiseBrokenPayload struct {
		ID int `json:"id"`
	}
	RumorToldPayload struct {
		From       int    `json:"from"`
		To         int    `json:"to"`
		RumorID    int    `json:"rumor_id"` // 0 = birth
		Subject    int    `json:"subject"`
		Tone       int    `json:"tone"`
		Text       string `json:"text"`
		Confidence int    `json:"confidence"`
		Secret     bool   `json:"secret,omitempty"`
	}
	SecretSeededPayload struct {
		Agent int    `json:"agent"`
		Text  string `json:"text"`
		Tone  int    `json:"tone"`
	}
	ConversationTurnPayload struct {
		Conv     int64  `json:"conv"` // tick of the founding talk = conversation id
		Speaker  int    `json:"speaker"`
		Listener int    `json:"listener"`
		Text     string `json:"text"`
	}
	ConversationPayload struct {
		Conv  int64  `json:"conv"`
		A     int    `json:"a"`
		B     int    `json:"b"`
		Gist  string `json:"gist"`
		Turns int    `json:"turns"`
		// TASK-22: scenes may hold more than a pair; empty means [A, B]
		// (pre-TASK-22 payloads replay unchanged).
		Participants []int    `json:"participants,omitempty"`
		Topics       []string `json:"topics,omitempty"`
		Tones        []int    `json:"tones,omitempty"` // per participant, -2..2
	}
	// ChestTakenPayload — social.chest_taken (spec 013 US4, FR-011): the distinct
	// taking happening co-emitted with a non-owner agent.withdrew. Canonical
	// field order per contracts/events.md; the reducer records nothing beyond the
	// event itself (chronicle/TUI material).
	ChestTakenPayload struct {
		Owner int `json:"owner"`
		Taker int `json:"taker"`
		X     int `json:"x"`
		Y     int `json:"y"`
	}
)

// ConvoRecord is the durable trace of one conversation (TASK-22) — the
// artifact future prompts read back ("last time you spoke…"). Kept as a
// bounded ring on State so relationship fodder survives beyond the moment.
type ConvoRecord struct {
	Conv         int64    `json:"conv"`
	Tick         int64    `json:"tick"`
	Participants []int    `json:"participants"`
	Gist         string   `json:"gist"`
	Topics       []string `json:"topics,omitempty"`
	Tones        []int    `json:"tones,omitempty"`
}

// convoRecordCap bounds State.Conversations; older records fall off. The
// event log keeps everything — the ring is the working set prompts can see.
const convoRecordCap = 64

// LastConversationBetween returns the newest record naming both agents.
func LastConversationBetween(s *State, a, b int) (ConvoRecord, bool) {
	for i := len(s.Conversations) - 1; i >= 0; i-- {
		r := s.Conversations[i]
		var hasA, hasB bool
		for _, p := range r.Participants {
			hasA = hasA || p == a
			hasB = hasB || p == b
		}
		if hasA && hasB {
			return r, true
		}
	}
	return ConvoRecord{}, false
}

// LastConversationInvolving returns the newest record naming the agent.
func LastConversationInvolving(s *State, a int) (ConvoRecord, bool) {
	for i := len(s.Conversations) - 1; i >= 0; i-- {
		for _, p := range s.Conversations[i].Participants {
			if p == a {
				return s.Conversations[i], true
			}
		}
	}
	return ConvoRecord{}, false
}

// --- reducer cases (called from State.Apply) ---

func (s *State) relation(from, to int) *Relation {
	for i := range s.Relations {
		if s.Relations[i].From == from && s.Relations[i].To == to {
			return &s.Relations[i]
		}
	}
	s.Relations = append(s.Relations, Relation{From: from, To: to})
	return &s.Relations[len(s.Relations)-1]
}

// RelationBetween is the read side (zero values when never touched).
func (s *State) RelationBetween(from, to int) Relation {
	for _, r := range s.Relations {
		if r.From == from && r.To == to {
			return r
		}
	}
	return Relation{From: from, To: to}
}

func (s *State) applySocial(e store.Event) error {
	switch e.Type {
	case "social.relation_changed":
		var p RelationChangedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		if p.A == p.B || p.A < 0 || p.B < 0 || p.A >= len(s.Agents) || p.B >= len(s.Agents) {
			return fmt.Errorf("apply %s: bad pair %d→%d", e.Type, p.A, p.B)
		}
		r := s.relation(p.A, p.B)
		r.Trust = clampInt(r.Trust+p.TrustDelta, relMin, relMax)
		r.Affection = clampInt(r.Affection+p.AffectionDelta, relMin, relMax)

	case "social.gave":
		var p GavePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		giver, recv := &s.Agents[p.From], &s.Agents[p.To]
		// TODO(T018): giving is denominated in raw food for now; the food rewrite
		// may reconsider which form is shared. Behavior-equivalent re-expression.
		if giver.Inv.FoodRaw <= 0 {
			return fmt.Errorf("apply %s: %s has no food", e.Type, giver.Name)
		}
		giver.Inv.FoodRaw--
		// T012: clamp the receiver at the bulk cap defensively — FR-001 says
		// carried bulk MUST never exceed the cap, even against a forged event.
		// The executor never emits a give into a full pouch (repayable/giveable
		// guard on the receiver's free bulk), so legit logs always have room and
		// this is a no-op there; the clamp only bites on an injected over-cap give.
		if bulk(recv.Inv) < bulkCap {
			recv.Inv.FoodRaw++
		}
		giver.LastGive = e.Tick
		// Ledger transition (reducer-internal): repayment first, else debt.
		for i := range s.Debts {
			d := &s.Debts[i]
			if d.Status == "open" && d.Debtor == p.From && d.Creditor == p.To && d.Kind == p.Kind {
				d.Status = "kept"
				return nil
			}
		}
		s.NextDebtID++
		s.Debts = append(s.Debts, Debt{
			ID: s.NextDebtID, Debtor: p.To, Creditor: p.From,
			Kind: p.Kind, Due: e.Tick + debtDueTicks, Status: "open",
		})

	case "social.promise_broken":
		var p PromiseBrokenPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		for i := range s.Debts {
			if s.Debts[i].ID == p.ID && s.Debts[i].Status == "open" {
				s.Debts[i].Status = "broken"
				break
			}
		}

	case "social.rumor_told":
		var p RumorToldPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		id := p.RumorID
		if id == 0 { // birth
			s.NextRumorID++
			id = s.NextRumorID
			s.Rumors = append(s.Rumors, Rumor{
				ID: id, Subject: p.Subject, Tone: p.Tone, Secret: p.Secret,
				OriginAgent: p.From, OriginTick: e.Tick,
			})
			// The originator holds their own version.
			teller := &s.Agents[p.From]
			if !knows(teller, id) {
				teller.Known = append(teller.Known, KnownRumor{
					RumorID: id, Text: p.Text, Confidence: 100, From: -1, Tick: e.Tick,
				})
			}
		}
		listener := &s.Agents[p.To]
		if !knows(listener, id) {
			listener.Known = append(listener.Known, KnownRumor{
				RumorID: id, Text: p.Text, Confidence: p.Confidence, From: p.From, Tick: e.Tick,
			})
			// Hearing shifts feeling toward the subject.
			if p.Subject != p.To && p.Subject >= 0 && p.Subject < len(s.Agents) {
				r := s.relation(p.To, p.Subject)
				r.Affection = clampInt(r.Affection+p.Tone/4, relMin, relMax)
			}
		}

	case "social.secret_seeded":
		var p SecretSeededPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		s.NextRumorID++
		s.Rumors = append(s.Rumors, Rumor{
			ID: s.NextRumorID, Subject: p.Agent, Tone: p.Tone, Secret: true,
			OriginAgent: p.Agent, OriginTick: e.Tick,
		})
		owner := &s.Agents[p.Agent]
		owner.Known = append(owner.Known, KnownRumor{
			RumorID: s.NextRumorID, Text: p.Text, Confidence: 100, From: -1, Tick: e.Tick,
		})

	case "social.conversation_turn":
		// Chronicle material; no state effect.

	case "social.conversation":
		// TASK-22: conversations leave a durable, bounded record.
		var p ConversationPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("apply %s: %w", e.Type, err)
		}
		parts := p.Participants
		if len(parts) == 0 {
			parts = []int{p.A, p.B} // pre-TASK-22 payloads
		}
		s.Conversations = append(s.Conversations, ConvoRecord{
			Conv: p.Conv, Tick: e.Tick, Participants: parts,
			Gist: p.Gist, Topics: p.Topics, Tones: p.Tones,
		})
		if len(s.Conversations) > convoRecordCap {
			s.Conversations = append(s.Conversations[:0], s.Conversations[len(s.Conversations)-convoRecordCap:]...)
		}
	}
	return nil
}

func knows(a *Agent, rumorID int) bool {
	for _, k := range a.Known {
		if k.RumorID == rumorID {
			return true
		}
	}
	return false
}

// --- computed views ---

// Reputation is derived from the ledger, never stored: 500 + 100·kept −
// 200·broken, clamped 0..1000.
func Reputation(s *State, agent int) int {
	rep := 500
	for _, d := range s.Debts {
		if d.Debtor != agent {
			continue
		}
		switch d.Status {
		case "kept":
			rep += 100
		case "broken":
			rep -= 200
		}
	}
	return clampInt(rep, 0, 1000)
}

// Tellable is what a teller would pass to a listener: an existing known
// rumor, or a salient memory about a third party (rumor birth). Secrets are
// EXCLUDED here — only the conversation driver may pass one, behind the
// trust gate.
type Tellable struct {
	RumorID    int // 0 = birth from memory
	Subject    int
	Tone       int
	Text       string
	Confidence int // already decayed for the retell
}

func TellableFor(s *State, teller, listener int) (Tellable, bool) {
	t := &s.Agents[teller]
	best := Tellable{Confidence: -1}

	for _, k := range t.Known {
		r := rumorByID(s, k.RumorID)
		if r == nil || r.Secret || r.Subject == listener || k.Confidence <= rumorFloor {
			continue
		}
		if knows(&s.Agents[listener], k.RumorID) {
			continue
		}
		if k.Confidence > best.Confidence {
			best = Tellable{
				RumorID: k.RumorID, Subject: r.Subject, Tone: r.Tone,
				Text: k.Text, Confidence: k.Confidence * rumorDecayNum / rumorDecayDen,
			}
		}
	}
	if best.Confidence >= 0 {
		return best, true
	}

	// Birth: the teller's most salient gossip-worthy memory.
	for _, m := range t.Memories {
		if m.Subject < 0 || m.Subject == listener || m.Subject == teller || m.Salience < rumorMinSalience {
			continue
		}
		conf := m.Salience * 10
		if conf > best.Confidence {
			best = Tellable{
				RumorID: 0, Subject: m.Subject, Tone: m.Tone,
				Text: m.Text, Confidence: conf * rumorDecayNum / rumorDecayDen,
			}
		}
	}
	if best.Confidence >= 0 {
		return best, true
	}
	return Tellable{}, false
}

func rumorByID(s *State, id int) *Rumor {
	for i := range s.Rumors {
		if s.Rumors[i].ID == id {
			return &s.Rumors[i]
		}
	}
	return nil
}

// SecretOf returns the owner's still-secret self-rumor, if any.
func SecretOf(s *State, agent int) (KnownRumor, *Rumor, bool) {
	for _, k := range s.Agents[agent].Known {
		r := rumorByID(s, k.RumorID)
		if r != nil && r.Secret && r.OriginAgent == agent {
			// Still secret only if nobody else knows it.
			holders := 0
			for i := range s.Agents {
				if knows(&s.Agents[i], r.ID) {
					holders++
				}
			}
			if holders == 1 {
				return k, r, true
			}
		}
	}
	return KnownRumor{}, nil, false
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
