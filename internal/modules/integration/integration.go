// Package integration provides `yapl export`: menu entries, Steam
// shortcuts and login autostart, so games are reachable from the couch
// interfaces people already use.
package integration

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"yapl/internal/console"
	"yapl/internal/integration/desktop"
	"yapl/internal/integration/steam"
	"yapl/internal/kernel"
)

// Version is bumped whenever an export target changes.
const Version = "1.0.0"

const (
	targetDesktop   = "desktop"
	targetSteam     = "steam"
	targetAutostart = "autostart"

	entryPrefix      = "yapl-"
	entrySuffix      = ".desktop"
	consoleEntryName = "YAPL Console"
	consoleEntryFile = entryPrefix + "console" + entrySuffix
	gamesCategory    = "Game"
)

// steamRunning is a variable so tests can simulate a running Steam.
var steamRunning = steam.Running

// Module registers the export command.
type Module struct{}

func (Module) Name() string    { return "integration" }
func (Module) Version() string { return Version }

func (Module) Commands() []kernel.Command {
	return []kernel.Command{{
		Verb:    "export",
		Usage:   "export <desktop|steam|autostart> [name] [flags]",
		Summary: "Add games to the app menu or Steam, or start the console on login",
		Description: "desktop writes a menu entry per installed game (and one for the console) to $XDG_DATA_HOME/applications. " +
			"steam adds non-Steam shortcuts with cover art for every Steam user, so Big Picture and the Steam Deck can launch them; Steam must be closed. " +
			"autostart starts 'yapl console' when you log in. Entries run this yapl binary with --workdir pointing at this library.",
		Examples: []string{
			"yapl export desktop",
			`yapl export desktop "Doom"`,
			"yapl export steam",
			"yapl export autostart",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.String("output-dir", "", "Write desktop or autostart entries to this `path` instead of the XDG default.")
			fs.String("steam-root", "", "Steam installation `path` (default ~/.local/share/Steam or ~/.steam/steam).")
			fs.String("steam-user", "", "Only export to this Steam user `id`.")
			fs.Bool("force", false, "Export to Steam even while it is running (Steam may overwrite the change when it exits).")
		},
		Run: run,
	}}
}

type target struct {
	exe  string
	root string
}

func run(ctx *kernel.Context) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate yapl: %w", err)
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	t := target{exe: exe, root: root}
	switch ctx.Arg(0) {
	case targetDesktop:
		return t.desktop(ctx)
	case targetSteam:
		return t.steam(ctx)
	case targetAutostart:
		return t.autostart(ctx)
	default:
		return kernel.UsageError("export target must be desktop, steam or autostart")
	}
}

// games returns the installed games to export: one by name, or all that
// are ready to play.
func (t target) games(ctx *kernel.Context) ([]console.Game, error) {
	if name := ctx.Arg(1); name != "" {
		g, _, err := console.FindGame(name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return []console.Game{g}, nil
	}
	all, err := console.ListLibrary()
	if err != nil {
		return nil, err
	}
	var ready []console.Game
	for _, g := range all {
		if g.Ready {
			ready = append(ready, g)
		} else {
			fmt.Fprintf(ctx.Stdout, "-> Skipping %s: no executable set yet\n", g.Name)
		}
	}
	return ready, nil
}

func (t target) cover(name string) string {
	_, cover, _ := console.FindGame(name)
	if cover == "" {
		return ""
	}
	return filepath.Join(t.root, cover)
}

func (t target) desktop(ctx *kernel.Context) error {
	games, err := t.games(ctx)
	if err != nil {
		return err
	}
	dir := ctx.String("output-dir")
	if dir == "" {
		dir = desktop.ApplicationsDir()
	}
	for _, g := range games {
		path, err := desktop.Write(dir, entryPrefix+desktop.Slug(g.Name)+entrySuffix, desktop.Entry{
			Name: g.Title, Comment: "Play " + g.Title + " with YAPL",
			Exec: []string{t.exe, "--workdir", t.root, "run", g.Name}, Icon: t.cover(g.Name), Categories: []string{gamesCategory},
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(ctx.Stdout, "-> Wrote %s\n", path)
	}
	if ctx.Arg(1) == "" {
		path, err := desktop.Write(dir, consoleEntryFile, t.consoleEntry())
		if err != nil {
			return err
		}
		fmt.Fprintf(ctx.Stdout, "-> Wrote %s\n", path)
	}
	return nil
}

func (t target) consoleEntry() desktop.Entry {
	return desktop.Entry{
		Name: consoleEntryName, Comment: "Pick a game and play",
		Exec: []string{t.exe, "--workdir", t.root, "console"}, Categories: []string{gamesCategory},
	}
}

func (t target) autostart(ctx *kernel.Context) error {
	dir := ctx.String("output-dir")
	if dir == "" {
		dir = desktop.AutostartDir()
	}
	e := t.consoleEntry()
	e.Extra = [][2]string{{"X-GNOME-Autostart-enabled", "true"}}
	path, err := desktop.Write(dir, consoleEntryFile, e)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout, "-> The console will start on login (%s)\n", path)
	return nil
}

func (t target) steam(ctx *kernel.Context) error {
	if steamRunning() && !ctx.Bool("force") {
		return fmt.Errorf("Steam is running; close Steam first (it rewrites its shortcuts when it exits) or pass --force")
	}
	root := ctx.String("steam-root")
	if root == "" {
		found, err := steam.FindRoot()
		if err != nil {
			return err
		}
		root = found
	}
	configs, err := steam.UserConfigDirs(root, ctx.String("steam-user"))
	if err != nil {
		return err
	}
	games, err := t.games(ctx)
	if err != nil {
		return err
	}
	items := make([]steam.Item, 0, len(games))
	for _, g := range games {
		items = append(items, steam.Item{Cover: t.cover(g.Name), Shortcut: steam.Shortcut{
			AppName:       g.Title,
			Exe:           quote(t.exe),
			StartDir:      quote(t.root),
			Icon:          t.cover(g.Name),
			LaunchOptions: "--workdir " + quote(t.root) + " run " + quote(g.Name),
		}})
	}
	for _, config := range configs {
		added, err := steam.Export(config, items)
		if err != nil {
			return err
		}
		fmt.Fprintf(ctx.Stdout, "-> %s: %d added, %d updated\n", config, added, len(items)-added)
	}
	return nil
}

// quote wraps a value in double quotes the way Steam expects for Exe,
// StartDir and launch arguments.
func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
