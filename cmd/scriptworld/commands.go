package main

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/daemon"
	"github.com/evanstern/script-world/internal/ipc"
	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/persona"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/tui"
	"github.com/evanstern/script-world/internal/world"
	"github.com/evanstern/script-world/internal/worlds"
)

func dirArg(fs *flag.FlagSet, args []string) (string, error) {
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() < 1 {
		return "", fmt.Errorf("missing world directory argument")
	}
	return fs.Arg(0), nil
}

// parseDirFlags handles both "cmd <dir> --flag" and "cmd --flag <dir>".
func parseDirFlags(fs *flag.FlagSet, args []string) (string, error) {
	var dir string
	var rest []string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		dir, rest = args[0], args[1:]
	} else {
		rest = args
	}
	if err := fs.Parse(rest); err != nil {
		return "", err
	}
	if dir == "" {
		if fs.NArg() < 1 {
			return "", fmt.Errorf("missing world directory argument")
		}
		dir = fs.Arg(0)
	}
	return dir, nil
}

// resolveWorld turns a per-world command's positional argument — a name or
// a path (FR-006) — into a directory. Path-shaped arguments bypass name
// resolution entirely and are returned verbatim, today's exact behavior
// (FR-012); bare names resolve via worlds.Resolve (FR-007/FR-011). Every
// per-world command except `new` (whose argument creates rather than
// resolves) routes through this.
func resolveWorld(arg string) (string, error) {
	if worlds.IsPathArg(arg) {
		return arg, nil
	}
	return worlds.Resolve(arg)
}

// worldArg is dirArg's name-or-path counterpart (see resolveWorld).
func worldArg(fs *flag.FlagSet, args []string) (string, error) {
	arg, err := dirArg(fs, args)
	if err != nil {
		return "", err
	}
	return resolveWorld(arg)
}

// parseWorldFlags is parseDirFlags's name-or-path counterpart (see
// resolveWorld).
func parseWorldFlags(fs *flag.FlagSet, args []string) (string, error) {
	arg, err := parseDirFlags(fs, args)
	if err != nil {
		return "", err
	}
	return resolveWorld(arg)
}

// cmdNew implements `scriptworld new` per contracts/cli.md (research.md D5):
// a bare-word argument is name-form — create <worlds-home>/<name> (or --at
// DIR exactly), manifest name = the argument. A path-shaped argument
// (worlds.IsPathArg) is legacy path-form, byte-compatible with today:
// create at that path, name from --name or the basename (FR-012).
func cmdNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	name := fs.String("name", "", "path-form only: world name (default: directory basename)")
	at := fs.String("at", "", "name-form only: create at this exact path instead of the default worlds home")
	seed := fs.Uint64("seed", 0, "world seed (default: random)")
	arg, err := parseDirFlags(fs, args)
	if err != nil {
		return err
	}

	nameForm := !worlds.IsPathArg(arg)
	var dir, worldName string
	if nameForm {
		if *name != "" {
			return fmt.Errorf("new %q: --name is not valid with a bare name argument (the argument is already the name)", arg)
		}
		if err := worlds.ValidateName(arg); err != nil {
			return err
		}
		worldName = arg
		if *at != "" {
			dir = *at
		} else {
			home, err := worlds.WorldsHome()
			if err != nil {
				return err
			}
			dir = filepath.Join(home, arg)
		}
	} else {
		if *at != "" {
			return fmt.Errorf("new %q: --at is only valid with a bare name, not a path — the path itself is already the location", arg)
		}
		dir = arg
		worldName = *name
		if worldName == "" {
			// Backward compatible: the auto-derived basename was never
			// validated before this feature and stays that way (FR-012).
			worldName = filepath.Base(filepath.Clean(dir))
		} else if err := worlds.ValidateName(worldName); err != nil {
			// An explicit --name IS validated (contracts/cli.md D5).
			return err
		}
	}

	if *seed == 0 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return err
		}
		*seed = binary.LittleEndian.Uint64(b[:]) >> 12 // keep it comfortably printable
	}
	w, err := world.Create(dir, worldName, *seed)
	if err != nil {
		return err
	}
	st, err := store.Open(w.DBPath())
	if err != nil {
		return err
	}
	defer st.Close()
	payload, err := json.Marshal(sim.WorldCreatedPayload{Name: worldName, Seed: *seed})
	if err != nil {
		return err
	}
	genesis := []store.Event{{Tick: 0, Type: "world.created", Payload: payload}}
	secretEvents, err := persona.SecretEvents()
	if err != nil {
		return err
	}
	genesis = append(genesis, secretEvents...)
	if err := st.AppendEvents(genesis); err != nil {
		return err
	}
	if err := llm.WriteDefault(w.LLMConfigPath()); err != nil {
		return err
	}
	if err := persona.Genesis(dir); err != nil {
		return err
	}

	// A name-form world at a custom --at location is outside the worlds
	// home, so it needs a registry pointer to be name-addressable later
	// (D1/D6) — the default name-form location is inside the home and is
	// scan-owned, no registry entry wanted. Advisory: never fatal.
	if nameForm && *at != "" {
		if err := worlds.Upsert(worldName, dir); err != nil {
			fmt.Printf("warning: could not register %q in the known-worlds registry (advisory, continuing): %v\n", worldName, err)
		}
	}

	startHint := dir
	if nameForm {
		startHint = worldName
	}
	fmt.Printf("created world %q in %s (seed %d)\nllm config: %s (edit tiers/budget; delete the file to disable LLM traffic)\nstart it with: scriptworld start %s\n",
		worldName, dir, *seed, w.LLMConfigPath(), startHint)
	return nil
}

