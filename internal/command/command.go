package command

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"yapl/internal/config"
	"yapl/internal/fs"
)

// InitializePrefix creates and sets up a new Wine prefix.
// It will always use the 'proton' script for initialization as it's the most reliable method.
func InitializePrefix(prefixPath string, appCfg config.App, globalCfg config.Global, debug bool) error {
	absPrefix := fs.MustGetAbsolutePath(prefixPath)
	if err := fs.MustCreateDirectory(absPrefix); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(absPrefix, "system.reg")); err == nil {
		return nil // Prefix already exists
	}

	wineArch := getWineArch(appCfg)
	protonVersionInfo := getProtonInfo(appCfg, globalCfg)
	protonBasePath, _ := filepath.Abs(getProtonPath(appCfg.ProtonVersion, protonVersionInfo, wineArch))

	// Handle 32-bit prefixes with a special direct method
	if wineArch == "win32" {
		fmt.Println("-> Initializing win32 Wine prefix directly...")
		wineExecutablePath, err := getWineExecutablePath(protonBasePath, wineArch)
		if err != nil {
			return err
		}

		// Build a minimal environment just for prefix creation
		env := os.Environ()
		env = append(env, "WINEPREFIX="+absPrefix)
		env = append(env, "WINEARCH="+wineArch)

		cmd := exec.Command(wineExecutablePath, "winecfg")
		cmd.Env = env
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("win32 prefix creation with winecfg failed: %w", err)
		}

		// Proton crashes if this directory doesn't exist in a 32-bit prefix
		syswow64Path := filepath.Join(absPrefix, "drive_c", "windows", "syswow64")
		if err := os.MkdirAll(syswow64Path, 0755); err != nil {
			return fmt.Errorf("failed to create syswow64 directory: %w", err)
		}

		// Create the pfx symlink for consistency
		if err := os.Symlink(".", filepath.Join(absPrefix, "pfx")); err != nil {
			return fmt.Errorf("failed to create pfx symlink: %w", err)
		}

		fmt.Println("-> Prefix created. Launching file explorer for application installation...")
		explorerCfg := appCfg
		explorerCfg.Executable = "drive_c/windows/explorer.exe"
		// Use RunDirectly for win32 setup
		return RunDirectly(prefixPath, explorerCfg, globalCfg, false, debug)
	}

	// Default 64-bit prefix initialization using the proton script
	fmt.Println("-> Initializing Wine prefix using the proton script...")

	if appCfg.ProtonVersion != "system" {
		protonScriptPath := getProtonScriptPath(appCfg, globalCfg, wineArch)
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

	}
	fmt.Println("-> Prefix created. Launching file explorer for application installation...")
	explorerCfg := appCfg
	explorerCfg.Executable = "drive_c/windows/explorer.exe"

	if appCfg.LaunchMethod == "direct" {
		return RunDirectly(prefixPath, explorerCfg, globalCfg, false, debug)
	}
	return RunInContainer(prefixPath, explorerCfg, globalCfg, debug)
}

// RunDirectly launches the application using the 'wine64' or 'wine' binary from the Proton distribution.
// This is a lightweight method that bypasses the Proton script and the Steam Runtime.
func RunDirectly(prefixPath string, appCfg config.App, globalCfg config.Global, isSteam, debug bool) error {
	if isSteam {
		return errors.New("--steam flag is not compatible with 'direct' launch_method. Use 'container' instead")
	}

	fmt.Println("-> Running in direct mode (using wine/wine64)...")

	absPrefix := fs.MustGetAbsolutePath(prefixPath)
	protonVersionInfo := getProtonInfo(appCfg, globalCfg)
	wineArch := getWineArch(appCfg)
	protonBasePath, _ := filepath.Abs(getProtonPath(appCfg.ProtonVersion, protonVersionInfo, wineArch))

	wineExecutablePath, err := getWineExecutablePath(protonBasePath, wineArch)
	if err != nil {
		return err
	}
	fmt.Printf("-> Found wine executable for %s: %s\n", wineArch, wineExecutablePath)

	if appCfg.SteamAppID != "" && appCfg.SteamAppID != "0" {
		fullExePath := filepath.Join(absPrefix, appCfg.Executable)
		exeDir := filepath.Dir(fullExePath)
		appIDPath := filepath.Join(exeDir, "steam_appid.txt")
		if err := os.WriteFile(appIDPath, []byte(appCfg.SteamAppID), 0644); err != nil {
			log.Printf("⚠️  Warning: Failed to write steam_appid.txt: %v", err)
		}
	}

	fullExePath := filepath.Join(absPrefix, appCfg.Executable)
	args := []string{fullExePath}
	args = append(args, appCfg.LaunchArgs...)

	cmd := exec.Command(wineExecutablePath, args...)
	cmd.Env = buildProtonEnv(absPrefix, protonBasePath, appCfg, protonVersionInfo, debug)

	return executeCommand(cmd)
}

