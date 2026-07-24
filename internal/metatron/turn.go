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
// 014 T021; re-pointed at send_vision when spec 029 retired nudge_dream):
// tool.Lookup("send_vision").Cost.TextCapBytes (400). It matches the sim
// reducer's NudgeTextMax enforcer — both derive from the same registry entry, so
// the metatron-side truncation and the door-side enforcement can never diverge.
var nudgeTextMax = func() int {
	t, _ := tool.Lookup("send_vision")
	return t.Cost.TextCapBytes
}()

// One console turn: player text in, charter-voiced reply out, at most one
// mediated nudge. The player's words have exactly one sink — the user turn
// of this prompt; villagers can only ever receive the model's `nudge.text`
// rendering, validated and landed through the InjectSocial door.

const playerTextMax = 2000

// ErrTurnBusy is returned while another console turn is in flight.
var ErrTurnBusy = errors.New("the angel is attending another matter")

// TurnResult is the console-facing outcome of one turn.
type TurnResult struct {
	Reply     string       `json:"reply"`
	Nudge     *Nudge       `json:"nudge,omitempty"`
	Miracle   *Miracle     `json:"miracle,omitempty"`
	Order     *OrderReport `json:"order,omitempty"`     // a placed standing order (spec 029 US2)
	Cancelled []string     `json:"cancelled,omitempty"` // released order ids (cancel_order)
	Clock     string       `json:"clock,omitempty"`     // a landed meta act's human line (spec 029 US5)
	Charges   int          `json:"charges"`
	Moments   []string     `json:"moments,omitempty"`
}

// OrderReport is the console-facing summary of a placed standing order (spec 029
// US2) — the id the player can name to cancel it, and its condition. Additive and
// omitempty; existing IPC clients ignore it.
type OrderReport struct {
	ID        string `json:"id"`
	Condition string `json:"condition"`
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

// turnOrigin distinguishes a console turn from a system-authored (triggered) turn
// (spec 029 US3, R6). Both run the SAME body (runTurn): same single-flight guard,
// same roster/handler/gate composition, same telemetry, same transcript append —
// only the framing differs. seed is the trailing directive: the player's words on
// the console path, the order's pre-authorized action instruction on the system
// path (which has NO player-text sink — the seed is the angel's own recorded
// instruction). jobPrefix threads the correlation id ("turn" | "watch"); system
// marks the transcript with a [watch] origin and suppresses moment consumption.
type turnOrigin struct {
	system    bool
	jobPrefix string
	seed      string
}

// Turn runs one mediated CONSOLE turn through the bounded tool-use loop (spec 017
// T020, spec 029 T012). The model may reply with words (converse — the
// transcript-only final-answer channel, Result.Final) or call exactly one acting
// tool (send_vision / send_omen / monitor_and_act / cancel_order / work_miracle),
// which lands through its door. The driver's cardinality enforces "at most one
// mediated act per turn". Serialized: a second concurrent call fails fast with
// ErrTurnBusy (the console never waits — triggered system turns do, via
// runSystemTurn's bounded acquisition, T013).
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
	return mt.runTurn(ctx, turnOrigin{jobPrefix: "turn", seed: playerText})
}

