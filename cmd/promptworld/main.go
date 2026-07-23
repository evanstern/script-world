// Command promptworld is the single binary for the promptworld daemon and
// its clients. See specs/001-world-daemon/contracts/cli.md for the contract.
package main

import (
	"fmt"
	"os"
)

const usage = `promptworld — the always-on promptworld daemon and client

A <world> argument below is a name or a path: a name (e.g. "aria") resolves
against the default worlds home (~/.promptworld/worlds, overridable via
PROMPTWORLD_HOME) then the known-worlds list; a path (contains "/", or
starts with "." or "~") is used exactly as given.

Usage:
  promptworld new <name> [--at DIR] [--seed N]     create a world by name in the worlds home
  promptworld new <path> [--name NAME] [--seed N]  create a world at an explicit path (legacy form)
  promptworld migrate <world>                      migrate a stopped older world (v1/v2) to the current format
  promptworld ps [--all] [--json]                  list world daemons machine-wide
  promptworld daemon <world>                       run the daemon in the foreground
  promptworld start <world>                        start a detached daemon
  promptworld stop <world>                         gracefully stop the daemon
  promptworld status <world> [--json]              report world/daemon status
  promptworld ui <world>                           full-screen TUI (map, chronicle, metatron, villagers)
  promptworld attach <world>                       line-mode event stream + commands
  promptworld tail <world> [--since SEQ] [--follow] print events from the log
  promptworld pause <world>                        pause game time
  promptworld resume <world>                       resume game time
  promptworld speed <world> <1x|4x|8x|16x|32x|max>     set game speed
  promptworld metatron <world> [message...]        converse with the angel (no message: status peek)
  promptworld miracle <world> <snap-time|give|move|remove> ... [--force]
                                                   land a Metatron miracle (--force waives the charge)
  promptworld llm <world> <kind> <prompt...>       one-shot LLM call via the daemon; prints the
                                                   serving provider and any fallback skips
                                                   (kinds: planner, conversation,
                                                    consolidation, narrator, drama)
  promptworld calibrate <world> [--provider name | --tier local|cloud|all] [--samples N]
                                                   benchmark seconds-per-point per declared
                                                   provider, write calibration.json
                                                   (--tier is a deprecated alias, see --help)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "new":
		err = cmdNew(args)
	case "migrate":
		err = cmdMigrate(args)
	case "ps":
		err = cmdPs(args)
	case "daemon":
		err = cmdDaemon(args)
	case "start":
		err = cmdStart(args)
	case "stop":
		err = cmdStop(args)
	case "status":
		err = cmdStatus(args)
	case "ui":
		err = cmdUI(args)
	case "attach":
		err = cmdAttach(args)
	case "tail":
		err = cmdTail(args)
	case "pause":
		err = cmdTimeCtl("pause", args)
	case "resume":
		err = cmdTimeCtl("resume", args)
	case "speed":
		err = cmdSpeed(args)
	case "llm":
		err = cmdLLM(args)
	case "calibrate":
		err = cmdCalibrate(args)
	case "metatron":
		err = cmdMetatron(args)
	case "miracle":
		err = cmdMiracle(args)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "promptworld: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "promptworld %s: %v\n", cmd, err)
		os.Exit(1)
	}
}