// resolveWorldForMigrate resolves a migrate argument to a directory. Unlike
// resolveWorld, it must reach v1 worlds — which this v2 build cannot
// world.Open, so worlds.Resolve (whose name lookup gates on openability) is
// blind to them. Path arguments pass through verbatim; bare names resolve
// against the worlds home then the known-worlds registry by manifest presence
// alone, never the version gate.
func resolveWorldForMigrate(arg string) (string, error) {
	if worlds.IsPathArg(arg) {
		return arg, nil
	}
	home, err := worlds.WorldsHome()
	if err != nil {
		return "", err
	}
	if cand := filepath.Join(home, arg); hasManifest(cand) {
		return cand, nil
	}
	if reg, err := worlds.LoadRegistry(); err == nil {
		if p, ok := reg.Worlds[arg]; ok && hasManifest(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("no world named %q (searched %s and the known-worlds list) — try `scriptworld ps --all`", arg, home)
}

func hasManifest(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, world.ManifestName))
	return err == nil
}

// cmdMigrate implements `scriptworld migrate <world>` (spec 012 US6, spec 013):
// the offline snapshot-cut migration that upgrades an older world (v1 or v2) to
// the current format — a v1 world chains 1→2→3 in one run. It resolves the
// world, then hands the whole archive/transform/rewrite ceremony to
// world.Migrate, and prints a human summary of what carried across the break.
func cmdMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	arg, err := dirArg(fs, args)
	if err != nil {
		return err
	}
	dir, err := resolveWorldForMigrate(arg)
	if err != nil {
		return err
	}
	res, err := world.Migrate(dir)
	if err != nil {
		return err
	}
	fmt.Printf("migrated %q (seed %d) to format v%d\n  %d villagers carried across the break at tick %d (%s)\n  %d source events archived in %s\nstart it with: scriptworld start %s\n",
		res.Name, res.Seed, world.FormatVersion, res.AgentsCarried, res.Tick, clock.Format(res.Tick),
		res.SourceEvents, res.ArchivePath, arg)
	return nil
}

