package command

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"yapl/internal/config"
	"yapl/internal/fs"
)

// InitializePrefix creates and sets up a new Wine prefix.
// The initialization strategy is chosen based on launch_method:
//   - "direct": uses wine64/wine from the Proton build via wineboot (no Proton script needed)
//   - "container" / "umu": uses the Proton script (creates pfx/ layout, then restructures)
//   - "system": uses the system-installed wine binary
//
// It also detects and migrates Lutris-style pfx/-subdirectory layouts.
func InitializePrefix(prefixPath string, appCfg config.App, globalCfg config.Global, debug bool) error {
	absPrefix, err := fs.GetAbsolutePath(prefixPath)
	if err != nil {
		return fmt.Errorf("could not resolve prefix path: %w", err)
	}
	if err := fs.MustCreateDirectory(absPrefix); err != nil {
		return err
	}

	if prefixIsInitialized(absPrefix) {
		// Migrate Lutris-style pfx/ layout to standard flat layout if needed.
		if err := restructureProtonPrefix(absPrefix); err != nil {
			return err
		}
		return nil
	}

	if appCfg.ProtonVersion == "system" {
		return initializePrefixWithSystemWine(absPrefix, appCfg, debug)
	}

	if appCfg.LaunchMethod == "direct" {
		return initializePrefixWithProtonWine(absPrefix, prefixPath, appCfg, globalCfg, debug)
	}

	return initializePrefixWithProtonScript(absPrefix, prefixPath, appCfg, globalCfg, debug)
}

// initializePrefixWithProtonWine creates a Wine prefix using wine64/wine from a Proton
// distribution, bypassing the Proton script. Used for "direct" launch method.
func initializePrefixWithProtonWine(absPrefix, prefixPath string, appCfg config.App, globalCfg config.Global, debug bool) error {
	fmt.Println("-> Initializing Wine prefix directly (using wine64, bypassing Proton script)...")

	protonVersionInfo, err := getProtonInfo(appCfg, globalCfg)
	if err != nil {
		return err
	}
	protonBasePath, err := filepath.Abs(getProtonPath(appCfg.ProtonVersion, protonVersionInfo))
	if err != nil {
		return fmt.Errorf("could not resolve proton base path: %w", err)
	}

	wineExecutablePath, err := getWineExecutablePath(protonBasePath)
	if err != nil {
		return err
	}

	env := os.Environ()

	var ldPaths []string
	for _, component := range protonVersionInfo.LDLibraryPathComponents {
		fullPath := filepath.Join(protonBasePath, component)
		if _, err := os.Stat(fullPath); err == nil {
			ldPaths = append(ldPaths, fullPath)
		}
	}
	if existing := os.Getenv("LD_LIBRARY_PATH"); existing != "" {
		ldPaths = append(ldPaths, existing)
	}
	if len(ldPaths) > 0 {
		env = append(env, "LD_LIBRARY_PATH="+strings.Join(ldPaths, ":"))
	}

	existingPath := os.Getenv("PATH")
	env = append(env, "PATH="+strings.Join([]string{
		filepath.Join(protonBasePath, "files", "bin"),
		filepath.Join(protonBasePath, "dist", "bin"),
		filepath.Join(protonBasePath, "bin"),
		existingPath,
	}, ":"))
	env = append(env, "WINEPREFIX="+absPrefix, "WINEARCH=win64")
	if debug {
		env = append(env, "WINEDEBUG=+all")
	}

	cmd := exec.Command(wineExecutablePath, "wineboot", "--init")
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Wine prefix initialization failed: %w", err)
	}

	fmt.Println("-> Prefix created. Launching file explorer for application installation...")
	explorerCfg := appCfg
	explorerCfg.Executable = "drive_c/windows/explorer.exe"
	return RunDirectly(prefixPath, explorerCfg, globalCfg, false, debug)
}

