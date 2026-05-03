package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/config"
)

// writeGameConfig writes a minimal game.json into games/<name>/
func writeGameConfig(t *testing.T, base, name string, cfg config.App) {
	t.Helper()
	dir := filepath.Join(base, "games", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func writeAppConfig(t *testing.T, base, name string, cfg config.App) {
	t.Helper()
	dir := filepath.Join(base, "apps", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

// --- Find ---

func TestFind_ReturnsGamesWhenOnlyInGames(t *testing.T) {
	// Given a game entry in games/ but nothing in apps/
	d := t.TempDir()
	chdir(t, d)
	writeGameConfig(t, d, "Doom", config.App{ProtonVersion: "ge9", Executable: "drive_c/doom.exe"})

	// When Find is called with the game name
	appType, appName, err := Find("Doom")

	// Then it returns "games" and the name with no error
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if appType != "games" || appName != "Doom" {
		t.Fatalf("expected games/Doom, got %s/%s", appType, appName)
	}
}

func TestFind_ReturnsAppsWhenOnlyInApps(t *testing.T) {
	// Given an app entry in apps/ but nothing in games/
	d := t.TempDir()
	chdir(t, d)
	writeAppConfig(t, d, "SteamCMD", config.App{ProtonVersion: "system", Executable: "drive_c/steamcmd.exe"})

	// When Find is called with the app name
	appType, appName, err := Find("SteamCMD")

	// Then it returns "apps" and the name with no error
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if appType != "apps" || appName != "SteamCMD" {
		t.Fatalf("expected apps/SteamCMD, got %s/%s", appType, appName)
	}
}

func TestFind_ErrorsWhenNotFound(t *testing.T) {
	// Given an empty working directory (no games/ or apps/)
	d := t.TempDir()
	chdir(t, d)

	// When Find is called with a name that doesn't exist
	_, _, err := Find("Ghost")

	// Then it returns an error mentioning the name
	if err == nil {
		t.Fatal("expected an error for a missing entry but got nil")
	}
	if !strings.Contains(err.Error(), "Ghost") {
		t.Fatalf("expected error to mention the missing name, got: %v", err)
	}
}

func TestFind_ErrorsWhenPresentInBoth(t *testing.T) {
	// Given the same name in both games/ and apps/ (ambiguous)
	d := t.TempDir()
	chdir(t, d)
	writeGameConfig(t, d, "Ambiguous", config.App{ProtonVersion: "ge9", Executable: "drive_c/game.exe"})
	writeAppConfig(t, d, "Ambiguous", config.App{ProtonVersion: "ge9", Executable: "drive_c/game.exe"})

	// When Find is called
	_, _, err := Find("Ambiguous")

	// Then it returns an error indicating the ambiguity
	if err == nil {
		t.Fatal("expected an error for an ambiguous name but got nil")
	}
	if !strings.Contains(err.Error(), "Ambiguous") {
		t.Fatalf("expected error to mention the name, got: %v", err)
	}
}

// --- ListAll ---

func TestListAll_PrintsAllConfiguredTitles(t *testing.T) {
	// Given a games/ dir with two games and an apps/ dir with one app
	d := t.TempDir()
	chdir(t, d)

	writeGameConfig(t, d, "Doom", config.App{
		ProtonVersion: "ge-proton-9",
		LaunchMethod:  "direct",
		Executable:    "drive_c/Games/Doom/doom.exe",
	})
	writeGameConfig(t, d, "Quake", config.App{
		ProtonVersion: "ge-proton-9",
		LaunchMethod:  "container",
		Executable:    "drive_c/Quake/quake.exe",
	})
	writeAppConfig(t, d, "SteamCMD", config.App{
		ProtonVersion: "system",
		LaunchMethod:  "direct",
		Executable:    "drive_c/steamcmd/steamcmd.exe",
	})

	// When ListAll is called
	var buf strings.Builder
	if err := ListAll(&buf); err != nil {
		t.Fatalf("ListAll returned unexpected error: %v", err)
	}

	// Then all three titles appear in the output
	out := buf.String()
	for _, name := range []string{"Doom", "Quake", "SteamCMD"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected %q in output but it was missing:\n%s", name, out)
		}
	}
}

func TestListAll_SkipsDirectoryWithNoConfigFile(t *testing.T) {
	// Given a games/ dir with one valid game and one directory with no game.json
	d := t.TempDir()
	chdir(t, d)

	writeGameConfig(t, d, "ValidGame", config.App{
		ProtonVersion: "ge-proton-9",
		LaunchMethod:  "direct",
		Executable:    "drive_c/game.exe",
	})
	// Create a dir with no config file
	os.MkdirAll(filepath.Join(d, "games", "EmptyDir"), 0755)

	// When ListAll is called
	var buf strings.Builder
	err := ListAll(&buf)

	// Then it returns no error and lists only the valid game
	if err != nil {
		t.Fatalf("ListAll returned unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ValidGame") {
		t.Errorf("expected ValidGame in output:\n%s", out)
	}
}

func TestListAll_HandlesMissingGamesDir(t *testing.T) {
	// Given a working directory with no games/ or apps/ directory
	d := t.TempDir()
	chdir(t, d)

	// When ListAll is called
	var buf strings.Builder
	err := ListAll(&buf)

	// Then it returns no error and prints only the header
	if err != nil {
		t.Fatalf("expected no error for missing games/ dir but got: %v", err)
	}
}

func TestListAll_PrintsHeaderLine(t *testing.T) {
	// Given any working directory
	d := t.TempDir()
	chdir(t, d)

	// When ListAll is called
	var buf strings.Builder
	ListAll(&buf)

	// Then the output starts with a header row containing TYPE and NAME
	out := buf.String()
	firstLine := strings.SplitN(out, "\n", 2)[0]
	if !strings.Contains(firstLine, "TYPE") || !strings.Contains(firstLine, "NAME") {
		t.Errorf("expected header with TYPE and NAME in first line, got: %q", firstLine)
	}
}

// --- Info ---

func TestInfo_PrintsProtonStatusPresent(t *testing.T) {
	// Given a game config referencing a proton version that is present on disk
	d := t.TempDir()
	chdir(t, d)

	protonDir := filepath.Join(d, "proton", "ge9")
	os.MkdirAll(protonDir, 0755)
	os.WriteFile(filepath.Join(protonDir, "proton"), []byte("#!/bin/sh"), 0755)

	a := &App{
		Type:   "games",
		Name:   "Doom",
		AppDir: "games/Doom",
		AppConfig: config.App{
			ProtonVersion: "ge9",
			LaunchMethod:  "direct",
			Executable:    "drive_c/game.exe",
		},
		PrefixPath: "games/Doom/prefix",
	}

	// When Info is called
	var buf strings.Builder
	if err := a.Info(&buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then the output contains a present marker for proton
	out := buf.String()
	if !strings.Contains(out, "✓") {
		t.Errorf("expected ✓ present marker in info output:\n%s", out)
	}
	if !strings.Contains(out, "ge9") {
		t.Errorf("expected proton version 'ge9' in info output:\n%s", out)
	}
}

func TestInfo_PrintsProtonStatusMissing(t *testing.T) {
	// Given a game config referencing a proton version that is NOT on disk
	d := t.TempDir()
	chdir(t, d)

	a := &App{
		Type:   "games",
		Name:   "Doom",
		AppDir: "games/Doom",
		AppConfig: config.App{
			ProtonVersion: "ge9",
			LaunchMethod:  "direct",
			Executable:    "drive_c/game.exe",
		},
		PrefixPath: "games/Doom/prefix",
	}

	// When Info is called
	var buf strings.Builder
	if err := a.Info(&buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then the output contains a missing marker for proton
	out := buf.String()
	if !strings.Contains(out, "✗") {
		t.Errorf("expected ✗ missing marker in info output:\n%s", out)
	}
}

func TestInfo_PrintsPrefixInitialisedStatus(t *testing.T) {
	// Given a game with an initialised Wine prefix (system.reg exists)
	d := t.TempDir()
	chdir(t, d)

	prefixDir := filepath.Join(d, "games", "Doom", "prefix")
	os.MkdirAll(prefixDir, 0755)
	os.WriteFile(filepath.Join(prefixDir, "system.reg"), []byte(""), 0644)

	a := &App{
		Type:       "games",
		Name:       "Doom",
		AppDir:     "games/Doom",
		AppConfig:  config.App{ProtonVersion: "ge9", LaunchMethod: "direct", Executable: "drive_c/game.exe"},
		PrefixPath: filepath.Join(d, "games", "Doom", "prefix"),
	}

	// When Info is called
	var buf strings.Builder
	if err := a.Info(&buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then the output says the prefix is initialised
	out := buf.String()
	if !strings.Contains(out, "initialised") {
		t.Errorf("expected 'initialised' in info output for ready prefix:\n%s", out)
	}
}

// --- Clean ---

func makeCleanApp(t *testing.T, dir, name, protonVersion, dxvkVersion, vkd3dVersion string) *App {
	t.Helper()
	appDir := filepath.Join("games", name)
	writeGameConfig(t, dir, name, config.App{
		ProtonVersion: protonVersion,
		LaunchMethod:  "direct",
		Executable:    "drive_c/game.exe",
		Dependencies: config.AppDependencies{
			DXVKVersion:  dxvkVersion,
			VKD3DVersion: vkd3dVersion,
		},
	})
	return &App{
		Type:      "games",
		Name:      name,
		AppDir:    appDir,
		AppConfig: config.App{ProtonVersion: protonVersion, Dependencies: config.AppDependencies{DXVKVersion: dxvkVersion, VKD3DVersion: vkd3dVersion}},
		PrefixPath: filepath.Join(appDir, "prefix"),
		GlobalConfig: config.Global{},
	}
}

func TestClean_PrefixDeletesOnlyPrefixDir(t *testing.T) {
	// Given a game with a prefix directory and a game.json config
	d := t.TempDir()
	chdir(t, d)
	a := makeCleanApp(t, d, "Doom", "ge9", "", "")

	prefixDir := filepath.Join(d, "games", "Doom", "prefix")
	os.MkdirAll(prefixDir, 0755)
	os.WriteFile(filepath.Join(prefixDir, "system.reg"), []byte(""), 0644)

	// When Clean is called with only the prefix target
	var buf strings.Builder
	err := a.Clean(CleanTargets{Prefix: true}, true, &buf)

	// Then prefix is deleted but game.json remains
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(prefixDir); !os.IsNotExist(statErr) {
		t.Error("expected prefix dir to be deleted")
	}
	if _, statErr := os.Stat(filepath.Join(d, "games", "Doom", "game.json")); statErr != nil {
		t.Error("expected game.json to remain after prefix clean")
	}
}

func TestClean_ProtonDeletesProtonDir(t *testing.T) {
	// Given a game referencing ge9 and a proton/ge9 directory on disk
	d := t.TempDir()
	chdir(t, d)
	a := makeCleanApp(t, d, "Doom", "ge9", "", "")

	protonDir := filepath.Join(d, "proton", "ge9")
	os.MkdirAll(protonDir, 0755)
	os.WriteFile(filepath.Join(protonDir, "proton"), []byte("#!/bin/sh"), 0755)

	// When Clean is called with only the proton target
	var buf strings.Builder
	err := a.Clean(CleanTargets{Proton: true}, true, &buf)

	// Then the proton directory is deleted
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(protonDir); !os.IsNotExist(statErr) {
		t.Error("expected proton dir to be deleted")
	}
}

func TestClean_ProtonWarnsWhenSharedByMultipleGames(t *testing.T) {
	// Given two games sharing the same proton version
	d := t.TempDir()
	chdir(t, d)
	a := makeCleanApp(t, d, "Doom", "ge9", "", "")
	writeGameConfig(t, d, "Quake", config.App{ProtonVersion: "ge9", Executable: "drive_c/quake.exe"})

	protonDir := filepath.Join(d, "proton", "ge9")
	os.MkdirAll(protonDir, 0755)
	os.WriteFile(filepath.Join(protonDir, "proton"), []byte("#!/bin/sh"), 0755)

	// When Clean is called with --proton
	var buf strings.Builder
	err := a.Clean(CleanTargets{Proton: true}, true, &buf)

	// Then no error is returned and output warns about the other game sharing the version
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Quake") {
		t.Errorf("expected warning mentioning Quake sharing ge9, got:\n%s", out)
	}
}

func TestClean_DepsSkipsIfVersionNotConfigured(t *testing.T) {
	// Given a game with no DXVK or VKD3D configured
	d := t.TempDir()
	chdir(t, d)
	a := makeCleanApp(t, d, "Doom", "ge9", "", "")

	// When Clean is called with the deps target
	var buf strings.Builder
	err := a.Clean(CleanTargets{Deps: true}, true, &buf)

	// Then no error is returned (nothing to delete)
	if err != nil {
		t.Fatalf("expected no error when no deps configured, got: %v", err)
	}
}

func TestClean_DepsDeletesConfiguredVersions(t *testing.T) {
	// Given a game with DXVK 2.3 and VKD3D 2.12 configured and their directories present
	d := t.TempDir()
	chdir(t, d)
	a := makeCleanApp(t, d, "Doom", "ge9", "2.3", "2.12")

	dxvkDir := filepath.Join(d, "dependencies", "dxvk", "2.3")
	vkd3dDir := filepath.Join(d, "dependencies", "vkd3d", "2.12")
	os.MkdirAll(dxvkDir, 0755)
	os.WriteFile(filepath.Join(dxvkDir, "x64", "d3d11.dll"), []byte("dll"), 0644)
	os.MkdirAll(vkd3dDir, 0755)
	os.WriteFile(filepath.Join(vkd3dDir, "x64", "d3d12.dll"), []byte("dll"), 0644)

	// When Clean is called with the deps target
	var buf strings.Builder
	err := a.Clean(CleanTargets{Deps: true}, true, &buf)

	// Then both dependency directories are deleted
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(dxvkDir); !os.IsNotExist(statErr) {
		t.Error("expected dxvk dir to be deleted")
	}
	if _, statErr := os.Stat(vkd3dDir); !os.IsNotExist(statErr) {
		t.Error("expected vkd3d dir to be deleted")
	}
}

func TestClean_AllDeletesPrefixProtonAndDeps(t *testing.T) {
	// Given a fully configured game with prefix, proton, and deps on disk
	d := t.TempDir()
	chdir(t, d)
	a := makeCleanApp(t, d, "Doom", "ge9", "2.3", "2.12")

	prefixDir := filepath.Join(d, "games", "Doom", "prefix")
	protonDir := filepath.Join(d, "proton", "ge9")
	dxvkDir := filepath.Join(d, "dependencies", "dxvk", "2.3")
	vkd3dDir := filepath.Join(d, "dependencies", "vkd3d", "2.12")
	for _, dir := range []string{prefixDir, protonDir, dxvkDir, vkd3dDir} {
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), 0644)
	}

	// When Clean is called with all targets
	var buf strings.Builder
	err := a.Clean(CleanTargets{Prefix: true, Proton: true, Deps: true}, true, &buf)

	// Then all four directories are deleted
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, dir := range []string{prefixDir, protonDir, dxvkDir, vkd3dDir} {
		if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
			t.Errorf("expected %s to be deleted", dir)
		}
	}
}
