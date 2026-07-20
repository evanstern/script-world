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
	return &World{Dir: dir, Manifest: m}, nil
}

// Map regenerates the world's terrain from the manifest — deterministic, so
// daemon and clients derive identical maps without any wire transfer.
func (w *World) Map() *worldmap.Map {
	return worldmap.Generate(w.Manifest.Seed, w.Manifest.MapWidth, w.Manifest.MapHeight)
}

func (w *World) DBPath() string        { return filepath.Join(w.Dir, "world.db") }
func (w *World) LLMConfigPath() string { return filepath.Join(w.Dir, "llm.json") }
func (w *World) SockPath() string      { return filepath.Join(w.Dir, "daemon.sock") }
func (w *World) PidPath() string       { return filepath.Join(w.Dir, "daemon.pid") }
func (w *World) CharterPath() string   { return filepath.Join(w.Dir, "charter.md") }
func (w *World) MetatronDir() string   { return filepath.Join(w.Dir, "metatron") }
func (w *World) LogPath() string       { return filepath.Join(w.Dir, "daemon.log") }
