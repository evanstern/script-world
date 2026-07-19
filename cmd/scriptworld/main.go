// Command scriptworld is the single binary for the script-world daemon and
// its clients. See specs/001-world-daemon/contracts/cli.md for the contract.
package main

import (
	"fmt"
	"os"
)

const usage = `scriptworld — the always-on script-world daemon and client

Usage:
  scriptworld new <dir> [--name NAME] [--seed N]   create a new world
  scriptworld daemon <dir>                         run the daemon in the foreground
  scriptworld start <dir>                          start a detached daemon
  scriptworld stop <dir>                           gracefully stop the daemon
  scriptworld status <dir> [--json]                report world/daemon status
  scriptworld attach <dir>                         interactive event stream + commands
  scriptworld tail <dir> [--since SEQ] [--follow]  print events from the log
  scriptworld pause <dir>                          pause game time
  scriptworld resume <dir>                         resume game time
  scriptworld speed <dir> <1x|4x|8x|16x|max>       set game speed
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
	case "daemon":
		err = cmdDaemon(args)
	case "start":
		err = cmdStart(args)
	case "stop":
		err = cmdStop(args)
	case "status":
		err = cmdStatus(args)
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
