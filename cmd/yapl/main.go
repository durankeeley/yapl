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

	// --- Flag Definition ---
	gameName := flag.String("game", "", "The name of the game directory inside ./games/.")
	appName := flag.String("app", "", "The name of the application directory inside ./apps/.")
	configName := flag.String("config", "", "Specify a custom config file name (e.g., mod-a.json). Defaults to game.json or app.json.")
	upgradeProton := flag.Bool("upgrade-proton", false, "Force re-download of the Proton version.")
	packageFormat := flag.String("format", "gz", "Compression format for packaging (gz, xz, zst).")
	bundleDeps := flag.Bool("bundle-deps", false, "Bundle Proton and dependencies into the package for offline deployment.")
	bundleYes := flag.Bool("yes", false, "Skip confirmation prompts (e.g. for --bundle-deps size warning).")
	debugMode := flag.Bool("debug", false, "Enable verbose Proton logging for debugging.")
	isSteamPrefix := flag.Bool("steam", false, "Run as a Steam client prefix, ignoring the configured executable.")
	flag.Parse()

	if flag.NArg() == 0 {
		log.Fatalf("❌ Error: No command provided. Use 'setup', 'package', 'unpackage', or 'run'.")
	}
	command := flag.Arg(0)

	// --- Command Dispatching ---
	if command == "unpackage" {
		handleUnpackage()
		return
	}

	if command == "list" {
		if err := app.ListAll(os.Stdout); err != nil {
			log.Fatalf("❌ List failed: %v", err)
		}
		return
	}

	a, err := initializeApp(*gameName, *appName, *configName, *upgradeProton, *debugMode, *isSteamPrefix)
	if err != nil {
		log.Fatalf("❌ Error initializing application: %v", err)
	}

	switch command {
	case "setup":
		if err := a.Setup(); err != nil {
			log.Fatalf("❌ Setup failed: %v", err)
		}
	case "package":
		if err := a.Package(*packageFormat, *bundleDeps, *bundleYes); err != nil {
			log.Fatalf("❌ Packaging failed: %v", err)
		}
	case "run":
		if err := a.Run(); err != nil {
			log.Fatalf("❌ Run failed: %v", err)
		}
	case "info":
		if err := a.Info(os.Stdout); err != nil {
			log.Fatalf("❌ Info failed: %v", err)
		}
	default:
		log.Fatalf("❌ Error: Unknown command '%s'.", command)
	}
}

// initializeApp determines the target, loads configuration, and constructs the main App object.
func initializeApp(gameName, appName, configName string, force, debug, steam bool) (*app.App, error) {
	if gameName == "" && appName == "" {
		return nil, fmt.Errorf("--game or --app flag is required")
	}

	targetType := "games"
	targetName := gameName
	if targetName == "" {
		targetType = "apps"
		targetName = appName
	}

	globalCfg, err := config.LoadOrCreateGlobal("runner.json")
	if err != nil {
		return nil, fmt.Errorf("could not load global config: %w", err)
	}

	appCfg, err := config.LoadOrCreateApp(targetType, targetName, configName, globalCfg)
	if err != nil {
		return nil, fmt.Errorf("could not load or create app config: %w", err)
	}

	return app.New(targetType, targetName, force, debug, steam, globalCfg, appCfg), nil
}

// handleUnpackage isolates the logic for the 'unpackage' command.
func handleUnpackage() {
	args := flag.Args()[1:]
	archiveType := "game" // Default type
	if len(args) > 0 && (args[0] == "app" || args[0] == "game") {
		archiveType = args[0]
		args = args[1:]
	}

	targetDir := archiveType + "s" // 'games' or 'apps'
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Fatalf("❌ Could not create directory %s: %v", targetDir, err)
	}

	if err := archive.Unpackage(targetDir, args); err != nil {
		log.Fatalf("❌ Unpackaging failed: %v", err)
	}
}
