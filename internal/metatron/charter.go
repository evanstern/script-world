package metatron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/evanstern/promptworld/internal/persona"
	"github.com/evanstern/promptworld/internal/tool"
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

// grantSet is a world's effective capability grant for one turn/peek (spec 021
// US2): which metatron loop tools are offered, and (when restricted) which
// miracle kinds. It drives all three gating layers alike — the declared roster,
// the derived guidance, and the door — so they cannot disagree (FR-005). Maps
// are used for O(1) membership; nothing is ever iterated into ordered output.
type grantSet struct {
	tools           map[string]bool // granted metatron loop-tool names
	kinds           map[string]bool // granted miracle kinds; meaningful only when restricted
	kindsRestricted bool            // true ⇒ only kinds in `kinds` are offered for work_miracle
	manifestDefault bool            // true ⇒ no capabilities.json on disk (full default grant)
}

// allows reports whether a metatron loop tool is granted this world.
func (g grantSet) allows(name string) bool { return g.tools[name] }

// allowsKind reports whether a miracle kind may land: unrestricted worlds allow
// every kind; a restricted world allows only its declared subset.
func (g grantSet) allowsKind(kind string) bool {
	if !g.kindsRestricted {
		return true
	}
	return g.kinds[kind]
}

// grantedTools returns the granted metatron loop-tool names in registry order
// (LoopRosterMetatron order) — the deterministic surface Status renders.
func (g grantSet) grantedTools() []string {
	var out []string
	for _, t := range tool.LoopRosterMetatron() {
		if g.tools[t.Name] {
			out = append(out, t.Name)
		}
	}
	return out
}

// grantedKinds returns the granted miracle kinds in registry order when
// restricted, else nil (all kinds). Deterministic — walks tool.MiracleKinds().
func (g grantSet) grantedKinds() []string {
	if !g.kindsRestricted {
		return nil
	}
	var out []string
	for _, k := range tool.MiracleKinds() {
		if g.kinds[k] {
			out = append(out, k)
		}
	}
	return out
}

// grantedToolLabels renders the granted roster for Status (contracts/status.md):
// registry order, with work_miracle suffixed `(kind,…)` ONLY when its kinds are
// restricted (an unrestricted work_miracle shows bare). nil when nothing is
// granted (a conversation-only world) so the field omits under omitempty.
func grantedToolLabels(g grantSet) []string {
	names := g.grantedTools()
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n == "work_miracle" && g.kindsRestricted {
			out = append(out, "work_miracle("+strings.Join(g.grantedKinds(), ",")+")")
			continue
		}
		out = append(out, n)
	}
	return out
}

// fullGrant is the default grant a world with no (or an unusable) manifest gets:
// the entire metatron loop roster, all miracle kinds — byte-compatible with the
// pre-021 angel (FR-007, SC-003).
func fullGrant() grantSet {
	tools := make(map[string]bool)
	for _, t := range tool.LoopRosterMetatron() {
		tools[t.Name] = true
	}
	return grantSet{tools: tools}
}

// manifestDoc is the parse target for capabilities.json. Absent vs empty is
// meaningful and preserved by encoding/json: an omitted key decodes to nil, an
// explicit [] to a non-nil empty slice.
type manifestDoc struct {
	Tools        []string `json:"tools"`
	MiracleKinds []string `json:"miracle_kinds"`
}

// loadManifest reads the world's capabilities.json fresh (FR-001) and returns
// the effective grant set plus one notice per issue, mirroring the charter's
// permissive-fallback teaching model (spec 021 R4, contracts/capability-manifest.md):
//   - no file → full grant, NO notice, manifestDefault true (byte-compatible today);
//   - unreadable / malformed JSON → full grant + notice (a typo never bricks the angel);
//   - unknown tool/kind names → ignored + notice, the valid remainder applies;
//   - omitted "tools" key → unconstrained (all tools), symmetric with an omitted
//     "miracle_kinds" meaning all kinds; explicit "tools": [] → conversation-only;
//   - "miracle_kinds" omitted → all kinds; present → exactly that (valid) subset.
//
// Conversation is never gateable here — it is the final-text channel, not a
// roster tool (FR-006); a world granting nothing still converses.
func loadManifest(worldDir string) (grantSet, []string) {
	path := filepath.Join(worldDir, "capabilities.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		g := fullGrant()
		g.manifestDefault = true
		return g, nil
	}
	if err != nil {
		return fullGrant(), []string{"capabilities.json could not be read — serving with the full tool roster"}
	}
	var doc manifestDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return fullGrant(), []string{"capabilities.json is not valid JSON — serving with the full tool roster"}
	}

	var notices []string
	known := make(map[string]bool)
	var order []string
	for _, t := range tool.LoopRosterMetatron() {
		known[t.Name] = true
		order = append(order, t.Name)
	}

	tools := make(map[string]bool)
	if doc.Tools == nil {
		for _, n := range order { // omitted key ⇒ unconstrained
			tools[n] = true
		}
	} else {
		var unknown []string
		for _, n := range doc.Tools {
			if known[n] {
				tools[n] = true
			} else {
				unknown = append(unknown, n)
			}
		}
		if len(unknown) > 0 {
			notices = append(notices, "capabilities.json lists unknown tool(s): "+strings.Join(unknown, ", ")+" — ignored")
		}
	}

	g := grantSet{tools: tools}
	if doc.MiracleKinds != nil {
		knownKind := make(map[string]bool)
		for _, k := range tool.MiracleKinds() {
			knownKind[k] = true
		}
		kinds := make(map[string]bool)
		var unknown []string
		for _, k := range doc.MiracleKinds {
			if knownKind[k] {
				kinds[k] = true
			} else {
				unknown = append(unknown, k)
			}
		}
		if len(unknown) > 0 {
			notices = append(notices, "capabilities.json lists unknown miracle kind(s): "+strings.Join(unknown, ", ")+" — ignored")
		}
		g.kinds = kinds
		g.kindsRestricted = true
	}
	return g, notices
}

// grantedRoster is the effective metatron loop roster for a turn: the full loop
// roster filtered to granted tools, with work_miracle's kind enum narrowed to
// the granted kinds when restricted (copy-on-write via tool.RestrictEnum, the
// registry untouched). This ONE roster feeds all three gating layers — Job.Roster
// (declaration), MetatronToolGuidance (prose), and the handler set (door) — so an
// ungranted tool or kind is structurally absent from every one of them (FR-005).
func grantedRoster(g grantSet) []tool.Tool {
	full := tool.LoopRosterMetatron()
	out := make([]tool.Tool, 0, len(full))
	for _, t := range full {
		if !g.tools[t.Name] {
			continue
		}
		if t.Name == "work_miracle" && g.kindsRestricted {
			t = tool.RestrictEnum(t, "kind", g.grantedKinds())
		}
		out = append(out, t)
	}
	return out
}