// runTurn is the shared turn body for the console and system-authored paths (spec
// 029 T012/R6). The CALLER owns turnBusy (Turn CAS-fails fast; runSystemTurn waits
// bounded) — runTurn assumes it is held. It composes the instruction surface, the
// user prompt (framing the directive per origin), drives the loop, lands the
// cog.tool_call + retry telemetry on every termination path, and records the
// transcript. Moment consumption is CONSOLE-ONLY: a system turn never drains the
// player-facing moment queue (those await the next console open); the trigger
// worker queues the system turn's OWN moment from the returned outcome (T013).
func (mt *Metatron) runTurn(ctx context.Context, o turnOrigin) (TurnResult, error) {
	// The player-editable instruction surface, all read fresh this turn (FR-001):
	// the charter, the skill files composed beneath it, and (US2) the capability
	// manifest. Every fallback/truncation/skip becomes a notice prefixed to the
	// reply, one combined line, exactly like the charter's today.
	charter, charterNotice := loadCharter(mt.worldDir)
	skills, skillNotices := loadSkills(mt.worldDir)
	grant, manifestNotices := loadManifest(mt.worldDir)
	var notices []string
	if charterNotice != "" {
		notices = append(notices, charterNotice)
	}
	notices = append(notices, skillNotices...)
	notices = append(notices, manifestNotices...)
	// The ONE granted roster for this turn — the manifest-filtered metatron loop
	// roster (work_miracle's kind enum narrowed when restricted). It feeds all
	// three gating layers alike: Job.Roster (declaration), the derived guidance
	// (prose), and the handler set built from it (door), so an ungranted tool or
	// kind is structurally absent from every one of them (FR-005).
	roster := grantedRoster(grant)
	mt.stateMu.Lock()
	charges := mt.charges
	tick := mt.clockAt
	night := mt.night
	alive := make(map[int]bool, len(mt.alive))
	for k, v := range mt.alive {
		alive[k] = v
	}
	moments := append([]string(nil), mt.moments...)
	story := append([]string(nil), mt.story...)
	orders := append([]sim.MetatronOrder(nil), mt.orders...)
	mt.stateMu.Unlock()

	// One correlation id per turn, mirroring mind's "<class>-<agent>-<tick>"
	// convention (telemetry.go newMeta): the console turn's class is "turn"; a
	// triggered system turn's is "watch" (R6). Threads every cog.tool_call.
	jobID := fmt.Sprintf("%s-metatron-%d", o.jobPrefix, tick)

	// The trailing directive: the player's words (console) or the order's
	// pre-authorized action (system). A system turn has no player-text sink — the
	// seed is the angel's OWN recorded instruction, so recording it is safe.
	directive := "The player says:\n" + o.seed
	if o.system {
		directive = "A standing order you placed has come due. Carry out its " +
			"pre-authorized action now, in a single act if it calls for one:\n" + o.seed
	}

	result := TurnResult{}
	d := &turnDispatch{mt: mt, charges: charges, alive: alive, night: night, tick: tick, result: &result, grant: grant}

	callCtx, cancel := context.WithTimeout(ctx, turnTimeout)
	res, err := mt.runLoop(callCtx, toolloop.Job{
		JobID:     jobID,
		Kind:      llm.KindMetatron,
		System:    turnSystemPrompt(charter, skills, roster),
		Seed:      turnUserPrompt(tick, charges, alive, orders, moments, story, mt.soulTail(), mt.transcriptTail(), directive),
		Roster:    roster,
		Handlers:  mt.turnHandlers(d),
		MaxRounds: mt.loopRounds,
		MaxTokens: mt.turnTokens, // llm.json max_tokens.metatron_turn (spec 025 US2), default 1024
		Record:    d.record,
	})
	cancel()

	// Land every buffered CallRecord as cog.tool_call telemetry (spec 017 FR-007,
	// T018), on EVERY termination path — a rejected / never-grounded call is
	// recorded even when nothing landed. A dedicated batch through the same
	// InjectSocial door as the nudge/miracle grounding events, so it neither
	// reorders nor entangles with them.
	mt.emitToolCalls(d.records, tick)

	// Transport retry visibility (spec 025 FR-004/SC-003): when the loop consumed
	// its one in-loop retry (recovered OR twice-failed), surface it as a
	// non-terminal cog.outcome through the same door — emitted BEFORE the
	// error-return below so a twice-failed turn's retry is still countable.
	if res.Retried {
		mt.emitRetried(jobID, tick, res.RetryReason)
	}

	if err != nil {
		// Honest unavailability; nothing landed (a landing returns a nil error),
		// nothing consumed, moments stay queued — exactly today's degraded path.
		// A system turn's caller (the trigger worker) maps this to an honest
		// model-free moment (T014).
		return TurnResult{}, err
	}

	// The reply is the model's closing/converse text (Result.Final). When the
	// model landed an act with no accompanying prose, Final may be empty — the
	// ⚡/✨/👁 report lines carry the turn. When NOTHING landed and nothing was
	// said, the loop ran dry (model_done with no text, cap exhaustion, or a soft
	// error) — the old scattered-thoughts fallback maps onto exactly these.
	reply := strings.TrimSpace(res.Final)
	if reply == "" && result.Nudge == nil && result.Miracle == nil && result.Order == nil && len(result.Cancelled) == 0 && result.Clock == "" {
		reply = "Forgive me — my thoughts scattered and I could not complete that. " +
			"Nothing was done and nothing was spent. Ask again."
	}
	result.Reply = reply
	if len(notices) > 0 {
		result.Reply = "(" + strings.Join(notices, "; ") + ")\n\n" + result.Reply
	}

	// Surfaced moments are consumed only on a completed CONSOLE turn — a system
	// (triggered) turn leaves the player-facing queue intact for the next console
	// open (R6), and reports no moments of its own here.
	if !o.system {
		mt.stateMu.Lock()
		result.Moments = moments
		mt.moments = mt.moments[len(moments):]
		result.Charges = mt.charges
		mt.stateMu.Unlock()
	} else {
		mt.stateMu.Lock()
		result.Charges = mt.charges
		mt.stateMu.Unlock()
	}

	mt.recordTurn(tick, o, result)
	return result, nil
}

