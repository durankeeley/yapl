package app

import (
	"fmt"
	"path/filepath"

	"yapl/internal/archive"
	"yapl/internal/command"
	"yapl/internal/config"
	"yapl/internal/dependency"
	"yapl/internal/fs"
)

// App holds the runtime state and configuration for a specific game or application.
type App struct {
	Type          string
	Name          string
	ForceUpgrade  bool
	DebugMode     bool
	IsSteamPrefix bool
	GlobalConfig  config.Global
	AppConfig     config.App
	AppDir        string
	PrefixPath    string
}

// New creates and initializes a new App instance.
func New(appType, appName string, force, debug, steam bool, gc config.Global, ac config.App) *App {
	appDir := filepath.Join(appType, appName)
	return &App{
		Type:          appType,
		Name:          appName,
		ForceUpgrade:  force,
		DebugMode:     debug,
		IsSteamPrefix: steam,
		GlobalConfig:  gc,
		AppConfig:     ac,
		AppDir:        appDir,
		PrefixPath:    filepath.Join(appDir, "prefix"),
	}
}

// Setup ensures all dependencies are present and initializes the Wine prefix.
func (a *App) Setup() error {
	fmt.Printf("🛠️ Setting up '%s'...\n", a.Name)
	if err := dependency.EnsureAll(a.AppConfig, a.ForceUpgrade, a.GlobalConfig); err != nil {
		return err
	}
	if err := dependency.EnsureRuntime(a.AppConfig, a.GlobalConfig); err != nil {
		return err
	}
	if err := command.InitializePrefix(a.PrefixPath, a.AppConfig, a.GlobalConfig); err != nil {
		return err
	}
	if a.AppConfig.Dependencies.DXVKMode == "custom" {
		if err := dependency.InstallCustomComponents(a.PrefixPath, a.AppConfig.Dependencies); err != nil {
			return err
		}
	}
	fmt.Println("\n✅ Setup complete!")
	fmt.Printf("➡️ If you haven't already, install your application into the prefix at '%s'\n", fs.MustGetAbsolutePath(a.PrefixPath))
	return nil
}

// Package creates a compressed tarball of the application directory.
func (a *App) Package(format string) error {
	fmt.Println("📦 Starting packaging process...")
	return archive.Package(a.AppDir, format)
}

// Run prepares the environment and launches the application.
func (a *App) Run() error {
	fmt.Printf("🚀 Launching '%s'...\n", a.Name)
	if err := dependency.EnsureAll(a.AppConfig, a.ForceUpgrade, a.GlobalConfig); err != nil {
		return err
	}
	if err := dependency.EnsureRuntime(a.AppConfig, a.GlobalConfig); err != nil {
		return err
	}
	if err := command.InitializePrefix(a.PrefixPath, a.AppConfig, a.GlobalConfig); err != nil {
		return err
	}

	method := a.AppConfig.LaunchMethod
	if method == "" {
		method = "container"
	}

	fmt.Printf("-> Using launch method from config: %s\n", method)
	switch method {
	case "direct":
		return command.RunDirectly(a.PrefixPath, a.AppConfig, a.GlobalConfig, a.IsSteamPrefix, a.DebugMode)
	case "container":
		return command.RunInContainer(a.PrefixPath, a.AppConfig, a.GlobalConfig, a.DebugMode)
	case "umu":
		return command.RunWithUMU(a.PrefixPath, a.AppConfig, a.GlobalConfig, a.DebugMode)
	default:
		return fmt.Errorf("unknown launch_method: '%s'. Please use 'direct', 'container', or 'umu'", method)
	}
}
