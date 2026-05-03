package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"yapl/internal/app"
	"yapl/internal/archive"
	"yapl/internal/client"
	"yapl/internal/config"
	"yapl/internal/server"
)

func main() {
	log.SetFlags(0)

	configName := flag.String("config", "", "Custom config file name (e.g. mod-a.json). Defaults to game.json or app.json.")
	upgradeProton := flag.Bool("upgrade-proton", false, "Force re-download of the Proton version.")
	packageFormat := flag.String("format", "xz", "Compression format for packaging: gz, xz, zst.")
	bundleDeps := flag.Bool("bundle-deps", false, "Bundle Proton and dependencies into the package for offline deployment.")
	bundleYes := flag.Bool("yes", false, "Skip confirmation prompts (e.g. for --bundle-deps size warning).")
	debugMode := flag.Bool("debug", false, "Enable verbose Proton logging for debugging.")
	isSteamPrefix := flag.Bool("steam", false, "Run as a Steam client prefix, ignoring the configured executable.")
	setupMethod := flag.String("method", "", "Launch method for a new config created by setup: direct, container, or umu.")

	// serve flags
	packagesDir := flag.String("packages-dir", "", "Directory containing packaged archives to serve.")
	servePort := flag.Int("port", 8471, "Port for the package server to listen on.")

	// shared auth flags (used by both serve and pull)
	authMode := flag.String("auth", "open", "Auth mode: open or password.")
	authPassword := flag.String("password", "", "Password for password auth mode.")

	// pull flags
	serverAddr := flag.String("server", "", "YAPL server address (host:port or URL). Auto-discovered when omitted.")
	outputDir := flag.String("output-dir", "", "Directory to save downloaded archive before unpackaging (default: system temp).")
	// Go's flag package stops at the first non-flag argument, so flags placed after
	// positional args (e.g. "yapl package game --bundle-deps") are never parsed.
	// Reorder args to move all flags before positional args before calling Parse.
	flag.CommandLine.Parse(reorderArgs(os.Args[1:]))

	if *setupMethod != "" && *setupMethod != "direct" && *setupMethod != "container" && *setupMethod != "umu" {
		log.Fatalf("❌ Invalid --method %q. Must be one of: direct, container, umu.", *setupMethod)
	}

	if flag.NArg() == 0 {
		printUsage()
		os.Exit(1)
	}

	command := flag.Arg(0)

	switch command {
	case "list":
		if err := app.ListAll(os.Stdout); err != nil {
			log.Fatalf("❌ List failed: %v", err)
		}

	case "serve":
		if *packagesDir == "" {
			log.Fatalf("❌ --packages-dir is required for 'serve'")
		}
		s, err := server.New(server.Config{
			PackagesDir: *packagesDir,
			Port:        *servePort,
			Auth:        *authMode,
			Password:    *authPassword,
		})
		if err != nil {
			log.Fatalf("❌ Server configuration error: %v", err)
		}
		if err := s.Start(); err != nil {
			log.Fatalf("❌ Server error: %v", err)
		}

	case "pull":
		addr := *serverAddr
		if addr == "" {
			fmt.Println("-> No --server specified; discovering YAPL server on LAN...")
			var discoverErr error
			addr, discoverErr = client.Discover(*servePort, 3*time.Second)
			if discoverErr != nil {
				log.Fatalf("❌ %v\n   Run 'yapl pull --server <host:port> ...' to specify manually.", discoverErr)
			}
			fmt.Printf("-> Found server at %s\n", addr)
		}
		c := client.New(client.Config{
			Server:   addr,
			Auth:     *authMode,
			Password: *authPassword,
		})
		subCmd := flag.Arg(1)
		switch subCmd {
		case "list", "":
			entries, err := c.List()
			if err != nil {
				log.Fatalf("❌ Pull list failed: %v", err)
			}
			fmt.Printf("%-30s %-8s %-6s %s\n", "NAME", "TYPE", "FORMAT", "SIZE")
			for _, e := range entries {
				fmt.Printf("%-30s %-8s %-6s %d MB\n", e.Name, e.Type, e.Format, e.SizeBytes>>20)
			}
		default:
			name := subCmd

			dlDir := *outputDir
			if dlDir == "" {
				var err error
				dlDir, err = os.MkdirTemp("", "yapl-pull-*")
				if err != nil {
					log.Fatalf("❌ Could not create temp dir: %v", err)
				}
				defer os.RemoveAll(dlDir)
			}

			archivePath, entryType, err := c.Download(name, dlDir, os.Stderr)
			if err != nil {
				log.Fatalf("❌ Download failed: %v", err)
			}
			fmt.Printf("-> Downloaded to %s\n", archivePath)

			targetDir := entryType + "s" // "game" → "games", "app" → "apps"
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				log.Fatalf("❌ Could not create %s: %v", targetDir, err)
			}
			if err := archive.Unpackage(targetDir, []string{archivePath}); err != nil {
				log.Fatalf("❌ Unpackage failed: %v", err)
			}
		}

	case "clean":
		name := flag.Arg(1)
		if name == "" {
			log.Fatalf("❌ Usage: yapl clean <name> [--prefix] [--proton] [--deps] [--all]")
		}
		cleanPrefix := flag.Bool("prefix", false, "Delete the game/app prefix directory.")
		cleanProton := flag.Bool("proton", false, "Delete the downloaded Proton version.")
		cleanDeps := flag.Bool("deps", false, "Delete DXVK and VKD3D for this game's configured versions.")
		cleanAll := flag.Bool("all", false, "Equivalent to --prefix --proton --deps.")
		flag.Parse()

		if !*cleanPrefix && !*cleanProton && !*cleanDeps && !*cleanAll {
			log.Fatalf("❌ Specify at least one of: --prefix, --proton, --deps, --all")
		}
		targets := app.CleanTargets{
			Prefix: *cleanPrefix || *cleanAll,
			Proton: *cleanProton || *cleanAll,
			Deps:   *cleanDeps || *cleanAll,
		}
		appType, appName, err := app.Find(name)
		if err != nil {
			log.Fatalf("❌ %v", err)
		}
		a, err := initializeApp(appType, appName, *configName, "", false, false, false)
		if err != nil {
			log.Fatalf("❌ %v", err)
		}
		if err := a.Clean(targets, *bundleYes, os.Stdout); err != nil {
			log.Fatalf("❌ Clean failed: %v", err)
		}

	case "unpackage":
		handleUnpackage()

	case "run", "package", "info":
		name := flag.Arg(1)
		if name == "" {
			log.Fatalf("❌ Usage: yapl %s <name>", command)
		}
		appType, appName, err := app.Find(name)
		if err != nil {
			log.Fatalf("❌ %v", err)
		}
		a, err := initializeApp(appType, appName, *configName, "", *upgradeProton, *debugMode, *isSteamPrefix)
		if err != nil {
			log.Fatalf("❌ %v", err)
		}
		switch command {
		case "run":
			if err := a.Run(); err != nil {
				log.Fatalf("❌ Run failed: %v", err)
			}
		case "package":
			if err := a.Package(*packageFormat, *bundleDeps, *bundleYes); err != nil {
				log.Fatalf("❌ Packaging failed: %v", err)
			}
		case "info":
			if err := a.Info(os.Stdout); err != nil {
				log.Fatalf("❌ Info failed: %v", err)
			}
		}

	case "setup":
		subType := flag.Arg(1)
		name := flag.Arg(2)
		if subType != "game" && subType != "app" {
			log.Fatalf("❌ Usage: yapl setup <game|app> <name>\n   Example: yapl setup game \"Need for Speed\"")
		}
		if name == "" {
			log.Fatalf("❌ Usage: yapl setup %s <name>", subType)
		}
		appType := subType + "s"
		a, err := initializeApp(appType, name, *configName, *setupMethod, *upgradeProton, *debugMode, *isSteamPrefix)
		if err != nil {
			log.Fatalf("❌ %v", err)
		}
		if err := a.Setup(); err != nil {
			log.Fatalf("❌ Setup failed: %v", err)
		}

	default:
		log.Fatalf("❌ Unknown command '%s'. Run 'yapl' with no arguments for usage.", command)
	}
}