// landVision validates a vision and lands it as one atomic batch (spec 029 US1,
// T005). A vision reaches exactly one living villager at ANY hour and costs one
// charge; target is the villager's name, text the model's rendering. The
// validation/batch/soul-append tail is UNCHANGED from the pre-029 landNudge (spec
// 029 T005: wrap, don't rewrite) — only the target resolution and the send_vision
// grant gate are vision-specific. Returns the landed nudge, or (nil, in-fiction
// refusal) which the handler maps to a rejected_gate the model may repair within
// the loop's round cap.
func (mt *Metatron) landVision(target, text string, charges int, alive map[int]bool, grant grantSet) (*Nudge, string) {
	if charges <= 0 {
		return nil, "no charges are banked"
	}
	// Capability gate (spec 021 R5.3, door layer): defense-in-depth behind the
	// handler-absence gate — a tool whose handler was never installed cannot reach
	// here, but the check keeps the door authoritative on its own.
	if !grant.allows("send_vision") {
		return nil, "that power is not granted in this world"
	}
	idx := agentIndexByName(target)
	if idx < 0 {
		return nil, fmt.Sprintf("no villager named %q", target)
	}
	if !alive[idx] {
		return nil, fmt.Sprintf("%s is beyond reach now", sim.AgentNames[idx])
	}
	return mt.landNudgeBatch("vision", []int{idx}, text)
}

// landOmen validates an omen and either lands it now or defers it to nightfall
// (spec 029 US1/US4, T005/T016). An omen reaches one villager, a named group, or
// everyone living — at NIGHT only — for one charge regardless of recipient count.
// targetsArg is send_omen's comma-separated living-villager name list or the word
// "everyone".
//
// Night path: land immediately, spending a charge (the reducer's night gate is
// the door authority; the mirrored night flag is the turn-side pre-check).
//
// Day path (T016/R11): an omen belongs to the dark, so a daytime call does NOT
// refuse — it places a system-origin standing order that re-sends the omen the
// instant night falls (event_types ["sim.night_started"], TTL 1 game day,
// cap-exempt). Placement is FREE: the charge is spent at trigger-time landing,
// never here (FR-012/SC-004). Returns one of: (nudge, nil, "") landed at night;
// (nil, order, "") deferred to nightfall; (nil, nil, why) an in-fiction refusal.
func (mt *Metatron) landOmen(targetsArg, text string, charges int, night bool, tick int64, alive map[int]bool, grant grantSet) (*Nudge, *sim.MetatronOrder, string) {
	if !grant.allows("send_omen") {
		return nil, nil, "that power is not granted in this world"
	}
	targets, why := resolveOmenTargets(targetsArg, alive)
	if why != "" {
		return nil, nil, why
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil, "the rendering was empty"
	}
	if !night {
		order, why := mt.deferOmen(targetsArg, targets, strings.TrimSpace(text), tick, grant)
		return nil, order, why
	}
	if charges <= 0 {
		return nil, nil, "no charges are banked"
	}
	nudge, why := mt.landNudgeBatch("omen", targets, text)
	return nudge, nil, why
}

