package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// --- LoadOrCreateGlobal ---

func TestLoadOrCreateGlobal_CreatesDefaultWhenFileMissing(t *testing.T) {
	// Given a working directory with no runner.json
	d := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	// When LoadOrCreateGlobal is called
	g, err := LoadOrCreateGlobal("runner.json")

	// Then it returns a non-empty default config with no error
	if err != nil {
		t.Fatalf("expected no error but got: %v", err)
	}
	if len(g.ProtonVersions) == 0 {
		t.Fatal("expected default config to contain at least one proton version placeholder")
	}

	// And the runner.json file is created on disk
	if _, err := os.Stat("runner.json"); err != nil {
		t.Fatal("expected runner.json to be created but it was not found")
	}
}

func TestLoadOrCreateGlobal_ReadsExistingFile(t *testing.T) {
	// Given an existing runner.json with a known proton version
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	content := `{"proton_versions":{"my-proton":{"url":"http://example.com"}},"runtime_versions":{},"dependency_versions":{}}`
	if err := os.WriteFile("runner.json", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// When LoadOrCreateGlobal is called
	g, err := LoadOrCreateGlobal("runner.json")

	// Then it reads the existing file and returns the correct config
	if err != nil {
		t.Fatalf("expected no error but got: %v", err)
	}
	if _, ok := g.ProtonVersions["my-proton"]; !ok {
		t.Fatal("expected 'my-proton' version from the existing file, but it was not found")
	}
}

func TestLoadOrCreateGlobal_RoundTripsJsonCorrectly(t *testing.T) {
	// Given a Global config struct with known values
	d := t.TempDir()
	path := filepath.Join(d, "runner.json")
	g1 := Global{
		ProtonVersions:     map[string]VersionInfo{"v1": {URL: "http://a"}},
		RuntimeVersions:    map[string]VersionInfo{"sniper": {URL: "http://b", CheckForUpdates: true}},
		DependencyVersions: map[string]map[string]VersionInfo{"dxvk": {"2.3": {URL: "http://c"}}},
	}

	// When it is written to and read from disk
	if err := writeJSONFile(path, g1); err != nil {
		t.Fatal(err)
	}
	var g2 Global
	if err := readJSONFile(path, &g2); err != nil {
		t.Fatal(err)
	}

	// Then all values are preserved exactly
	if g2.ProtonVersions["v1"].URL != "http://a" {
		t.Fatalf("proton URL was not preserved: %+v", g2.ProtonVersions)
	}
	if !g2.RuntimeVersions["sniper"].CheckForUpdates {
		t.Fatal("check_for_updates boolean was not preserved in round-trip")
	}
	if g2.DependencyVersions["dxvk"]["2.3"].URL != "http://c" {
		t.Fatalf("dependency URL was not preserved: %+v", g2.DependencyVersions)
	}
}

// --- LoadOrCreateApp ---

func TestLoadOrCreateApp_CreatesDefaultGameJson(t *testing.T) {
	// Given a working directory with no game directory or game.json
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	globalCfg := Global{
		ProtonVersions: map[string]VersionInfo{"ge-proton-9": {URL: "http://example.com"}},
	}

	// When LoadOrCreateApp is called for a new game
	app, err := LoadOrCreateApp("games", "TestGame", "", globalCfg, "")

	// Then it returns a default config seeded with the first proton version from the global config
	if err != nil {
		t.Fatalf("expected no error but got: %v", err)
	}
	if app.ProtonVersion != "ge-proton-9" {
		t.Fatalf("expected proton version 'ge-proton-9', got %q", app.ProtonVersion)
	}

	// And the game.json file is created in the correct location
	if _, err := os.Stat(filepath.Join("games", "TestGame", "game.json")); err != nil {
		t.Fatal("expected games/TestGame/game.json to be created but it was not found")
	}
}

func TestLoadOrCreateApp_DefaultsToContainerLaunchMethod(t *testing.T) {
	// Given a global config with a proton version and a runtime version
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	globalCfg := Global{
		ProtonVersions:  map[string]VersionInfo{"ge-proton-9": {URL: "http://example.com"}},
		RuntimeVersions: map[string]VersionInfo{"sniper": {URL: "http://example.com/runtime"}},
	}

	// When LoadOrCreateApp creates a fresh default config
	app, err := LoadOrCreateApp("games", "NewGame", "", globalCfg, "")

	// Then the default launch method is "container" (not "direct") — safest default
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.LaunchMethod != "container" {
		t.Fatalf("expected default LaunchMethod to be 'container', got %q", app.LaunchMethod)
	}
	// And the runtime is populated from the global config so the config is immediately usable
	if app.RuntimeVersion == "" {
		t.Fatal("expected RuntimeVersion to be populated from the global config's first runtime")
	}
}

func TestLoadOrCreateApp_UsesCustomConfigName(t *testing.T) {
	// Given a working directory and a custom config file name specified
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	globalCfg := Global{ProtonVersions: map[string]VersionInfo{"v1": {URL: "http://example.com"}}}

	// When LoadOrCreateApp is called with a custom config name
	_, err := LoadOrCreateApp("games", "TestGame", "mod-a.json", globalCfg, "")

	// Then the file is created under the custom name, not the default name
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("games", "TestGame", "mod-a.json")); err != nil {
		t.Fatal("expected games/TestGame/mod-a.json to be created but it was not found")
	}
	if _, err := os.Stat(filepath.Join("games", "TestGame", "game.json")); err == nil {
		t.Fatal("default game.json should not have been created when a custom name was provided")
	}
}

