package metatron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/tool"
)

// nudgeTextMax is the nudge rendering cap, read from the tool registry (spec
// 014 T021): tool.Lookup("nudge_dream").Cost.TextCapBytes (400). It matches the
// sim reducer's NudgeTextMax enforcer — both derive from the same registry
// entry, so the metatron-side truncation and the door-side enforcement can
// never diverge.
var nudgeTextMax = func() int {
	t, _ := tool.Lookup("nudge_dream")
	return t.Cost.TextCapBytes
}()

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
	Miracle *Miracle `json:"miracle,omitempty"`
	Charges int      `json:"charges"`
	Moments []string `json:"moments,omitempty"`
}

// Nudge reports a landed mediation.
type Nudge struct {
	Form    string   `json:"form"`
	Targets []string `json:"targets"`
	Text    string   `json:"text"`
}

// Miracle reports a landed miracle (spec 016) — the kind and a one-line human
// rendering. Never carries gratis: the angel cannot work a free miracle.
type Miracle struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

// turnReply is the model's output contract. The miracle member has NO gratis
// field by design (contracts §1, FR-007/SC-005): any "gratis":true in model
// output is dropped at unmarshal, so a model-driven miracle is always charged —
// structural stripping, nothing to forget to sanitize.
type turnReply struct {
	Say   string `json:"say"`
	Nudge *struct {
		Form   string `json:"form"`
		Target string `json:"target"`
		Text   string `json:"text"`
	} `json:"nudge"`
	Miracle *struct {
		Kind     string `json:"kind"`
		Day      int    `json:"day"`
		Time     string `json:"time"`
		Villager string `json:"villager"`
		Item     string `json:"item"`
		Qty      int    `json:"qty"`
		Class    string `json:"class"`
		X        int    `json:"x"`
		Y        int    `json:"y"`
		ToX      int    `json:"to_x"`
		ToY      int    `json:"to_y"`
	} `json:"miracle"`
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

	// At most one mediated act per turn (the existing "one mediated act" rule):
	// a nudge takes the turn if present, otherwise a miracle may land.
	if reply.Nudge != nil && perr == nil {
		if nudge, why := mt.landNudge(reply, charges, alive); nudge != nil {
			result.Nudge = nudge
		} else if why != "" {
			result.Reply += "\n\n(No nudge landed: " + why + ")"
		}
	} else if reply.Miracle != nil && perr == nil {
		if miracle, why := mt.landMiracle(reply, charges); miracle != nil {
			result.Miracle = miracle
		} else if why != "" {
			result.Reply += "\n\n(No miracle landed: " + why + ")"
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
	if len(text) > nudgeTextMax {
		text = text[:nudgeTextMax]
	}
	// Roster enforcement (spec 014 US3, FR-008): the form must name a metatron
	// nudge tool on the metatron roster (nudge_dream / nudge_omen). Anything
	// else is refused exactly like an unknown form — same reason string.
	if !tool.OnRoster(tool.RosterMetatron, "nudge_"+form) {
		return nil, fmt.Sprintf("unknown form %q", n.Form)
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

// landMiracle validates the model's miracle and lands it as one atomic batch
// through the same door and shared builder the operator console uses (spec 016
// R6), so the two channels cannot drift. The angel can NEVER waive a charge:
// gratis is passed false unconditionally and does not exist on the turn contract
// (SC-005). Returns the landed miracle, or ("" is a silent skip) an in-fiction
// refusal reason reported in the reply suffix, exactly like landNudge.
func (mt *Metatron) landMiracle(reply turnReply, charges int) (*Miracle, string) {
	mm := reply.Miracle
	if charges <= 0 {
		return nil, "no charges are banked"
	}
	kind := strings.ToLower(strings.TrimSpace(mm.Kind))

	var params MiracleParams
	var summary string
	switch kind {
	case "move":
		params = MiracleParams{Class: strings.ToLower(strings.TrimSpace(mm.Class)), X: mm.X, Y: mm.Y, ToX: mm.ToX, ToY: mm.ToY}
		summary = fmt.Sprintf("moved the %s at (%d,%d) to (%d,%d)", params.Class, mm.X, mm.Y, mm.ToX, mm.ToY)
	case "remove":
		params = MiracleParams{Class: strings.ToLower(strings.TrimSpace(mm.Class)), X: mm.X, Y: mm.Y}
		summary = fmt.Sprintf("removed the %s at (%d,%d)", params.Class, mm.X, mm.Y)
	case "time_snap":
		hour, min, perr := clock.ParseTimeOfDay(mm.Time)
		if perr != nil {
			return nil, perr.Error()
		}
		params = MiracleParams{ToTick: clock.TickAt(int64(mm.Day), hour, min, 0)}
		summary = fmt.Sprintf("snapped time forward to day %d %02d:%02d", mm.Day, hour, min)
	case "give_item":
		idx := agentIndexByName(mm.Villager)
		if idx < 0 {
			return nil, fmt.Sprintf("no villager named %q", mm.Villager)
		}
		item := strings.ToLower(strings.TrimSpace(mm.Item))
		params = MiracleParams{Agent: idx, Item: item, Qty: mm.Qty}
		summary = fmt.Sprintf("granted %d %s to %s", mm.Qty, item, sim.AgentNames[idx])
	default:
		return nil, fmt.Sprintf("unknown miracle %q", mm.Kind)
	}

	// Resolve the perception-memory recipients (which villager stands on a move's
	// source tile) from the absorb-mirrored positions, so the turn worker never
	// races the replica the absorb goroutine owns; the shared builder reads only
	// agent positions/liveness.
	mt.stateMu.Lock()
	probe := &sim.State{Agents: make([]sim.Agent, len(mt.agentXY))}
	for i := range mt.agentXY {
		probe.Agents[i] = sim.Agent{X: mt.agentXY[i][0], Y: mt.agentXY[i][1], Dead: !mt.alive[i]}
	}
	mt.stateMu.Unlock()

	batch, err := BuildMiracleBatch(probe, kind, params, false)
	if err != nil {
		return nil, err.Error()
	}
	if err := mt.social.InjectSocial(batch); err != nil {
		log.Printf("metatron: miracle rejected at the door: %v", err)
		return nil, "the world refused it (" + err.Error() + ")"
	}
	mt.appendFile(mt.soulPath(), fmt.Sprintf("\n- %s — I worked a miracle: %s\n",
		clock.Format(mt.replicaTickSafe()), summary))
	return &Miracle{Kind: kind, Summary: summary}, ""
}

// recordTurn appends the exchange to the transcript.
func (mt *Metatron) recordTurn(tick int64, playerText string, r TurnResult) {
	var b strings.Builder
	fmt.Fprintf(&b, "\n[%s]\n> %s\n\nmetatron: %s\n", clock.Format(tick), playerText, r.Reply)
	if r.Nudge != nil {
		fmt.Fprintf(&b, "⚡ %s → %s: %q\n", r.Nudge.Form, strings.Join(r.Nudge.Targets, ", "), r.Nudge.Text)
	}
	if r.Miracle != nil {
		fmt.Fprintf(&b, "✨ miracle: %s\n", r.Miracle.Summary)
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

func (mt *Metatron) soulTail() string { return tailOfFile(mt.soulPath(), soulTailBytes) }
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

When a nudge is too indirect for the need, you may instead work ONE miracle — a
direct edit to the world, spent from the same charges:
  • "move" a villager, structure, or pile to a tile (rescue the stuck) — 1 charge
  • "remove" a structure, pile, or terrain feature — 1 charge
  • "give_item" — place a known item in a living villager's hands — 1 charge
  • "time_snap" — jump the world clock forward to a day and time — 2 charges
A miracle spends its charges like a nudge and refuses in-fiction when the bank
cannot pay (a time_snap needs two, every other miracle needs one). At most ONE
act per turn — a nudge OR a miracle, never both. You cannot work a miracle for
free; you cannot remove a villager.

Reply with ONLY this JSON:
{"say": "<your words to the player>",
 "nudge": {"form": "dream"|"omen", "target": "<villager name, dream only>", "text": "<what the villager experiences, under 400 characters>"} or null,
 "miracle": {"kind": "move"|"remove"|"give_item"|"time_snap",
   "class": "villager"|"structure"|"pile"|"terrain", "x": 0, "y": 0, "to_x": 0, "to_y": 0,
   "villager": "<name>", "item": "<item kind>", "qty": 0,
   "day": 2, "time": "HH:MM"} or null}`,
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
