package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"yapl/internal/defaults"
	"yapl/internal/fs"
	"yapl/internal/paths"
)

// --- Configuration Structs ---

type VersionInfo struct {
	URL                     string   `json:"url,omitempty"`
	Path                    string   `json:"path,omitempty"`
	BinPath                 string   `json:"bin_path,omitempty"`
	CheckForUpdates         bool     `json:"check_for_updates,omitempty"`
	LDLibraryPathComponents []string `json:"ld_library_path_components,omitempty"`
	WineDllPathComponents   []string `json:"wine_dll_path_components,omitempty"`
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
	LaunchArgs      []string          `json:"launch_args,omitempty"`
	Winetricks      []string          `json:"winetricks,omitempty"`
	UMUOptions      UMUOptions        `json:"umu_options,omitempty"`
	Dependencies    AppDependencies   `json:"dependencies"`
	DLLOverrides    map[string]string `json:"dll_overrides"`
	EnvironmentVars map[string]string `json:"environment_vars"`

	BackgroundServices       []string `json:"background_services,omitempty"`
	BackgroundServiceDelayMs int      `json:"background_service_delay_ms,omitempty"`
}

// DXVKDirectXVersions lists the accepted dxvk_directx_version values.
var DXVKDirectXVersions = []string{"9", "10", "11"}

// ValidateApp checks that cfg contains all required fields before any command runs.
// All violations are collected and returned as a single error.
func ValidateApp(cfg App) error {
	var errs []string

	if cfg.ProtonVersion == "" {
		errs = append(errs, "proton_version is required")
	}
	if cfg.Executable == "" {
		errs = append(errs, "executable is required")
	}

	method := cfg.LaunchMethod
	if method == "" {
		method = defaults.LaunchMethod
	}
	switch method {
	case "direct", "umu":
	case "container":
		if cfg.RuntimeVersion == "" {
			errs = append(errs, "runtime_version is required when launch_method is container")
		}
	default:
		errs = append(errs, "launch_method must be one of "+strings.Join(defaults.LaunchMethods, ", "))
	}

	if v := cfg.Dependencies.DXVKDirectXVersion; v != "" && !slices.Contains(DXVKDirectXVersions, v) {
		errs = append(errs, fmt.Sprintf(`dxvk_directx_version must be "9", "10", or "11" (got %q)`, v))
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("config validation failed: %s", strings.Join(errs, "; "))
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

func LoadOrCreateApp(appType, appName, configName string, globalCfg Global, defaultMethod string) (App, error) {
	appDir := paths.AppDir(appType, appName)

	var configFileName string
	if configName != "" {
		configFileName = configName
	} else {
		// No custom name, use the default
		configFileName = paths.DefaultConfigName(appType)
	}

	configPath := filepath.Join(appDir, configFileName)

	var cfg App
	err := readJSONFile(configPath, &cfg)
	if !os.IsNotExist(err) {
		return cfg, err // Return on success or any error other than file not found
	}

	fmt.Printf("-> No config found. Creating a default '%s' in '%s'...\n", configFileName, appDir)
	if err := fs.MustCreateDirectory(appDir); err != nil {
		return App{}, err
	}

	firstProton := "PLEASE_SET_A_VERSION_FROM_RUNNER.JSON"
	for key := range globalCfg.ProtonVersions {
		firstProton = key
		break
	}

	firstRuntime := ""
	for key := range globalCfg.RuntimeVersions {
		firstRuntime = key
		break
	}

	launchMethod := defaults.LaunchMethod
	if defaultMethod != "" {
		launchMethod = defaultMethod
	}

	defaultCfg := App{
		ProtonVersion:  firstProton,
		RuntimeVersion: firstRuntime,
		LaunchMethod:   launchMethod,
		Executable:     "drive_c/windows/explorer.exe",
		LaunchArgs:     []string{},
		Winetricks:     []string{},
	}

	if err := writeJSONFile(configPath, defaultCfg); err != nil {
		return App{}, err
	}
	fmt.Printf("✅ Default %s created.\n", configFileName)
	return defaultCfg, nil
}

// LoadGlobal reads runner.json without creating a default when it is missing.
func LoadGlobal(path string) (Global, error) {
	var g Global
	err := readJSONFile(path, &g)
	return g, err
}

// SaveGlobal writes g to path as indented JSON.
func SaveGlobal(path string, g Global) error {
	return writeJSONFile(path, g)
}

// MergeGlobal adds every version from bundled that local does not define.
// Local entries always win. It returns the merged config and the sorted
// "kind/version" keys that were added, without mutating local.
func MergeGlobal(local, bundled Global) (Global, []string) {
	var added []string
	merged := Global{
		ProtonVersions:     mergeVersions(local.ProtonVersions, bundled.ProtonVersions, paths.KindProton, &added),
		RuntimeVersions:    mergeVersions(local.RuntimeVersions, bundled.RuntimeVersions, paths.KindRuntime, &added),
		DependencyVersions: map[string]map[string]VersionInfo{},
	}
	for kind, versions := range local.DependencyVersions {
		merged.DependencyVersions[kind] = versions
	}
	for kind, versions := range bundled.DependencyVersions {
		merged.DependencyVersions[kind] = mergeVersions(local.DependencyVersions[kind], versions, paths.DependenciesDir+"/"+kind, &added)
	}
	sort.Strings(added)
	return merged, added
}

func mergeVersions(local, bundled map[string]VersionInfo, label string, added *[]string) map[string]VersionInfo {
	out := make(map[string]VersionInfo, len(local)+len(bundled))
	for k, v := range local {
		out[k] = v
	}
	for k, v := range bundled {
		if _, exists := out[k]; !exists {
			out[k] = v
			*added = append(*added, label+"/"+k)
		}
	}
	return out
}

// LoadApp reads an existing app config file without creating one if missing.
func LoadApp(path string) (App, error) {
	var cfg App
	err := readJSONFile(path, &cfg)
	return cfg, err
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