func cmdLLM(args []string) error {
	fs := flag.NewFlagSet("llm", flag.ContinueOnError)
	system := fs.String("system", "", "system prompt")
	maxTokens := fs.Int64("max-tokens", 0, "max output tokens (cloud tier)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 3 {
		return fmt.Errorf("usage: scriptworld llm <world> <kind> <prompt...>")
	}
	dir, err := resolveWorld(fs.Arg(0))
	if err != nil {
		return err
	}
	kind := fs.Arg(1)
	prompt := strings.Join(fs.Args()[2:], " ")
	w, err := world.Open(dir)
	if err != nil {
		return err
	}
	c, err := ipc.Dial(w.SockPath())
	if err != nil {
		return err
	}
	defer c.Close()
	data, err := c.Call("llm_call", ipc.LLMCallArgs{
		Kind: kind, System: *system, Prompt: prompt, MaxTokens: *maxTokens,
	})
	if err != nil {
		return err
	}
	var resp llm.Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	fmt.Printf("[%s tier · %s · %d in / %d out tokens · $%.4f · %dms]\n%s\n",
		resp.Tier, resp.Model, resp.InputTokens, resp.OutputTokens, resp.CostUSD, resp.Millis, resp.Text)
	return nil
}

// cmdMetatron is the console one-shot (TASK-12): with a message, one
// mediated turn; without, the model-free status peek.
func cmdMetatron(args []string) error {
	fs := flag.NewFlagSet("metatron", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: scriptworld metatron <world> [message...]")
	}
	dir, err := resolveWorld(fs.Arg(0))
	if err != nil {
		return err
	}
	w, err := world.Open(dir)
	if err != nil {
		return err
	}
	c, err := ipc.Dial(w.SockPath())
	if err != nil {
		return err
	}
	defer c.Close()

	if fs.NArg() == 1 {
		st, err := c.MetatronStatus()
		if err != nil {
			return err
		}
		charter := "custom charter in effect"
		if st.CharterDefault {
			charter = "default charter"
		}
		fmt.Printf("charges %s (%d/%d) · %s · charter.md at %s\n",
			chargeGlyphs(st.Charges), st.Charges, sim.MetatronChargeCap, charter, w.CharterPath())
		if strings.TrimSpace(st.SoulTail) != "" {
			fmt.Printf("\n--- recent notes ---\n%s\n", strings.TrimSpace(st.SoulTail))
		}
		return nil
	}

	r, err := c.MetatronChat(strings.Join(fs.Args()[1:], " "))
	if err != nil {
		return err
	}
	for _, m := range r.Moments {
		fmt.Printf("! %s\n", m)
	}
	fmt.Printf("\n%s\n", r.Reply)
	if r.Nudge != nil {
		fmt.Printf("\n⚡ %s → %s: %q\n", r.Nudge.Form, strings.Join(r.Nudge.Targets, ", "), r.Nudge.Text)
	}
	fmt.Printf("\n[charges %s %d/%d]\n", chargeGlyphs(r.Charges), r.Charges, sim.MetatronChargeCap)
	return nil
}

func chargeGlyphs(n int) string {
	return strings.Repeat("⚡", n) + strings.Repeat("·", sim.MetatronChargeCap-n)
}

func cmdDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	dir, err := worldArg(fs, args)
	if err != nil {
		return err
	}
	return daemon.Run(dir)
}

