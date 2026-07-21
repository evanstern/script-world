# Pattern: chronicle grammar

How one event becomes one feed entry. Applies wherever the chronicle renders
(dock tab, solo, narrow fallback). Goal: conversations pop out of the scroll at a
glance; everything stays JSON-shaped, never prose; the verbatim payload is always
one keypress away.

## Line format

```
#<seq> <HH:MM>  <type>  <subject>  <payload>
```

- `#seq` + clock time, dim. `type` in the type color. Payload compact
  single-line JSON, dim — **with agent integer indices replaced by quoted names**
  resolved from the replica (the existing `chronNames` mechanism).
- Width overflow: truncate with `…` (solo: 1 line/event; dock: wrap to ≤ 3 lines
  first). The inspector always has the full event.

## Treatment by event class

| Class | Events (today) | Treatment |
|---|---|---|
| **speech** | `social.conversation_turn`, `social.rumor_told` | privileged: `{"Speaker"→"Listener"}` in name color, then the utterance quoted in bright text: `{"Ash"→"Rowan"} "the fire's low again"` |
| **scene summary** | `social.conversation` | dim compact: `{"gist":"argued about firewood","turns":6,"tones":[-1,1]}` — gist first |
| **narration** | `chronicle.entry` | narrated view only (`r` toggle): prose paragraph under a day header, as today |
| **clock** | `clock.paused`, `clock.resumed`, `clock.speed_set` | type in yellow, payload compact |
| **default** | everything else (`agent.talked`, movement, foraging, …) | dim: type + compact resolved-name payload |

The class table is the extension point: new event types land in **default** until a
row here promotes them. Speech events are the reason this grammar exists — the feed
should read as `Ash said → Rowan replied` in JSON form.

## Inspector (paused expand)

The expanded view shows the **stored event verbatim**, pretty-printed with 2-space
indent: `seq`, `tick`, `type`, and the raw `payload` exactly as persisted — integer
indices intact. Name resolution appears as trailing `// name` annotations, never as
payload rewrites:

```
{
  "seq": 1202, "tick": 8846,
  "type": "social.conversation_turn",
  "payload": {
    "conv": 102,
    "speaker": 1,     // Rowan
    "listener": 0,    // Ash
    "text": "I stacked wood at dawn, ask Birch"
  }
}
```

Rationale: the feed line is a *view* (names resolved, fields elided); the inspector
is *evidence* (exact bytes, annotated). Both must exist; neither substitutes for the
other.

## Color roles

Style tokens (bind to Lipgloss styles, one per role — see
[layout.md](layout.md)): `dim` (seq/time/default payloads) · `type` (event type) ·
`name` (speaker→listener) · `speech` (quoted utterance, brightest) · `clock`
(yellow) · `selection` (inspect-mode row background).
