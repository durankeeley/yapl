package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/config"
)

// --- PRD-21: writeSteamAppID helper ---

func TestWriteSteamAppID_CreatesFileNextToExecutable(t *testing.T) {
	// Given a prefix directory with the game's directory structure present
	d := t.TempDir()
	gameDir := filepath.Join(d, "drive_c", "games")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}

	// When writeSteamAppID is called with a valid app ID
	err := writeSteamAppID(d, "drive_c/games/game.exe", "12345")

	// Then no error is returned
	if err != nil {
		t.Fatalf("writeSteamAppID returned unexpected error: %v", err)
	}

	// And steam_appid.txt is created next to the executable with the correct content
	content, err := os.ReadFile(filepath.Join(gameDir, "steam_appid.txt"))
	if err != nil {
		t.Fatalf("expected steam_appid.txt to be created next to the executable: %v", err)
	}
	if string(content) != "12345" {
		t.Fatalf("expected content '12345', got %q", string(content))
	}
}

func TestWriteSteamAppID_SkipsWhenAppIDEmpty(t *testing.T) {
	// Given a prefix directory with no subdirectories (so a write attempt would fail)
	d := t.TempDir()

	// When writeSteamAppID is called with an empty app ID
	err := writeSteamAppID(d, "drive_c/game.exe", "")

	// Then no error is returned and no file is created
	if err != nil {
		t.Fatalf("expected nil error for empty appID, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(d, "drive_c", "steam_appid.txt")); !os.IsNotExist(statErr) {
		t.Error("expected no steam_appid.txt to be created when appID is empty")
	}
}

// --- WINEARCH environment variable ---

func TestBuildProtonEnv_AlwaysSetsWin64Arch(t *testing.T) {
	// Given any app config (wine_arch no longer exists; WoW64 handles 32-bit transparently)
	// When buildProtonEnv is called
	env := buildProtonEnv(t.TempDir(), t.TempDir(), config.App{Executable: "drive_c/game.exe"}, config.VersionInfo{}, false)

	// Then WINEARCH is always set to win64
	for _, e := range env {
		if e == "WINEARCH=win64" {
			return
		}
	}
	t.Fatal("expected WINEARCH=win64 in environment but it was not found")
}

// --- getProtonPath ---

func TestGetProtonPath_UsesCustomPathWhenSet(t *testing.T) {
	// Given a version info with a custom local path configured
	// When getProtonPath is called
	got := getProtonPath("ge9", config.VersionInfo{Path: "/custom/proton"})
	// Then it returns the custom path
	if got != "/custom/proton" {
		t.Fatalf("expected /custom/proton, got %q", got)
	}
}

func TestGetProtonPath_UsesStandardPathWhenNoCustomPath(t *testing.T) {
	// Given a version with no custom local path (download-based)
	// When getProtonPath is called
	got := getProtonPath("ge9", config.VersionInfo{})
	// Then it returns the standard versioned path under proton/
	want := filepath.Join("proton", "ge9")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestGetProtonPath_NeverAppendsWin32Suffix(t *testing.T) {
	// Given any version (win32 suffix logic should not exist after PRD-2)
	// When getProtonPath is called
	got := getProtonPath("ge9", config.VersionInfo{})
	// Then the returned path never contains a -win32 suffix
	if strings.Contains(got, "win32") {
		t.Fatalf("expected no win32 suffix in path after win32 removal, got %q", got)
	}
}

// --- getProtonInfo ---

func TestGetProtonInfo_KnownVersion(t *testing.T) {
	// Given a global config that defines a proton version
	globalCfg := config.Global{
		ProtonVersions: map[string]config.VersionInfo{
			"ge9": {URL: "http://example.com/ge9.tar.xz"},
		},
	}
	appCfg := config.App{ProtonVersion: "ge9"}

	// When getProtonInfo is called
	vinfo, err := getProtonInfo(appCfg, globalCfg)

	// Then it returns the version info with no error
	if err != nil {
		t.Fatalf("expected no error for a known version, got: %v", err)
	}
	if vinfo.URL != "http://example.com/ge9.tar.xz" {
		t.Fatalf("expected URL to match, got %q", vinfo.URL)
	}
}

func TestGetProtonInfo_UnknownVersion(t *testing.T) {
	// Given a global config that does NOT define the requested proton version
	globalCfg := config.Global{
		ProtonVersions: map[string]config.VersionInfo{},
	}
	appCfg := config.App{ProtonVersion: "ghost-version"}

	// When getProtonInfo is called
	_, err := getProtonInfo(appCfg, globalCfg)

	// Then it returns an error (not a panic or os.Exit)
	if err == nil {
		t.Fatal("expected an error for an unknown proton version but got nil")
	}
	if !strings.Contains(err.Error(), "ghost-version") {
		t.Fatalf("expected error to mention the version name, got: %v", err)
	}
}

// --- buildDllOverridesString ---

func TestBuildDllOverridesString_ProducesDeterministicSortedOutput(t *testing.T) {
	// Given a DLL overrides map with multiple entries
	overrides := map[string]string{
		"d3d11":   "n,b",
		"d3d9":    "n",
		"dxgi":    "n,b",
		"dinput8": "n",
	}

	// When buildDllOverridesString is called multiple times
	results := make(map[string]int)
	for i := 0; i < 20; i++ {
		results[buildDllOverridesString(overrides)]++
	}

	// Then it always produces the same string
	if len(results) != 1 {
		t.Fatalf("buildDllOverridesString produced %d different outputs — it is non-deterministic", len(results))
	}

	// And the DLL names appear in sorted (alphabetical) order
	result := buildDllOverridesString(overrides)
	parts := strings.Split(result, ";")
	prev := ""
	for _, p := range parts {
		key := strings.SplitN(p, "=", 2)[0]
		if key < prev {
			t.Fatalf("DLL keys are not sorted: %q appears after %q in %q", key, prev, result)
		}
		prev = key
	}
}

func TestBuildDllOverridesString_EmptyOverrides(t *testing.T) {
	// Given no DLL overrides configured
	// When buildDllOverridesString is called
	got := buildDllOverridesString(nil)
	// Then it returns an empty string (no env var should be set)
	if got != "" {
		t.Fatalf("expected empty string for nil overrides, got %q", got)
	}
}

// --- restructureProtonPrefix ---

func TestRestructureProtonPrefix_MovesFilesAndCreatesSymlink(t *testing.T) {
	// Given a prefix directory containing a pfx/ subdirectory with files
	d := t.TempDir()
	pfxDir := filepath.Join(d, "pfx")
	if err := os.Mkdir(pfxDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pfxDir, "system.reg"), []byte("reg"), 0644); err != nil {
		t.Fatal(err)
	}

	// When restructureProtonPrefix is called
	if err := restructureProtonPrefix(d); err != nil {
		t.Fatalf("restructureProtonPrefix returned an unexpected error: %v", err)
	}

	// Then system.reg is moved to the prefix root
	if _, err := os.Stat(filepath.Join(d, "system.reg")); err != nil {
		t.Fatal("system.reg was not moved to the prefix root")
	}

	// And pfx/ becomes a symlink pointing to the current directory
	info, err := os.Lstat(filepath.Join(d, "pfx"))
	if err != nil {
		t.Fatalf("pfx entry not present after restructure: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected pfx to be a symlink after restructure, got mode %v", info.Mode())
	}
}

func TestRestructureProtonPrefix_NoPfxDirectoryIsNoop(t *testing.T) {
	// Given a prefix directory that has no pfx/ subdirectory
	d := t.TempDir()

	// When restructureProtonPrefix is called
	err := restructureProtonPrefix(d)

	// Then it returns no error and leaves the directory unchanged
	if err != nil {
		t.Fatalf("expected no error when pfx directory is missing, got: %v", err)
	}
}

// --- buildProtonEnv ---

func TestBuildProtonEnv_ContainsCriticalEnvironmentVariables(t *testing.T) {
	// Given a complete app config with a Steam app ID and custom env var
	d := t.TempDir()
	protonBase := t.TempDir()
	appCfg := config.App{
		Executable:      "drive_c/game.exe",
		SteamAppID:      "12345",
		EnvironmentVars: map[string]string{"CUSTOM_VAR": "custom_value"},
		DLLOverrides:    map[string]string{"d3d11": "n,b"},
	}
	vinfo := config.VersionInfo{}

	// When buildProtonEnv is called
	env := buildProtonEnv(d, protonBase, appCfg, vinfo, false)

	// Then the environment contains all critical Proton/Wine/Steam variables
	mustContain := []string{
		"WINEPREFIX=",
		"WINEARCH=",
		"STEAM_COMPAT_DATA_PATH=",
		"STEAM_COMPAT_CLIENT_INSTALL_PATH=",
		"STEAM_COMPAT_APP_ID=12345",
		"SteamAppId=12345",
		"UMU_ID=12345",
		"CUSTOM_VAR=custom_value",
		"WINEDLLOVERRIDES=",
	}
	for _, prefix := range mustContain {
		found := false
		for _, e := range env {
			if strings.HasPrefix(e, prefix) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("environment is missing an entry with prefix %q", prefix)
		}
	}
}

func TestBuildProtonEnv_DebugModeEnablesProtonLogging(t *testing.T) {
	// Given an app config and debug=true
	d := t.TempDir()
	appCfg := config.App{Executable: "drive_c/game.exe"}

	// When buildProtonEnv is called with debug enabled
	env := buildProtonEnv(d, t.TempDir(), appCfg, config.VersionInfo{}, true)

	// Then PROTON_LOG=1 is present in the environment
	found := false
	for _, e := range env {
		if e == "PROTON_LOG=1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected PROTON_LOG=1 in debug environment but it was not found")
	}
}

