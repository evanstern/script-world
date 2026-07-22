package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/evanstern/promptworld/internal/llm"
	"github.com/evanstern/promptworld/internal/worlds"
)

// cmdPs implements `promptworld ps [--all] [--json]` (specs/008-instance-manager,
// User Story 1): a machine-wide, from-any-directory listing of world daemons,
// re-proven live at query time (FR-002) — never from records.
func cmdPs(args []string) error {
	fs := flag.NewFlagSet("ps", flag.ContinueOnError)
	all := fs.Bool("all", false, "also list stopped, missing, and unreadable worlds")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	candidates, err := worlds.Discover()
	if err != nil {
		return err
	}
	instances := worlds.Probe(candidates) // already sorted by name (Discover)

	if !*all {
		instances = liveOnly(instances)
	}

	if *asJSON {
		return printJSON(psRows(instances))
	}
	printPsTable(instances)
	return nil
}

// liveOnly keeps only the states a live-pid probe can produce: running,
// paused, unresponsive. Non-live states (stopped/missing/unreadable) are
// `--all`-only (contracts/cli.md).
func liveOnly(in []worlds.Instance) []worlds.Instance {
	out := make([]worlds.Instance, 0, len(in))
	for _, inst := range in {
		switch inst.State {
		case worlds.Running, worlds.Paused, worlds.Unresponsive:
			out = append(out, inst)
		}
	}
	return out
}

func printPsTable(instances []worlds.Instance) {
	if len(instances) == 0 {
		fmt.Println("no worlds running")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSTATE\tPID\tTICK\tGAME TIME\tSPEED\tLLM\tPATH")
	for _, inst := range instances {
		pid := "-"
		if inst.Pid > 0 {
			pid = strconv.Itoa(inst.Pid)
		}
		tick, gameTime, speed, llmOn := psTableFields(inst)
		llm := "off"
		if llmOn {
			llm = "on"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			inst.Name, inst.State, pid, tick, gameTime, speed, llm, inst.Path)
	}
	tw.Flush()
}

// psTableFields resolves the display strings that differ by state: live
// rows read the fresh status reply, stopped rows read the last-known
// snapshot, everything else (unresponsive/missing/unreadable) shows dashes.
func psTableFields(inst worlds.Instance) (tick, gameTime, speed string, llmOn bool) {
	switch {
	case inst.Status != nil:
		return strconv.FormatInt(inst.Status.Clock.Tick, 10), inst.Status.Clock.GameTime, inst.Status.Clock.Speed, inst.Status.LLM != nil
	case inst.State == worlds.Stopped:
		return strconv.FormatInt(inst.Tick, 10), inst.GameTime, inst.OfflineSpeed, inst.LLMConfigured
	default:
		return "-", "-", "-", false
	}
}

func psRows(instances []worlds.Instance) []psRow {
	rows := make([]psRow, len(instances))
	for i, inst := range instances {
		rows[i] = psRowFor(inst)
	}
	return rows
}

// psRow is the `--json` element shape (contracts/cli.md `ps`): it reuses
// the `status --json` vocabulary (world/clock/daemon/llm) plus name/path/state.
type psRow struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`

	World         *psWorld    `json:"world,omitempty"`
	Clock         *psClock    `json:"clock,omitempty"`
	Daemon        *psDaemon   `json:"daemon,omitempty"`
	LLM           *llm.Status `json:"llm,omitempty"`
	LLMConfigured *bool       `json:"llm_configured,omitempty"`
}

type psWorld struct {
	Name          string `json:"name"`
	Seed          uint64 `json:"seed"`
	FormatVersion int    `json:"format_version"`
}

type psClock struct {
	Tick            int64   `json:"tick"`
	GameTime        string  `json:"game_time"`
	Paused          bool    `json:"paused"`
	Speed           string  `json:"speed"`
	EffectiveRate   float64 `json:"effective_rate,omitempty"`
	Degraded        bool    `json:"degraded,omitempty"`
	MetatronCharges int     `json:"metatron_charges,omitempty"`
}

type psDaemon struct {
	Pid           int   `json:"pid,omitempty"`
	UptimeSeconds int64 `json:"uptime_seconds,omitempty"`
	Subscribers   int   `json:"subscribers,omitempty"`
	Running       *bool `json:"running,omitempty"`
}

func psRowFor(inst worlds.Instance) psRow {
	row := psRow{Name: inst.Name, Path: inst.Path, State: string(inst.State)}
	switch inst.State {
	case worlds.Missing:
		return row
	case worlds.Unreadable:
		row.Error = inst.Error
		return row
	}

	row.World = &psWorld{Name: inst.WorldName, Seed: inst.Seed, FormatVersion: inst.FormatVersion}

	if inst.Status != nil {
		sd := inst.Status
		row.Clock = &psClock{
			Tick: sd.Clock.Tick, GameTime: sd.Clock.GameTime, Paused: sd.Clock.Paused,
			Speed: sd.Clock.Speed, EffectiveRate: sd.Clock.EffectiveRate,
			Degraded: sd.Clock.Degraded, MetatronCharges: sd.Clock.MetatronCharges,
		}
		row.Daemon = &psDaemon{Pid: sd.Daemon.Pid, UptimeSeconds: sd.Daemon.UptimeSeconds, Subscribers: sd.Daemon.Subscribers}
		row.LLM = sd.LLM
		return row
	}

	// unresponsive (live pid, no reply) or stopped (offline last-known).
	row.Daemon = &psDaemon{}
	if inst.Pid > 0 {
		row.Daemon.Pid = inst.Pid // unresponsive: a pid exists, "running" would be misleading either way
	} else {
		notRunning := false
		row.Daemon.Running = &notRunning
	}
	if inst.State == worlds.Stopped {
		row.Clock = &psClock{Tick: inst.Tick, GameTime: inst.GameTime, Paused: inst.OfflinePaused, Speed: inst.OfflineSpeed}
		llmConfigured := inst.LLMConfigured
		row.LLMConfigured = &llmConfigured
	}
	return row
}
