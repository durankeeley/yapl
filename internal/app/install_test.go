package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/config"
)

// installFixture builds a library with an initialised prefix for "Doom",
// so Install never needs to download anything.
func installFixture(t *testing.T, cfg config.App) *App {
	t.Helper()
	dir := t.TempDir()
	chdir(t, dir)
	writeGameConfig(t, dir, "Doom", cfg)
	prefix := filepath.Join("games", "Doom", "prefix")
	if err := os.MkdirAll(filepath.Join(prefix, "drive_c"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(prefix, "system.reg"), []byte("WINE REGISTRY"), 0644)
	return New("games", "Doom", false, false, false, config.Global{}, cfg)
}

func portableGame(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "doom.exe"), make([]byte, 64), 0755)
	os.WriteFile(filepath.Join(src, "uninstall.exe"), make([]byte, 128), 0755)
	os.MkdirAll(filepath.Join(src, "base"), 0755)
	os.WriteFile(filepath.Join(src, "base", "doom.wad"), []byte("wad"), 0644)
	return src
}

func savedConfig(t *testing.T) config.App {
	t.Helper()
	cfg, err := config.LoadApp(filepath.Join("games", "Doom", "game.json"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

var placeholderConfig = config.App{ProtonVersion: "system", LaunchMethod: "direct", Executable: config.PlaceholderExecutable}

func TestInstall_FromFolder(t *testing.T) {
	t.Run("given a portable game folder when installed then it is copied and its executable is detected", func(t *testing.T) {
		// Given
		a := installFixture(t, placeholderConfig)
		var out bytes.Buffer

		// When
		err := a.Install(InstallOptions{From: portableGame(t)}, &out)

		// Then
		if err != nil {
			t.Fatalf("install: %v\n%s", err, out.String())
		}
		if _, err := os.Stat(filepath.Join("games", "Doom", "prefix", "drive_c", "Games", "Doom", "base", "doom.wad")); err != nil {
			t.Fatal("game files were not copied into drive_c/Games/Doom")
		}
		if got := savedConfig(t).Executable; got != "drive_c/Games/Doom/doom.exe" {
			t.Fatalf("executable = %q", got)
		}
		if !strings.Contains(out.String(), "drive_c/Games/Doom/doom.exe") {
			t.Fatalf("output does not list the chosen executable:\n%s", out.String())
		}
	})

	t.Run("given a config with a customised executable when installed then the config is unchanged", func(t *testing.T) {
		// Given
		custom := placeholderConfig
		custom.Executable = "drive_c/Custom/start.exe"
		a := installFixture(t, custom)
		var out bytes.Buffer

		// When
		err := a.Install(InstallOptions{From: portableGame(t)}, &out)

		// Then
		if err != nil {
			t.Fatal(err)
		}
		if got := savedConfig(t).Executable; got != "drive_c/Custom/start.exe" {
			t.Fatalf("executable overwritten: %q", got)
		}
		if !strings.Contains(out.String(), "drive_c/Games/Doom/doom.exe") {
			t.Fatalf("candidates not printed:\n%s", out.String())
		}
	})

	t.Run("given an explicit executable when installed then it is used instead of detection", func(t *testing.T) {
		// Given
		a := installFixture(t, placeholderConfig)

		// When
		err := a.Install(InstallOptions{From: portableGame(t), Executable: "drive_c/Games/Doom/uninstall.exe"}, &bytes.Buffer{})

		// Then
		if err != nil {
			t.Fatal(err)
		}
		if got := savedConfig(t).Executable; got != "drive_c/Games/Doom/uninstall.exe" {
			t.Fatalf("executable = %q", got)
		}
	})

	t.Run("given an explicit executable that does not exist when installed then it fails", func(t *testing.T) {
		a := installFixture(t, placeholderConfig)
		err := a.Install(InstallOptions{From: portableGame(t), Executable: "drive_c/nope.exe"}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "drive_c/nope.exe") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given a source that is not a directory when installed then it fails", func(t *testing.T) {
		a := installFixture(t, placeholderConfig)
		file := filepath.Join(t.TempDir(), "setup.exe")
		os.WriteFile(file, nil, 0644)
		if err := a.Install(InstallOptions{From: file}, &bytes.Buffer{}); err == nil {
			t.Fatal("expected an error for a non-directory source")
		}
	})
}

func TestInstall_Options(t *testing.T) {
	tests := []struct {
		name string
		opts InstallOptions
	}{
		{"given neither an installer nor a folder then install is rejected", InstallOptions{}},
		{"given both an installer and a folder then install is rejected", InstallOptions{Installer: "a.exe", From: "dir"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := installFixture(t, placeholderConfig)
			if err := a.Install(tc.opts, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "exactly one") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestInstall_RunsInstaller(t *testing.T) {
	t.Run("given an installer when installed in direct mode then it runs in the prefix, the wineserver is awaited and the new exe is detected", func(t *testing.T) {
		// Given a fake Proton whose wine64 "installs" Doom into $WINEPREFIX
		proton := t.TempDir()
		bin := filepath.Join(proton, "files", "bin")
		os.MkdirAll(bin, 0755)
		marker := filepath.Join(t.TempDir(), "wineserver-waited")
		os.WriteFile(filepath.Join(bin, "wine64"), []byte("#!/bin/sh\nmkdir -p \"$WINEPREFIX/drive_c/Games/Doom\"\n: > \"$WINEPREFIX/drive_c/Games/Doom/Doom.exe\"\n"), 0755)
		os.WriteFile(filepath.Join(bin, "wineserver"), []byte("#!/bin/sh\n[ \"$1\" = \"-w\" ] && : > \""+marker+"\"\nexit 0\n"), 0755)
		cfg := config.App{ProtonVersion: "fake", LaunchMethod: "direct", Executable: config.PlaceholderExecutable, SteamAppID: "123"}
		a := installFixture(t, cfg)
		a.GlobalConfig = config.Global{ProtonVersions: map[string]config.VersionInfo{"fake": {Path: proton}}}
		installerDir := t.TempDir()
		installer := filepath.Join(installerDir, "setup_doom.exe")
		os.WriteFile(installer, nil, 0644)

		// When
		err := a.Install(InstallOptions{Installer: installer}, &bytes.Buffer{})

		// Then
		if err != nil {
			t.Fatalf("install: %v", err)
		}
		if got := savedConfig(t).Executable; got != "drive_c/Games/Doom/Doom.exe" {
			t.Fatalf("executable = %q", got)
		}
		if _, err := os.Stat(marker); err != nil {
			t.Fatal("wineserver -w was not called after the installer")
		}
		if _, err := os.Stat(filepath.Join(installerDir, "steam_appid.txt")); !os.IsNotExist(err) {
			t.Fatal("steam_appid.txt was written next to the installer")
		}
	})
}
