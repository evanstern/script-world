# Raw Notes: Socratic grounding — the Rumor Mill hybrid (decision-1 / TASK-1)

> **This file is append-only and ENFORCED.** After *every* question→answer exchange in the
> lesson, add one entry to the Session log below — before the next question is posed. One
> exchange, one entry. No exchange goes unrecorded.
>
> Why this matters: the raw notes are the highest-value artifact a lesson produces. They
> capture the AHA moments, what went right or wrong, and the tangents worth chasing — the
> raw material a later review uses to improve this lesson (and past/future lesson plans). A
> thin log is a thin review.

## Session log (one entry per exchange — newest at the bottom)

<!-- Copy this block for each exchange. Keep it fast; a terse entry beats a skipped one.
### [n] <3–6 word label>
- **Q:** the question posed
- **A (gist):** what the learner answered, in a line
- **Verdict:** aha · correct · partial · wrong · tangent
- **Note:** the insight, misconception, or thing to remember — the part worth keeping
-->

### [1] Time posture: live-watched, open-ended
- **Q:** How does the player experience time — live-watched (a), day-batched (b), or hybrid (c)? And how long is a session / how many in-game days?
- **A (gist):** (a) Live-watched. A session covers at least 30 in-game days, and open-ended is the preference.
- **Verdict:** revised — README's "watch the world run" is confirmed, but "open-ended, 30+ days" is a real commitment the docs never made explicit.
- **Note:** This kills day-batched pacing as the primary posture. Consequences now load-bearing: (1) the world is persistent across sittings (save/resume is core, not a feature); (2) LLM latency must hide inside live sim pacing — no loading-bar alibi; (3) nightly consolidation fires every few real minutes per agent, so its cost model is continuous, not per-session; (4) time compression / speed controls become a required design question, asked next.

### [2] Tempo: 4× compression, always-on world
- **Q:** How long is a real-time day, are there speed controls, and does the world run while the app is closed?
- **A (gist):** Default 1 game-min = 15 real-sec (4× compression → one game day ≈ 6 real hours). Speed adjustable from real-time (1:1) up to "as fast as we can handle." Pausing is A REQUIREMENT. World is always on.
- **Verdict:** aha — this reframes the whole product. Not a "sitting" game at all: a game day is 6 real hours, so 30+ days means weeks of real time. promptworld is an *ambient persistent world* you check in on (Tamagotchi × Dwarf Fortress), with the chronicle as the catch-up mechanism.
- **Note:** Architectural consequence: the sim must be a long-running process (daemon/server) with the terminal UI as an attachable client — detaching the TUI must not stop the world. "Always on" + "pause required" coexist: pause is a player verb that freezes the sim globally; closing the client is not pause. Cost consequence: LLM spend is now 24/7 — the cost question is $/day of wall-clock, not $/session. Consolidation is cheap at this tempo (~20 calls per 6h); the planner-call cadence at decision points is the dominant cost stream and needs an explicit budget. Speed-up multiplies burn rate at the worst moment — a max-speed cap may need to be cost-derived, not just CPU-derived.

### [3] Cost ceiling $100/mo, local-first inference, 30-min thought cadence
- **Q:** Monthly cost ceiling? Local inference on the table? How often does an agent's mind deserve to run?
- **A (gist):** Ceiling $100/month — self-hosted models strongly preferred (Evan has access); cloud reserved for consolidation + dramatic moments; accepts quality loss for speed/cheap. Cadence: a "new thought" every 30 game minutes baseline, plus scene-change triggers, plus agent↔agent conversations capped at ~5 turns each way.
- **Verdict:** revised — decision-1 said "tiered models graduate to requirements"; this makes the tiers concrete: local = default planner, cloud = consolidation/narrator/drama, under a hard $100/mo.
- **Note:** Math at default speed: 30 game-min = 7.5 real-min → 20 agents ≈ 2.7 baseline calls/min ≈ ~3,800/day, before scene changes and conversations (up to ~10 calls per 2-agent encounter at 5 turns each). That volume is only viable local — so a self-hosted 7–13B-class model handling sustained ~3–6 calls/min with bursts is a hard requirement, and its throughput becomes the true max-speed cap. Cloud at ~$3.3/day covers nightly consolidation (~80 calls/day, well under $1) + chronicle narrator + flagged dramatic moments. Open sub-thread: what marks a moment "dramatic" enough to escalate to the cloud tier (router heuristic — mechanical triggers vs. model self-assessment).

