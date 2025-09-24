package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// --- Configuration Structs ---

type VersionInfo struct {
	URL                   string   `json:"url,omitempty"`
	Path                  string   `json:"path,omitempty"`
	BinPath               string   `json:"bin_path,omitempty"`
	LaunchMethod          string   `json:"launch_method,omitempty"`
	WineDllPathComponents []string `json:"wine_dll_path_components,omitempty"`
	PythonHome            string   `json:"python_home,omitempty"`
	PythonPath            string   `json:"python_path,omitempty"`
}

type GlobalConfig struct {
	ProtonVersions     map[string]VersionInfo            `json:"proton_versions"`
	DependencyVersions map[string]map[string]VersionInfo `json:"dependency_versions"`
}

type UMUOptions struct {
	Version         string   `json:"version,omitempty"`
	UseSystemBinary bool     `json:"use_system_binary,omitempty"`
	GameID          string   `json:"game_id,omitempty"`
	Store           string   `json:"store,omitempty"`
	LaunchArgs      []string `json:"launch_args,omitempty"`
}

type AppDependencies struct {
	DXVKVersion        string `json:"dxvk_version,omitempty"`
	VKD3DVersion       string `json:"vkd3d_version,omitempty"`
	DXVKMode           string `json:"dxvk_mode,omitempty"`
	DXVKInstallPath    string `json:"dxvk_install_path,omitempty"`
	DXVKDirectXVersion string `json:"dxvk_directx_version,omitempty"`
	VKD3DInstallPath   string `json:"vkd3d_install_path,omitempty"`
}

type AppConfig struct {
	ProtonVersion   string            `json:"proton_version"`
	LaunchMethod    string            `json:"launch_method,omitempty"`
	ProtonBinName   string            `json:"proton_bin_name,omitempty"`
	Executable      string            `json:"executable"`
	UMUOptions      UMUOptions        `json:"umu_options,omitempty"`
	Dependencies    AppDependencies   `json:"dependencies"`
	DLLOverrides    map[string]string `json:"dll_overrides"`
	EnvironmentVars map[string]string `json:"environment_vars"`
}

// --- App Struct ---
type App struct {
	Type         string
	Name         string
	ForceUpgrade bool
	GlobalConfig GlobalConfig
	AppConfig    AppConfig
	AppDir       string
	PrefixPath   string
}

func NewApp(appType, appName string, force bool, gc GlobalConfig, ac AppConfig) *App {
	appDir := filepath.Join(appType, appName)
	return &App{
		Type:         appType,
		Name:         appName,
		ForceUpgrade: force,
		GlobalConfig: gc,
		AppConfig:    ac,
		AppDir:       appDir,
		PrefixPath:   filepath.Join(appDir, "prefix"),
	}
}

func main() {
	log.SetFlags(0)
	gameName := flag.String("game", "", "The name of the game directory inside ./games/.")
	appName := flag.String("app", "", "The name of the application directory inside ./apps/.")
	upgradeProton := flag.Bool("upgrade-proton", false, "Force re-download of the Proton version specified in the config.")
	packageFormat := flag.String("format", "gz", "Compression format for packaging (gz, xz, zst).")
	flag.Parse()

	if flag.NArg() == 0 {
		log.Fatalf("❌ Error: No command provided. Use 'setup', 'package', 'unpackage', or 'run'.")
	}
	command := flag.Arg(0)

	if command == "unpackage" {
		args := flag.Args()[1:]
		archiveType := "game" // Default type
		if len(args) > 0 && (args[0] == "app" || args[0] == "game") {
			archiveType = args[0]
			args = args[1:]
		}
		if err := UnpackageArchives(archiveType, args); err != nil {
			log.Fatalf("❌ Unpackaging failed: %v", err)
		}
		return
	}

	if *gameName == "" && *appName == "" {
		log.Fatalf("❌ Error: --game or --app flag is required for this command.")
	}

	targetType := "games"
	targetName := *gameName
	if targetName == "" {
		targetType = "apps"
		targetName = *appName
	}

	if command == "setup" {
		if err := ensureAppDirAndDefaultConfig(filepath.Join(targetType, targetName), targetType); err != nil {
			log.Fatalf("❌ Initial setup failed: %v", err)
		}
	}

	appConfig, err := loadAppConfig(filepath.Join(targetType, targetName), targetType)
	if err != nil {
		log.Fatalf("❌ Could not load app config: %v", err)
	}

	globalConfig, err := loadOrCreateGlobalConfig("runner.json")
	if err != nil {
		log.Fatalf("❌ Could not load global config: %v", err)
	}

	app := NewApp(targetType, targetName, *upgradeProton, globalConfig, appConfig)

	switch command {
	case "setup":
		if err := app.Setup(); err != nil {
			log.Fatalf("❌ Setup failed: %v", err)
		}
	case "package":
		if err := app.Package(*packageFormat); err != nil {
			log.Fatalf("❌ Packaging failed: %v", err)
		}
	case "run":
		if err := app.Run(); err != nil {
			log.Fatalf("❌ Run failed: %v", err)
		}
	default:
		log.Fatalf("❌ Error: Unknown command '%s'.", command)
	}
}