// initializePrefixWithProtonScript creates a Wine prefix using the Proton wrapper script.
// Used for "container" and "umu" launch methods.
func initializePrefixWithProtonScript(absPrefix, prefixPath string, appCfg config.App, globalCfg config.Global, debug bool) error {
	fmt.Println("-> Initializing Wine prefix using the proton script...")

	protonVersionInfo, err := getProtonInfo(appCfg, globalCfg)
	if err != nil {
		return err
	}
	protonBasePath, err := filepath.Abs(getProtonPath(appCfg.ProtonVersion, protonVersionInfo))
	if err != nil {
		return fmt.Errorf("could not resolve proton base path: %w", err)
	}

	protonScriptPath, err := getProtonScriptPath(appCfg, globalCfg)
	if err != nil {
		return err
	}
	if _, err := os.Stat(protonScriptPath); os.IsNotExist(err) {
		return fmt.Errorf("could not find 'proton' script at %s", protonScriptPath)
	}

	initCmd := exec.Command(protonScriptPath, "run", "cmd", "/c", "echo", "Initializing prefix...")
	initCmd.Env = buildProtonEnv(absPrefix, protonBasePath, appCfg, protonVersionInfo, false)
	if err := initCmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log.Printf("-> Prefix creation output:\n%s", string(exitError.Stderr))
		}
		return fmt.Errorf("prefix initialization with proton script failed: %w", err)
	}

	if err := restructureProtonPrefix(absPrefix); err != nil {
		return err
	}

	fmt.Println("-> Prefix created. Launching file explorer for application installation...")
	explorerCfg := appCfg
	explorerCfg.Executable = "drive_c/windows/explorer.exe"
	return RunInContainer(prefixPath, explorerCfg, globalCfg, debug)
}

// PrefixIsInitialized is the exported form of prefixIsInitialized for use in other packages.
func PrefixIsInitialized(absPrefix string) bool {
	return prefixIsInitialized(absPrefix)
}

// prefixIsInitialized returns true if the directory looks like an existing Wine prefix.
// It checks both the flat (YAPL standard) layout and the pfx/ subdirectory (Lutris/raw Proton) layout.
func prefixIsInitialized(absPrefix string) bool {
	// Standard flat layout: system.reg at the prefix root.
	if _, err := os.Stat(filepath.Join(absPrefix, "system.reg")); err == nil {
		return true
	}
	// Lutris/raw Proton layout: system.reg inside a real pfx/ directory.
	// Use Lstat so we don't follow a pfx symlink (which would loop back to root).
	pfxInfo, err := os.Lstat(filepath.Join(absPrefix, "pfx"))
	if err != nil || pfxInfo.Mode()&os.ModeSymlink != 0 {
		return false
	}
	_, err = os.Stat(filepath.Join(absPrefix, "pfx", "system.reg"))
	return err == nil
}

// initializePrefixWithSystemWine creates a Wine prefix using the system-installed wine64 or wine binary.
func initializePrefixWithSystemWine(absPrefix string, appCfg config.App, debug bool) error {
	fmt.Println("-> Initializing Wine prefix with system Wine...")

	wineBin, err := exec.LookPath("wine64")
	if err != nil {
		wineBin, err = exec.LookPath("wine")
		if err != nil {
			return fmt.Errorf("system Wine not found: install wine or wine64 and ensure it is in your PATH")
		}
	}

	env := os.Environ()
	env = append(env, "WINEPREFIX="+absPrefix, "WINEARCH=win64")
	if debug {
		env = append(env, "WINEDEBUG=+all")
	}

	cmd := exec.Command(wineBin, "wineboot", "--init")
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("system Wine prefix initialization failed: %w", err)
	}

	fmt.Println("-> Prefix created with system Wine.")
	fmt.Printf("-> Install your application into the prefix at '%s'\n", absPrefix)
	return nil
}

