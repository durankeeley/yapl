package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"yapl/internal/fs"
)

// --- Configuration Structs ---

type VersionInfo struct {
	URL                     string   `json:"url,omitempty"`
	Path                    string   `json:"path,omitempty"`
	BinPath                 string   `json:"bin_path,omitempty"`
	CheckForUpdates         bool     `json:"check_for_updates,omitempty"`
	LDLibraryPathComponents []string `json:"ld_library_path_components,omitempty"`
	WineDllPathComponents   []string `json:"wine_dll_path_components,omitempty"`
	PythonHome              string   `json:"python_home,omitempty"`
	PythonPath              string   `json:"python_path,omitempty"`
}

type Global struct {
	ProtonVersions     map[string]VersionInfo            `json:"proton_versions"`
	RuntimeVersions    map[string]VersionInfo            `json:"runtime_versions"`
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

type App struct {
	ProtonVersion   string            `json:"proton_version"`
	RuntimeVersion  string            `json:"runtime_version,omitempty"`
	LaunchMethod    string            `json:"launch_method,omitempty"`
	Executable      string            `json:"executable"`
	SteamAppID      string            `json:"steam_app_id,omitempty"`
	WineArch        string            `json:"wine_arch,omitempty"`
	LaunchArgs      []string          `json:"launch_args,omitempty"`
	Winetricks      []string          `json:"winetricks,omitempty"`
	UMUOptions      UMUOptions        `json:"umu_options,omitempty"`
	Dependencies    AppDependencies   `json:"dependencies"`
	DLLOverrides    map[string]string `json:"dll_overrides"`
	EnvironmentVars map[string]string `json:"environment_vars"`
}

// --- Loading and Saving Logic ---

func LoadOrCreateGlobal(path string) (Global, error) {
	var g Global
	err := readJSONFile(path, &g)
	if !os.IsNotExist(err) {
		return g, err
	}

	fmt.Println("-> No global 'runner.json' found. Creating a default one.")
	defaultCfg := Global{
		ProtonVersions:     map[string]VersionInfo{"EDIT_ME": {URL: "URL_TO_PROTON_TAR", Path: "OR_PROVIDE_ABSOLUTE_PATH_TO_PROTON_DIR"}},
		RuntimeVersions:    map[string]VersionInfo{"sniper": {URL: "https://repo.steampowered.com/steamrt-images-sniper/snapshots/latest-container-runtime-public-beta/SteamLinuxRuntime_sniper.tar.xz", CheckForUpdates: true}},
		DependencyVersions: map[string]map[string]VersionInfo{"dxvk": {"EDIT_ME": {URL: "URL_TO_DXVK_TAR"}}},
	}
	if err := writeJSONFile(path, defaultCfg); err != nil {
		return Global{}, fmt.Errorf("failed writing default runner.json: %w", err)
	}
	fmt.Println("✅ Default runner.json created. Please edit it with download URLs or local paths.")
	return defaultCfg, nil
}

func LoadOrCreateApp(appType, appName string, globalCfg Global) (App, error) {
	appDir := filepath.Join(appType, appName)
	configName := "game.json"
	if appType == "apps" {
		configName = "app.json"
	}
	configPath := filepath.Join(appDir, configName)

	var cfg App
	err := readJSONFile(configPath, &cfg)
	if !os.IsNotExist(err) {
		return cfg, err // Return on success or any error other than file not found
	}

	fmt.Printf("-> No config found. Creating a default '%s' in '%s'...\n", configName, appDir)
	if err := fs.MustCreateDirectory(appDir); err != nil {
		return App{}, err
	}

	firstProton := "PLEASE_SET_A_VERSION_FROM_RUNNER.JSON"
	for key := range globalCfg.ProtonVersions {
		firstProton = key
		break
	}

	defaultCfg := App{
		ProtonVersion: firstProton,
		LaunchMethod:  "direct",
		Executable:    "drive_c/windows/explorer.exe",
		LaunchArgs:    []string{},
		Winetricks:    []string{},
	}

	if err := writeJSONFile(configPath, defaultCfg); err != nil {
		return App{}, err
	}
	fmt.Printf("✅ Default %s created.\n", configName)
	return defaultCfg, nil
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