// --- Archive Struct and Methods ---

// Archive represents a local or remote archive file.
type Archive struct {
	// Source can be a URL or a local file path.
	Source string
}

// open returns a readable stream for the archive, whether local or remote.
func (a *Archive) open() (io.ReadCloser, error) {
	if strings.HasPrefix(a.Source, "http") {
		fmt.Printf(" Downloading from %s...\n", a.Source)
		resp, err := http.Get(a.Source)
		if err != nil {
			return nil, fmt.Errorf("http get: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("download failed: %s", resp.Status)
		}
		return resp.Body, nil
	}

	fmt.Printf(" Reading local file %s...\n", a.Source)
	return os.Open(a.Source)
}

// Extract unpacks the archive to the given destination path.
func (a *Archive) Extract(destPath string) error {
	if a.Source == "" {
		return errors.New("archive source cannot be empty")
	}

	stream, err := a.open()
	if err != nil {
		return err
	}
	defer stream.Close()

	decompressedReader, err := getDecompressedReader(stream, a.Source)
	if err != nil {
		return err
	}

	return extractTar(decompressedReader, destPath)
}

// --- Core App Methods ---

func (a *App) Setup() error {
	fmt.Printf("🛠️ Setting up '%s'...\n", a.Name)
	if err := a.ensureAllDependencies(); err != nil {
		return err
	}
	if err := a.initializePrefix(); err != nil {
		return err
	}
	if a.AppConfig.Dependencies.DXVKMode == "custom" {
		if err := a.installCustomComponents(); err != nil {
			return err
		}
	}
	fmt.Println("\n✅ Setup complete!")
	fmt.Printf("➡️ If you haven't already, install your application into the prefix at '%s'\n", mustGetAbsolutePath(a.PrefixPath))
	return nil
}

func (a *App) Package(format string) error {
	fmt.Println("📦 Starting packaging process...")
	if _, err := os.Stat(a.AppDir); os.IsNotExist(err) {
		return fmt.Errorf("application directory '%s' not found", a.AppDir)
	}

	var extension string
	switch format {
	case "gz":
		extension = ".tar.gz"
	case "xz":
		extension = ".tar.xz"
	case "zst":
		extension = ".tar.zst"
	default:
		return fmt.Errorf("unsupported package format: %s. Use 'gz', 'xz', or 'zst'", format)
	}

	packageName := filepath.Base(a.AppDir) + extension
	fmt.Printf("-> Creating %s bundle '%s'...\n", strings.ToUpper(format), packageName)
	if err := createBundle(packageName, a.AppDir, format); err != nil {
		return fmt.Errorf("failed to create package: %w", err)
	}
	fmt.Println("\n✅ Packaging complete!")
	fmt.Printf("➡️ Distribute '%s' to other machines.\n", packageName)
	return nil
}

func UnpackageArchives(archiveType string, archivePaths []string) error {
	if len(archivePaths) == 0 {
		return errors.New("no archive files provided")
	}
	fmt.Println("📦 Starting unpackaging process...")
	for _, archivePath := range archivePaths {
		fmt.Printf("-> Unpackaging '%s'...\n", archivePath)

		baseName := filepath.Base(archivePath)
		var nameWithoutExt string
		if strings.HasSuffix(baseName, ".tar.gz") {
			nameWithoutExt = strings.TrimSuffix(baseName, ".tar.gz")
		} else if strings.HasSuffix(baseName, ".tar.xz") {
			nameWithoutExt = strings.TrimSuffix(baseName, ".tar.xz")
		} else if strings.HasSuffix(baseName, ".tar.zst") {
			nameWithoutExt = strings.TrimSuffix(baseName, ".tar.zst")
		} else {
			log.Printf("⚠️  Skipping '%s': unrecognized archive extension.", archivePath)
			continue
		}

		destPath := filepath.Join(archiveType+"s", nameWithoutExt)

		if _, err := os.Stat(destPath); err == nil {
			log.Printf("⚠️  Skipping '%s': destination '%s' already exists.", archivePath, destPath)
			continue
		}

		archive := &Archive{Source: archivePath}
		if err := archive.Extract(destPath); err != nil {
			log.Printf("❌ Failed to unpackage '%s': %v", archivePath, err)
		} else {
			fmt.Printf("✅ Successfully unpackaged to '%s'\n", destPath)
		}
	}
	fmt.Println("\n✨ Unpackaging complete!")
	return nil
}

func (a *App) Run() error {
	fmt.Printf("🚀 Launching '%s'...\n", a.Name)
	if err := a.ensureAllDependencies(); err != nil {
		return err
	}
	if err := a.initializePrefix(); err != nil {
		return err
	}

	launchMethod := a.AppConfig.LaunchMethod
	if launchMethod == "" {
		launchMethod = "direct"
	}

	fmt.Printf("-> Using launch method from game.json: %s\n", launchMethod)

	if launchMethod == "umu" {
		return a.runWithUMU()
	}
	return a.runDirectly()
}

// --- Logic Sub-Routines ---

func (a *App) runDirectly() error {
	fmt.Println("-> Running directly...")

	protonExecutable := a.getProtonExecutablePath()
	absPrefix := mustGetAbsolutePath(a.PrefixPath)

	var fullExecutablePath string
	binName := a.getProtonBinName()
	protonVersionInfo := a.getProtonInfo()
	protonBasePath := a.getProtonPath(protonVersionInfo)

	if binName == "proton" {
		fullExecutablePath = filepath.Join(absPrefix, "pfx", a.AppConfig.Executable)
	} else {
		fullExecutablePath = filepath.Join(absPrefix, a.AppConfig.Executable)
	}

	var cmd *exec.Cmd
	if binName == "proton" {
		fmt.Println("-> Using Proton 'run' command.")
		cmd = exec.Command(protonExecutable, "run", fullExecutablePath)
		cmd.Env = a.buildProtonEnv(absPrefix, protonBasePath, protonVersionInfo)
	} else {
		fmt.Printf("-> Using direct Wine-like execution ('%s').\n", binName)
		cmd = exec.Command(protonExecutable, fullExecutablePath)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "WINEPREFIX="+absPrefix)
		for k, v := range a.AppConfig.EnvironmentVars {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		if overrideStr := a.buildDllOverridesString(); overrideStr != "" {
			cmd.Env = append(cmd.Env, "WINEDLLOVERRIDES="+overrideStr)
		}
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("-> Executing: %s\n", strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		log.Printf("❌ Application exited with an error: %v", err)
	}
	return nil
}

func (a *App) buildProtonEnv(absPrefix, protonBasePath string, vinfo VersionInfo) []string {
	clientInstallPath := filepath.Dir(filepath.Join(absPrefix, a.AppConfig.Executable))

	env := os.Environ()
	env = append(env, "STEAM_COMPAT_DATA_PATH="+absPrefix)
	env = append(env, "STEAM_COMPAT_CLIENT_INSTALL_PATH="+clientInstallPath)
	env = append(env, "STEAM_COMPAT_TOOL_PATHS="+protonBasePath)

	for k, v := range a.AppConfig.EnvironmentVars {
		env = append(env, k+"="+v)
	}

	if overrideStr := a.buildDllOverridesString(); overrideStr != "" {
		env = append(env, "WINEDLLOVERRIDES="+overrideStr)
	}

	if len(vinfo.WineDllPathComponents) > 0 {
		var dllPaths []string
		for _, component := range vinfo.WineDllPathComponents {
			dllPaths = append(dllPaths, filepath.Join(protonBasePath, component))
		}
		env = append(env, "WINEDLLPATH="+strings.Join(dllPaths, ":"))
	}
	if vinfo.PythonHome != "" {
		env = append(env, "PYTHONHOME="+vinfo.PythonHome)
	}
	if vinfo.PythonPath != "" {
		env = append(env, "PYTHONPATH="+vinfo.PythonPath)
	}

	env = append(env, "PROTON_LOG=1")
	return env
}

func (a *App) runWithUMU() error {
	fmt.Println("-> Running with umu-launcher...")

	umuRunPath := "umu-run"
	if !a.AppConfig.UMUOptions.UseSystemBinary {
		ver := a.AppConfig.UMUOptions.Version
		if ver == "" {
			return errors.New("'umu_options.version' must be set in game.json")
		}
		vinfo, err := a.getDependencyInfo("umu-launcher", ver)
		if err != nil {
			return err
		}
		umuRunPath = filepath.Join("dependencies", "umu-launcher", ver, vinfo.BinPath, "umu-run")
	}

	absPrefix := mustGetAbsolutePath(a.PrefixPath)
	protonVersionInfo := a.getProtonInfo()
	protonBasePath := a.getProtonPath(protonVersionInfo)
	absProton := mustGetAbsolutePath(protonBasePath)
	fullExecutablePath := filepath.Join(absPrefix, a.AppConfig.Executable)

	args := append([]string{fullExecutablePath}, a.AppConfig.UMUOptions.LaunchArgs...)
	cmd := exec.Command(umuRunPath, args...)

	cmd.Env = a.buildProtonEnv(absPrefix, protonBasePath, protonVersionInfo)
	cmd.Env = append(cmd.Env, "PROTONPATH="+absProton)
	if a.AppConfig.UMUOptions.GameID != "" {
		cmd.Env = append(cmd.Env, "GAMEID="+a.AppConfig.UMUOptions.GameID)
	}
	if a.AppConfig.UMUOptions.Store != "" {
		cmd.Env = append(cmd.Env, "STORE="+a.AppConfig.UMUOptions.Store)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("-> Executing: %s\n", strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		log.Printf("❌ Application exited with an error: %v", err)
	}
	return nil
}

func (a *App) ensureAllDependencies() error {
	fmt.Println("-> Checking dependencies...")
	if err := a.ensureProton(); err != nil {
		return err
	}

	if a.AppConfig.LaunchMethod == "umu" && !a.AppConfig.UMUOptions.UseSystemBinary {
		if err := a.ensureDependency("umu-launcher", a.AppConfig.UMUOptions.Version); err != nil {
			return err
		}
	}
	if err := a.ensureDependency("dxvk", a.AppConfig.Dependencies.DXVKVersion); err != nil {
		return err
	}
	if err := a.ensureDependency("vkd3d", a.AppConfig.Dependencies.VKD3DVersion); err != nil {
		return err
	}
	return nil
}

func (a *App) ensureProton() error {
	vinfo := a.getProtonInfo()
	if vinfo.Path != "" {
		if _, err := os.Stat(vinfo.Path); os.IsNotExist(err) {
			return fmt.Errorf("custom proton path does not exist: %s", vinfo.Path)
		}
		fmt.Println("-> Using local Proton version.")
		return nil
	}

	if vinfo.URL == "" {
		return fmt.Errorf("proton version '%s' has no URL or local path in runner.json", a.AppConfig.ProtonVersion)
	}

	protonPath := filepath.Join("proton", a.AppConfig.ProtonVersion)
	if !dirExistsAndIsNotEmpty(protonPath) || a.ForceUpgrade {
		fmt.Printf("-> Acquiring Proton '%s'...\n", a.AppConfig.ProtonVersion)
		if a.ForceUpgrade {
			if err := os.RemoveAll(protonPath); err != nil {
				return fmt.Errorf("failed to remove existing proton path: %w", err)
			}
		}
		archive := &Archive{Source: vinfo.URL}
		if err := archive.Extract(protonPath); err != nil {
			return fmt.Errorf("failed to acquire proton: %w", err)
		}
	}
	return nil
}

func (a *App) ensureDependency(name, version string) error {
	if version == "" {
		return nil
	}
	depPath := filepath.Join("dependencies", name, version)
	if !dirExistsAndIsNotEmpty(depPath) {
		vinfo, err := a.getDependencyInfo(name, version)
		if err != nil {
			return err
		}
		fmt.Printf("-> Acquiring %s '%s'...\n", name, version)
		archive := &Archive{Source: vinfo.URL}
		if err := archive.Extract(depPath); err != nil {
			return fmt.Errorf("failed to acquire dependency '%s': %w", name, err)
		}
	}
	return nil
}

func (a *App) initializePrefix() error {
	absPrefix := mustGetAbsolutePath(a.PrefixPath)
	if err := mustCreateDirectory(absPrefix); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(absPrefix, "system.reg")); err == nil {
		return nil
	}

	fmt.Println("-> Initializing Wine prefix...")
	protonExecutable := a.getProtonExecutablePath()

	var cmd *exec.Cmd
	binName := a.getProtonBinName()
	protonVersionInfo := a.getProtonInfo()
	protonBasePath := a.getProtonPath(protonVersionInfo)

	if binName == "proton" {
		fmt.Println("-> Using 'proton run cmd' for initialization.")
		cmd = exec.Command(protonExecutable, "run", "cmd", "/c", "echo", "Initializing...")
		cmd.Env = a.buildProtonEnv(absPrefix, protonBasePath, protonVersionInfo)
	} else {
		fmt.Printf("-> Using 'wineboot' with '%s' for initialization.\n", binName)
		cmd = exec.Command(protonExecutable, "wineboot", "-u")
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "WINEPREFIX="+absPrefix)
	}

	if err := cmd.Run(); err != nil {
		log.Printf("❌ Prefix initialization failed: %v", err)
		if exitError, ok := err.(*exec.ExitError); ok {
			log.Printf("-> Prefix creation output:\n%s", string(exitError.Stderr))
		}
		return fmt.Errorf("prefix initialization command failed: %w", err)
	}
	fmt.Println("-> Prefix initialized successfully.")

	if binName == "proton" {
		fmt.Println("-> Restructuring prefix to standard layout...")
		pfxDir := filepath.Join(absPrefix, "pfx")

		if _, err := os.Stat(pfxDir); err == nil {
			files, err := filepath.Glob(filepath.Join(pfxDir, "*"))
			if err != nil {
				return fmt.Errorf("could not list files in pfx dir for restructuring: %w", err)
			}
			for _, file := range files {
				dest := filepath.Join(absPrefix, filepath.Base(file))
				if err := os.Rename(file, dest); err != nil {
					return fmt.Errorf("failed to move '%s' during restructure: %w", file, err)
				}
			}

			if err := os.Remove(pfxDir); err != nil {
				return fmt.Errorf("failed to remove temporary pfx directory: %w", err)
			}

			if err := os.Symlink(".", pfxDir); err != nil {
				return fmt.Errorf("failed to create pfx symlink: %w", err)
			}
			fmt.Println("-> Prefix restructured.")
		}
	}

	return nil
}

func (a *App) installCustomComponents() error {
	dxvkMap := map[string][]string{
		"9":  {"d3d9.dll"},
		"10": {"d3d10.dll", "d3d10_1.dll", "d3d10core.dll", "d3d11.dll", "dxgi.dll"},
		"11": {"d3d11.dll", "dxgi.dll"},
	}
	vkd3dList := []string{"d3d12.dll", "d3d12core.dll"}

	if err := a.installComponent("dxvk", a.AppConfig.Dependencies.DXVKVersion, a.AppConfig.Dependencies.DXVKInstallPath, dxvkMap[a.AppConfig.Dependencies.DXVKDirectXVersion]); err != nil {
		return err
	}
	if err := a.installComponent("vkd3d", a.AppConfig.Dependencies.VKD3DVersion, a.AppConfig.Dependencies.VKD3DInstallPath, vkd3dList); err != nil {
		return err
	}
	return nil
}

func (a *App) installComponent(name, version, installPath string, dlls []string) error {
	if installPath == "" || version == "" || len(dlls) == 0 {
		return nil
	}
	fmt.Printf("-> Installing custom %s DLLs...\n", name)
	sourceDir := filepath.Join("dependencies", name, version, "x64")
	destDir := filepath.Join(mustGetAbsolutePath(a.PrefixPath), "drive_c", installPath)
	if err := mustCreateDirectory(destDir); err != nil {
		return err
	}
	for _, file := range dlls {
		srcPath := filepath.Join(sourceDir, file)
		dstPath := filepath.Join(destDir, file)
		if err := copyFile(srcPath, dstPath); err != nil {
			log.Printf("⚠️ Failed to copy %s: %v", file, err)
		}
	}
	return nil
}

// --- Helper Functions ---

func (a *App) buildDllOverridesString() string {
	if len(a.AppConfig.DLLOverrides) == 0 {
		return ""
	}
	var parts []string
	for dll, setting := range a.AppConfig.DLLOverrides {
		parts = append(parts, fmt.Sprintf("%s=%s", dll, setting))
	}
	return strings.Join(parts, ";")
}

func (a *App) getProtonInfo() VersionInfo {
	vinfo, ok := a.GlobalConfig.ProtonVersions[a.AppConfig.ProtonVersion]
	if !ok {
		log.Fatalf("❌ Proton version '%s' not defined in runner.json", a.AppConfig.ProtonVersion)
	}
	return vinfo
}

func (a *App) getProtonPath(vinfo VersionInfo) string {
	if vinfo.Path != "" {
		return vinfo.Path
	}
	return filepath.Join("proton", a.AppConfig.ProtonVersion)
}

func (a *App) getProtonBinName() string {
	if a.AppConfig.ProtonBinName == "" {
		return "proton"
	}
	return a.AppConfig.ProtonBinName
}

func (a *App) getProtonExecutablePath() string {
	protonVersionInfo := a.getProtonInfo()
	protonBasePath := a.getProtonPath(protonVersionInfo)
	binName := a.getProtonBinName()
	return filepath.Join(protonBasePath, binName)
}

func (a *App) getDependencyInfo(name, version string) (VersionInfo, error) {
	vinfoMap, ok := a.GlobalConfig.DependencyVersions[name]
	if !ok {
		return VersionInfo{}, fmt.Errorf("dependency type '%s' not defined in runner.json", name)
	}
	vinfo, ok := vinfoMap[version]
	if !ok {
		return VersionInfo{}, fmt.Errorf("version '%s' for '%s' not defined in runner.json", version, name)
	}
	return vinfo, nil
}

func ensureAppDirAndDefaultConfig(appDir, appType string) error {
	configName := "game.json"
	if appType == "apps" {
		configName = "app.json"
	}

	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		fmt.Printf("-> Directory '%s' not found. Creating it with a default config...\n", appDir)
		if err := mustCreateDirectory(appDir); err != nil {
			return err
		}
		defaultCfg := AppConfig{
			ProtonVersion: "PLEASE_SET_A_VERSION_FROM_RUNNER.JSON",
			LaunchMethod:  "direct",
			ProtonBinName: "proton",
			Executable:    "drive_c/path/to/your/app.exe",
		}
		if err := writeJSONFile(filepath.Join(appDir, configName), defaultCfg); err != nil {
			return err
		}
		fmt.Printf("✅ Default %s created. Please edit it and run 'setup'.\n", configName)
		os.Exit(0)
	}
	return nil
}

