package metatron

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/evanstern/promptworld/internal/persona"
)

// The charter is the game's base player-editable prompt, joined by an optional
// folder of player-authored skill files (spec 021) — the assistant-shaped
// instruction surface (CLAUDE.md + SKILL.md). Every one of these files is read
// fresh at the start of every Metatron turn and status peek — that per-read
// discipline IS the "edits are live next turn" mechanism (no watcher, no
// cache). The angel never runs charterless: missing files are restored, empty
// files fall back, oversized files are truncated, over-cap skill folders are
// trimmed — and each case is reported in a `notice` so the next reply can tell
// the player, one model of "the game tells you when your file didn't load".

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

// maxSkillFiles is the number of skill files composed into a single turn, the
// file-count cap half of the skills surface (per-file size reuses the charter's
// 4,000-char cap, persona.CharterMaxChars). Surplus files (in sort order) are
// skipped with a notice — deterministic, never silent (spec 021 FR-002).
const maxSkillFiles = 8

// skillFile is one composed skill: its filename (provenance) and its effective
// (post-cap) text.
type skillFile struct {
	name string
	text string
}

// loadSkills reads the world's skills/ folder fresh (FR-001) and returns the
// eligible skill files in deterministic composition order, plus one notice per
// issue. Eligibility (contracts/instruction-surface.md rule 3): regular .md
// files that are direct children of skills/ — subdirectories, dotfiles, and
// other extensions are silently excluded (.DS_Store noise is not a notice).
// Composition order is ascending bytewise filename order (players prefix 10-,
// 20-), the same order two byte-identical world dirs produce, so identical dirs
// compose identical prompts (FR-012). Caps mirror the charter's discipline:
// over-cap files truncate + notice; files beyond the count cap skip + notice;
// an unreadable file skips + notice. A missing/unreadable folder is the common,
// unremarkable case — no skills, no notice.
func loadSkills(worldDir string) (skills []skillFile, notices []string) {
	dir := filepath.Join(worldDir, "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue // no recursion (rule 3)
		}
		n := e.Name()
		if strings.HasPrefix(n, ".") || !strings.HasSuffix(n, ".md") {
			continue // dotfiles / non-.md — silently excluded, not a notice
		}
		names = append(names, n)
	}
	sort.Strings(names) // ascending bytewise — the deterministic composition order
	if len(names) > maxSkillFiles {
		skipped := make([]string, 0, len(names)-maxSkillFiles)
		for _, n := range names[maxSkillFiles:] {
			skipped = append(skipped, "skills/"+n)
		}
		names = names[:maxSkillFiles]
		notices = append(notices, fmt.Sprintf("more than %d skill files present — %s not composed",
			maxSkillFiles, strings.Join(skipped, ", ")))
	}
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			notices = append(notices, fmt.Sprintf("skills/%s could not be read — skipped", n))
			continue
		}
		text := string(data)
		if len(text) > persona.CharterMaxChars {
			text = text[:persona.CharterMaxChars]
			notices = append(notices, fmt.Sprintf("skills/%s exceeds the cap — only the first 4,000 characters are in effect", n))
		}
		skills = append(skills, skillFile{name: n, text: text})
	}
	return skills, notices
}

// skillNames returns just the composition-ordered filenames of the effective
// skill files — the model-free provenance list Status surfaces (spec 021 R8).
func skillNames(worldDir string) []string {
	skills, _ := loadSkills(worldDir)
	if len(skills) == 0 {
		return nil
	}
	out := make([]string, len(skills))
	for i, s := range skills {
		out[i] = s.name
	}
	return out
}
