package app

import (
	"fmt"
	"io"
	"os"
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
	if err := command.InitializePrefix(a.PrefixPath, a.AppConfig, a.GlobalConfig, a.DebugMode); err != nil {
		return err
	}
	if err := dependency.EnsureWinetricks(a.PrefixPath, a.AppConfig.Winetricks); err != nil {
		return err
	}
	if a.AppConfig.Dependencies.DXVKMode == "custom" {
		if err := dependency.InstallCustomComponents(a.PrefixPath, a.AppConfig.Dependencies); err != nil {
			return err
		}
	}
	fmt.Println("\n✅ Setup complete!")
	if absPath, err := fs.GetAbsolutePath(a.PrefixPath); err == nil {
		fmt.Printf("➡️ If you haven't already, install your application into the prefix at '%s'\n", absPath)
	}
	return nil
}

// ListAll scans the games/ and apps/ directories and writes a tab-separated summary to w.
func ListAll(w io.Writer) error {
	fmt.Fprintln(w, "TYPE\tNAME\tPROTON\tMETHOD\tEXECUTABLE")

	dirs := []struct {
		dir     string
		cfgName string
		label   string
	}{
		{"games", "game.json", "game"},
		{"apps", "app.json", "app"},
	}

	for _, d := range dirs {
		entries, err := os.ReadDir(d.dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			cfgPath := filepath.Join(d.dir, e.Name(), d.cfgName)
			cfg, err := config.LoadApp(cfgPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %s: %v\n", e.Name(), err)
				continue
			}
			method := cfg.LaunchMethod
			if method == "" {
				method = "container"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				d.label, e.Name(), cfg.ProtonVersion, method, cfg.Executable)
		}
	}
	return nil
}

// Info writes a human-readable readiness summary for this app to w without modifying any state.
func (a *App) Info(w io.Writer) error {
	absPrefix, _ := fs.GetAbsolutePath(a.PrefixPath)

	protonPath := filepath.Join("proton", a.AppConfig.ProtonVersion)
	if vinfo, ok := a.GlobalConfig.ProtonVersions[a.AppConfig.ProtonVersion]; ok && vinfo.Path != "" {
		protonPath = vinfo.Path
	}
	protonStatus := statusMark(fs.DirExistsAndIsNotEmpty(protonPath))

	var prefixStatus string
	if command.PrefixIsInitialized(absPrefix) {
		prefixStatus = "✓ initialised"
	} else {
		prefixStatus = "✗ not initialised"
	}

	cfgFile := filepath.Join(a.AppDir, "game.json")
	if a.Type == "apps" {
		cfgFile = filepath.Join(a.AppDir, "app.json")
	}

	fmt.Fprintf(w, "Game:          %s\n", a.Name)
	fmt.Fprintf(w, "Config file:   %s\n", cfgFile)
	fmt.Fprintf(w, "Proton:        %-14s [%s]  %s\n", a.AppConfig.ProtonVersion, protonPath, protonStatus)

	if v := a.AppConfig.Dependencies.DXVKVersion; v != "" {
		p := filepath.Join("dependencies", "dxvk", v)
		fmt.Fprintf(w, "DXVK:          %-14s [%s]  %s\n", v, p, statusMark(fs.DirExistsAndIsNotEmpty(p)))
	} else {
		fmt.Fprintf(w, "DXVK:          (not set)\n")
	}

	if v := a.AppConfig.Dependencies.VKD3DVersion; v != "" {
		p := filepath.Join("dependencies", "vkd3d", v)
		fmt.Fprintf(w, "VKD3D:         %-14s [%s]  %s\n", v, p, statusMark(fs.DirExistsAndIsNotEmpty(p)))
	} else {
		fmt.Fprintf(w, "VKD3D:         (not set)\n")
	}

	if v := a.AppConfig.RuntimeVersion; v != "" {
		p := filepath.Join("dependencies", "runtime", v)
		fmt.Fprintf(w, "Runtime:       %-14s [%s]  %s\n", v, p, statusMark(fs.DirExistsAndIsNotEmpty(p)))
	} else {
		fmt.Fprintf(w, "Runtime:       (not set)\n")
	}

	fmt.Fprintf(w, "Prefix:        %s  %s\n", a.PrefixPath, prefixStatus)
	fmt.Fprintf(w, "Launch method: %s\n", a.AppConfig.LaunchMethod)
	fmt.Fprintf(w, "Executable:    %s\n", a.AppConfig.Executable)
	return nil
}

func statusMark(present bool) string {
	if present {
		return "✓ present"
	}
	return "✗ missing"
}