// RunInContainer launches the application inside the self-managed Steam Linux Runtime container.
func RunInContainer(prefixPath string, appCfg config.App, globalCfg config.Global, debug bool) error {
	if appCfg.RuntimeVersion == "" {
		return errors.New("launch_method 'container' requires 'runtime_version' to be set in game.json")
	}

	fmt.Println("-> Running in container mode...")
	protonVersionInfo := getProtonInfo(appCfg, globalCfg)
	wineArch := getWineArch(appCfg)
	protonBasePath, _ := filepath.Abs(getProtonPath(appCfg.ProtonVersion, protonVersionInfo, wineArch))
	absPrefix := fs.MustGetAbsolutePath(prefixPath)

	runtimeDir := filepath.Join("dependencies", "runtime", appCfg.RuntimeVersion)
	entryPointPath := filepath.Join(runtimeDir, "yapl-entry-point")
	shimPath := filepath.Join(runtimeDir, "yapl-shim")
	protonScriptPath := getProtonScriptPath(appCfg, globalCfg, wineArch)

	if _, err := os.Stat(protonScriptPath); os.IsNotExist(err) {
		return fmt.Errorf("could not find 'proton' script. The 'container' method requires a full Proton build (like GE-Proton), not a Wine-only build")
	}

	if appCfg.SteamAppID != "" && appCfg.SteamAppID != "0" {
		fullExePath := filepath.Join(absPrefix, appCfg.Executable)
		exeDir := filepath.Dir(fullExePath)
		appIDPath := filepath.Join(exeDir, "steam_appid.txt")
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

	absPrefix := fs.MustGetAbsolutePath(prefixPath)
	protonVersionInfo := getProtonInfo(appCfg, globalCfg)
	wineArch := getWineArch(appCfg)
	protonBasePath, _ := filepath.Abs(getProtonPath(appCfg.ProtonVersion, protonVersionInfo, wineArch))
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

// buildProtonEnv constructs the necessary environment for Proton/Wine to run.
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

	env = append(env, "WINEARCH="+getWineArch(appCfg))
	env = append(env, "WINEPREFIX="+absPrefix)
	env = append(env, "STEAM_COMPAT_DATA_PATH="+absPrefix)
	env = append(env, "STEAM_COMPAT_CLIENT_INSTALL_PATH="+clientInstallPath)
	env = append(env, "STEAM_COMPAT_TOOL_PATHS="+protonBasePath)
	env = append(env, "STEAM_COMPAT_MOUNTS="+protonBasePath)
	env = append(env, "STEAM_COMPAT_SHADER_PATH="+filepath.Join(absPrefix, "shadercache"))
	env = append(env, "PROTON_VERB=waitforexitandrun")

	// Set UMU_ID for compatibility with patched Proton scripts.
	// This signals that we are a third-party launcher.
	var umuID string
	if appCfg.SteamAppID != "" && appCfg.SteamAppID != "0" {
		appID := appCfg.SteamAppID
		env = append(env, "STEAM_COMPAT_APP_ID="+appID)
		env = append(env, "SteamAppId="+appID)
		env = append(env, "SteamGameId="+appID)
		umuID = appID
	} else {
		// Use a default ID for non-steam games to ensure the safe launch path is taken.
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
	if err := cmd.Run(); err != nil {
		log.Printf("❌ Application exited with an error: %v", err)
	}
	return nil
}

func restructureProtonPrefix(absPrefix string) error {
	fmt.Println("-> Restructuring prefix to standard layout...")
	pfxDir := filepath.Join(absPrefix, "pfx")
	if _, err := os.Stat(pfxDir); os.IsNotExist(err) {
		return nil // Nothing to do
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
	var parts []string
	for dll, setting := range overrides {
		parts = append(parts, fmt.Sprintf("%s=%s", dll, setting))
	}
	return strings.Join(parts, ";")
}

func getProtonInfo(appCfg config.App, globalCfg config.Global) config.VersionInfo {
	vinfo, ok := globalCfg.ProtonVersions[appCfg.ProtonVersion]
	if !ok {
		log.Fatalf("❌ Proton version '%s' not defined in runner.json", appCfg.ProtonVersion)
	}
	return vinfo
}

func getProtonPath(version string, vinfo config.VersionInfo, wineArch string) string {
	if vinfo.Path != "" {
		return vinfo.Path
	}
	if wineArch == "win32" {
		return filepath.Join("proton", version+"-win32")
	}
	return filepath.Join("proton", version)
}

// getProtonScriptPath returns the absolute path to the main 'proton' script.
func getProtonScriptPath(appCfg config.App, globalCfg config.Global, wineArch string) string {
	vinfo := getProtonInfo(appCfg, globalCfg)
	// Make sure we get the path from the correct (potentially patched) directory
	basePath := getProtonPath(appCfg.ProtonVersion, vinfo, wineArch)
	return filepath.Join(basePath, "proton")
}

// getWineExecutablePath finds the correct wine binary within a Proton distribution based on architecture.
func getWineExecutablePath(protonBasePath string, wineArch string) (string, error) {
	var binariesToSearch []string
	if wineArch == "win32" {
		binariesToSearch = []string{"wine"}
	} else {
		// Default to win64, but also check for 'wine' as a fallback.
		binariesToSearch = []string{"wine64", "wine"}
	}

	// Wine builds can place the binaries in different locations. Check the most common ones.
	possibleBasePaths := []string{
		filepath.Join(protonBasePath, "files", "bin"),
		filepath.Join(protonBasePath, "dist", "bin"),
		filepath.Join(protonBasePath, "bin"),
	}

	for _, binName := range binariesToSearch {
		for _, basePath := range possibleBasePaths {
			fullPath := filepath.Join(basePath, binName)
			if _, err := os.Stat(fullPath); err == nil {
				return filepath.Abs(fullPath)
			}
		}
	}

	return "", fmt.Errorf("could not find a suitable wine/wine64 executable in %s for architecture %s", protonBasePath, wineArch)
}

func getWineArch(appCfg config.App) string {
	if appCfg.WineArch != "" {
		return appCfg.WineArch
	}
	return "win64"
}
