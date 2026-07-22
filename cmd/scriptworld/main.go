// Command scriptworld is the single binary for the script-world daemon and
// its clients. See specs/001-world-daemon/contracts/cli.md for the contract.
package main

import (
	"fmt"
	"os"
)

const usage = `scriptworld — the always-on script-world daemon and client

A <world> argument below is a name or a path: a name (e.g. "aria") resolves
against the default worlds home (~/.scriptworld/worlds, overridable via
SCRIPTWORLD_HOME) then the known-worlds list; a path (contains "/", or
starts with "." or "~") is used exactly as given.

Usage:
  scriptworld new <name> [--at DIR] [--seed N]     create a world by name in the worlds home
  scriptworld new <path> [--name NAME] [--seed N]  create a world at an explicit path (legacy form)
  scriptworld migrate <world>                      migrate a stopped v1 world to the current format
  scriptworld ps [--all] [--json]                  list world daemons machine-wide
  scriptworld daemon <world>                       run the daemon in the foreground
  scriptworld start <world>                        start a detached daemon
  scriptworld stop <world>                         gracefully stop the daemon
  scriptworld status <world> [--json]              report world/daemon status
  scriptworld ui <world>                           full-screen TUI (map, chronicle, metatron, souls)
  scriptworld attach <world>                       line-mode event stream + commands
  scriptworld tail <world> [--since SEQ] [--follow] print events from the log
  scriptworld pause <world>                        pause game time
  scriptworld resume <world>                       resume game time
  scriptworld speed <world> <1x|4x|8x|16x|32x|max>     set game speed
  scriptworld metatron <world> [message...]        converse with the angel (no message: status peek)
  scriptworld llm <world> <kind> <prompt...>       one-shot LLM call via the daemon
                                                   (kinds: planner, conversation, musing,
                                                    consolidation, narrator, drama)
  scriptworld calibrate <world> [--tier local|cloud|all] [--samples N]
                                                   benchmark seconds-per-point, write calibration.json
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
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "scriptworld: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "scriptworld %s: %v\n", cmd, err)
		os.Exit(1)
	}
}