// Package creates a compressed tarball of the application directory.
// When bundleDeps is true, Proton and configured dependencies are staged into a _bundle/
// directory inside the archive for true offline portability.
func (a *App) Package(format string, bundleDeps, yes bool) error {
	fmt.Println("📦 Starting packaging process...")

	if !bundleDeps {
		return archive.Package(a.AppDir, format)
	}

	// Estimate the total bundle size before committing.
	totalBytes, err := bundleSize(a)
	if err != nil {
		return fmt.Errorf("could not estimate bundle size: %w", err)
	}
	sizeGB := float64(totalBytes) / float64(1<<30)

	if !yes {
		fmt.Printf("Warning: bundled package will be approximately %.1f GB. Continue? [y/N]: ", sizeGB)
		var resp string
		fmt.Scanln(&resp)
		if resp != "y" && resp != "Y" {
			return fmt.Errorf("packaging cancelled")
		}
	} else {
		fmt.Printf("-> Bundling approximately %.1f GB of dependencies...\n", sizeGB)
	}

	stagingDir, err := os.MkdirTemp("", "yapl-bundle-*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	// Copy game dir into staging as <name>/
	if err := fs.CopyDir(a.AppDir, filepath.Join(stagingDir, a.Name)); err != nil {
		return fmt.Errorf("copy game dir to staging: %w", err)
	}

	// Copy Proton.
	protonSrc := filepath.Join("proton", a.AppConfig.ProtonVersion)
	if !fs.DirExistsAndIsNotEmpty(protonSrc) {
		return fmt.Errorf("proton '%s' not downloaded; run 'setup' before bundling", a.AppConfig.ProtonVersion)
	}
	protonDst := filepath.Join(stagingDir, "_bundle", "proton", a.AppConfig.ProtonVersion)
	if err := fs.CopyDir(protonSrc, protonDst); err != nil {
		return fmt.Errorf("copy proton to bundle: %w", err)
	}

	// Copy DXVK if configured.
	if v := a.AppConfig.Dependencies.DXVKVersion; v != "" {
		src := filepath.Join("dependencies", "dxvk", v)
		if fs.DirExistsAndIsNotEmpty(src) {
			if err := fs.CopyDir(src, filepath.Join(stagingDir, "_bundle", "dependencies", "dxvk", v)); err != nil {
				return fmt.Errorf("copy dxvk to bundle: %w", err)
			}
		}
	}

	// Copy VKD3D if configured.
	if v := a.AppConfig.Dependencies.VKD3DVersion; v != "" {
		src := filepath.Join("dependencies", "vkd3d", v)
		if fs.DirExistsAndIsNotEmpty(src) {
			if err := fs.CopyDir(src, filepath.Join(stagingDir, "_bundle", "dependencies", "vkd3d", v)); err != nil {
				return fmt.Errorf("copy vkd3d to bundle: %w", err)
			}
		}
	}

	// Copy runtime if configured.
	if v := a.AppConfig.RuntimeVersion; v != "" {
		src := filepath.Join("dependencies", "runtime", v)
		if fs.DirExistsAndIsNotEmpty(src) {
			if err := fs.CopyDir(src, filepath.Join(stagingDir, "_bundle", "dependencies", "runtime", v)); err != nil {
				return fmt.Errorf("copy runtime to bundle: %w", err)
			}
		}
	}

	// Copy runner.json so the target machine knows which versions to reference.
	if _, err := os.Stat("runner.json"); err == nil {
		if err := fs.CopyFile("runner.json", filepath.Join(stagingDir, "_bundle", "runner.json")); err != nil {
			return fmt.Errorf("copy runner.json to bundle: %w", err)
		}
	}

	return archive.PackageFromDir(stagingDir, a.Name, format)
}

func bundleSize(a *App) (int64, error) {
	var total int64
	paths := []string{a.AppDir, filepath.Join("proton", a.AppConfig.ProtonVersion)}
	if v := a.AppConfig.Dependencies.DXVKVersion; v != "" {
		paths = append(paths, filepath.Join("dependencies", "dxvk", v))
	}
	if v := a.AppConfig.Dependencies.VKD3DVersion; v != "" {
		paths = append(paths, filepath.Join("dependencies", "vkd3d", v))
	}
	if v := a.AppConfig.RuntimeVersion; v != "" {
		paths = append(paths, filepath.Join("dependencies", "runtime", v))
	}
	for _, p := range paths {
		_ = filepath.Walk(p, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
	}
	return total, nil
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
	if err := command.InitializePrefix(a.PrefixPath, a.AppConfig, a.GlobalConfig, a.DebugMode); err != nil {
		return err
	}
	if err := dependency.EnsureWinetricks(a.PrefixPath, a.AppConfig.Winetricks); err != nil {
		return err
	}

	method := a.AppConfig.LaunchMethod
	if method == "" {
		method = "container"
	}

	fmt.Printf("-> Using launch method from config: %s\n", method)
	switch method {
	case "direct":
		return command.RunDirectly(a.AppDir, a.AppConfig, a.GlobalConfig, a.IsSteamPrefix, a.DebugMode)
	case "container":
		return command.RunInContainer(a.PrefixPath, a.AppConfig, a.GlobalConfig, a.DebugMode)
	case "umu":
		return command.RunWithUMU(a.PrefixPath, a.AppConfig, a.GlobalConfig, a.DebugMode)
	default:
		return fmt.Errorf("unknown launch_method: '%s'. Please use 'direct', 'container', or 'umu'", method)
	}
}
