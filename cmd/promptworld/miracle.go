package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/evanstern/promptworld/internal/ipc"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/world"
)

const miracleUsage = `usage:
  promptworld miracle <world> snap-time <day> <HH:MM>       [--force]
  promptworld miracle <world> give <villager> <item> <qty>  [--force]
  promptworld miracle <world> move <class> <x,y> <x1,y1>    [--force]
  promptworld miracle <world> remove <class> <x,y>          [--force]

<class> is villager|structure|pile|terrain (terrain is remove-only; villager
cannot be removed). --force waives the charge — the operator override the angel
structurally cannot reach.`

// cmdMiracle is the operator door for Metatron's miracles (spec 016 R6): a
// dedicated subcommand family backed by the daemon's "miracle" IPC command. It
// spends charges like the angel unless --force is given; exit 0 on a landed
// miracle (summary + remaining charges), exit 1 with the door/reducer reason.
func cmdMiracle(args []string) error {
	// --force may sit anywhere; pull it out and keep the positional order.
	var pos []string
	gratis := false
	for _, a := range args {
		if a == "--force" || a == "-force" {
			gratis = true
			continue
		}
		pos = append(pos, a)
	}
	if len(pos) < 2 {
		return fmt.Errorf("%s", miracleUsage)
	}
	worldName, verb, rest := pos[0], pos[1], pos[2:]

	ma := ipc.MiracleArgs{Gratis: gratis}
	switch verb {
	case "snap-time":
		if len(rest) != 2 {
			return fmt.Errorf("snap-time needs <day> <HH:MM>\n%s", miracleUsage)
		}
		day, err := strconv.Atoi(rest[0])
		if err != nil {
			return fmt.Errorf("bad day %q: %w", rest[0], err)
		}
		ma.Kind, ma.Day, ma.Time = "time_snap", day, rest[1]
	case "give":
		if len(rest) != 3 {
			return fmt.Errorf("give needs <villager> <item> <qty>\n%s", miracleUsage)
		}
		qty, err := strconv.Atoi(rest[2])
		if err != nil {
			return fmt.Errorf("bad qty %q: %w", rest[2], err)
		}
		ma.Kind, ma.Villager, ma.Item, ma.Qty = "give_item", rest[0], rest[1], qty
	case "move":
		if len(rest) != 3 {
			return fmt.Errorf("move needs <class> <x,y> <x1,y1>\n%s", miracleUsage)
		}
		x, y, err := parseCoord(rest[1])
		if err != nil {
			return err
		}
		tx, ty, err := parseCoord(rest[2])
		if err != nil {
			return err
		}
		ma.Kind, ma.Class, ma.X, ma.Y, ma.ToX, ma.ToY = "move", rest[0], x, y, tx, ty
	case "remove":
		if len(rest) != 2 {
			return fmt.Errorf("remove needs <class> <x,y>\n%s", miracleUsage)
		}
		x, y, err := parseCoord(rest[1])
		if err != nil {
			return err
		}
		ma.Kind, ma.Class, ma.X, ma.Y = "remove", rest[0], x, y
	default:
		return fmt.Errorf("unknown miracle verb %q\n%s", verb, miracleUsage)
	}

	dir, err := resolveWorld(worldName)
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

	data, err := c.Call("miracle", ma)
	if err != nil {
		return err
	}
	var md ipc.MiracleData
	if err := json.Unmarshal(data, &md); err != nil {
		return err
	}
	force := ""
	if md.Gratis {
		force = " (forced)"
	}
	fmt.Printf("%s%s\n[charges %s %d/%d]\n", md.Summary, force, chargeGlyphs(md.Charges), md.Charges, sim.MetatronChargeCap)
	return nil
}

// parseCoord reads an "x,y" tile coordinate.
func parseCoord(s string) (int, int, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("bad coordinate %q (want x,y)", s)
	}
	x, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("bad coordinate %q: %w", s, err)
	}
	y, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("bad coordinate %q: %w", s, err)
	}
	return x, y, nil
}
