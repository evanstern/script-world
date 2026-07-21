# Pattern: focus contract

Who owns the keyboard, when. This replaces the current rule at
`internal/tui/tui.go:305-309` ("the metatron console owns the keyboard while the
pane is active"), which traps users: once pane 3 is active, `1–4`, `q`, and
spacebar-pause are silently swallowed and only an undiscoverable `esc` (or
`ctrl+c`) escapes.

## The contract

1. **Viewing ≠ typing.** Selecting a pane or dock tab only *shows* it. Text capture
   begins solely on an explicit focus action (`m`), never as a side effect of
   navigation.
2. **Focus is drawn, not remembered.** The focused input renders an amber border, a
   visible cursor, and the inline hint `esc release · ⏎ send`. If a user must know
   one thing, the chrome itself says it on every frame — discoverability lives in
   the UI, not in documentation or a leader-key convention.
3. **`esc` always releases.** One keypress returns full keyboard control,
   instantly. `ctrl+c` quits the app from any state whatsoever.
4. **No silent swallows.** While focused: printable keys append to the buffer *and
   visibly appear there*; `backspace`, `↑`/`↓` (history), `⏎`, `esc` each have a
   visible effect. There is no key whose press produces no observable change — that
   is the exact bug class being deleted.
5. **Unfocused = everything global works.** Pause (`space`), speed (`[` `]`), tabs,
   pan, quit. While focused, `space` is just a space character — acceptable,
   because leaving focus is now one obvious keypress away.

## Scope

- Widescreen: the minibuffer is the only focusable input, so the contract has
  exactly one client.
- Narrow fallback: the Metatron pane keeps its input line, but it obeys this same
  contract — entering pane 3 does **not** focus it; `m` (or `⏎`) does, and the
  focused/dormant states render the same hints.

## Acceptance checks

- From any screen, type `3` then `1` — you are looking at the map. No focus was
  acquired in between.
- Focus the minibuffer, type "hello", press `esc`, press `space` — the clock pauses.
- Focus the minibuffer and press every key on the keyboard — each press changed
  something visible.