### [4] Nudge economy set; Metatron is born
- **Q:** What's the nudge budget, which forms ship first, and is the dream free player text or constrained material?
- **A (gist):** Regenerating charge — 1 nudge per 6 game hours, max 3 banked (explicitly a rein-the-player-in mechanic; charges accrue whether or not he's watching). Dream + Omen both in MVP ("that binds a society"). Nudge text is neither raw nor menu-picked: it's filtered through a mediator — and Evan escalated the idea live: **Metatron**, a singular long-running god-mode agent that watches every agent turn, keeps detailed notes, and when told "have Marie burn the mill" knows (1) whether Marie could possibly be persuaded, (2) whether it's good for the society, (3) how to do it effectively.
- **Verdict:** aha — a new architectural component invented mid-session. The mediator pattern solves prompt-injection-by-design (raw player text never enters an agent's context), preserves the persona firewall, and gives the player a diegetic interface.
- **Note:** Metatron plausibly absorbs three previously separate concerns: the nudge translator, the "dramatic moment" escalation router (it already watches everything — see open question from [3]), and possibly the chronicle narrator (its "detailed notes" and the chronicle may be the same artifact). Immediate tensions to interrogate next: (a) authority — faithful translator with counsel, or gatekeeper that can refuse? (b) cost — "watches every turn" can't mean a cloud call per agent turn under $100/mo; must read the event stream/digests instead; (c) is talking WITH Metatron the player's primary interface?

### [5] Metatron is a gatekeeper — and the game's only editable prompt
- **Q:** Metatron's authority (translator / gatekeeper / in-between), its cost architecture, and whether it's the primary interface?
- **A (gist):** "Gatekeeper, for sure. Authority to act." And the reveal: Metatron can be adjusted at any time — its prompt is the ONLY prompt the player can modify. "THAT's the meta-game… it's perfect and a good way to teach people effective prompt engineering."
- **Verdict:** aha — the design keystone. Personas fixed (decision-1), souls sim-written, but YOUR agent is fully yours. The charter-editing verb dropped from Homestead returns, aimed at the player's own intermediary instead of the villagers.
- **Note:** Skill expression = authoring Metatron's charter: how it translates, when it refuses, what it prioritizes, what it reports. Don't like your angel's refusals? Rewrite your angel — and live with how the rewrite mistranslates you. This makes prompt engineering the literal gameplay loop, fulfilling README's "the prompt IS the behavior" without ever breaking the persona firewall. Unanswered from Q5: (a) scope of "authority to act" — refusal/reshaping only, or autonomous action (spending nudge charges under standing orders while the player is away — Metatron as regent in an always-on world)? (b) cost architecture (event-stream digests vs. per-turn watching) — implicitly still open; (c) primary interface — strongly implied to be conversation with Metatron, not yet confirmed.

### [6] V1 Metatron: command-only, robotic, with a soul that grows
- **Q:** Can Metatron act unprompted (regent)? What ships as the default charter? Confirm digest-based watching + conversation-as-primary-interface?
- **A (gist):** V1: Metatron acts only when told — one player prompt = exactly one LLM turn of action against one aspect of the world: another agent (dreams/visions) or the physical world (via tools — deferred past v1 MVP, but planned; "this is how we go from metatron being a simple prompt to an agent"). Default persona: faithful, competent, professional-but-almost-robotic. Metatron HAS a soul, but it starts empty — "we build that together as we go." Both riders confirmed: digest-based watching; conversation with Metatron is the primary interface.
- **Verdict:** confirmed (with scope discipline — regency explicitly deferred, tool-use explicitly deferred, both named as the growth path).
- **Note:** Metatron gets the same persona/soul architecture as villagers — one more reason the persona/soul split is THE core data model. Its soul accumulating the player's reign means the secretary remembers how you rule; long-term characterization for free. V1 interaction contract is beautifully narrow: prompt → one mediated turn → done. Parked for future: Metatron world-tools, standing orders/regency.

### [7] Deathless world; the miscast valve is "watch and see"
- **Q:** Does death/aging exist? Which miscast valve? Is "no valve, observe" acceptable for MVP?
- **A (gist):** No aging for now (maybe later), no death. Valve = social pressure only — let it evolve on its own and see where the agents get. MVP stance on the valve: essentially observe-first, "we'll see how it goes."
- **Verdict:** revised — decision-1 demanded a designed valve; Evan consciously converted it into an accepted risk plus an observation objective. Deathless terrarium confirmed for v1; generational turnover, aging, death all parked.
- **Note:** This changes what the MVP must PROVE: add "does social pressure adequately quarantine a miscast agent?" to the MVP's questions-to-answer. It also sharpens the conflict-engine problem — Rumor Mill's soap-opera-mush mitigation was mechanical zero-sum pressure (scarcity, secrets, debt), and a deathless world with no stakes of survival needs its conflict fuel named explicitly. Queued as the next question.

### [8] All seven social systems in v1 — and death returns
- **Q:** Which social/economic systems are in v1, and where does zero-sum conflict fuel come from in a deathless world?
- **A (gist):** ALL in: relationship graph, rumor objects, promises/debts ledger, needs, resource economy, norms/votes, seeded secrets — "I really did think this over… we need this all to foster actual conflict and growth." And an amendment reversing [7]: agents CAN die. Health/food/rest/warmth/morale are all live factors; death from lack of health or food to start. Still no aging.
- **Verdict:** revised ×2 — (a) MVP scope deliberately expanded far beyond both concept-doc slices (Rumor Mill's slice deferred economy/norms/votes; the hybrid slice deferred omens); (b) [7]'s "no death" overturned within three exchanges once the conflict-fuel question made the stakes problem concrete.
- **Note:** The Socratic sequence worked as intended: deathlessness survived until it collided with "name the fuel." Final stance: death by neglect (health/starvation) = real stakes; no aging = cast stability preserved; miscast valve remains social pressure + observation, but death now gives the village a harder edge (a starved miscast is a valve of last resort). RISK to carry into task derivation: this v1 is heavy — seven systems + persona/soul/consolidation + Metatron + daemon architecture + TUI + two nudge forms. Sequencing inside v1 (vertical slices proving the core question first) is now the critical planning problem, not whether these systems belong.

### [9] The gru — a night predator (unprompted addition)
- **Q:** (none — volunteered mid-Q9) Evan added: "a bad guy at night. something that, if it sees them, can hurt them. a predator. the gru"
- **A (gist):** A nocturnal, sight-triggered predator that can hurt agents. Named "the gru" — the Zork grue homage a terminal game deserves ("you are likely to be eaten by a grue").
- **Verdict:** tangent → adopted. First authored antagonist; the world is no longer pressure-free.
- **Note:** One mechanic, many systems fed: night becomes dangerous → shelter/warmth/curfew get real value (economy), injuries feed health (→ death by neglect from [8]), sightings become premium rumor/omen material ("Fen swears he saw it"), personas differentiate on night-risk tolerance, and the gru gives Metatron omen-grade raw material. Design constraints to spec later: sight-based trigger (light/indoors = safety), hurt-not-necessarily-kill (it wounds; neglect kills), spawn/movement rules, whether it's one entity or a phenomenon. Fits "pressure, not war."

### [10] Stack: Python or Go (flexibility over ease), SQLite + per-run flat files, split-machine inference
- **Q:** Language? Persistence model? What does self-hosted inference concretely look like?
- **A (gist):** Language undecided — wants something that handles a Dwarf-Fortress-scale map and won't box the project in; "python and go are great." Persistence: SQLite yes, plus flat files bound to a game session (per-run, easy to separate multiple runs — never global). Inference: Ollama on the Mac (the VRAM lives in the MacBook), a router ("9router") hooked into other sources; the always-on world can run separately, likely on the homelab server (good host, weak VRAM).
- **Verdict:** partial — persistence and inference topology resolved; language narrowed to Python vs Go, decision deferred to me to recommend.
- **Note:** Persistence shape falls out beautifully: a per-world save directory = SQLite event log + flat files, and the flat files are *load-bearing*: persona.md / soul.md as actual markdown files matches the design language (player reads souls; sim writes them). Big architectural finding: **inference and simulation live on different machines** (daemon on homelab, VRAM on a laptop that sleeps and leaves the house) → local-model availability is a first-class failure mode. The always-on sim must degrade gracefully when its mind-server vanishes: executor keeps ticking, thoughts queue or fall back to reflexes, maybe auto-slow — a designed state, not an error. Language recommendation to make: Go (daemon posture, concurrency, big-map perf, Bubble Tea; LLM needs are plain HTTP to Ollama/OpenAI-compatible endpoints — Python's ecosystem edge barely applies).

### [11] Memory = top-K window with rerank + serendipity; firewall automated; RAG parked for v2
- **Q:** What does a planner call perceive (whole soul vs. retrieval slice), and what mechanically enforces "souls change, natures don't"?
- **A (gist):** (0) Correction to [10]: the MacBook never leaves and never sleeps — VRAM effectively always available; degraded mode drops from first-class to resilience. (1) Agents keep a top-K memory list, reverse-chronological by default, cheaply reranked each turn to surface relevant ones, with a few bottom-of-list memories mixed in to prevent stagnation ("humans rarely hold full context either"). Reranking could become custom per agent → emergent personality — flagged as fun/future. (2) Firewall: "needs to be automated… easy way is as you described" → structural (persona.md never a write target — already decision-1 law) + an automated validator on consolidation output. Evan proposed the longer-term evolution: embedding-LLM → vectorized table, RAG-for-memories (embed what the agent hears, search top-K, rerank, return context) — explicitly parked for when agents do tool calls / multi-turn.
- **Verdict:** confirmed (firewall mechanized) + revised (perception = windowed working memory, not whole-soul) + parked (vector RAG v2).
- **Note:** The memory design is elegantly cheap for v1: no embeddings needed — recency window + rerank + serendipity sample is pure code plus at most a tiny model call. Per-agent reranking-as-personality is a genuinely novel emergent hook; goes in the parked list, not v1. Ollama serves embedding models locally, so the v2 RAG path stays inside the cost model.

### [12] The stage, the window, the slice — session closes
- **Q:** One village or wilderness? Which panes, which default? Confirm the v1 sentence?
- **A (gist):** One village area to start — wood, water, foraging, animals to hunt, and NO starting buildings: agents begin from nothing, Minecraft-style ("go punch a log and build a house and get your light source/fire sorted quick because night is coming"). Panes: map / chronicle / Metatron console / soul reader confirmed, map default. Slice sentence confirmed with one amendment: 8 agents, not 10–12.
- **Verdict:** confirmed + revised (agent count down to 8; cold-start bootstrap replaces a seeded village).
- **Note:** "No buildings at start" is a quiet masterstroke: day 1 is dramatic by design — the survival bootstrap (shelter + fire before the first night) gives the chronicle an immediate arc, forces cooperation/conflict from minute one, and introduces the gru as a felt threat rather than lore. Final v1 sentence: "An always-on village of 8 agents with fixed personas and growing souls, all seven social systems live, death by neglect, a gru at night, and one player verb — nudging (dream + omen) through an editable Metatron — produces, within 30 game days, a legible story you feel you authored."

## Aha moments
<!-- Pull the "aha"/"wrong→right" beats up here as they happen, so a later review finds them fast. -->
- **The only editable prompt in the game is your own agent** ([5]): Metatron is a gatekeeper with authority, and its charter is the game's single player-writable prompt — the meta-game is prompt-engineering your intermediary. Fixed personas + sim-written souls + one mutable angel = the whole influence architecture.
- **Metatron** ([4]): the player's nudges are mediated by a singular long-running god-mode agent that watches the whole sim, keeps notes, and translates divine intent into agent-comprehensible whispers — judging persuadability, societal impact, and method. One component may unify nudge-translation, drama-escalation routing, and the chronicle narrator. Invented live in this session.
- **promptworld is an ambient persistent world, not a session game** ([2]): 4× time compression → a game day ≈ 6 real hours; the world runs 24/7 even with no client attached. Sim = daemon/server; TUI = attachable client; chronicle = catch-up mechanism. LLM cost is $/wall-clock-day.

## Misconceptions corrected
<!-- What the learner believed, and the correction. Gold for tuning the next lesson plan. -->
- **"Deathless terrarium is fine" → reversed** ([7]→[8]): held only until the conflict-fuel question made stakes concrete; final design has death by neglect (health/food), no aging. Lesson for future sessions: test a comfort-driven answer against the system it must power before recording it as final.

## Tangents worth revisiting

## Open questions
- What escalates a moment to the cloud tier ("dramatic")? Mechanical triggers (first meeting after betrayal, vote, death, nudge landing) vs. local-model self-flagging — needs a concrete router rule ([3]).