// deferOmen places the daytime omen's nightfall deferral order (spec 029 T016/
// R11): a system-origin standing order whose one-shot trigger re-runs send_omen
// at night. The action is the seed the night SYSTEM turn reads (runTurn frames it
// as a due standing order), so it must lead the angel to send_omen with these
// targets and this text; terse framing keeps it within the reducer's 400-rune
// action cap for all but the very longest renderings. "everyone" is preserved as
// the target word so the night turn re-resolves against whoever lives THEN; a
// named list re-sends to those still living. The charge is spent when the night
// turn lands, not here — placement is free and cap-exempt (origin "system"). A
// rejected placement maps to omen-appropriate counsel the model may repair.
func (mt *Metatron) deferOmen(targetsArg string, targets []int, text string, tick int64, grant grantSet) (*sim.MetatronOrder, string) {
	who := "everyone"
	if !strings.EqualFold(strings.TrimSpace(targetsArg), "everyone") {
		names := make([]string, len(targets))
		for i, t := range targets {
			names[i] = sim.AgentNames[t]
		}
		who = strings.Join(names, ", ")
	}
	a := orderArgs{
		Condition:  fmt.Sprintf("nightfall — an omen awaits %s", who),
		Action:     fmt.Sprintf("Night has fallen. Send the omen you promised to %s: %s", who, text),
		EventTypes: []string{"sim.night_started"},
		TTLDays:    1,
	}
	order, why := mt.placeOrder("system", a, tick, grant)
	if why != "" {
		return nil, "an omen belongs to the night, and I could not set it aside — " + why
	}
	return order, ""
}

// resolveOmenTargets parses send_omen's `targets` argument (spec 029 R3): a
// comma-separated list of living villager names, or the single word "everyone",
// into a deduplicated set of living villager indices. Every named villager must
// be alive — an unknown or dead name refuses the WHOLE act with counsel (one act,
// one charge, one atomic batch; never a partial omen). "everyone" resolves to the
// living set in index order.
func resolveOmenTargets(arg string, alive map[int]bool) ([]int, string) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return nil, `name the villagers the omen should reach, or say "everyone"`
	}
	if strings.EqualFold(arg, "everyone") {
		var targets []int
		for i := range sim.AgentNames {
			if alive[i] {
				targets = append(targets, i)
			}
		}
		if len(targets) == 0 {
			return nil, "no living villager remains to witness it"
		}
		return targets, ""
	}
	seen := map[int]bool{}
	var targets []int
	for _, part := range strings.Split(arg, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		idx := agentIndexByName(name)
		if idx < 0 {
			return nil, fmt.Sprintf("no villager named %q", name)
		}
		if !alive[idx] {
			return nil, fmt.Sprintf("%s is beyond reach now", sim.AgentNames[idx])
		}
		if !seen[idx] {
			seen[idx] = true
			targets = append(targets, idx)
		}
	}
	if len(targets) == 0 {
		return nil, "name at least one living villager for the omen"
	}
	return targets, ""
}

