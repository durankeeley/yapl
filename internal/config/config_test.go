package config

import (
	"os"
	"path/filepath"
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
	app, err := LoadOrCreateApp("games", "TestGame", "", globalCfg)

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

func TestLoadOrCreateApp_UsesCustomConfigName(t *testing.T) {
	// Given a working directory and a custom config file name specified
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	globalCfg := Global{ProtonVersions: map[string]VersionInfo{"v1": {URL: "http://example.com"}}}

	// When LoadOrCreateApp is called with a custom config name
	_, err := LoadOrCreateApp("games", "TestGame", "mod-a.json", globalCfg)

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
	app, err := LoadOrCreateApp("games", "TestGame", "", Global{})

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
