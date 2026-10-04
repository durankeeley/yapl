package command

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"yapl/internal/config"
	"yapl/internal/fs"
	"yapl/internal/paths"
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

	env = append(env, "PATH="+strings.Join(append(wineBinPaths(protonBasePath), os.Getenv("PATH")), ":"))

	var wineDllPaths []string
	for _, component := range protonVersionInfo.WineDllPathComponents {
		fullPath := filepath.Join(protonBasePath, component)
		if _, err := os.Stat(fullPath); err == nil {
			wineDllPaths = append(wineDllPaths, fullPath)
		}
	}
	if len(wineDllPaths) > 0 {
		env = append(env, "WINEDLLPATH="+strings.Join(wineDllPaths, ":"))
	}

	env = appendStandaloneWineEnv(env, protonBasePath)

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

	fmt.Println("-> Prefix created (direct method). Copy your game files into the prefix or install via 'yapl run'.")
	return nil
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
			fmt.Fprintf(os.Stderr, "-> Prefix creation output:\n%s\n", string(exitError.Stderr))
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

	if err := writeSteamAppID(absPrefix, appCfg.Executable, appCfg.SteamAppID); err != nil {
		return fmt.Errorf("failed to write steam_appid.txt: %w", err)
	}

	fullExePath := executablePath(absPrefix, appCfg.Executable)
	args := append([]string{fullExePath}, appCfg.LaunchArgs...)

	cmd := exec.Command(wineExecutablePath, args...)
	cmd.Env = buildProtonEnv(absPrefix, protonBasePath, appCfg, protonVersionInfo, debug)

	// A stale wineserver from a previous run (possibly a different Wine version) will
	// cause a protocol version mismatch. Kill it so wine starts a fresh one.
	killStaleWineserver(protonBasePath, cmd.Env)

	stopServices := launchBackgroundServices(appCfg.BackgroundServices, wineExecutablePath, nil, cmd.Env, absPrefix, backgroundServiceDelay(appCfg))
	defer stopServices()

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

	runtimeDir := paths.Runtime(appCfg.RuntimeVersion)
	entryPointPath := filepath.Join(runtimeDir, "yapl-entry-point")
	shimPath := filepath.Join(runtimeDir, "yapl-shim")
	protonScriptPath, err := getProtonScriptPath(appCfg, globalCfg)
	if err != nil {
		return err
	}

	if _, err := os.Stat(protonScriptPath); os.IsNotExist(err) {
		return fmt.Errorf("could not find 'proton' script. The 'container' method requires a full Proton build (like GE-Proton), not a Wine-only build")
	}

	if err := writeSteamAppID(absPrefix, appCfg.Executable, appCfg.SteamAppID); err != nil {
		return fmt.Errorf("failed to write steam_appid.txt: %w", err)
	}

	fullExePath := executablePath(absPrefix, appCfg.Executable)
	protonVerb := "waitforexitandrun"

	runnerArgs := []string{
		"--verb=" + protonVerb,
		"--",
		shimPath,
		protonScriptPath,
		protonVerb,
	}
	args := append(append(append([]string{}, runnerArgs...), fullExePath), appCfg.LaunchArgs...)

	cmd := exec.Command(entryPointPath, args...)
	cmd.Env = buildProtonEnv(absPrefix, protonBasePath, appCfg, protonVersionInfo, debug)

	stopServices := launchBackgroundServices(appCfg.BackgroundServices, entryPointPath, runnerArgs, cmd.Env, absPrefix, backgroundServiceDelay(appCfg))
	defer stopServices()

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
		umuRunPath = filepath.Join(paths.Dependency(paths.KindUMU, ver), vinfo.BinPath, "umu-run")
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
	fullExePath := executablePath(absPrefix, appCfg.Executable)

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

	stopServices := launchBackgroundServices(appCfg.BackgroundServices, umuRunPath, nil, cmd.Env, absPrefix, backgroundServiceDelay(appCfg))
	defer stopServices()

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

	env = append(env, "PATH="+strings.Join(append(wineBinPaths(protonBasePath), os.Getenv("PATH")), ":"))
	env = appendStandaloneWineEnv(env, protonBasePath)

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

// writeSteamAppID writes the Steam app ID file next to the executable inside the Wine prefix.
// It is a no-op when appID is empty or "0".
func writeSteamAppID(absPrefix, executable, appID string) error {
	// An absolute executable is something outside the prefix (an installer),
	// which must not get a steam_appid.txt dropped next to it.
	if appID == "" || appID == "0" || filepath.IsAbs(executable) {
		return nil
	}
	fullExePath := filepath.Join(absPrefix, executable)
	appIDPath := filepath.Join(filepath.Dir(fullExePath), "steam_appid.txt")
	return os.WriteFile(appIDPath, []byte(appID), 0644)
}

// WaitForWineserver blocks until every Wine process in the prefix has
// exited. Installers often start a child process and return immediately.
func WaitForWineserver(prefixPath string, appCfg config.App, globalCfg config.Global) error {
	absPrefix, err := fs.GetAbsolutePath(prefixPath)
	if err != nil {
		return fmt.Errorf("could not resolve prefix path: %w", err)
	}
	vinfo, err := getProtonInfo(appCfg, globalCfg)
	if err != nil {
		return err
	}
	protonBasePath, err := filepath.Abs(getProtonPath(appCfg.ProtonVersion, vinfo))
	if err != nil {
		return err
	}
	for _, dir := range append(wineBinPaths(protonBasePath), filepath.Join(protonBasePath, "usr", "bin")) {
		server := filepath.Join(dir, "wineserver")
		if !fileExists(server) {
			continue
		}
		cmd := exec.Command(server, "-w")
		cmd.Env = buildProtonEnv(absPrefix, protonBasePath, appCfg, vinfo, false)
		return cmd.Run()
	}
	return nil
}