func TestLoadOrCreateApp_MethodFlagOverridesDefault(t *testing.T) {
	// Given a global config and no existing game.json
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	globalCfg := Global{
		ProtonVersions:  map[string]VersionInfo{"ge-proton-9": {URL: "http://example.com"}},
		RuntimeVersions: map[string]VersionInfo{"sniper": {URL: "http://example.com/runtime"}},
	}

	// When LoadOrCreateApp is called with defaultMethod "direct"
	app, err := LoadOrCreateApp("games", "NFSGame", "", globalCfg, "direct")

	// Then the created config uses "direct" as the launch method
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.LaunchMethod != "direct" {
		t.Fatalf("expected LaunchMethod 'direct', got %q", app.LaunchMethod)
	}
}

func TestLoadOrCreateApp_ExistingConfigIgnoresMethodFlag(t *testing.T) {
	// Given an existing game.json with launch_method "container"
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	gameDir := filepath.Join("games", "ExistingGame")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `{"proton_version":"ge-proton-9","launch_method":"container","executable":"drive_c/game.exe","dependencies":{},"dll_overrides":{},"environment_vars":{}}`
	if err := os.WriteFile(filepath.Join(gameDir, "game.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// When LoadOrCreateApp is called with defaultMethod "direct"
	app, err := LoadOrCreateApp("games", "ExistingGame", "", Global{}, "direct")

	// Then the existing config's launch method is returned unchanged
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.LaunchMethod != "container" {
		t.Fatalf("expected existing LaunchMethod 'container' to be preserved, got %q", app.LaunchMethod)
	}
}

func TestLoadOrCreateApp_ReadsExistingConfig(t *testing.T) {
	// Given an existing game.json with known settings
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	gameDir := filepath.Join("games", "TestGame")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `{"proton_version":"custom-proton","executable":"drive_c/game.exe","dependencies":{},"dll_overrides":{},"environment_vars":{}}`
	if err := os.WriteFile(filepath.Join(gameDir, "game.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// When LoadOrCreateApp is called
	app, err := LoadOrCreateApp("games", "TestGame", "", Global{}, "")

	// Then it reads the existing config without overwriting it
	if err != nil {
		t.Fatal(err)
	}
	if app.ProtonVersion != "custom-proton" {
		t.Fatalf("expected proton version 'custom-proton', got %q", app.ProtonVersion)
	}
	if app.Executable != "drive_c/game.exe" {
		t.Fatalf("expected executable 'drive_c/game.exe', got %q", app.Executable)
	}
}

// --- VersionInfo struct shape (PRD-15) ---

func TestVersionInfo_DoesNotContainPythonFields(t *testing.T) {
	// Given the VersionInfo struct
	vt := reflect.TypeOf(VersionInfo{})

	// When we inspect its fields
	// Then neither PythonHome nor PythonPath should exist
	for _, name := range []string{"PythonHome", "PythonPath"} {
		if _, ok := vt.FieldByName(name); ok {
			t.Errorf("VersionInfo still has deprecated field %q — remove it from config.go", name)
		}
	}
}