func TestBuildProtonEnv_NoSteamAppIDUsesDefaultUMUID(t *testing.T) {
	// Given an app config with no SteamAppID
	d := t.TempDir()
	appCfg := config.App{Executable: "drive_c/game.exe"}

	// When buildProtonEnv is called
	env := buildProtonEnv(d, t.TempDir(), appCfg, config.VersionInfo{}, false)

	// Then UMU_ID is set to the yapl default fallback value
	for _, e := range env {
		if e == "UMU_ID=yapl-default" {
			return
		}
	}
	t.Fatal("expected UMU_ID=yapl-default when no SteamAppID is configured")
}

// --- executeCommand ---

func TestExecuteCommand_ReturnsErrorWhenCommandFails(t *testing.T) {
	// Given a command that exits with a non-zero status
	cmd := buildFalseCmd()

	// When executeCommand is called
	err := executeCommand(cmd)

	// Then it returns an error (not nil)
	if err == nil {
		t.Fatal("expected an error when the command exits non-zero but got nil")
	}
}

func TestExecuteCommand_ReturnsNilWhenCommandSucceeds(t *testing.T) {
	// Given a command that exits successfully
	cmd := buildTrueCmd()

	// When executeCommand is called
	err := executeCommand(cmd)

	// Then it returns nil
	if err != nil {
		t.Fatalf("expected nil for a successful command but got: %v", err)
	}
}