func loadAppConfig(appDir, appType string) (AppConfig, error) {
	configName := "game.json"
	if appType == "apps" {
		configName = "app.json"
	}
	var cfg AppConfig
	if err := readJSONFile(filepath.Join(appDir, configName), &cfg); err != nil {
		return AppConfig{}, fmt.Errorf("could not load config '%s': %w", configName, err)
	}
	return cfg, nil
}

func loadOrCreateGlobalConfig(path string) (GlobalConfig, error) {
	var g GlobalConfig
	err := readJSONFile(path, &g)
	if os.IsNotExist(err) {
		fmt.Println("-> No global 'runner.json' found. Creating a default one.")
		defaultCfg := GlobalConfig{
			ProtonVersions:     map[string]VersionInfo{"EDIT_ME": {URL: "URL_TO_PROTON_TAR", Path: "OR_PROVIDE_ABSOLUTE_PATH_TO_PROTON_DIR"}},
			DependencyVersions: map[string]map[string]VersionInfo{"dxvk": {"EDIT_ME": {URL: "URL_TO_DXVK_TAR"}}},
		}
		if err := writeJSONFile(path, defaultCfg); err != nil {
			return GlobalConfig{}, fmt.Errorf("failed writing default runner.json: %w", err)
		}
		fmt.Println("✅ Default runner.json created. Please edit it with download URLs or local paths.")
		return defaultCfg, nil
	}
	if err != nil {
		return GlobalConfig{}, fmt.Errorf("could not read runner.json: %w", err)
	}
	return g, nil
}

