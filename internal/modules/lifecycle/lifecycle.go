// Package lifecycle provides the commands that create, run, package and
// remove games and apps on this machine.
package lifecycle

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"yapl/internal/app"
	"yapl/internal/archive"
	"yapl/internal/catalog"
	"yapl/internal/config"
	"yapl/internal/defaults"
	"yapl/internal/kernel"
	"yapl/internal/paths"
)

// Version is bumped whenever a lifecycle command changes behaviour.
const Version = "1.2.0"

// Module registers the lifecycle commands with the kernel.
type Module struct{}

func (Module) Name() string    { return "lifecycle" }
func (Module) Version() string { return Version }

func (Module) Commands() []kernel.Command {
	return []kernel.Command{
		setupCommand(),
		installCommand(),
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
			a, err := newApp(ctx, ctx.Arg(0), ctx.Arg(1))
			if err != nil {
				return err
			}
			return a.Setup()
		},
	}
}

func installCommand() kernel.Command {
	return kernel.Command{
		Verb:        "install",
		Usage:       "install <game|app> <name> (--installer <file> | --from <dir>) [flags]",
		Summary:     "Install a game from an installer or a folder and detect its executable",
		Description: "Sets the game up if needed, then runs the installer inside the prefix or copies a portable game folder to drive_c/Games/<name>. The new executable that best matches the name is written to the config, unless the config already names a custom one.",
		Examples: []string{
			`yapl install game "Doom" --installer ~/Downloads/setup_doom.exe`,
			`yapl install game "Half-Life" --from /mnt/cdrom/Half-Life`,
			`yapl install game "Half-Life" --from ./hl --executable drive_c/Games/Half-Life/hl.exe`,
		},
		Flags: func(fs *flag.FlagSet) {
			launchFlags(fs)
			fs.String("method", "", "Launch `method` for a newly created config: "+strings.Join(defaults.LaunchMethods, ", ")+".")
			fs.String("installer", "", "Windows installer `file` to run inside the prefix.")
			fs.String("from", "", "Portable game `directory` to copy into the prefix.")
			fs.String("executable", "", "Executable `path` relative to the prefix; skips detection.")
		},
		Run: func(ctx *kernel.Context) error {
			opts := app.InstallOptions{Installer: ctx.String("installer"), From: ctx.String("from"), Executable: ctx.String("executable")}
			if (opts.Installer == "") == (opts.From == "") {
				return kernel.UsageError("pass exactly one of --installer or --from")
			}
			a, err := newApp(ctx, ctx.Arg(0), ctx.Arg(1))
			if err != nil {
				return err
			}
			return a.Install(opts, ctx.Stdout)
		},
	}
}

// newApp validates "<game|app> <name>" plus --method and loads or creates
// the config.
func newApp(ctx *kernel.Context, kind, name string) (*app.App, error) {
	if kind != paths.KindGame && kind != paths.KindApp {
		return nil, kernel.UsageError("the first argument must be game or app")
	}
	if name == "" {
		return nil, kernel.UsageError("a name is required")
	}
	method := ctx.String("method")
	if method != "" && !slices.Contains(defaults.LaunchMethods, method) {
		return nil, kernel.UsageError("invalid --method %q: use one of %s", method, strings.Join(defaults.LaunchMethods, ", "))
	}
	return loadApp(ctx, paths.TypeDir(kind), name, method)
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
		Verb:        "package",
		Usage:       "package <name> [flags]",
		Summary:     "Create a distributable archive with a manifest",
		Description: "Writes <name>.tar.<format>, plus <name>.manifest.json (title, version, SHA-256, required components) and the cover art when the game has one. metadata.json is created in the game directory if missing.",
		Examples: []string{
			`yapl package "Doom"`,
			`yapl package "Doom" --version 1.2.0 --output-dir ./dist`,
			`yapl package "Doom" --format zst --bundle-deps --yes`,
		},
		Flags: func(fs *flag.FlagSet) {
			configFlag(fs)
			fs.String("format", defaults.PackageFormat, "Compression `format`: "+strings.Join(defaults.PackageFormats, ", ")+".")
			fs.Bool("bundle-deps", false, "Bundle Proton and dependencies into the archive for offline machines.")
			fs.Bool("yes", false, "Skip confirmation prompts.")
			fs.String("version", "", "Package `version` (MAJOR.MINOR.PATCH); saved to metadata.json.")
			fs.String("output-dir", ".", "Directory `path` to write the archive and its sidecar files to.")
			fs.Bool("fetch-cover", false, "Download cover art from Steam when steam_app_id is set and the game has no cover.")
		},
		Run: func(ctx *kernel.Context) error {
			a, err := findApp(ctx)
			if err != nil {
				return err
			}
			_, err = a.Package(app.PackageOptions{
				Format:     ctx.String("format"),
				BundleDeps: ctx.Bool("bundle-deps"),
				Yes:        ctx.Bool("yes"),
				OutputDir:  ctx.String("output-dir"),
				Version:    ctx.String("version"),
				FetchCover: ctx.Bool("fetch-cover"),
				CreatedBy:  "yapl lifecycle/" + Version,
			})
			return err
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
			if kind != paths.KindGame && kind != paths.KindApp {
				return kernel.UsageError("the first argument must be game or app")
			}
			archives := ctx.Args[1:]
			if len(archives) == 0 {
				return kernel.UsageError("at least one archive file is required")
			}
			targetDir := paths.TypeDir(kind)
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
	globalCfg, err := config.LoadOrCreateGlobal(paths.RunnerConfig, catalog.Default())
	if err != nil {
		return nil, fmt.Errorf("could not load global config: %w", err)
	}
	appCfg, err := config.LoadOrCreateApp(appType, name, ctx.String("config"), globalCfg, method)
	if err != nil {
		return nil, fmt.Errorf("could not load or create app config: %w", err)
	}
	a := app.New(appType, name, ctx.Bool("upgrade-proton"), ctx.Bool("debug"), ctx.Bool("steam"), globalCfg, appCfg)
	if custom := ctx.String("config"); custom != "" {
		a.ConfigFile = filepath.Join(a.AppDir, custom)
	}
	return a, nil
}
