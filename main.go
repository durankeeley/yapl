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
	BinPath               string   `json:"bin_path,omitempty"` // For dependencies like umu-launcher
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
	ProtonBinName   string            `json:"proton_bin_name,omitempty"`
	Executable      string            `json:"executable"`
	UseUMULauncher  bool              `json:"use_umu_launcher,omitempty"`
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
	flag.Parse()

	if flag.NArg() == 0 {
		log.Fatalf("❌ Error: No command provided. Use 'setup', 'package', or 'run'.")
	}
	command := flag.Arg(0)

	if *gameName == "" && *appName == "" {
		log.Fatalf("❌ Error: --game or --app flag is required.")
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
		if err := app.Package(); err != nil {
			log.Fatalf("❌ Packaging failed: %v", err)
		}
	case "run":
		if err := app.Run(); err != nil {
			log.Fatalf("❌ Run failed: %v", err)
		}
	default:
		log.Fatalf("❌ Error: Unknown command '%s'. Use 'setup', 'package', or 'run'.", command)
	}
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
	fmt.Printf("➡️ If you haven't already, install your application into '%s'\n", mustGetAbsolutePath(a.PrefixPath))
	return nil
}

func (a *App) Package() error {
	fmt.Println("📦 Starting packaging process...")
	if _, err := os.Stat(a.AppDir); os.IsNotExist(err) {
		return fmt.Errorf("application directory '%s' not found", a.AppDir)
	}
	packageName := filepath.Base(a.AppDir) + ".tar.gz"
	fmt.Printf("-> Creating bundle '%s'...\n", packageName)
	if err := createBundle(packageName, a.AppDir); err != nil {
		return fmt.Errorf("failed to create package: %w", err)
	}
	fmt.Println("\n✅ Packaging complete!")
	fmt.Printf("➡️ Distribute '%s' to other machines.\n", packageName)
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
	if a.AppConfig.UseUMULauncher {
		return a.runWithUMU()
	}
	return a.runDirectly()
}

// --- Logic Sub-Routines ---

func (a *App) runDirectly() error {
	fmt.Println("-> Running directly...")

	protonExecutable := a.getProtonExecutablePath()
	absPrefix := mustGetAbsolutePath(a.PrefixPath)
	fullExecutablePath := filepath.Join(absPrefix, a.AppConfig.Executable)

	var cmd *exec.Cmd
	binName := a.getProtonBinName()
	protonVersionInfo := a.getProtonInfo()
	protonBasePath := a.getProtonPath(protonVersionInfo)

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
		// MODIFIED: Manually add DLL overrides for direct wine calls
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
	steamPath := mustGetAbsolutePath("steam")
	if err := mustCreateDirectory(steamPath); err != nil {
		log.Printf("⚠️ Could not create placeholder steam directory: %v", err)
	}
	env := os.Environ()
	env = append(env, "STEAM_COMPAT_DATA_PATH="+absPrefix)
	env = append(env, "STEAM_COMPAT_CLIENT_INSTALL_PATH="+steamPath)

	for k, v := range a.AppConfig.EnvironmentVars {
		env = append(env, k+"="+v)
	}

	// MODIFIED: Add WINEDLLOVERRIDES from the now-functional dll_overrides map
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
			return errors.New("'umu_options.version' must be set when not using system binary")
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
	if a.AppConfig.UseUMULauncher && !a.AppConfig.UMUOptions.UseSystemBinary {
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
		if err := downloadAndExtractArchive(vinfo.URL, protonPath); err != nil {
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
	if dirExistsAndIsNotEmpty(depPath) {
		return nil
	}
	vinfo, err := a.getDependencyInfo(name, version)
	if err != nil {
		return err
	}
	fmt.Printf("-> Acquiring %s '%s'...\n", name, version)
	if err := downloadAndExtractArchive(vinfo.URL, depPath); err != nil {
		return fmt.Errorf("failed to acquire dependency '%s': %w", name, err)
	}
	return nil
}

func (a *App) initializePrefix() error {
	absPrefix := mustGetAbsolutePath(a.PrefixPath)
	if err := mustCreateDirectory(absPrefix); err != nil {
		return err
	}

	if !dirExistsAndIsNotEmpty(filepath.Join(absPrefix, "drive_c")) {
		fmt.Println("-> Initializing Wine prefix...")
		protonExecutable := a.getProtonExecutablePath()

		var cmd *exec.Cmd
		binName := a.getProtonBinName()
		protonVersionInfo := a.getProtonInfo()
		protonBasePath := a.getProtonPath(protonVersionInfo)

		if binName == "proton" {
			fmt.Println("-> Using 'proton run' for initialization.")
			cmd = exec.Command(protonExecutable, "run")
			cmd.Env = a.buildProtonEnv(absPrefix, protonBasePath, protonVersionInfo)
		} else {
			fmt.Printf("-> Using 'wineboot' with '%s' for initialization.\n", binName)
			cmd = exec.Command(protonExecutable, "wineboot", "-u")
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, "WINEPREFIX="+absPrefix)
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("-> Prefix creation output:\n%s", string(output))
			return fmt.Errorf("prefix initialization failed: %w", err)
		}
		fmt.Println("-> Prefix initialized successfully.")
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

// NEW: Helper function to build the WINEDLLOVERRIDES string.
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

func createBundle(bundleName, sourceDir string) error {
	f, err := os.Create(bundleName)
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
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

func downloadAndExtractArchive(source, destPath string) error {
	if source == "" {
		return errors.New("empty source")
	}

	var src io.ReadCloser
	if strings.HasPrefix(source, "http") {
		fmt.Printf(" Downloading from %s...\n", source)
		resp, err := http.Get(source)
		if err != nil {
			return fmt.Errorf("http get: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("download failed: %s", resp.Status)
		}
		src = resp.Body
	} else {
		fmt.Printf(" Reading local file %s...\n", source)
		f, err := os.Open(source)
		if err != nil {
			return fmt.Errorf("open local file: %w", err)
		}
		src = f
	}
	defer src.Close()

	var rdr io.Reader
	switch {
	case strings.HasSuffix(source, ".tar.gz"):
		gzr, err := gzip.NewReader(src)
		if err != nil {
			return fmt.Errorf("gzip reader: %w", err)
		}
		defer gzr.Close()
		rdr = gzr
	case strings.HasSuffix(source, ".tar.xz"):
		xzr, err := xz.NewReader(src)
		if err != nil {
			return fmt.Errorf("xz reader: %w", err)
		}
		rdr = xzr
	case strings.HasSuffix(source, ".tar.zst"):
		zr, err := zstd.NewReader(src)
		if err != nil {
			return fmt.Errorf("zstd reader: %w", err)
		}
		defer zr.Close()
		rdr = zr
	case strings.HasSuffix(source, ".tar"):
		rdr = src
	default:
		return fmt.Errorf("unsupported archive format: %s", source)
	}

	return extractTar(rdr, destPath)
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
		target := filepath.Join(destPath, strings.Join(parts[1:], string(filepath.Separator)))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
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