// RunDirectly launches the application using wine64 from the Proton distribution,
// bypassing the Proton script and the Steam Runtime.
func RunDirectly(prefixPath string, appCfg config.App, globalCfg config.Global, isSteam, debug bool) error {
	if isSteam {
		return errors.New("--steam flag is not compatible with 'direct' launch_method. Use 'container' instead")
	}

	fmt.Println("-> Running in direct mode (using wine64)...")

	absPrefix, err := fs.GetAbsolutePath(prefixPath)
	if err != nil {
		return fmt.Errorf("could not resolve prefix path: %w", err)
	}
	protonVersionInfo, err := getProtonInfo(appCfg, globalCfg)
	if err != nil {
		return err
	}
	protonBasePath, err := filepath.Abs(getProtonPath(appCfg.ProtonVersion, protonVersionInfo))
	if err != nil {
		return fmt.Errorf("could not resolve proton base path: %w", err)
	}

	wineExecutablePath, err := getWineExecutablePath(protonBasePath)
	if err != nil {
		return err
	}
	fmt.Printf("-> Found wine executable: %s\n", wineExecutablePath)

	if appCfg.SteamAppID != "" && appCfg.SteamAppID != "0" {
		fullExePath := filepath.Join(absPrefix, appCfg.Executable)
		appIDPath := filepath.Join(filepath.Dir(fullExePath), "steam_appid.txt")
		if err := os.WriteFile(appIDPath, []byte(appCfg.SteamAppID), 0644); err != nil {
			log.Printf("⚠️  Warning: Failed to write steam_appid.txt: %v", err)
		}
	}

	fullExePath := filepath.Join(absPrefix, appCfg.Executable)
	args := append([]string{fullExePath}, appCfg.LaunchArgs...)

	cmd := exec.Command(wineExecutablePath, args...)
	cmd.Env = buildProtonEnv(absPrefix, protonBasePath, appCfg, protonVersionInfo, debug)

	return executeCommand(cmd)
}

// RunInContainer launches the application inside the Steam Linux Runtime container.
func RunInContainer(prefixPath string, appCfg config.App, globalCfg config.Global, debug bool) error {
	if appCfg.RuntimeVersion == "" {
		return errors.New("launch_method 'container' requires 'runtime_version' to be set in game.json")
	}

	fmt.Println("-> Running in container mode...")
	protonVersionInfo, err := getProtonInfo(appCfg, globalCfg)
	if err != nil {
		return err
	}
	protonBasePath, err := filepath.Abs(getProtonPath(appCfg.ProtonVersion, protonVersionInfo))
	if err != nil {
		return fmt.Errorf("could not resolve proton base path: %w", err)
	}
	absPrefix, err := fs.GetAbsolutePath(prefixPath)
	if err != nil {
		return fmt.Errorf("could not resolve prefix path: %w", err)
	}

	runtimeDir := filepath.Join("dependencies", "runtime", appCfg.RuntimeVersion)
	entryPointPath := filepath.Join(runtimeDir, "yapl-entry-point")
	shimPath := filepath.Join(runtimeDir, "yapl-shim")
	protonScriptPath, err := getProtonScriptPath(appCfg, globalCfg)
	if err != nil {
		return err
	}

	if _, err := os.Stat(protonScriptPath); os.IsNotExist(err) {
		return fmt.Errorf("could not find 'proton' script. The 'container' method requires a full Proton build (like GE-Proton), not a Wine-only build")
	}

	if appCfg.SteamAppID != "" && appCfg.SteamAppID != "0" {
		fullExePath := filepath.Join(absPrefix, appCfg.Executable)
		appIDPath := filepath.Join(filepath.Dir(fullExePath), "steam_appid.txt")
		if err := os.WriteFile(appIDPath, []byte(appCfg.SteamAppID), 0644); err != nil {
			log.Printf("⚠️  Warning: Failed to write steam_appid.txt: %v", err)
		}
	}

	fullExePath := filepath.Join(absPrefix, appCfg.Executable)
	protonVerb := "waitforexitandrun"

	args := []string{
		"--verb=" + protonVerb,
		"--",
		shimPath,
		protonScriptPath,
		protonVerb,
		fullExePath,
	}
	args = append(args, appCfg.LaunchArgs...)

	cmd := exec.Command(entryPointPath, args...)
	cmd.Env = buildProtonEnv(absPrefix, protonBasePath, appCfg, protonVersionInfo, debug)

	return executeCommand(cmd)
}

