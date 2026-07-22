package metatron

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/evanstern/promptworld/internal/persona"
)

// The charter is the game's only player-editable prompt. It is read fresh at
// the start of every Metatron turn and digest — that per-read discipline IS
// the "edits are live next turn" mechanism. The angel never runs
// charterless: missing files are restored, empty files fall back, oversized
// files are truncated — and each case is reported in `notice` so the next
// reply can tell the player.

// loadCharter returns the effective charter text and a human-readable notice
// ("" when the player's charter loaded cleanly).
func loadCharter(worldDir string) (text, notice string) {
	path := filepath.Join(worldDir, "charter.md")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Restore the default so the player has a file to edit again.
		os.WriteFile(path, []byte(persona.DefaultCharter), 0o644)
		return persona.DefaultCharter, "charter.md was missing — the default charter has been restored"
	}
	if err != nil {
		return persona.DefaultCharter, "charter.md could not be read — serving under the default charter"
	}
	t := string(data)
	if strings.TrimSpace(t) == "" {
		return persona.DefaultCharter, "charter.md is empty — serving under the default charter"
	}
	if len(t) > persona.CharterMaxChars {
		return t[:persona.CharterMaxChars], "charter.md exceeds the cap — only the first 4,000 characters are in effect"
	}
	return t, ""
}

// charterIsDefault reports whether the file on disk matches the authored
// default (for status display).
func charterIsDefault(worldDir string) bool {
	data, err := os.ReadFile(filepath.Join(worldDir, "charter.md"))
	if err != nil {
		return true
	}
	return string(data) == persona.DefaultCharter
}
