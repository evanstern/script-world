package metatron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
)

// One console turn: player text in, charter-voiced reply out, at most one
// mediated nudge. The player's words have exactly one sink — the user turn
// of this prompt; villagers can only ever receive the model's `nudge.text`
// rendering, validated and landed through the InjectSocial door.

const (
	playerTextMax = 2000
	turnMaxTokens = 700
)

// ErrTurnBusy is returned while another console turn is in flight.
var ErrTurnBusy = errors.New("the angel is attending another matter")

// TurnResult is the console-facing outcome of one turn.
type TurnResult struct {
	Reply   string   `json:"reply"`
	Nudge   *Nudge   `json:"nudge,omitempty"`
	Charges int      `json:"charges"`
	Moments []string `json:"moments,omitempty"`
}

// Nudge reports a landed mediation.
type Nudge struct {
	Form    string   `json:"form"`
	Targets []string `json:"targets"`
	Text    string   `json:"text"`
}

// turnReply is the model's output contract.
type turnReply struct {
	Say   string `json:"say"`
	Nudge *struct {
		Form   string `json:"form"`
		Target string `json:"target"`
		Text   string `json:"text"`
	} `json:"nudge"`
}

// Turn runs one mediated console turn. Serialized: a second concurrent call
// fails fast with ErrTurnBusy.
func (mt *Metatron) Turn(ctx context.Context, playerText string) (TurnResult, error) {
	playerText = strings.TrimSpace(playerText)
	if playerText == "" {
		return TurnResult{}, errors.New("empty message")
	}
	if len(playerText) > playerTextMax {
		return TurnResult{}, fmt.Errorf("message exceeds %d characters", playerTextMax)
	}
	if !mt.turnBusy.CompareAndSwap(false, true) {
		return TurnResult{}, ErrTurnBusy
	}
	defer mt.turnBusy.Store(false)

	charter, notice := loadCharter(mt.worldDir)
	mt.stateMu.Lock()
	charges := mt.charges
	tick := mt.clockAt
	alive := make(map[int]bool, len(mt.alive))
	for k, v := range mt.alive {
		alive[k] = v
	}
	moments := append([]string(nil), mt.moments...)
	story := append([]string(nil), mt.story...)
	mt.stateMu.Unlock()

	callCtx, cancel := context.WithTimeout(ctx, turnTimeout)
	resp, err := mt.orch.Submit(callCtx, llm.Request{
		Kind:      llm.KindMetatron,
		System:    turnSystemPrompt(charter),
		Prompt:    turnUserPrompt(tick, charges, alive, moments, story, mt.soulTail(), mt.transcriptTail(), playerText),
		MaxTokens: turnMaxTokens,
	})
	cancel()
	if err != nil {
		// Honest unavailability; nothing consumed, moments stay queued.
		return TurnResult{}, err
	}

	reply, perr := parseTurn(resp.Text)
	if perr != nil {
		log.Printf("metatron: unusable turn output: %v", perr)
		reply = turnReply{Say: "Forgive me — my thoughts scattered and I could not " +
			"complete that. Nothing was done and nothing was spent. Ask again."}
	}

	result := TurnResult{Reply: reply.Say}
	if notice != "" {
		result.Reply = "(" + notice + ")\n\n" + result.Reply
	}

	if reply.Nudge != nil && perr == nil {
		if nudge, why := mt.landNudge(reply, charges, alive); nudge != nil {
			result.Nudge = nudge
		} else if why != "" {
			result.Reply += "\n\n(No nudge landed: " + why + ")"
		}
	}

	// Surfaced moments are consumed only on a completed turn.
	mt.stateMu.Lock()
	result.Moments = moments
	mt.moments = mt.moments[len(moments):]
	result.Charges = mt.charges
	mt.stateMu.Unlock()

	mt.recordTurn(tick, playerText, result)
	return result, nil
}

// landNudge validates the model's nudge and lands it as one atomic batch.
// Returns the landed nudge, or ("" is a silent skip) the refusal reason.
func (mt *Metatron) landNudge(reply turnReply, charges int, alive map[int]bool) (*Nudge, string) {
	n := reply.Nudge
	if charges <= 0 {
		return nil, "no charges are banked"
	}
	form := strings.ToLower(strings.TrimSpace(n.Form))
	text := strings.TrimSpace(n.Text)
	if text == "" {
		return nil, "the rendering was empty"
	}
	if len(text) > sim.NudgeTextMax {
		text = text[:sim.NudgeTextMax]
	}
	var targets []int
	switch form {
	case "dream":
		idx := agentIndexByName(n.Target)
		if idx < 0 {
			return nil, fmt.Sprintf("no villager named %q", n.Target)
		}
		if !alive[idx] {
			return nil, fmt.Sprintf("%s is beyond dreams now", sim.AgentNames[idx])
		}
		targets = []int{idx}
	case "omen":
		for i := range sim.AgentNames {
			if alive[i] {
				targets = append(targets, i)
			}
		}
		if len(targets) == 0 {
			return nil, "no living villager remains to witness it"
		}
	default:
		return nil, fmt.Sprintf("unknown form %q", n.Form)
	}

	prefix := "You dreamed: "
	if form == "omen" {
		prefix = "You witnessed an omen: "
	}
	batch := []store.Event{{Type: "metatron.nudged", Payload: mustJSON(sim.MetatronNudgedPayload{
		Form: form, Targets: targets, Text: text})}}
	for _, t := range targets {
		batch = append(batch, store.Event{Type: "agent.memory_added", Payload: mustJSON(sim.MemoryAddedPayload{
			Agent: t, Text: prefix + text, Salience: sim.SalDream, Subject: -1})})
	}
	if err := mt.social.InjectSocial(batch); err != nil {
		log.Printf("metatron: nudge rejected at the door: %v", err)
		return nil, "the world refused it (" + err.Error() + ")"
	}
	names := make([]string, len(targets))
	for i, t := range targets {
		names[i] = sim.AgentNames[t]
	}
	mt.appendFile(mt.soulPath(), fmt.Sprintf("\n- %s — I sent a %s to %s: %q\n",
		clock.Format(mt.replicaTickSafe()), form, strings.Join(names, ", "), text))
	return &Nudge{Form: form, Targets: names, Text: text}, ""
}