// RunWithUMU launches the application using the umu-launcher helper.
func RunWithUMU(prefixPath string, appCfg config.App, globalCfg config.Global, debug bool) error {
	fmt.Println("-> Running with umu-launcher...")

	umuRunPath := "umu-run"
	if !appCfg.UMUOptions.UseSystemBinary {
		ver := appCfg.UMUOptions.Version
		if ver == "" {
			return errors.New("'umu_options.version' must be set")
		}
		vinfo, ok := globalCfg.DependencyVersions["umu-launcher"][ver]
		if !ok {
			return fmt.Errorf("umu-launcher version '%s' not defined in runner.json", ver)
		}
		umuRunPath = filepath.Join("dependencies", "umu-launcher", ver, vinfo.BinPath, "umu-run")
	}

	absPrefix, err := fs.GetAbsolutePath(prefixPath)
	if err != nil {
		return fmt.Errorf("could not resolve prefix path: %w", err)
	}
	protonVersionInfo, err := getProtonInfo(appCfg, globalCfg)
	if err != nil {
		return err
	}
	protonBasePath, err := filepath.Abs(getProtonPath(appCfg.ProtonVersion, protonVersionInfo))
	if err != nil {
		return fmt.Errorf("could not resolve proton base path: %w", err)
	}
	fullExePath := filepath.Join(absPrefix, appCfg.Executable)

	args := append([]string{fullExePath}, append(appCfg.LaunchArgs, appCfg.UMUOptions.LaunchArgs...)...)
	cmd := exec.Command(umuRunPath, args...)

	cmd.Env = buildProtonEnv(absPrefix, protonBasePath, appCfg, protonVersionInfo, debug)
	cmd.Env = append(cmd.Env, "PROTONPATH="+protonBasePath)
	if appCfg.UMUOptions.GameID != "" {
		cmd.Env = append(cmd.Env, "GAMEID="+appCfg.UMUOptions.GameID)
	}
	if appCfg.UMUOptions.Store != "" {
		cmd.Env = append(cmd.Env, "STORE="+appCfg.UMUOptions.Store)
	}

	return executeCommand(cmd)
}

// buildProtonEnv constructs the full environment variable set needed by Proton/Wine.
func buildProtonEnv(absPrefix, protonBasePath string, appCfg config.App, vinfo config.VersionInfo, debug bool) []string {
	clientInstallPath := filepath.Dir(filepath.Join(absPrefix, appCfg.Executable))
	env := os.Environ()

	var newLdPaths []string
	for _, component := range vinfo.LDLibraryPathComponents {
		fullPath := filepath.Join(protonBasePath, component)
		if _, err := os.Stat(fullPath); err == nil {
			newLdPaths = append(newLdPaths, fullPath)
		}
	}

	var newWineDllPaths []string
	for _, component := range vinfo.WineDllPathComponents {
		fullPath := filepath.Join(protonBasePath, component)
		if _, err := os.Stat(fullPath); err == nil {
			newWineDllPaths = append(newWineDllPaths, fullPath)
		}
	}

	if existingLdPath := os.Getenv("LD_LIBRARY_PATH"); existingLdPath != "" {
		newLdPaths = append(newLdPaths, existingLdPath)
	}
	if len(newLdPaths) > 0 {
		env = append(env, "LD_LIBRARY_PATH="+strings.Join(newLdPaths, ":"))
	}
	if len(newWineDllPaths) > 0 {
		env = append(env, "WINEDLLPATH="+strings.Join(newWineDllPaths, ":"))
	}

	existingPath := os.Getenv("PATH")
	protonBin := filepath.Join(protonBasePath, "bin")
	protonDistBin := filepath.Join(protonBasePath, "dist", "bin")
	env = append(env, "PATH="+strings.Join([]string{protonBin, protonDistBin, existingPath}, ":"))

	// WoW64 in Wine 11+ handles 32-bit transparently — always win64.
	env = append(env, "WINEARCH=win64")
	env = append(env, "WINEPREFIX="+absPrefix)
	env = append(env, "STEAM_COMPAT_DATA_PATH="+absPrefix)
	env = append(env, "STEAM_COMPAT_CLIENT_INSTALL_PATH="+clientInstallPath)
	env = append(env, "STEAM_COMPAT_TOOL_PATHS="+protonBasePath)
	env = append(env, "STEAM_COMPAT_MOUNTS="+protonBasePath)
	env = append(env, "STEAM_COMPAT_SHADER_PATH="+filepath.Join(absPrefix, "shadercache"))
	env = append(env, "PROTON_VERB=waitforexitandrun")

	var umuID string
	if appCfg.SteamAppID != "" && appCfg.SteamAppID != "0" {
		appID := appCfg.SteamAppID
		env = append(env, "STEAM_COMPAT_APP_ID="+appID)
		env = append(env, "SteamAppId="+appID)
		env = append(env, "SteamGameId="+appID)
		umuID = appID
	} else {
		umuID = "yapl-default"
	}
	env = append(env, "UMU_ID="+umuID)

	for k, v := range appCfg.EnvironmentVars {
		env = append(env, k+"="+v)
	}
	if overrideStr := buildDllOverridesString(appCfg.DLLOverrides); overrideStr != "" {
		env = append(env, "WINEDLLOVERRIDES="+overrideStr)
	}

	if debug {
		fmt.Println("-> Debug mode enabled.")
		env = append(env, "PROTON_LOG=1", "DXVK_LOG_LEVEL=info")
	}

	return env
}