func printUsage() {
	fmt.Println(`Usage: yapl <command> [arguments] [flags]

Commands:
  run      <name>             Launch a game or app
  setup    game|app <name>    Create prefix and download dependencies
  package  <name>             Create a distributable archive
  info     <name>             Show readiness status
  list                        List all configured games and apps
  unpackage game|app <files>  Extract one or more archives
  clean    <name>             Remove prefix, Proton, or dependency directories
  serve                       Start a LAN package server
  pull     list               List packages on a YAPL server (auto-discovered)
  pull     <name>             Download and unpackage a game from a YAPL server

Flags:
  --config <file>       Custom config file name (default: game.json / app.json)
  --method <method>     Launch method for a new config created by setup: direct, container, umu
  --format  xz|gz|zst  Compression format for package (default: xz)
  --bundle-deps         Bundle Proton and dependencies for offline deployment
  --yes                 Skip confirmation prompts
  --upgrade-proton      Force re-download of the configured Proton build
  --debug               Enable verbose Proton/Wine logging
  --steam               Steam client prefix mode (container launch only)

  clean flags:
  --prefix              Delete the game/app prefix directory
  --proton              Delete the downloaded Proton version
  --deps                Delete DXVK and VKD3D for this game's configured versions
  --all                 Equivalent to --prefix --proton --deps

  serve flags:
  --packages-dir <path>  Directory containing packaged archives (required)
                         Organise as: <dir>/games/*.tar.* and <dir>/apps/*.tar.*
  --port <int>           Port to listen on (default: 8471)
  --auth open|password   Auth mode (default: open)
  --password <string>    Password for password auth

  pull flags:
  --server <addr>        YAPL server address (host:port or URL)
                         Omit to auto-discover via UDP broadcast
  --auth open|password   Auth mode (default: open)
  --password <string>    Password for password auth
  --output-dir <path>    Where to save the downloaded archive (default: temp dir)

Examples:
  yapl setup game "Need for Speed Most Wanted"
  yapl run "Need for Speed Most Wanted"
  yapl package "Need for Speed Most Wanted" --bundle-deps
  yapl unpackage game NeedForSpeedMostWanted.tar.xz
  yapl clean "Need for Speed Most Wanted" --prefix
  yapl serve --packages-dir /srv/games
  yapl pull list
  yapl pull "Need for Speed Most Wanted"
  yapl pull --server 192.168.1.10:8471 "Need for Speed Most Wanted"`)
}

