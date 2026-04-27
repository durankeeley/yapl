package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"yapl/internal/app"
	"yapl/internal/archive"
	"yapl/internal/config"
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
	flag.Parse()

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
  run      <name>              Launch a game or app
  setup    game|app <name>     Create prefix and download dependencies
  package  <name>              Create a distributable archive
  info     <name>              Show readiness status
  list                         List all configured games and apps
  unpackage game|app <files>   Extract one or more archives

Flags:
  --config <file>    Custom config file name (default: game.json / app.json)
  --method <method>  Launch method for a new config created by setup: direct, container, umu
  --format  xz|gz|zst  Compression format for package (default: xz)
  --bundle-deps      Bundle Proton and dependencies for offline deployment
  --yes              Skip confirmation prompts
  --upgrade-proton   Force re-download of the configured Proton build
  --debug            Enable verbose Proton/Wine logging
  --steam            Steam client prefix mode (container launch only)

Examples:
  yapl setup game "Need for Speed Most Wanted"
  yapl run "Need for Speed Most Wanted"
  yapl package "Need for Speed Most Wanted" --bundle-deps
  yapl unpackage game NeedForSpeedMostWanted.tar.xz`)
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
