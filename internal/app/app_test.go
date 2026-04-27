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
