// Package lifecycle provides the commands that create, run, package and
// remove games and apps on this machine.
package lifecycle

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"yapl/internal/app"
	"yapl/internal/archive"
	"yapl/internal/config"
	"yapl/internal/defaults"
	"yapl/internal/kernel"
)

// Version is bumped whenever a lifecycle command changes behaviour.
const Version = "1.0.0"

const (
	kindGame = "game"
	kindApp  = "app"
)

// Module registers the lifecycle commands with the kernel.
type Module struct{}

func (Module) Name() string    { return "lifecycle" }
func (Module) Version() string { return Version }

func (Module) Commands() []kernel.Command {
	return []kernel.Command{
		setupCommand(),
		runCommand(),
		packageCommand(),
		unpackageCommand(),
		infoCommand(),
		listCommand(),
		cleanCommand(),
	}
}

func configFlag(fs *flag.FlagSet) {
	fs.String("config", "", "Config `file` name inside the game directory (default game.json or app.json).")
}

func launchFlags(fs *flag.FlagSet) {
	configFlag(fs)
	fs.Bool("upgrade-proton", false, "Force a fresh download of the configured Proton build.")
	fs.Bool("debug", false, "Enable verbose Proton and Wine logging.")
	fs.Bool("steam", false, "Run as a Steam client prefix, ignoring the configured executable (container method only).")
}

func setupCommand() kernel.Command {
	return kernel.Command{
		Verb:        "setup",
		Usage:       "setup <game|app> <name> [flags]",
		Summary:     "Create a prefix and download dependencies",
		Description: "Creates games/<name>/ (or apps/<name>/) with a default config when missing, downloads Proton and dependencies, and initialises the Wine prefix.",
		Examples:    []string{`yapl setup game "Need for Speed Most Wanted"`, `yapl setup app "Notepad++" --method direct`},
		Flags: func(fs *flag.FlagSet) {
			launchFlags(fs)
			fs.String("method", "", "Launch `method` for a newly created config: "+strings.Join(defaults.LaunchMethods, ", ")+".")
		},
		Run: func(ctx *kernel.Context) error {
			kind, name := ctx.Arg(0), ctx.Arg(1)
			if kind != kindGame && kind != kindApp {
				return kernel.UsageError("the first argument must be game or app")
			}
			if name == "" {
				return kernel.UsageError("a name is required")
			}
			method := ctx.String("method")
			if method != "" && !slices.Contains(defaults.LaunchMethods, method) {
				return kernel.UsageError("invalid --method %q: use one of %s", method, strings.Join(defaults.LaunchMethods, ", "))
			}
			a, err := loadApp(ctx, kind+"s", name, method)
			if err != nil {
				return err
			}
			return a.Setup()
		},
	}
}

func runCommand() kernel.Command {
	return kernel.Command{
		Verb:     "run",
		Usage:    "run <name> [flags]",
		Summary:  "Launch a game or app",
		Examples: []string{`yapl run "Doom"`, `yapl run "HalfLife" --config cstrike.json --debug`},
		Flags:    launchFlags,
		Run: func(ctx *kernel.Context) error {
			a, err := findApp(ctx)
			if err != nil {
				return err
			}
			return a.Run()
		},
	}
}

func packageCommand() kernel.Command {
	return kernel.Command{
		Verb:     "package",
		Usage:    "package <name> [flags]",
		Summary:  "Create a distributable archive",
		Examples: []string{`yapl package "Doom"`, `yapl package "Doom" --format zst --bundle-deps --yes`},
		Flags: func(fs *flag.FlagSet) {
			configFlag(fs)
			fs.String("format", defaults.PackageFormat, "Compression `format`: "+strings.Join(defaults.PackageFormats, ", ")+".")
			fs.Bool("bundle-deps", false, "Bundle Proton and dependencies into the archive for offline machines.")
			fs.Bool("yes", false, "Skip confirmation prompts.")
		},
		Run: func(ctx *kernel.Context) error {
			a, err := findApp(ctx)
			if err != nil {
				return err
			}
			return a.Package(ctx.String("format"), ctx.Bool("bundle-deps"), ctx.Bool("yes"))
		},
	}
}