// landNudgeBatch is the shared landing tail for landVision / landOmen (spec 029
// T005): the text cap, the ONE atomic InjectSocial batch (metatron.nudged + one
// prefixed agent.memory_added per target at SalDream), and the soul append —
// VERBATIM the pre-029 landNudge body (wrap, don't rewrite). form fixes the memory
// prefix and the recorded form; the reducer dry-run is the door authority (charge
// spend, night gate for omen, living targets). Returns the landed nudge, or (nil,
// refusal) the handler maps to a rejected_gate.
func (mt *Metatron) landNudgeBatch(form string, targets []int, text string) (*Nudge, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, "the rendering was empty"
	}
	if len(text) > nudgeTextMax {
		text = text[:nudgeTextMax]
	}
	prefix := "You saw a vision: "
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
func (mt *Metatron) landMiracle(mm miracleArgs, charges int, grant grantSet) (*Miracle, string) {
	if charges <= 0 {
		return nil, "no charges are banked"
	}
	kind := strings.ToLower(strings.TrimSpace(mm.Kind))
	// Capability gate (spec 021 R5.3, door layer): work_miracle must be granted
	// and this kind offered by the world. Defense-in-depth behind handler-absence
	// (ungranted work_miracle installs no handler) and the declared kind enum
	// (ungranted kinds are never declared) — the door refuses in-fiction even if
	// a prompt-injected model conjures a call for an ungranted kind.
	if !grant.allows("work_miracle") || !grant.allowsKind(kind) {
		return nil, "that miracle is not granted in this world"
	}

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

// recordTurn appends the exchange to the transcript. A console turn opens with
// the player's line ("> …"); a system-authored turn opens with a "[watch]" origin
// marker over the order's pre-authorized action (spec 029 T012/R6) — never a
// player-text line, because a triggered turn has no player text.
func (mt *Metatron) recordTurn(tick int64, o turnOrigin, r TurnResult) {
	var b strings.Builder
	if o.system {
		fmt.Fprintf(&b, "\n[%s] [watch]\n%s\n\nmetatron: %s\n", clock.Format(tick), o.seed, r.Reply)
	} else {
		fmt.Fprintf(&b, "\n[%s]\n> %s\n\nmetatron: %s\n", clock.Format(tick), o.seed, r.Reply)
	}
	if r.Nudge != nil {
		fmt.Fprintf(&b, "⚡ %s → %s: %q\n", r.Nudge.Form, strings.Join(r.Nudge.Targets, ", "), r.Nudge.Text)
	}
	if r.Miracle != nil {
		fmt.Fprintf(&b, "✨ miracle: %s\n", r.Miracle.Summary)
	}
	if r.Order != nil {
		fmt.Fprintf(&b, "👁 watch set (%s): %q\n", r.Order.ID, r.Order.Condition)
	}
	for _, id := range r.Cancelled {
		fmt.Fprintf(&b, "👁 watch released: %s\n", id)
	}
	if r.Clock != "" {
		fmt.Fprintf(&b, "⏲ %s\n", r.Clock)
	}
	mt.appendFile(mt.transcriptPath(), b.String())
}

// Status is the model-free peek: charges, charter provenance, soul tail, and
// (spec 021 R8, US3) the instruction-file + capability provenance a player reads
// to answer "what is my angel running on, and what can it do". The new fields
// are additive and omitempty where sensible, so existing IPC clients ignore them
// (encoding/json) — no protocol version bump (contracts/status.md).
type Status struct {
	Charges         int           `json:"charges"`
	CharterDefault  bool          `json:"charter_default"`
	SoulTail        string        `json:"soul_tail"`
	Skills          []string      `json:"skills,omitempty"`        // effective skill filenames, composition order
	GrantedTools    []string      `json:"granted_tools,omitempty"` // granted roster, registry order; work_miracle(kind,…) when restricted
	ManifestDefault bool          `json:"manifest_default"`        // true ⇒ no capabilities.json (full default grant)
	Orders          []OrderStatus `json:"orders,omitempty"`        // standing orders (spec 029 US2, FR-016) — active + recent
}

// OrderStatus is the model-free peek at one standing order (spec 029 US2/US3,
// data-model §6): what the player reads to answer "what watches stand, and how
// long". Additive and omitempty — existing IPC clients ignore it (the spec-021
// precedent).
type OrderStatus struct {
	ID         string `json:"id"`
	Condition  string `json:"condition"`
	Origin     string `json:"origin"`
	Fuzzy      bool   `json:"fuzzy,omitempty"`
	ExpiresDay int64  `json:"expires_day"`
	Status     string `json:"status"`
}

// Status is computed fresh per call from disk (same per-read discipline as the
// turn, FR-001): a skill added or a manifest edited between peeks shows on the
// next read with no cache to go stale.
func (mt *Metatron) Status() Status {
	mt.stateMu.Lock()
	c := mt.charges
	orders := mt.orderStatuses()
	mt.stateMu.Unlock()
	grant, _ := loadManifest(mt.worldDir)
	return Status{
		Charges:         c,
		CharterDefault:  charterIsDefault(mt.worldDir),
		SoulTail:        mt.soulTail(),
		Skills:          skillNames(mt.worldDir),
		GrantedTools:    grantedToolLabels(grant),
		ManifestDefault: grant.manifestDefault,
		Orders:          orders,
	}
}

// orderStatuses projects the mirrored standing orders into the model-free status
// surface (spec 029 FR-016). Caller holds stateMu. nil when no orders stand (the
// field omits under omitempty — byte-compatible with pre-029 status).
func (mt *Metatron) orderStatuses() []OrderStatus {
	if len(mt.orders) == 0 {
		return nil
	}
	out := make([]OrderStatus, 0, len(mt.orders))
	for i := range mt.orders {
		o := mt.orders[i]
		out = append(out, OrderStatus{
			ID:         o.ID,
			Condition:  o.Condition,
			Origin:     o.Origin,
			Fuzzy:      o.Confirm,
			ExpiresDay: o.ExpiresTick / ticksPerGameDay,
			Status:     o.Status,
		})
	}
	return out
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

// metatronInitiativeFrame pins meta control and standing orders to player
// authority (spec 029 T019, contracts/tools.md): the clock-control tools and any
// standing order may be used ONLY when the player asks for them, or when a
// standing order the player already placed authorizes the act — never on the
// angel's own initiative. Like metatronNonNegotiables it is a compile-time
// constant appended last (INV-1), so no charter or skill byte can override it,
// and it appears on every path (the door-side grant gate backs it independently).
const metatronInitiativeFrame = `Two more powers are the player's to command, never yours to take up alone: the ` +
	`world's clock (pausing, starting, changing its pace) and standing orders (watches you keep and act on). ` +
	`Use them only when the player asks, or when a standing order the player placed tells you to act — never on your own initiative.`

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
		"You are the intermediary between the player and the village of eight: %s.\n%s\n%s\n\n",
		strings.Join(sim.AgentNames[:], ", "), metatronNonNegotiables, metatronInitiativeFrame)

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

func turnUserPrompt(tick int64, charges int, alive map[int]bool, orders []sim.MetatronOrder, moments, story []string, soulTail, transcriptTail, playerText string) string {
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
	// Standing orders (spec 029 FR-017): the angel's counsel and confirmations
	// stay truthful to live state only if it carries its active watches — id (so
	// the player can name one to cancel), condition, remaining game-days, and
	// whether the order is fuzzy (needs a confirm) or purely structural.
	writeStandingOrders(&b, tick, orders)
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

// writeStandingOrders renders the active-order block of the turn user prompt (spec
// 029 T010, FR-017). Only ACTIVE orders show (consumed ones are history the status
// surface carries); remaining days floor at 0. A fuzzy order (Confirm) is marked
// so the angel sets honest expectations about the confirm step.
func writeStandingOrders(b *strings.Builder, tick int64, orders []sim.MetatronOrder) {
	var active []sim.MetatronOrder
	for _, o := range orders {
		if o.Status == "active" {
			active = append(active, o)
		}
	}
	if len(active) == 0 {
		return
	}
	b.WriteString("\nStanding orders you keep watch over:\n")
	for _, o := range active {
		days := (o.ExpiresTick - tick) / ticksPerGameDay
		if days < 0 {
			days = 0
		}
		kind := "structural"
		if o.Confirm {
			kind = "fuzzy"
		}
		fmt.Fprintf(b, "- %s: %q (%d day(s) left, %s)\n", o.ID, o.Condition, days, kind)
	}
}