func createBundle(bundleName, sourceDir, format string) error {
	f, err := os.Create(bundleName)
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}
	defer f.Close()

	var compressor io.WriteCloser
	switch format {
	case "gz":
		compressor = gzip.NewWriter(f)
	case "xz":
		compressor, err = xz.NewWriter(f)
		if err != nil {
			return fmt.Errorf("create xz writer: %w", err)
		}
	case "zst":
		compressor, err = zstd.NewWriter(f)
		if err != nil {
			return fmt.Errorf("create zstd writer: %w", err)
		}
	}
	defer compressor.Close()

	tw := tar.NewWriter(compressor)
	defer tw.Close()

	return filepath.Walk(sourceDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Dir(sourceDir), path)
		if err != nil {
			return err
		}
		header.Name = rel
		if fi.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			header.Linkname = linkTarget
		}

		header.Uid = 65534
		header.Gid = 65534
		header.Uname = "nobody"
		header.Gname = "nobody"

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			defer in.Close()
			if _, err := io.Copy(tw, in); err != nil {
				return err
			}
		}
		return nil
	})
}

func getDecompressedReader(r io.Reader, sourceFilename string) (io.Reader, error) {
	switch {
	case strings.HasSuffix(sourceFilename, ".tar.gz"):
		return gzip.NewReader(r)
	case strings.HasSuffix(sourceFilename, ".tar.xz"):
		return xz.NewReader(r)
	case strings.HasSuffix(sourceFilename, ".tar.zst"):
		return zstd.NewReader(r)
	case strings.HasSuffix(sourceFilename, ".tar"):
		return r, nil
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", sourceFilename)
	}
}

func extractTar(r io.Reader, destPath string) error {
	tr := tar.NewReader(r)
	fmt.Println(" Extracting archive...")
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		parts := strings.Split(hdr.Name, string(filepath.Separator))
		if len(parts) <= 1 {
			continue
		}

		relativePath := strings.Join(parts[1:], string(filepath.Separator))
		target := filepath.Join(destPath, relativePath)

		if relativePath == "" {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("mkdirAll failed for %s: %w", filepath.Dir(target), err)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("mkdir dir: %w", err)
			}
		case tar.TypeReg:
			out, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("create file: %w", err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("copy file: %w", err)
			}
			out.Close()
		case tar.TypeSymlink:
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return fmt.Errorf("create symlink: %w", err)
			}
		case tar.TypeLink:
			if err := os.Link(hdr.Linkname, target); err != nil {
				return fmt.Errorf("create hardlink: %w", err)
			}
		}
	}
}

func readJSONFile(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func mustCreateDirectory(p string) error {
	return os.MkdirAll(p, 0755)
}

func mustGetAbsolutePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		log.Fatalf("❌ Could not get absolute path for '%s': %v", p, err)
	}
	return abs
}

func dirExistsAndIsNotEmpty(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	return err == nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}