func unpackageCommand() kernel.Command {
	return kernel.Command{
		Verb:        "unpackage",
		Usage:       "unpackage <game|app> <file>... [flags]",
		Summary:     "Extract one or more archives",
		Description: "Extracts each archive into games/ or apps/. Bundled Proton and dependencies are installed and registered in runner.json.",
		Examples:    []string{"yapl unpackage game Doom.tar.xz", "yapl unpackage game Doom.tar.xz Quake.tar.zst"},
		Run: func(ctx *kernel.Context) error {
			kind := ctx.Arg(0)
			if kind != kindGame && kind != kindApp {
				return kernel.UsageError("the first argument must be game or app")
			}
			archives := ctx.Args[1:]
			if len(archives) == 0 {
				return kernel.UsageError("at least one archive file is required")
			}
			targetDir := kind + "s"
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return fmt.Errorf("create %s: %w", targetDir, err)
			}
			return archive.Unpackage(targetDir, archives)
		},
	}
}

func infoCommand() kernel.Command {
	return kernel.Command{
		Verb:     "info",
		Usage:    "info <name> [flags]",
		Summary:  "Show readiness status",
		Examples: []string{`yapl info "Doom"`},
		Flags:    configFlag,
		Run: func(ctx *kernel.Context) error {
			a, err := findApp(ctx)
			if err != nil {
				return err
			}
			return a.Info(ctx.Stdout)
		},
	}
}

func listCommand() kernel.Command {
	return kernel.Command{
		Verb:    "list",
		Usage:   "list",
		Summary: "List all configured games and apps",
		Run: func(ctx *kernel.Context) error {
			return app.ListAll(ctx.Stdout)
		},
	}
}

func cleanCommand() kernel.Command {
	return kernel.Command{
		Verb:     "clean",
		Usage:    "clean <name> [flags]",
		Summary:  "Remove prefix, Proton, or dependency directories",
		Examples: []string{`yapl clean "Doom" --prefix`, `yapl clean "Doom" --all --yes`},
		Flags: func(fs *flag.FlagSet) {
			configFlag(fs)
			fs.Bool("prefix", false, "Delete the game or app prefix directory.")
			fs.Bool("proton", false, "Delete the downloaded Proton version.")
			fs.Bool("deps", false, "Delete DXVK and VKD3D for this game's configured versions.")
			fs.Bool("all", false, "Equivalent to --prefix --proton --deps.")
			fs.Bool("yes", false, "Skip the confirmation prompt.")
		},
		Run: func(ctx *kernel.Context) error {
			all := ctx.Bool("all")
			targets := app.CleanTargets{
				Prefix: ctx.Bool("prefix") || all,
				Proton: ctx.Bool("proton") || all,
				Deps:   ctx.Bool("deps") || all,
			}
			if ctx.Arg(0) == "" {
				return kernel.UsageError("a name is required")
			}
			if !targets.Prefix && !targets.Proton && !targets.Deps {
				return kernel.UsageError("specify at least one of --prefix, --proton, --deps, --all")
			}
			a, err := findApp(ctx)
			if err != nil {
				return err
			}
			return a.Clean(targets, ctx.Bool("yes"), ctx.Stdout)
		},
	}
}

// findApp resolves the name in the first argument to an existing game or app.
func findApp(ctx *kernel.Context) (*app.App, error) {
	name := ctx.Arg(0)
	if name == "" {
		return nil, kernel.UsageError("a name is required")
	}
	appType, appName, err := app.Find(name)
	if err != nil {
		return nil, err
	}
	return loadApp(ctx, appType, appName, "")
}

func loadApp(ctx *kernel.Context, appType, name, method string) (*app.App, error) {
	globalCfg, err := config.LoadOrCreateGlobal("runner.json")
	if err != nil {
		return nil, fmt.Errorf("could not load global config: %w", err)
	}
	appCfg, err := config.LoadOrCreateApp(appType, name, ctx.String("config"), globalCfg, method)
	if err != nil {
		return nil, fmt.Errorf("could not load or create app config: %w", err)
	}
	return app.New(appType, name, ctx.Bool("upgrade-proton"), ctx.Bool("debug"), ctx.Bool("steam"), globalCfg, appCfg), nil
}
