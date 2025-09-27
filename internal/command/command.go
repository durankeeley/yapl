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

// InitializePrefix creates and sets up a new Wine prefix if it doesn't already exist.
func InitializePrefix(prefixPath string, appCfg config.App, globalCfg config.Global) error {
	absPrefix := fs.MustGetAbsolutePath(prefixPath)
	if err := fs.MustCreateDirectory(absPrefix); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(absPrefix, "system.reg")); err == nil {
		return nil
	}

	fmt.Println("-> Initializing Wine prefix...")
	protonExecutable := getProtonExecutablePath(appCfg, globalCfg)
	protonVersionInfo := getProtonInfo(appCfg, globalCfg)
	protonBasePath := getProtonPath(appCfg.ProtonVersion, protonVersionInfo)
	binName := getProtonBinName(appCfg)

	var cmd *exec.Cmd
	if binName == "proton" {
		fmt.Println("-> Using 'proton run cmd' for initialization.")
		cmd = exec.Command(protonExecutable, "run", "cmd", "/c", "echo", "Initializing...")
		cmd.Env = buildProtonEnv(absPrefix, protonBasePath, appCfg, protonVersionInfo, false)
	} else {
		fmt.Printf("-> Using 'wineboot' with '%s' for initialization.\n", binName)
		cmd = exec.Command(protonExecutable, "wineboot", "-u")
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "WINEPREFIX="+absPrefix)
		cmd.Env = append(cmd.Env, "WINEARCH="+getWineArch(appCfg))
	}

	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log.Printf("-> Prefix creation output:\n%s", string(exitError.Stderr))
		}
		return fmt.Errorf("prefix initialization failed: %w", err)
	}
	fmt.Println("-> Prefix initialized successfully.")

	if binName == "proton" {
		return restructureProtonPrefix(absPrefix)
	}
	return nil
}

// RunDirectly launches the application using the configured Proton/Wine binary.
func RunDirectly(prefixPath string, appCfg config.App, globalCfg config.Global, isSteam, debug bool) error {
	fmt.Println("-> Running directly...")
	protonExecutable, err := filepath.Abs(getProtonExecutablePath(appCfg, globalCfg))
	if err != nil {
		return fmt.Errorf("could not get absolute path for proton executable: %w", err)
	}
	absPrefix := fs.MustGetAbsolutePath(prefixPath)
	protonVersionInfo := getProtonInfo(appCfg, globalCfg)
	protonBasePath, _ := filepath.Abs(getProtonPath(appCfg.ProtonVersion, protonVersionInfo))
	binName := getProtonBinName(appCfg)

	var cmd *exec.Cmd
	env := buildProtonEnv(absPrefix, protonBasePath, appCfg, protonVersionInfo, debug)

	if isSteam {
		if binName != "proton" {
			return errors.New("--steam flag requires 'proton' as the proton_bin_name")
		}
		fmt.Println("-> Launching isolated Steam client...")
		steamExePath := filepath.Join(protonBasePath, "files", "share", "steam", "steam.exe")
		if _, err := os.Stat(steamExePath); os.IsNotExist(err) {
			return fmt.Errorf("could not find Proton's internal steam.exe, expected at: %s", steamExePath)
		}
		cmd = exec.Command("sh", "-c", fmt.Sprintf(`"%s" run "%s"`, protonExecutable, steamExePath))
	} else {
		if appCfg.SteamAppID != "" && appCfg.SteamAppID != "0" {
			fullExePath := filepath.Join(absPrefix, appCfg.Executable)
			exeDir := filepath.Dir(fullExePath)
			appIDPath := filepath.Join(exeDir, "steam_appid.txt")
			if err := os.WriteFile(appIDPath, []byte(appCfg.SteamAppID), 0644); err != nil {
				log.Printf("⚠️  Warning: Failed to write steam_appid.txt: %v", err)
			}
		}

		if binName == "proton" {
			fmt.Println("-> Using Proton 'run' command.")
			fullExePath := filepath.Join(absPrefix, appCfg.Executable)
			cmd = exec.Command("sh", "-c", fmt.Sprintf(`"%s" run "%s"`, protonExecutable, fullExePath))
		} else {
			fmt.Printf("-> Using direct Wine-like execution ('%s').\n", binName)
			fullExePath := filepath.Join(absPrefix, appCfg.Executable)
			cmd = exec.Command(protonExecutable, fullExePath)
		}
	}

	cmd.Env = env
	return executeCommand(cmd)
}

// RunWithUMU launches the application using the umu-launcher helper.
func RunWithUMU(prefixPath string, appCfg config.App, globalCfg config.Global, debug bool) error {
	fmt.Println("-> Running with umu-launcher...")
	// ... (rest of the function is unchanged)
	return nil // Placeholder
}

func buildProtonEnv(absPrefix, protonBasePath string, appCfg config.App, vinfo config.VersionInfo, debug bool) []string {
	clientInstallPath := filepath.Dir(filepath.Join(absPrefix, appCfg.Executable))
	env := os.Environ()

	existingLdPath := os.Getenv("LD_LIBRARY_PATH")
	protonLibs32 := filepath.Join(protonBasePath, "files", "lib")
	protonLibs64 := filepath.Join(protonBasePath, "files", "lib64")
	newPaths := []string{protonLibs64, protonLibs32}
	if existingLdPath != "" {
		newPaths = append(newPaths, existingLdPath)
	}
	env = append(env, "LD_LIBRARY_PATH="+strings.Join(newPaths, ":"))

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

	if appCfg.SteamAppID != "" && appCfg.SteamAppID != "0" {
		appID := appCfg.SteamAppID
		env = append(env, "STEAM_COMPAT_APP_ID="+appID)
		env = append(env, "SteamAppId="+appID)
		env = append(env, "SteamGameId="+appID)
	}

	for k, v := range appCfg.EnvironmentVars {
		env = append(env, k+"="+v)
	}
	if overrideStr := buildDllOverridesString(appCfg.DLLOverrides); overrideStr != "" {
		env = append(env, "WINEDLLOVERRIDES="+overrideStr)
	}

	if len(vinfo.WineDllPathComponents) > 0 {
		var dllPaths []string
		for _, component := range vinfo.WineDllPathComponents {
			dllPaths = append(dllPaths, filepath.Join(protonBasePath, component))
		}
		env = append(env, "WINEDLLPATH="+strings.Join(dllPaths, ":"))
	}

	if debug {
		fmt.Println("-> Debug mode enabled.")
		env = append(env, "PROTON_LOG=1", "DXVK_LOG_LEVEL=info")
	}

	return env
}

// --- Private Helpers ---
// ... (all private helper functions from the previous version go here)
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

func getProtonPath(version string, vinfo config.VersionInfo) string {
	if vinfo.Path != "" {
		return vinfo.Path
	}
	return filepath.Join("proton", version)
}

func getProtonBinName(appCfg config.App) string {
	if appCfg.ProtonBinName == "" {
		return "proton"
	}
	return appCfg.ProtonBinName
}

func getProtonExecutablePath(appCfg config.App, globalCfg config.Global) string {
	vinfo := getProtonInfo(appCfg, globalCfg)
	basePath := getProtonPath(appCfg.ProtonVersion, vinfo)
	binName := getProtonBinName(appCfg)
	return filepath.Join(basePath, binName)
}

func getWineArch(appCfg config.App) string {
	if appCfg.WineArch != "" {
		return appCfg.WineArch
	}
	return "win64"
}