// --- Private Helpers ---

// executablePath resolves a config executable: relative paths live inside
// the prefix, absolute ones (installers) are used as given.
func executablePath(absPrefix, executable string) string {
	if filepath.IsAbs(executable) {
		return executable
	}
	return filepath.Join(absPrefix, executable)
}

// DefaultBackgroundServiceDelayMs gives helper processes time to open their
// sockets before the main executable starts looking for them.
const DefaultBackgroundServiceDelayMs = 1000

func backgroundServiceDelay(appCfg config.App) int {
	if appCfg.BackgroundServiceDelayMs == 0 {
		return DefaultBackgroundServiceDelayMs
	}
	return appCfg.BackgroundServiceDelayMs
}

// launchBackgroundServices starts each service without waiting for it, sleeps
// delayMs once if anything started, and returns a function that stops them all.
func launchBackgroundServices(services []string, runner string, runnerArgs []string, env []string, absPrefix string, delayMs int) func() {
	var started []*exec.Cmd
	for _, service := range services {
		args := append(append([]string{}, runnerArgs...), filepath.Join(absPrefix, service))
		cmd := exec.Command(runner, args...)
		cmd.Env = env
		cmd.Dir = absPrefix
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// A new process group lets cleanup take down the children Wine spawns too.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: background service %q failed to start: %v\n", service, err)
			continue
		}
		fmt.Printf("-> Started background service %s (pid %d)\n", service, cmd.Process.Pid)
		started = append(started, cmd)
	}

	if len(started) > 0 && delayMs > 0 {
		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}

	return func() {
		for _, cmd := range started {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			cmd.Wait()
		}
	}
}

func executeCommand(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("-> Executing: %s\n", strings.Join(cmd.Args, " "))
	return cmd.Run()
}

func restructureProtonPrefix(absPrefix string) error {
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
	fmt.Println("-> Restructuring prefix to standard layout...")

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

// wineBinPaths returns all existing bin directories for a given Proton or Wine base path,
// covering both Proton layouts (files/bin, dist/bin, bin) and standard Wine (usr/bin).
func wineBinPaths(protonBasePath string) []string {
	candidates := []string{
		filepath.Join(protonBasePath, "files", "bin"),
		filepath.Join(protonBasePath, "dist", "bin"),
		filepath.Join(protonBasePath, "bin"),
		filepath.Join(protonBasePath, "usr", "bin"),
	}
	var found []string
	for _, p := range candidates {
		if fileExists(p) {
			found = append(found, p)
		}
	}
	return found
}

// appendStandaloneWineEnv adds WINELOADER and WINESERVER overrides for standalone Wine
// builds (e.g. wine-tkg) that were compiled with --prefix=/usr. Without these, wine finds
// its loader and server via the hardcoded /usr path, picking up the system Wine instead.
func appendStandaloneWineEnv(env []string, protonBasePath string) []string {
	loaderPath := filepath.Join(protonBasePath, "usr", "lib", "wine", "x86_64-unix", "wine")
	if abs, err := filepath.Abs(loaderPath); err == nil && fileExists(loaderPath) {
		env = append(env, "WINELOADER="+abs)
	}
	serverPath := filepath.Join(protonBasePath, "usr", "bin", "wineserver")
	if abs, err := filepath.Abs(serverPath); err == nil && fileExists(serverPath) {
		env = append(env, "WINESERVER="+abs)
	}
	return env
}

// killStaleWineserver terminates any wineserver running against the prefix in env.
// A stale server from a different Wine version causes protocol mismatch errors.
// Uses the wineserver from the standalone wine layout (usr/bin/wineserver) if present.
// Errors are silently ignored — no running server is a normal state.
func killStaleWineserver(protonBasePath string, env []string) {
	serverPath := filepath.Join(protonBasePath, "usr", "bin", "wineserver")
	abs, err := filepath.Abs(serverPath)
	if err != nil || !fileExists(abs) {
		return
	}
	killCmd := exec.Command(abs, "-k")
	killCmd.Env = env
	killCmd.Run()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
	return paths.Proton(version)
}

func getProtonScriptPath(appCfg config.App, globalCfg config.Global) (string, error) {
	vinfo, err := getProtonInfo(appCfg, globalCfg)
	if err != nil {
		return "", err
	}
	return filepath.Join(getProtonPath(appCfg.ProtonVersion, vinfo), "proton"), nil
}

// getWineExecutablePath finds wine64 (or wine as fallback) within a Proton or standalone Wine
// distribution. Checks both Proton layouts (files/bin, dist/bin, bin) and the standard
// Wine install layout (usr/bin).
func getWineExecutablePath(protonBasePath string) (string, error) {
	possibleBasePaths := []string{
		filepath.Join(protonBasePath, "files", "bin"),
		filepath.Join(protonBasePath, "dist", "bin"),
		filepath.Join(protonBasePath, "bin"),
		filepath.Join(protonBasePath, "usr", "bin"),
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
