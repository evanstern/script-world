package world

// The snapshot-cut migration that carries a world's people (spec 012 US6 for
// v1→v2 while the land resets, research R10; spec 013 for v2→v3 which preserves
// people AND land, research R3). This is a client-side, offline operation: the
// daemon must be stopped. It never replays old events under new rules — it reads
// the source world's covering snapshot, transforms it (internal/sim), and writes
// a fresh log whose single world.migrated event carries the full transformed
// state, so the log alone reproduces the migrated world byte-identically. A v1
// world chains 1→2→3 in one run; the archive name is keyed to the source format
// (world.v1.db or world.v2.db).

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/worldmap"
)

// MigrateResult is the human-facing summary of a completed migration.
type MigrateResult struct {
	Name          string
	Seed          uint64
	AgentsCarried int
	Tick          int64 // the continuation tick (carried from v1)
	SourceEvents  int64 // v1 events archived in world.v1.db
	ArchivePath   string
}

// OpenForMigration loads a world directory WITHOUT the current version gate,
// for the sole purpose of migrating it. It admits any older supported source
// format (v1 or v2) and refuses everything else: an already-current (or future)
// world has nothing to migrate, and a corrupt manifest is refused exactly as
// Open would. Map dimensions are defaulted identically to Open so a regenerated
// map matches what the daemon will boot.
func OpenForMigration(dir string) (*World, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, fmt.Errorf("not a world directory (missing %s): %w", ManifestName, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("corrupt %s: %w", ManifestName, err)
	}
	if m.FormatVersion == FormatVersion {
		return nil, fmt.Errorf("world %q is already format_version %d — nothing to migrate", m.Name, FormatVersion)
	}
	if m.FormatVersion != 1 && m.FormatVersion != 2 {
		return nil, fmt.Errorf("world %q is format_version %d; only v1 and v2 worlds can be migrated to v%d", m.Name, m.FormatVersion, FormatVersion)
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

// V1DBPath / V2DBPath are the archived original databases — the archive name is
// keyed to the SOURCE format so a v1→(2→)3 run parks world.v1.db and a v2→3 run
// parks world.v2.db. The archive's existence is the already-migrated guard for
// that source format, and restoring is "delete world.db, rename this back, reset
// the manifest to the source version".
func (w *World) V1DBPath() string { return filepath.Join(w.Dir, "world.v1.db") }
func (w *World) V2DBPath() string { return filepath.Join(w.Dir, "world.v2.db") }

// archiveDBPath is the archive name for this world's SOURCE format version.
func (w *World) archiveDBPath() string {
	if w.Manifest.FormatVersion == 2 {
		return w.V2DBPath()
	}
	return w.V1DBPath()
}

// Migrate performs the whole v1→v2 migration in place (research R10). The
// archive is sacred: world.db is renamed to world.v1.db (never deleted), and
// the migration refuses to run if that archive already exists. It refuses a
// running daemon and an un-covered event tail (no clean-shutdown snapshot),
// leaving the world untouched in both cases.
func Migrate(dir string) (*MigrateResult, error) {
	w, err := OpenForMigration(dir)
	if err != nil {
		return nil, err
	}

	// Refuse a live daemon: migration rewrites the database out from under any
	// process holding it. The pidfile liveness check is version-gate-free (the
	// v1 world cannot be world.Open'd under this build).
	if running, pid := daemonAlive(w); running {
		return nil, fmt.Errorf("daemon is running (pid %d) — stop it first: scriptworld stop %s", pid, dir)
	}

	// Already-migrated guard: the archive is never overwritten (FR-025). The
	// guard is on the SOURCE-format archive (world.v1.db for a v1 source,
	// world.v2.db for a v2 source), so a v2 world produced by an earlier v1
	// migration — which would carry a stale world.v1.db — is still migratable
	// to v3.
	archivePath := w.archiveDBPath()
	if _, err := os.Stat(archivePath); err == nil {
		return nil, fmt.Errorf("this world is already migrated (%s exists); the archive is never overwritten", filepath.Base(archivePath))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// Read the v1 covering snapshot. The migration NEVER replays v1 events under
	// v2 rules (FR-024) — the clean-shutdown snapshot is the only v1 state it
	// reads.
	st, err := store.Open(w.DBPath())
	if err != nil {
		return nil, err
	}
	if cerr := st.CheckContiguity(); cerr != nil {
		st.Close()
		return nil, cerr
	}
	maxSeq := st.LastSeq()
	snap, err := st.LatestValidSnapshot()
	if err != nil {
		st.Close()
		return nil, err
	}
	if snap == nil {
		st.Close()
		return nil, migrateNeedsCleanStop(dir, "this world has no valid snapshot")
	}
	// The clean-shutdown guarantee (FR-024) is that the covering snapshot holds
	// all *sim* state. A real v1 daemon appends its `daemon.stopped` bookkeeping
	// AFTER the shutdown snapshot, so a cleanly-stopped world normally has a
	// one-event tail past snap.Seq — observed on myworld-01: seq 114507 trailing
	// a 114506-covering snapshot. `daemon.*` events are reducer no-ops carrying
	// zero sim state, so a tail consisting only of them is tolerated (and simply
	// dropped — its information content is nil, nothing to carry into the v2
	// log). Any sim-affecting event past the snapshot is un-snapshotted history
	// and still refuses.
	tail, err := st.EventsSince(snap.Seq, 0)
	if err != nil {
		st.Close()
		return nil, err
	}
	for _, e := range tail {
		if !strings.HasPrefix(e.Type, "daemon.") {
			st.Close()
			return nil, migrateNeedsCleanStop(dir,
				fmt.Sprintf("the latest valid snapshot covers seq %d but the log runs to seq %d with a sim-affecting event (%s at seq %d) past it (an unclean stop left un-snapshotted history)",
					snap.Seq, maxSeq, e.Type, e.Seq))
		}
	}

	// Transform the covering-snapshot state to the current (v3) format. A v1
	// world chains both transforms in one run: v1→v2 re-places souls on the v2
	// regeneration of the same seed (w.Map() uses this build's generator, so
	// they stand on passable v2 tiles, rock outcrops included), then v2→v3
	// carries everything verbatim and spills any over-cap carry. A v2 world runs
	// the v2→v3 step alone (no map needed — no land reset). srcTick is the
	// carried continuation tick in every path.
	var finalState *sim.State
	var srcTick int64
	switch w.Manifest.FormatVersion {
	case 1:
		var v2state *sim.State
		v2state, srcTick, err = sim.TransformV1Snapshot(snap.State, w.Map())
		if err != nil {
			st.Close()
			return nil, err
		}
		finalState = sim.TransformV2State(v2state)
	case 2:
		finalState, srcTick, err = sim.TransformV2Snapshot(snap.State)
		if err != nil {
			st.Close()
			return nil, err
		}
	default:
		st.Close()
		return nil, fmt.Errorf("unsupported source format_version %d", w.Manifest.FormatVersion)
	}
	if err := st.Close(); err != nil {
		return nil, err
	}

	// Archive the original database (and any WAL sidecars) intact under the
	// source-format archive name. This is the point of no easy return, so
	// everything that could refuse has already run.
	if err := archiveDB(w.DBPath(), archivePath); err != nil {
		return nil, err
	}

	// Fresh log: world.created (same name/seed) then world.migrated carrying the
	// full transformed state. Both stamped at the continuation tick.
	fresh, err := store.Open(w.DBPath())
	if err != nil {
		return nil, err
	}
	defer fresh.Close()

	createdPayload, err := json.Marshal(sim.WorldCreatedPayload{Name: w.Manifest.Name, Seed: w.Manifest.Seed})
	if err != nil {
		return nil, err
	}
	migratedPayload, err := json.Marshal(sim.WorldMigratedPayload{
		FromFormat:   w.Manifest.FormatVersion,
		SourceEvents: maxSeq,
		SourceTick:   srcTick,
		State:        *finalState,
	})
	if err != nil {
		return nil, err
	}
	if err := fresh.AppendEvents([]store.Event{
		{Tick: srcTick, Type: "world.created", Payload: createdPayload},
		{Tick: srcTick, Type: "world.migrated", Payload: migratedPayload},
	}); err != nil {
		return nil, err
	}

	// Initial snapshot at the migrated tick: the covering snapshot of the fresh
	// log. Deleting it and replaying (world.created → world.migrated) must
	// reproduce this exact state — the determinism half of SC-007.
	if err := fresh.SaveSnapshot(srcTick, fresh.LastSeq(), finalState.Marshal()); err != nil {
		return nil, err
	}

	// Bump the manifest last: with the manifest still at the source version, a
	// crash between the archive and here leaves a recoverable state (the
	// source-format archive present, manifest unbumped — restore is the same
	// rename-back).
	w.Manifest.FormatVersion = FormatVersion
	if err := writeManifest(w); err != nil {
		return nil, err
	}

	return &MigrateResult{
		Name:          w.Manifest.Name,
		Seed:          w.Manifest.Seed,
		AgentsCarried: len(finalState.Agents),
		Tick:          srcTick,
		SourceEvents:  maxSeq,
		ArchivePath:   archivePath,
	}, nil
}

// migrateNeedsCleanStop wraps the "no covering snapshot" refusals with the
// remedy: a clean start+stop under the source-format binary produces the
// shutdown snapshot migration relies on (FR-024).
func migrateNeedsCleanStop(dir, why string) error {
	return fmt.Errorf("%s — start and stop this world once with its own binary so a covering shutdown snapshot exists, then re-run: scriptworld migrate %s", why, dir)
}

// archiveDB renames the live database (and any WAL/SHM sidecars) to the archive
// name. Moving the sidecars matters twice: the archive stays a complete,
// restorable database, and the fresh world.db is not corrupted by a stale WAL
// from the old one.
func archiveDB(dbPath, archivePath string) error {
	if err := os.Rename(dbPath, archivePath); err != nil {
		return fmt.Errorf("archive %s: %w", filepath.Base(dbPath), err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		src := dbPath + suffix
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, archivePath+suffix); err != nil {
				return fmt.Errorf("archive %s: %w", filepath.Base(src), err)
			}
		}
	}
	return nil
}

// writeManifest rewrites world.json from the (mutated) manifest, matching
// Create's indentation so the file stays human-readable and diff-friendly.
func writeManifest(w *World) error {
	data, err := json.MarshalIndent(w.Manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(w.Dir, ManifestName), append(data, '\n'), 0o644)
}

// daemonAlive is a version-gate-free pidfile liveness check (a v1 world cannot
// be world.Open'd under the v2 build, so daemon.IsRunning would falsely report
// "not running"). It mirrors internal/daemon's acquirePidfile/IsRunning check;
// duplicated rather than imported to avoid an import cycle (daemon → world).
func daemonAlive(w *World) (bool, int) {
	data, err := os.ReadFile(w.PidPath())
	if err != nil {
		return false, 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !pidAlive(pid) {
		return false, 0
	}
	return true, pid
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
