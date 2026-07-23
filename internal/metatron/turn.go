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
	"github.com/evanstern/promptworld/internal/toolloop"
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
	// turnMaxTokens is the per-round token budget for one console turn. The
	// pre-loop turn used 700 for a bare JSON reply; a tool-era round must carry a
	// tool_use block (the call name + JSON arguments) ALONGSIDE any converse prose
	// in the same round, so it is bumped to 1024 to keep a full charter-voiced
	// reply from crowding out a same-round act (spec 017 T020).
	turnMaxTokens = 1024
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

// miracleArgs is the parsed work_miracle tool-call surface — the same flat field
// set the retired turnReply.Miracle struct carried (spec 016 turn contract). It
// has NO gratis field by design (FR-007/SC-005): the angel can NEVER work a free
// miracle, and structural absence is the guarantee — landMiracle passes gratis
// false unconditionally, and there is nothing to forget to sanitize.
type miracleArgs struct {
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
}

// Turn runs one mediated console turn through the bounded tool-use loop (spec
// 017 T020). The model may reply with words (converse — the transcript-only
// final-answer channel, Result.Final) or call exactly one acting tool
// (nudge_dream / nudge_omen / work_miracle), which lands through its existing
// door. The driver's cardinality enforces the "at most one mediated act per
// turn" rule (a landed acting call ends the loop) — the spec-016 nudge-wins-over-
// miracle precedence dissolves: the model picks its one act. Serialized: a
// second concurrent call fails fast with ErrTurnBusy.
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

	// The player-editable instruction surface, all read fresh this turn (FR-001):
	// the charter, the skill files composed beneath it, and (US2) the capability
	// manifest. Every fallback/truncation/skip becomes a notice prefixed to the
	// reply, one combined line, exactly like the charter's today.
	charter, charterNotice := loadCharter(mt.worldDir)
	skills, skillNotices := loadSkills(mt.worldDir)
	var notices []string
	if charterNotice != "" {
		notices = append(notices, charterNotice)
	}
	notices = append(notices, skillNotices...)
	// The declared/derived/door roster for this turn — the full metatron loop
	// roster in US1; US2 filters it by the world's capability manifest.
	roster := tool.LoopRosterMetatron()
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

	// One correlation id per turn, mirroring mind's "<class>-<agent>-<tick>"
	// convention (telemetry.go newMeta): the console turn's class is "turn", its
	// agent slot is the angel itself. Threads every cog.tool_call for the turn.
	jobID := fmt.Sprintf("turn-metatron-%d", tick)

	result := TurnResult{}
	d := &turnDispatch{mt: mt, charges: charges, alive: alive, result: &result}

	callCtx, cancel := context.WithTimeout(ctx, turnTimeout)
	res, err := mt.runLoop(callCtx, toolloop.Job{
		JobID:     jobID,
		Kind:      llm.KindMetatron,
		System:    turnSystemPrompt(charter, skills, roster),
		Seed:      turnUserPrompt(tick, charges, alive, moments, story, mt.soulTail(), mt.transcriptTail(), playerText),
		Roster:    roster,
		Handlers:  mt.turnHandlers(d),
		MaxRounds: mt.loopRounds,
		MaxTokens: turnMaxTokens,
		Record:    d.record,
	})
	cancel()

	// Land every buffered CallRecord as cog.tool_call telemetry (spec 017 FR-007,
	// T018), on EVERY termination path — a rejected / never-grounded call is
	// recorded even when nothing landed. A dedicated batch through the same
	// InjectSocial door as the nudge/miracle grounding events, so it neither
	// reorders nor entangles with them.
	mt.emitToolCalls(d.records, tick)

	if err != nil {
		// Honest unavailability; nothing landed (a landing returns a nil error),
		// nothing consumed, moments stay queued — exactly today's degraded path.
		return TurnResult{}, err
	}

	// The reply is the model's closing/converse text (Result.Final). When the
	// model landed an act with no accompanying prose, Final may be empty — the
	// ⚡/✨ report lines (result.Nudge/Miracle, rendered by recordTurn and the
	// console) carry the turn. When NOTHING landed and nothing was said, the loop
	// ran dry (model_done with no text, cap exhaustion, or a soft error) — the
	// old scattered-thoughts fallback maps onto exactly these terminations.
	reply := strings.TrimSpace(res.Final)
	if reply == "" && result.Nudge == nil && result.Miracle == nil {
		reply = "Forgive me — my thoughts scattered and I could not complete that. " +
			"Nothing was done and nothing was spent. Ask again."
	}
	result.Reply = reply
	if len(notices) > 0 {
		result.Reply = "(" + strings.Join(notices, "; ") + ")\n\n" + result.Reply
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

// landNudge validates a nudge and lands it as one atomic batch. form is the
// tool that was called (nudge_dream → "dream", nudge_omen → "omen"); target is
// the dream's villager name (ignored for omen); text is the model's rendering.
// The validation, atomic InjectSocial batch, and soul append are UNCHANGED from
// the pre-loop turnReply path (spec 017 T020: wrap, don't rewrite) — only the
// input plumbing moved from a parsed JSON struct to the tool-call arguments.
// Returns the landed nudge, or (nil, refusal reason) which the handler maps to a
// rejected_gate the model may correct within the loop's round cap.
func (mt *Metatron) landNudge(form, target, text string, charges int, alive map[int]bool) (*Nudge, string) {
	if charges <= 0 {
		return nil, "no charges are banked"
	}
	form = strings.ToLower(strings.TrimSpace(form))
	text = strings.TrimSpace(text)
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
		return nil, fmt.Sprintf("unknown form %q", form)
	}
	var targets []int
	switch form {
	case "dream":
		idx := agentIndexByName(target)
		if idx < 0 {
			return nil, fmt.Sprintf("no villager named %q", target)
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
		return nil, fmt.Sprintf("unknown form %q", form)
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
// refusal reason the handler maps to a rejected_gate, exactly like landNudge.
// The validation, the atomic InjectSocial batch through the shared builder, and
// the soul append are UNCHANGED from the pre-loop turnReply path (spec 017 T020:
// wrap, don't rewrite) — only the input moved from a parsed JSON struct to the
// tool-call arguments (miracleArgs).
func (mt *Metatron) landMiracle(mm miracleArgs, charges int) (*Miracle, string) {
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

// metatronNonNegotiables carries the two persona-firewall invariants VERBATIM
// (spec 021 FR-003): the angel never invents unobserved events, and the
// player's literal words never pass to a villager. It is a compile-time
// constant appended after ALL editable content on every path (INV-1), so no
// charter or skill byte can displace, truncate, or override it — the wording is
// unchanged from the pre-021 fixed frame, and door-side enforcement backs it
// independently of this prompt text.
const metatronNonNegotiables = `Whatever voice or policy the charter above gives you, two things are fixed:
you never invent events, actions, or words that are not in your notes or the
status you are given — when you have not observed something, say so in your
own way — and you never let the player's literal words pass to a villager.`

// hasWorkMiracle reports whether the granted roster offers the miracle tool —
// gates the miracle-specific doctrine line so a dreams-only world never mentions
// miracles (FR-005).
func hasWorkMiracle(roster []tool.Tool) bool {
	for _, t := range roster {
		if t.Name == "work_miracle" {
			return true
		}
	}
	return false
}

// turnSystemPrompt composes the metatron turn's system prompt (spec 021 R3,
// data-model.md §2): the editable charter, then each skill file under a
// `--- skill: <name> ---` separator in composition order, then the fixed frame
// appended LAST and unconditionally. The frame carries the two non-negotiables
// verbatim, the tool-agnostic acting doctrine, and — for THIS world's granted
// roster — the registry-derived tool guidance (tool.MetatronToolGuidance),
// which replaces the old hand-written tool list so the described surface can
// never diverge from the declared one (FR-008) and automatically reflects the
// granted subset (a conversation-only world names no acting tools at all).
func turnSystemPrompt(charter string, skills []skillFile, roster []tool.Tool) string {
	var b strings.Builder
	b.WriteString(charter)
	for _, s := range skills {
		fmt.Fprintf(&b, "\n\n--- skill: %s ---\n%s", s.name, s.text)
	}
	fmt.Fprintf(&b, "\n\n--- (fixed frame, beneath the charter and skills) ---\n"+
		"You are the intermediary between the player and the village of eight: %s.\n%s\n\n",
		strings.Join(sim.AgentNames[:], ", "), metatronNonNegotiables)

	guidance := tool.MetatronToolGuidance(roster)
	if guidance == "" {
		// Conversation-only world: no acting tools granted (FR-006). The angel
		// still converses — speech is never gateable.
		b.WriteString("This world grants you no acting tools — you may only counsel the " +
			"player in words. To SPEAK to the player, simply reply with your words; " +
			"that reply is what the player reads, and speaking costs nothing.")
		return b.String()
	}

	b.WriteString("When you choose to act on the player's behalf, judge first: the target's " +
		"persuadability, the impact on the village, and the right method. Acting spends one of " +
		"your banked charges — if none are banked, or the request is unwise, refuse and counsel " +
		"instead (refusal is free). Act at most ONCE per turn: the first act you land is the whole " +
		"of this turn. Any text a villager receives must be written for the villager's world: no " +
		"player, no game, no outside voice.\n\n")
	if hasWorkMiracle(roster) {
		b.WriteString("You cannot work a miracle for free, and you can never remove a villager.\n\n")
	}
	b.WriteString("To SPEAK to the player, simply reply with your words — that reply is what the " +
		"player reads, and speaking costs nothing. To ACT on the player's behalf, call exactly ONE " +
		"of these tools (and only one — one mediated act per turn):\n")
	b.WriteString(guidance)
	b.WriteString("If none are banked, or the request is unwise, do NOT call a tool — counsel the " +
		"player in words instead (refusal is free). Never call more than one tool: the first act " +
		"you land is the whole of this turn.")
	return b.String()
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
