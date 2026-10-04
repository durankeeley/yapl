package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

	// When LoadOrCreateGlobal is called with a fallback catalog
	fallback := Global{ProtonVersions: map[string]VersionInfo{"GE-Proton11-7": {URL: "https://example/ge.tar.gz"}}, Defaults: Defaults{Proton: "GE-Proton11-7"}}
	g, err := LoadOrCreateGlobal("runner.json", fallback)

	// Then it returns the fallback config with no error
	if err != nil {
		t.Fatalf("expected no error but got: %v", err)
	}
	if g.Defaults.Proton != "GE-Proton11-7" || len(g.ProtonVersions) != 1 {
		t.Fatalf("expected the fallback config, got %+v", g)
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
	g, err := LoadOrCreateGlobal("runner.json", Global{ProtonVersions: map[string]VersionInfo{"fallback": {}}})

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

// --- ValidateApp ---

func TestValidateApp_PassesForValidDirectConfig(t *testing.T) {
	cfg := App{
		ProtonVersion: "ge-proton-9",
		Executable:    "drive_c/game.exe",
		LaunchMethod:  "direct",
	}
	if err := ValidateApp(cfg); err != nil {
		t.Fatalf("expected no error for valid direct config, got: %v", err)
	}
}

func TestValidateApp_PassesForValidContainerConfig(t *testing.T) {
	cfg := App{
		ProtonVersion:  "ge-proton-9",
		Executable:     "drive_c/game.exe",
		LaunchMethod:   "container",
		RuntimeVersion: "sniper",
	}
	if err := ValidateApp(cfg); err != nil {
		t.Fatalf("expected no error for valid container config, got: %v", err)
	}
}

func TestValidateApp_PassesForSystemProton(t *testing.T) {
	cfg := App{
		ProtonVersion: "system",
		Executable:    "drive_c/game.exe",
		LaunchMethod:  "direct",
	}
	if err := ValidateApp(cfg); err != nil {
		t.Fatalf("'system' is a valid proton_version and should not be rejected: %v", err)
	}
}

func TestValidateApp_ReturnsErrorForMissingProtonVersion(t *testing.T) {
	cfg := App{
		Executable:   "drive_c/game.exe",
		LaunchMethod: "direct",
	}
	err := ValidateApp(cfg)
	if err == nil {
		t.Fatal("expected error for missing proton_version, got nil")
	}
	if !strings.Contains(err.Error(), "proton_version") {
		t.Fatalf("error should mention proton_version, got: %v", err)
	}
}

func TestValidateApp_ReturnsErrorForMissingExecutable(t *testing.T) {
	cfg := App{
		ProtonVersion: "ge-proton-9",
		LaunchMethod:  "direct",
	}
	err := ValidateApp(cfg)
	if err == nil {
		t.Fatal("expected error for missing executable, got nil")
	}
	if !strings.Contains(err.Error(), "executable") {
		t.Fatalf("error should mention executable, got: %v", err)
	}
}

func TestValidateApp_ReturnsErrorForInvalidLaunchMethod(t *testing.T) {
	cfg := App{
		ProtonVersion: "ge-proton-9",
		Executable:    "drive_c/game.exe",
		LaunchMethod:  "magic",
	}
	err := ValidateApp(cfg)
	if err == nil {
		t.Fatal("expected error for invalid launch_method, got nil")
	}
	if !strings.Contains(err.Error(), "launch_method") {
		t.Fatalf("error should mention launch_method, got: %v", err)
	}
}

func TestValidateApp_ReturnsErrorForContainerWithNoRuntime(t *testing.T) {
	cfg := App{
		ProtonVersion: "ge-proton-9",
		Executable:    "drive_c/game.exe",
		LaunchMethod:  "container",
	}
	err := ValidateApp(cfg)
	if err == nil {
		t.Fatal("expected error for container launch without runtime_version, got nil")
	}
	if !strings.Contains(err.Error(), "runtime_version") {
		t.Fatalf("error should mention runtime_version, got: %v", err)
	}
}

func TestValidateApp_ReturnsAllErrorsTogether(t *testing.T) {
	cfg := App{} // all fields empty
	err := ValidateApp(cfg)
	if err == nil {
		t.Fatal("expected error for empty config, got nil")
	}
	msg := err.Error()
	for _, field := range []string{"proton_version", "executable", "launch_method"} {
		if !strings.Contains(msg, field) {
			t.Errorf("expected error to mention %q, got: %v", field, msg)
		}
	}
}

func TestValidateApp_ErrorPrefixedWithConfigValidationFailed(t *testing.T) {
	cfg := App{}
	err := ValidateApp(cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "config validation failed:") {
		t.Fatalf("error should start with 'config validation failed:', got: %v", err)
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

func TestMergeGlobal(t *testing.T) {
	proton := func(url string) map[string]VersionInfo { return map[string]VersionInfo{"ge9": {URL: url}} }

	tests := []struct {
		name        string
		local       Global
		bundled     Global
		wantProton  string
		wantAdded   []string
		wantDXVKURL string
	}{
		{
			name:       "given an empty local config when a bundle is merged then every bundled entry is added",
			local:      Global{},
			bundled:    Global{ProtonVersions: proton("bundled"), DependencyVersions: map[string]map[string]VersionInfo{"dxvk": {"2.4": {URL: "dxvk"}}}},
			wantProton: "bundled", wantDXVKURL: "dxvk",
			wantAdded: []string{"dependencies/dxvk/2.4", "proton/ge9"},
		},
		{
			name:       "given a local entry with the same key when a bundle is merged then the local entry wins",
			local:      Global{ProtonVersions: proton("local")},
			bundled:    Global{ProtonVersions: proton("bundled")},
			wantProton: "local",
			wantAdded:  nil,
		},
		{
			name:       "given a bundled runtime the local config lacks when merged then the runtime is added",
			local:      Global{ProtonVersions: proton("local")},
			bundled:    Global{RuntimeVersions: map[string]VersionInfo{"sniper": {URL: "rt"}}},
			wantProton: "local",
			wantAdded:  []string{"runtime/sniper"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// When
			merged, added := MergeGlobal(tc.local, tc.bundled)

			// Then
			if got := merged.ProtonVersions["ge9"].URL; got != tc.wantProton {
				t.Errorf("proton ge9 url = %q, want %q", got, tc.wantProton)
			}
			if tc.wantDXVKURL != "" && merged.DependencyVersions["dxvk"]["2.4"].URL != tc.wantDXVKURL {
				t.Errorf("dxvk 2.4 not merged: %+v", merged.DependencyVersions)
			}
			if strings.Join(added, ",") != strings.Join(tc.wantAdded, ",") {
				t.Errorf("added = %v, want %v", added, tc.wantAdded)
			}
		})
	}
}

func TestMergeGlobal_DoesNotMutateLocal(t *testing.T) {
	// Given
	local := Global{ProtonVersions: map[string]VersionInfo{"a": {}}}

	// When
	MergeGlobal(local, Global{ProtonVersions: map[string]VersionInfo{"b": {}}})

	// Then
	if _, ok := local.ProtonVersions["b"]; ok {
		t.Fatal("MergeGlobal mutated the local config map")
	}
}

func TestSaveAndLoadGlobal_RoundTrip(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "runner.json")
	want := Global{ProtonVersions: map[string]VersionInfo{"ge9": {URL: "u"}}}

	// When
	if err := SaveGlobal(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGlobal(path)

	// Then
	if err != nil || got.ProtonVersions["ge9"].URL != "u" {
		t.Fatalf("round trip failed: %+v, %v", got, err)
	}
}

func TestLoadGlobal_MissingFileReturnsNotExist(t *testing.T) {
	// Given / When
	_, err := LoadGlobal(filepath.Join(t.TempDir(), "missing.json"))

	// Then
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

func TestValidateApp_DXVKDirectXVersion(t *testing.T) {
	base := App{ProtonVersion: "ge", LaunchMethod: "direct", Executable: "drive_c/game.exe"}
	tests := []struct {
		name    string
		version string
		wantErr string
	}{
		{"given directx version 9 then the config is valid", "9", ""},
		{"given directx version 10 then the config is valid", "10", ""},
		{"given directx version 11 then the config is valid", "11", ""},
		{"given no directx version then the config is valid", "", ""},
		{"given directx version 12 then it is rejected naming the valid values", "12", `dxvk_directx_version must be "9", "10", or "11" (got "12")`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			cfg := base
			cfg.Dependencies.DXVKDirectXVersion = tc.version

			// When
			err := ValidateApp(cfg)

			// Then
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateApp_EmptyLaunchMethodMeansDefault(t *testing.T) {
	t.Run("given no launch method and a runtime then the config is valid as container", func(t *testing.T) {
		cfg := App{ProtonVersion: "ge", Executable: "x.exe", RuntimeVersion: "sniper"}
		if err := ValidateApp(cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("given no launch method and no runtime then the container runtime requirement applies", func(t *testing.T) {
		cfg := App{ProtonVersion: "ge", Executable: "x.exe"}
		if err := ValidateApp(cfg); err == nil || !strings.Contains(err.Error(), "runtime_version is required") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestLoadOrCreateApp_ChoosesVersionsDeterministically(t *testing.T) {
	three := map[string]VersionInfo{"b-proton": {}, "a-proton": {}, "c-proton": {}}
	tests := []struct {
		name        string
		global      Global
		wantProton  string
		wantRuntime string
	}{
		{
			name:       "given a defaults block then the preferred proton is used",
			global:     Global{ProtonVersions: three, Defaults: Defaults{Proton: "c-proton"}},
			wantProton: "c-proton",
		},
		{
			name:       "given no defaults block then the alphabetically first proton is used",
			global:     Global{ProtonVersions: three},
			wantProton: "a-proton",
		},
		{
			name:       "given a preferred proton that is not defined then the alphabetically first is used",
			global:     Global{ProtonVersions: three, Defaults: Defaults{Proton: "missing"}},
			wantProton: "a-proton",
		},
		{
			name:        "given several runtimes and a preferred one then it is used",
			global:      Global{ProtonVersions: three, RuntimeVersions: map[string]VersionInfo{"soldier": {}, "sniper": {}}, Defaults: Defaults{Runtime: "sniper"}},
			wantProton:  "a-proton",
			wantRuntime: "sniper",
		},
		{
			name:        "given several runtimes and no preference then the alphabetically first is used",
			global:      Global{ProtonVersions: three, RuntimeVersions: map[string]VersionInfo{"soldier": {}, "sniper": {}}},
			wantProton:  "a-proton",
			wantRuntime: "sniper",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			d := t.TempDir()
			orig, _ := os.Getwd()
			os.Chdir(d)
			defer os.Chdir(orig)

			// When
			app, err := LoadOrCreateApp("games", "Doom", "", tc.global, "")

			// Then
			if err != nil {
				t.Fatal(err)
			}
			if app.ProtonVersion != tc.wantProton || app.RuntimeVersion != tc.wantRuntime {
				t.Fatalf("got proton %q runtime %q, want %q %q", app.ProtonVersion, app.RuntimeVersion, tc.wantProton, tc.wantRuntime)
			}
		})
	}
}

func TestMergeGlobal_KeepsLocalDefaults(t *testing.T) {
	t.Run("given local defaults when a bundle with other defaults is merged then local defaults are kept", func(t *testing.T) {
		merged, _ := MergeGlobal(Global{Defaults: Defaults{Proton: "local"}}, Global{Defaults: Defaults{Proton: "bundled"}})
		if merged.Defaults.Proton != "local" {
			t.Fatalf("defaults = %+v", merged.Defaults)
		}
	})
}

func TestVersionInfoFor(t *testing.T) {
	g := Global{
		ProtonVersions:     map[string]VersionInfo{"ge": {BinPath: "p"}},
		RuntimeVersions:    map[string]VersionInfo{"sniper": {BinPath: "r"}},
		DependencyVersions: map[string]map[string]VersionInfo{"dxvk": {"3.1.1": {BinPath: "d"}}},
	}
	tests := []struct {
		name, kind, version, wantBin string
		wantOK                       bool
	}{
		{"given a proton version then its proton entry is returned", "proton", "ge", "p", true},
		{"given a runtime version then its runtime entry is returned", "runtime", "sniper", "r", true},
		{"given a dependency version then its dependency entry is returned", "dxvk", "3.1.1", "d", true},
		{"given an unknown version then nothing is found", "dxvk", "9", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, ok := g.VersionInfoFor(tc.kind, tc.version)
			if ok != tc.wantOK || v.BinPath != tc.wantBin {
				t.Fatalf("got %+v, %v", v, ok)
			}
		})
	}
}

func TestRegister(t *testing.T) {
	t.Run("given an empty config when components are registered then each lands in its section", func(t *testing.T) {
		// Given
		var g Global

		// When
		g.Register("proton", "ge", VersionInfo{URL: "p"})
		g.Register("runtime", "sniper", VersionInfo{URL: "r"})
		g.Register("dxvk", "3.1.1", VersionInfo{URL: "d"})

		// Then
		if g.ProtonVersions["ge"].URL != "p" || g.RuntimeVersions["sniper"].URL != "r" || g.DependencyVersions["dxvk"]["3.1.1"].URL != "d" {
			t.Fatalf("config = %+v", g)
		}
	})
}

func TestClientSettings(t *testing.T) {
	t.Run("given saved client settings when loaded then the server and auth mode come back", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "client.json")
		if err := SaveClientSettings(path, ClientSettings{Server: "10.0.0.5:8471", Auth: "password"}); err != nil {
			t.Fatal(err)
		}
		got, err := LoadClientSettings(path)
		if err != nil || got.Server != "10.0.0.5:8471" || got.Auth != "password" {
			t.Fatalf("got %+v, %v", got, err)
		}
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "password\":") && strings.Contains(string(data), "secret") {
			t.Fatal("password written to disk")
		}
	})
	t.Run("given no client settings file then empty settings are returned without error", func(t *testing.T) {
		got, err := LoadClientSettings(filepath.Join(t.TempDir(), "missing.json"))
		if err != nil || got.Server != "" {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
}
