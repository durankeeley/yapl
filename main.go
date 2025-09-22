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
	URL              string `json:"url"`
	BinPathInArchive string `json:"bin_path_in_archive,omitempty"`
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
	ProtonPath   string
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
		ProtonPath:   filepath.Join("proton", ac.ProtonVersion),
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

	if a.AppConfig.UseUMULauncher {
		return a.runWithUMU()
	}
	return a.runDirectly()
}

// --- Logic Sub-Routines ---

func (a *App) runDirectly() error {
	fmt.Println("-> Running directly with Proton...")

	protonVersionInfo, ok := a.GlobalConfig.ProtonVersions[a.AppConfig.ProtonVersion]
	if !ok || protonVersionInfo.URL == "" {
		return fmt.Errorf("proton version '%s' not defined in runner.json", a.AppConfig.ProtonVersion)
	}

	protonBin := protonBinPath(a.ProtonPath, protonVersionInfo)
	wineExecutable := filepath.Join(protonBin, "wine64")

	absPrefix := mustGetAbsolutePath(a.PrefixPath)
	fullExecutablePath := filepath.Join(absPrefix, a.AppConfig.Executable)

	cmd := exec.Command(wineExecutable, fullExecutablePath)
	cmd.Env = buildCommandEnv(a, absPrefix)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("-> Executing: %s\n", fullExecutablePath)
	if err := cmd.Run(); err != nil {
		log.Printf("❌ Application exited with an error: %v", err)
	}
	return nil
}

func (a *App) runWithUMU() error {
	fmt.Println("-> Running with umu-launcher...")

	umuRunPath := "umu-run"
	if !a.AppConfig.UMUOptions.UseSystemBinary {
		ver := a.AppConfig.UMUOptions.Version
		if ver == "" {
			return errors.New("'umu_options.version' must be set when not using system binary")
		}
		vinfoMap, ok := a.GlobalConfig.DependencyVersions["umu-launcher"]
		if !ok {
			return errors.New("dependency 'umu-launcher' not defined in runner.json")
		}
		vinfo, ok := vinfoMap[ver]
		if !ok || vinfo.URL == "" {
			return fmt.Errorf("version '%s' for 'umu-launcher' not defined", ver)
		}
		umuRunPath = filepath.Join("dependencies", "umu-launcher", ver, vinfo.BinPathInArchive, "umu-run")
	}

	absPrefix := mustGetAbsolutePath(a.PrefixPath)
	fullExecutablePath := filepath.Join(absPrefix, a.AppConfig.Executable)

	args := append([]string{fullExecutablePath}, a.AppConfig.UMUOptions.LaunchArgs...)
	cmd := exec.Command(umuRunPath, args...)
	cmd.Env = buildUMUEnv(a, absPrefix)
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
	if !dirExistsAndIsNotEmpty(a.ProtonPath) || a.ForceUpgrade {
		fmt.Printf("-> Acquiring Proton '%s'...\n", a.AppConfig.ProtonVersion)
		vinfo, ok := a.GlobalConfig.ProtonVersions[a.AppConfig.ProtonVersion]
		if !ok || vinfo.URL == "" {
			return fmt.Errorf("proton version '%s' not defined in runner.json", a.AppConfig.ProtonVersion)
		}
		if a.ForceUpgrade {
			if err := os.RemoveAll(a.ProtonPath); err != nil {
				return fmt.Errorf("failed to remove existing proton path: %w", err)
			}
		}
		if err := downloadAndExtractArchive(vinfo.URL, a.ProtonPath); err != nil {
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
	fmt.Printf("-> Acquiring %s '%s'...\n", name, version)
	versionMap, ok := a.GlobalConfig.DependencyVersions[name]
	if !ok {
		return fmt.Errorf("dependency type '%s' not defined in runner.json", name)
	}
	vinfo, ok := versionMap[version]
	if !ok || vinfo.URL == "" {
		return fmt.Errorf("version '%s' for '%s' not defined in runner.json", version, name)
	}
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
	protonVersionInfo := a.GlobalConfig.ProtonVersions[a.AppConfig.ProtonVersion]
	protonBin := protonBinPath(a.ProtonPath, protonVersionInfo)
	wineBin := filepath.Join(protonBin, "wine64")

	fmt.Println("-> Initializing Wine prefix...")
	winebootCmd := exec.Command(wineBin, "wineboot", "-u")
	winebootEnv := map[string]string{"WINEPREFIX": absPrefix}
	if a.AppConfig.Dependencies.DXVKMode == "custom" || a.AppConfig.Dependencies.DXVKMode == "wined3d" {
		winebootEnv["PROTON_USE_WINED3D"] = "1"
	}
	return runCmd(winebootCmd, winebootEnv)
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
			ProtonVersions:     map[string]VersionInfo{"EDIT_ME": {URL: "URL_TO_PROTON_TAR"}},
			DependencyVersions: map[string]map[string]VersionInfo{"dxvk": {"EDIT_ME": {URL: "URL_TO_DXVK_TAR"}}},
		}
		if err := writeJSONFile(path, defaultCfg); err != nil {
			return GlobalConfig{}, fmt.Errorf("failed writing default runner.json: %w", err)
		}
		fmt.Println("✅ Default runner.json created. Please edit it with download URLs.")
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
		fmt.Printf("   Downloading from %s...\n", source)
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
		fmt.Printf("   Reading local file %s...\n", source)
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
	fmt.Println("   Extracting archive...")
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

func runCmd(cmd *exec.Cmd, extraEnv map[string]string) error {
	if len(extraEnv) > 0 {
		cmd.Env = os.Environ()
		for k, v := range extraEnv {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command '%s' failed: %w", strings.Join(cmd.Args, " "), err)
	}
	return nil
}

func buildCommandEnv(a *App, absPrefix string) []string {
	env := os.Environ()
	env = append(env, "WINEPREFIX="+absPrefix, "STEAM_COMPAT_DATA_PATH="+absPrefix)
	ldPath := fmt.Sprintf("%s:%s", filepath.Join(a.ProtonPath, "dist", "lib64"), filepath.Join(a.ProtonPath, "dist", "lib"))
	env = append(env, "LD_LIBRARY_PATH="+ldPath)
	for k, v := range a.AppConfig.EnvironmentVars {
		env = append(env, k+"="+v)
	}
	return env
}

func buildUMUEnv(a *App, absPrefix string) []string {
	env := os.Environ()
	absProton := mustGetAbsolutePath(a.ProtonPath)
	env = append(env, "WINEPREFIX="+absPrefix, "PROTONPATH="+absProton)
	if a.AppConfig.UMUOptions.GameID != "" {
		env = append(env, "GAMEID="+a.AppConfig.UMUOptions.GameID)
	}
	if a.AppConfig.UMUOptions.Store != "" {
		env = append(env, "STORE="+a.AppConfig.UMUOptions.Store)
	}
	for k, v := range a.AppConfig.EnvironmentVars {
		env = append(env, k+"="+v)
	}
	return env
}

func protonBinPath(protonPath string, vinfo VersionInfo) string {
	if vinfo.BinPathInArchive != "" {
		return filepath.Join(protonPath, vinfo.BinPathInArchive)
	}
	return filepath.Join(protonPath, "dist", "bin")
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