func cmdStart(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	dir, err := worldArg(fs, args)
	if err != nil {
		return err
	}
	w, err := world.Open(dir)
	if err != nil {
		return err
	}
	if running, pid := daemon.IsRunning(dir); running {
		return fmt.Errorf("daemon already running (pid %d)", pid)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logf, err := os.OpenFile(w.LogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd := exec.Command(exe, "daemon", dir)
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from our session
	if err := cmd.Start(); err != nil {
		return err
	}
	// The child is re-parented on our exit; never wait on it.

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := ipc.Dial(w.SockPath()); err == nil {
			sd, err := c.Status("status", nil)
			c.Close()
			if err == nil {
				fmt.Printf("daemon started (pid %d): %s\n", sd.Daemon.Pid, clockLine(sd))
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not answer within 5s — check %s", w.LogPath())
}

func cmdStop(args []string) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	dir, err := worldArg(fs, args)
	if err != nil {
		return err
	}
	w, err := world.Open(dir)
	if err != nil {
		return err
	}
	running, pid := daemon.IsRunning(dir)
	if !running {
		fmt.Println("daemon not running")
		return nil // idempotent
	}
	if c, err := ipc.Dial(w.SockPath()); err == nil {
		c.Call("shutdown", nil)
		c.Close()
	} else {
		// Socket dead but pid alive: fall back to SIGTERM (same graceful path).
		syscall.Kill(pid, syscall.SIGTERM)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if running, _ := daemon.IsRunning(dir); !running {
			fmt.Println("daemon stopped")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon (pid %d) did not stop within 30s", pid)
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "machine-readable output")
	dir, err := parseWorldFlags(fs, args)
	if err != nil {
		return err
	}
	w, err := world.Open(dir)
	if err != nil {
		return err
	}

	if c, err := ipc.Dial(w.SockPath()); err == nil {
		defer c.Close()
		sd, err := c.Status("status", nil)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(sd)
		}
		fmt.Printf("world %q (seed %d) — daemon running (pid %d, up %ds, %d subscriber(s))\n%s\nlog: last seq %d\n",
			sd.World.Name, sd.World.Seed, sd.Daemon.Pid, sd.Daemon.UptimeSeconds, sd.Daemon.Subscribers,
			clockLine(sd), sd.Log.LastSeq)
		return nil
	}

	// Offline: last-known state from the store, read-only (shared with
	// `ps --all`'s stopped rows — specs/008-instance-manager D7).
	tick, paused, speed, lastSeq, err := worlds.OfflineSnapshot(w)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(map[string]any{
			"world":  map[string]any{"name": w.Manifest.Name, "seed": w.Manifest.Seed, "format_version": w.Manifest.FormatVersion},
			"daemon": map[string]any{"running": false},
			"clock": map[string]any{
				"tick": tick, "game_time": clock.Format(tick),
				"paused": paused, "speed": speed,
			},
			"log": map[string]any{"last_seq": lastSeq},
		})
	}
	fmt.Printf("world %q (seed %d) — daemon not running\nlast known: tick %d (%s), speed %s, paused %v\nlog: last seq %d\n",
		w.Manifest.Name, w.Manifest.Seed, tick, clock.Format(tick), speed, paused, lastSeq)
	return nil
}

func cmdTimeCtl(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	dir, err := worldArg(fs, args)
	if err != nil {
		return err
	}
	w, err := world.Open(dir)
	if err != nil {
		return err
	}
	c, err := ipc.Dial(w.SockPath())
	if err != nil {
		return err
	}
	defer c.Close()
	sd, err := c.Status(cmd, nil)
	if err != nil {
		return err
	}
	fmt.Println(clockLine(sd))
	return nil
}

func cmdSpeed(args []string) error {
	fs := flag.NewFlagSet("speed", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: scriptworld speed <world> <1x|4x|8x|16x|32x|max>")
	}
	val := fs.Arg(1)
	if _, err := clock.ParseSpeed(val); err != nil {
		return err
	}
	dir, err := resolveWorld(fs.Arg(0))
	if err != nil {
		return err
	}
	w, err := world.Open(dir)
	if err != nil {
		return err
	}
	c, err := ipc.Dial(w.SockPath())
	if err != nil {
		return err
	}
	defer c.Close()
	sd, err := c.Status("set_speed", ipc.SetSpeedArgs{Speed: val})
	if err != nil {
		return err
	}
	fmt.Println(clockLine(sd))
	return nil
}

func cmdUI(args []string) error {
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	dir, err := worldArg(fs, args)
	if err != nil {
		return err
	}
	w, err := world.Open(dir)
	if err != nil {
		return err
	}
	m, err := tea.NewProgram(tui.New(w), tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	// An unrecoverable protocol failure (e.g. reply over the cap, TASK-19)
	// quits the TUI; surface it as a real error and a non-zero exit.
	if fm, ok := m.(tui.Model); ok && fm.FatalErr() != "" {
		return fmt.Errorf("%s", fm.FatalErr())
	}
	return nil
}

func cmdAttach(args []string) error {
	fs := flag.NewFlagSet("attach", flag.ContinueOnError)
	dir, err := worldArg(fs, args)
	if err != nil {
		return err
	}
	w, err := world.Open(dir)
	if err != nil {
		return err
	}
	c, err := ipc.Dial(w.SockPath())
	if err != nil {
		return err
	}
	defer c.Close()

	sd, err := c.Status("status", nil)
	if err != nil {
		return err
	}
	fmt.Printf("attached to %q — %s\ncommands: pause | resume | speed <v> | status | quit\n", sd.World.Name, clockLine(sd))
	if err := c.Subscribe(nil); err != nil {
		return err
	}

	go func() {
		for p := range c.Pushes() {
			switch p.Push {
			case "event":
				fmt.Println(eventLine(*p.Event))
			case "dropped":
				fmt.Printf("-- stream overflowed at seq %d; re-syncing --\n", p.LastSeq)
				since := p.LastSeq
				if err := c.Subscribe(&since); err != nil {
					return
				}
			}
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "quit", "exit", "q":
			fmt.Println("detached (the world keeps running)")
			return nil
		case "pause", "resume", "status":
			if sd, err := c.Status(fields[0], nil); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println(clockLine(sd))
			}
		case "speed":
			if len(fields) < 2 {
				fmt.Println("usage: speed <1x|4x|8x|16x|32x|max>")
				continue
			}
			if sd, err := c.Status("set_speed", ipc.SetSpeedArgs{Speed: fields[1]}); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println(clockLine(sd))
			}
		default:
			fmt.Printf("unknown command %q\n", fields[0])
		}
	}
	return scanner.Err()
}

func cmdTail(args []string) error {
	fs := flag.NewFlagSet("tail", flag.ContinueOnError)
	since := fs.Int64("since", -1, "start after this seq (default: last 20 events)")
	follow := fs.Bool("follow", false, "keep following live events (requires a running daemon)")
	dir, err := parseWorldFlags(fs, args)
	if err != nil {
		return err
	}
	w, err := world.Open(dir)
	if err != nil {
		return err
	}

	// History always comes read-only from the store, daemon or not.
	st, err := store.Open(w.DBPath())
	if err != nil {
		return err
	}
	from := *since
	if from < 0 {
		from = st.LastSeq() - 20
		if from < 0 {
			from = 0
		}
	}
	events, err := st.EventsSince(from, 0)
	if err != nil {
		st.Close()
		return err
	}
	last := from
	for _, e := range events {
		fmt.Println(eventLine(e))
		last = e.Seq
	}
	st.Close()

	if !*follow {
		return nil
	}
	c, err := ipc.Dial(w.SockPath())
	if err != nil {
		return fmt.Errorf("--follow needs a running daemon: %w", err)
	}
	defer c.Close()
	if err := c.Subscribe(&last); err != nil {
		return err
	}
	for p := range c.Pushes() {
		switch p.Push {
		case "event":
			fmt.Println(eventLine(*p.Event))
		case "dropped":
			since := p.LastSeq
			if err := c.Subscribe(&since); err != nil {
				return err
			}
		}
	}
	return nil
}

func clockLine(sd *ipc.StatusData) string {
	state := "running"
	if sd.Clock.Paused {
		state = "paused"
	}
	extra := ""
	if sd.Clock.Degraded {
		extra = " [degraded]"
	}
	return fmt.Sprintf("tick %d (%s) — %s, speed %s (%.1f ticks/s effective)%s",
		sd.Clock.Tick, sd.Clock.GameTime, state, sd.Clock.Speed, sd.Clock.EffectiveRate, extra)
}

func eventLine(e store.Event) string {
	return fmt.Sprintf("#%-6d t%-8d %-14s %-18s %s", e.Seq, e.Tick, clock.Format(e.Tick), e.Type, string(e.Payload))
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