func initializeApp(appType, appName, configName, method string, force, debug, steam bool) (*app.App, error) {
	globalCfg, err := config.LoadOrCreateGlobal("runner.json")
	if err != nil {
		return nil, fmt.Errorf("could not load global config: %w", err)
	}
	appCfg, err := config.LoadOrCreateApp(appType, appName, configName, globalCfg, method)
	if err != nil {
		return nil, fmt.Errorf("could not load or create app config: %w", err)
	}
	return app.New(appType, appName, force, debug, steam, globalCfg, appCfg), nil
}

// reorderArgs moves all flag arguments (and their values) before positional
// arguments so that flag.Parse stops at the right place regardless of where
// the user placed the flags on the command line.
func reorderArgs(args []string) []string {
	var flagArgs, positionalArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionalArgs = append(positionalArgs, args[i:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") {
			positionalArgs = append(positionalArgs, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		// If this flag doesn't embed its value with "=", peek at the next arg.
		// If the flag is not boolean and the next arg is not itself a flag,
		// treat the next arg as the flag's value.
		if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			flagName := strings.TrimLeft(arg, "-")
			if f := flag.Lookup(flagName); f != nil && !isBoolFlag(f) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		}
	}
	return append(flagArgs, positionalArgs...)
}

func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}

func handleUnpackage() {
	args := flag.Args()[1:]
	if len(args) == 0 || (args[0] != "game" && args[0] != "app") {
		log.Fatalf("❌ Usage: yapl unpackage <game|app> <file> [<file>...]\n   Example: yapl unpackage game Doom.tar.xz")
	}
	subType := args[0]
	archives := args[1:]
	if len(archives) == 0 {
		log.Fatalf("❌ No archive files provided. Usage: yapl unpackage %s <file>", subType)
	}

	targetDir := subType + "s"
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Fatalf("❌ Could not create directory %s: %v", targetDir, err)
	}
	if err := archive.Unpackage(targetDir, archives); err != nil {
		log.Fatalf("❌ Unpackaging failed: %v", err)
	}
}
