package console

import (
	"os"
	"path/filepath"
	"testing"

	"yapl/internal/manifest"
)

func inLibrary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })
	return dir
}

func addGame(t *testing.T, typeDir, name, cfg string) string {
	t.Helper()
	dir := filepath.Join(typeDir, name)
	os.MkdirAll(dir, 0755)
	file := "game.json"
	if typeDir == "apps" {
		file = "app.json"
	}
	os.WriteFile(filepath.Join(dir, file), []byte(cfg), 0644)
	return dir
}

const readyConfig = `{"proton_version":"ge","launch_method":"direct","executable":"drive_c/doom.exe"}`

func TestListLibrary(t *testing.T) {
	t.Run("given games with metadata, install records and covers then each is described for the console", func(t *testing.T) {
		// Given
		inLibrary(t)
		doom := addGame(t, "games", "Doom", readyConfig)
		manifest.SaveMetadata(doom, manifest.Metadata{Title: "DOOM", Version: "1.9.0", Description: "Rip and tear", Players: "1-4", Tags: []string{"fps"}})
		manifest.WriteInstalled(doom, manifest.Installed{Name: "Doom", Type: "game", Version: "1.9.0", Server: "http://10.0.0.5:8471"})
		os.WriteFile(filepath.Join(doom, "cover.jpg"), []byte("jpg"), 0644)
		addGame(t, "apps", "Notepad", `{"proton_version":"ge","launch_method":"direct","executable":"drive_c/windows/explorer.exe"}`)

		// When
		games, err := ListLibrary()

		// Then
		if err != nil || len(games) != 2 {
			t.Fatalf("games = %+v, %v", games, err)
		}
		d := games[0]
		if d.Name != "Doom" || d.Title != "DOOM" || d.Version != "1.9.0" || !d.HasCover || !d.Ready || d.Source != "http://10.0.0.5:8471" || d.Players != "1-4" || d.Type != "game" {
			t.Fatalf("doom = %+v", d)
		}
		n := games[1]
		if n.Name != "Notepad" || n.Title != "Notepad" || n.Ready || n.HasCover || n.Type != "app" {
			t.Fatalf("notepad = %+v", n)
		}
	})

	t.Run("given a directory without a config then it is not listed", func(t *testing.T) {
		inLibrary(t)
		os.MkdirAll(filepath.Join("games", "Broken"), 0755)
		games, err := ListLibrary()
		if err != nil || len(games) != 0 {
			t.Fatalf("games = %+v, %v", games, err)
		}
	})

	t.Run("given an empty library then the list is empty", func(t *testing.T) {
		inLibrary(t)
		if games, err := ListLibrary(); err != nil || len(games) != 0 {
			t.Fatalf("games = %+v, %v", games, err)
		}
	})
}

func TestFindGame(t *testing.T) {
	t.Run("given an installed game when found then its cover path is returned", func(t *testing.T) {
		inLibrary(t)
		doom := addGame(t, "games", "Doom", readyConfig)
		os.WriteFile(filepath.Join(doom, "cover.png"), []byte("png"), 0644)
		g, cover, err := FindGame("Doom")
		if err != nil || g.Name != "Doom" || cover != filepath.Join("games", "Doom", "cover.png") {
			t.Fatalf("game %+v cover %q err %v", g, cover, err)
		}
	})

	t.Run("given an unknown game then ErrNotInstalled is returned", func(t *testing.T) {
		inLibrary(t)
		if _, _, err := FindGame("Quake"); err != ErrNotInstalled {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestUninstall(t *testing.T) {
	t.Run("given an installed game when uninstalled then its directory is removed", func(t *testing.T) {
		inLibrary(t)
		addGame(t, "games", "Doom", readyConfig)
		if err := Uninstall("Doom"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join("games", "Doom")); !os.IsNotExist(err) {
			t.Fatal("game directory still exists")
		}
	})

	t.Run("given an unknown game when uninstalled then ErrNotInstalled is returned", func(t *testing.T) {
		inLibrary(t)
		if err := Uninstall("Quake"); err != ErrNotInstalled {
			t.Fatalf("error = %v", err)
		}
	})
}