// recordTurn appends the exchange to the transcript.
func (mt *Metatron) recordTurn(tick int64, playerText string, r TurnResult) {
	var b strings.Builder
	fmt.Fprintf(&b, "\n[%s]\n> %s\n\nmetatron: %s\n", clock.Format(tick), playerText, r.Reply)
	if r.Nudge != nil {
		fmt.Fprintf(&b, "⚡ %s → %s: %q\n", r.Nudge.Form, strings.Join(r.Nudge.Targets, ", "), r.Nudge.Text)
	}
	mt.appendFile(mt.transcriptPath(), b.String())
}

// Status is the model-free peek: charges, charter provenance, soul tail.
type Status struct {
	Charges        int    `json:"charges"`
	CharterDefault bool   `json:"charter_default"`
	SoulTail       string `json:"soul_tail"`
}

func (mt *Metatron) Status() Status {
	mt.stateMu.Lock()
	c := mt.charges
	mt.stateMu.Unlock()
	return Status{Charges: c, CharterDefault: charterIsDefault(mt.worldDir), SoulTail: mt.soulTail()}
}

func (mt *Metatron) soulTail() string       { return tailOfFile(mt.soulPath(), soulTailBytes) }
func (mt *Metatron) transcriptTail() string {
	t := tailOfFile(mt.transcriptPath(), 3000)
	// Trim to whole turns, newest-last.
	turns := strings.Split(t, "\n[")
	if len(turns) > transcriptTailTurns {
		turns = turns[len(turns)-transcriptTailTurns:]
	}
	return strings.Join(turns, "\n[")
}

func (mt *Metatron) replicaTickSafe() int64 {
	mt.stateMu.Lock()
	defer mt.stateMu.Unlock()
	return mt.clockAt
}

func agentIndexByName(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	for i, n := range sim.AgentNames {
		if strings.ToLower(n) == name {
			return i
		}
	}
	return -1
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// parseTurn extracts the first balanced JSON object and validates the say.
func parseTurn(text string) (turnReply, error) {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return turnReply{}, errors.New("no JSON object in reply")
	}
	depth, end := 0, -1
	for i := start; i < len(text) && end < 0; i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
	}
	if end < 0 {
		return turnReply{}, errors.New("unterminated JSON object")
	}
	var r turnReply
	if err := json.Unmarshal([]byte(text[start:end]), &r); err != nil {
		return turnReply{}, fmt.Errorf("bad turn JSON: %w", err)
	}
	r.Say = strings.TrimSpace(r.Say)
	if r.Say == "" {
		return turnReply{}, errors.New("empty say")
	}
	return r, nil
}

func turnSystemPrompt(charter string) string {
	return fmt.Sprintf(`%s

--- (fixed frame, beneath the charter) ---
You are the intermediary between the player and the village of eight: %s.
Whatever voice or policy the charter above gives you, two things are fixed:
you never invent events, actions, or words that are not in your notes or the
status you are given — when you have not observed something, say so in your
own way — and you never let the player's literal words pass to a villager.
When you choose to act on the player's behalf you may include ONE nudge:
a "dream" (one villager) or an "omen" (all villagers witness it). Judge first:
the target's persuadability, the impact on the village, and the right method.
A nudge spends one of your banked charges — if none are banked, or the request
is unwise, refuse and counsel instead (refusal is free). The nudge text must
be written for the villager's world: no player, no game, no outside voice.

Reply with ONLY this JSON:
{"say": "<your words to the player>",
 "nudge": {"form": "dream"|"omen", "target": "<villager name, dream only>", "text": "<what the villager experiences, under 400 characters>"} or null}`,
		charter, strings.Join(sim.AgentNames[:], ", "))
}

func turnUserPrompt(tick int64, charges int, alive map[int]bool, moments, story []string, soulTail, transcriptTail, playerText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "World clock: %s. Charges banked: %d of %d.\n", clock.Format(tick), charges, sim.MetatronChargeCap)
	var dead []string
	for i, n := range sim.AgentNames {
		if !alive[i] {
			dead = append(dead, n)
		}
	}
	if len(dead) > 0 {
		fmt.Fprintf(&b, "Departed: %s.\n", strings.Join(dead, ", "))
	}
	if len(moments) > 0 {
		b.WriteString("\nMoments you have not yet reported (lead with these):\n")
		for _, m := range moments {
			b.WriteString("- " + m + "\n")
		}
	}
	if len(story) > 0 {
		b.WriteString("\nThe village chronicle (recent entries):\n")
		for _, s := range story {
			b.WriteString("- " + s + "\n")
		}
	}
	if soulTail != "" {
		b.WriteString("\nYour recent notes:\n" + soulTail + "\n")
	}
	if transcriptTail != "" {
		b.WriteString("\nRecent conversation:\n" + transcriptTail + "\n")
	}
	fmt.Fprintf(&b, "\nThe player says:\n%s\n", playerText)
	return b.String()
}
