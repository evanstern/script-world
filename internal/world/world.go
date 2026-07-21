// Package world owns the save-directory layout: one directory = one world run.
// Everything belonging to a run lives inside it; nothing is global.
package world

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/evanstern/script-world/internal/worldmap"
)

const (
	ManifestName  = "world.json"
	FormatVersion = 1
)

type Manifest struct {
	Name            string `json:"name"`
	Seed            uint64 `json:"seed"`
	CreatedAt       string `json:"created_at"`
	FormatVersion   int    `json:"format_version"`
	TickGameSeconds int    `json:"tick_game_seconds"`
	// Map dimensions; zero values (older saves) default to
	// worldmap.DefaultSize on Open. Terrain is regenerated from
	// (Seed, MapWidth, MapHeight), never persisted.
	MapWidth  int `json:"map_width,omitempty"`
	MapHeight int `json:"map_height,omitempty"`
	// Meeting is an optional per-world meeting convention (TASK-36). When
	// present, the daemon seeds a meeting.convention_established event on boot
	// so the convene→open lifecycle honors it. Absent, no meeting convenes
	// unless one emerges in-world. `scriptworld new` never writes it —
	// emergent is the default.
	Meeting *MeetingConfig `json:"meeting,omitempty"`
}

// MeetingConfig declares when (and optionally where) the daily assembly
// convenes. Convene and Open are 24-hour game clock times, "HH:MM"; Convene
// must fall before Open and both within the day. X/Y are optional map
// coordinates for the meeting place — omitted, the daemon derives it (first
// fire, else first shelter, else map center).
type MeetingConfig struct {
	Convene string `json:"convene"`
	Open    string `json:"open"`
	X       *int   `json:"x,omitempty"`
	Y       *int   `json:"y,omitempty"`
}

// Seconds parses Convene/Open into seconds-of-day and validates the window:
// both well-formed "HH:MM" within the day, convene strictly before open.
func (c *MeetingConfig) Seconds() (convene, open int, err error) {
	if convene, err = parseClock(c.Convene); err != nil {
		return 0, 0, fmt.Errorf("meeting.convene: %w", err)
	}
	if open, err = parseClock(c.Open); err != nil {
		return 0, 0, fmt.Errorf("meeting.open: %w", err)
	}
	if convene >= open {
		return 0, 0, fmt.Errorf("meeting.convene (%s) must be before meeting.open (%s)", c.Convene, c.Open)
	}
	return convene, open, nil
}

// parseClock reads an "HH:MM" 24-hour time into a second-of-day in [0, 86400).
func parseClock(s string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("time %q is not HH:MM: %w", s, err)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("time %q out of range (00:00–23:59)", s)
	}
	return h*3600 + m*60, nil
}

type World struct {
	Dir      string
	Manifest Manifest
}

// Create initializes a new save directory. The directory may exist only if
// empty; anything else is refused so runs can never bleed into each other.
func Create(dir, name string, seed uint64) (*World, error) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("directory %s is not empty", dir)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "agents"), 0o755); err != nil {
		return nil, err
	}
	m := Manifest{
		Name:            name,
		Seed:            seed,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		FormatVersion:   FormatVersion,
		TickGameSeconds: 1,
		MapWidth:        worldmap.DefaultSize,
		MapHeight:       worldmap.DefaultSize,
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	return &World{Dir: dir, Manifest: m}, nil
}

// Open loads and validates an existing save directory.
func Open(dir string) (*World, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, fmt.Errorf("not a world directory (missing %s): %w", ManifestName, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("corrupt %s: %w", ManifestName, err)
	}
	if m.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("world format_version %d unsupported (this build supports %d)", m.FormatVersion, FormatVersion)
	}
	if m.TickGameSeconds != 1 {
		return nil, fmt.Errorf("tick_game_seconds %d unsupported (must be 1)", m.TickGameSeconds)
	}
	if m.MapWidth <= 0 {
		m.MapWidth = worldmap.DefaultSize
	}
	if m.MapHeight <= 0 {
		m.MapHeight = worldmap.DefaultSize
	}
	if m.Meeting != nil {
		if _, _, err := m.Meeting.Seconds(); err != nil {
			return nil, fmt.Errorf("corrupt %s: %w", ManifestName, err)
		}
	}
	return &World{Dir: dir, Manifest: m}, nil
}

// Map regenerates the world's terrain from the manifest — deterministic, so
// daemon and clients derive identical maps without any wire transfer.
func (w *World) Map() *worldmap.Map {
	return worldmap.Generate(w.Manifest.Seed, w.Manifest.MapWidth, w.Manifest.MapHeight)
}

func (w *World) DBPath() string        { return filepath.Join(w.Dir, "world.db") }
func (w *World) LLMConfigPath() string { return filepath.Join(w.Dir, "llm.json") }

// CalibrationPath is the seconds-per-point profile written only by
// `scriptworld calibrate` (specs/007-cognition-horizon); an absent file is
// legal — pessimistic bootstrap defaults apply.
func (w *World) CalibrationPath() string { return filepath.Join(w.Dir, "calibration.json") }
func (w *World) SockPath() string        { return filepath.Join(w.Dir, "daemon.sock") }
func (w *World) PidPath() string         { return filepath.Join(w.Dir, "daemon.pid") }
func (w *World) CharterPath() string     { return filepath.Join(w.Dir, "charter.md") }

// VillageCharterPath is the village's law (TASK-13) — a scribe-rendered
// derived view of event-sourced norms, distinct from Metatron's
// player-editable charter.md above.
func (w *World) VillageCharterPath() string { return filepath.Join(w.Dir, "village_charter.md") }
func (w *World) MetatronDir() string        { return filepath.Join(w.Dir, "metatron") }
func (w *World) LogPath() string            { return filepath.Join(w.Dir, "daemon.log") }