// --- Private Helpers ---

func executeCommand(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("-> Executing: %s\n", strings.Join(cmd.Args, " "))
	return cmd.Run()
}

func restructureProtonPrefix(absPrefix string) error {
	fmt.Println("-> Restructuring prefix to standard layout...")
	pfxDir := filepath.Join(absPrefix, "pfx")
	info, err := os.Lstat(pfxDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not stat pfx directory: %w", err)
	}
	// Already restructured — pfx is a symlink to .
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}

	files, err := os.ReadDir(pfxDir)
	if err != nil {
		return fmt.Errorf("could not read pfx dir: %w", err)
	}
	for _, file := range files {
		oldPath := filepath.Join(pfxDir, file.Name())
		newPath := filepath.Join(absPrefix, file.Name())
		if err := os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("failed to move '%s': %w", file.Name(), err)
		}
	}

	if err := os.Remove(pfxDir); err != nil {
		return fmt.Errorf("failed to remove temporary pfx directory: %w", err)
	}
	if err := os.Symlink(".", pfxDir); err != nil {
		return fmt.Errorf("failed to create pfx symlink: %w", err)
	}
	fmt.Println("-> Prefix restructured.")
	return nil
}

func buildDllOverridesString(overrides map[string]string) string {
	if len(overrides) == 0 {
		return ""
	}
	keys := make([]string, 0, len(overrides))
	for dll := range overrides {
		keys = append(keys, dll)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, dll := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", dll, overrides[dll]))
	}
	return strings.Join(parts, ";")
}

func getProtonInfo(appCfg config.App, globalCfg config.Global) (config.VersionInfo, error) {
	vinfo, ok := globalCfg.ProtonVersions[appCfg.ProtonVersion]
	if !ok {
		return config.VersionInfo{}, fmt.Errorf("proton version '%s' not defined in runner.json", appCfg.ProtonVersion)
	}
	return vinfo, nil
}

func getProtonPath(version string, vinfo config.VersionInfo) string {
	if vinfo.Path != "" {
		return vinfo.Path
	}
	return filepath.Join("proton", version)
}

func getProtonScriptPath(appCfg config.App, globalCfg config.Global) (string, error) {
	vinfo, err := getProtonInfo(appCfg, globalCfg)
	if err != nil {
		return "", err
	}
	return filepath.Join(getProtonPath(appCfg.ProtonVersion, vinfo), "proton"), nil
}

// getWineExecutablePath finds wine64 (or wine as fallback) within a Proton distribution.
// Wine 11+ WoW64 mode handles 32-bit apps transparently — only a 64-bit binary is needed.
func getWineExecutablePath(protonBasePath string) (string, error) {
	possibleBasePaths := []string{
		filepath.Join(protonBasePath, "files", "bin"),
		filepath.Join(protonBasePath, "dist", "bin"),
		filepath.Join(protonBasePath, "bin"),
	}

	for _, binName := range []string{"wine64", "wine"} {
		for _, basePath := range possibleBasePaths {
			fullPath := filepath.Join(basePath, binName)
			if _, err := os.Stat(fullPath); err == nil {
				return filepath.Abs(fullPath)
			}
		}
	}

	return "", fmt.Errorf("could not find wine64 or wine in %s", protonBasePath)
}
