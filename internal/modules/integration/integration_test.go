package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/integration/steam"
	"yapl/internal/kernel"
)

func library(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })
	for name, exe := range map[string]string{"Doom": "drive_c/doom.exe", "Need for Speed": "drive_c/nfs/speed.exe", "Unfinished": "drive_c/windows/explorer.exe"} {
		gameDir := filepath.Join("games", name)
		os.MkdirAll(gameDir, 0755)
		os.WriteFile(filepath.Join(gameDir, "game.json"), []byte(`{"proton_version":"ge","launch_method":"direct","executable":"`+exe+`"}`), 0644)
	}
	os.WriteFile(filepath.Join("games", "Doom", "cover.png"), []byte("png"), 0644)
	return dir
}

func execute(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	k := kernel.New("yapl", &out, &errOut)
	if err := k.Register(Module{}); err != nil {
		t.Fatal(err)
	}
	return k.Execute(args), out.String(), errOut.String()
}

func TestExportDesktop(t *testing.T) {
	t.Run("given a library when desktop entries are exported then ready games and the console get entries", func(t *testing.T) {
		// Given
		root := library(t)
		out := t.TempDir()

		// When
		code, stdout, stderr := execute(t, "export", "desktop", "--output-dir", out)

		// Then
		if code != kernel.ExitOK {
			t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
		}
		nfs, err := os.ReadFile(filepath.Join(out, "yapl-need-for-speed.desktop"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(nfs), `--workdir `+root+` run "Need for Speed"`) {
			t.Fatalf("exec line wrong:\n%s", nfs)
		}
		doom, _ := os.ReadFile(filepath.Join(out, "yapl-doom.desktop"))
		if !strings.Contains(string(doom), "Icon="+filepath.Join(root, "games", "Doom", "cover.png")) {
			t.Fatalf("doom icon wrong:\n%s", doom)
		}
		if _, err := os.Stat(filepath.Join(out, "yapl-console.desktop")); err != nil {
			t.Fatal("console entry missing")
		}
		if _, err := os.Stat(filepath.Join(out, "yapl-unfinished.desktop")); !os.IsNotExist(err) {
			t.Fatal("entry written for a game that is not installed")
		}
		if !strings.Contains(stdout, "Unfinished") {
			t.Fatalf("skipped game not reported:\n%s", stdout)
		}
	})

	t.Run("given one game name then only that entry is written", func(t *testing.T) {
		library(t)
		out := t.TempDir()
		execute(t, "export", "desktop", "Doom", "--output-dir", out)
		entries, _ := os.ReadDir(out)
		if len(entries) != 1 || entries[0].Name() != "yapl-doom.desktop" {
			t.Fatalf("entries = %v", entries)
		}
	})

	t.Run("given an unknown game name then it fails", func(t *testing.T) {
		library(t)
		if code, _, stderr := execute(t, "export", "desktop", "Quake", "--output-dir", t.TempDir()); code != kernel.ExitError || !strings.Contains(stderr, "Quake") {
			t.Fatalf("exit %d\n%s", code, stderr)
		}
	})
}

func TestExportAutostart(t *testing.T) {
	t.Run("given autostart then the console starts on login for this library", func(t *testing.T) {
		root := library(t)
		out := t.TempDir()
		if code, _, stderr := execute(t, "export", "autostart", "--output-dir", out); code != kernel.ExitOK {
			t.Fatalf("exit %d %s", code, stderr)
		}
		data, _ := os.ReadFile(filepath.Join(out, "yapl-console.desktop"))
		if !strings.Contains(string(data), "--workdir "+root+" console") || !strings.Contains(string(data), "X-GNOME-Autostart-enabled=true") {
			t.Fatalf("autostart:\n%s", data)
		}
	})
}

func TestExportSteam(t *testing.T) {
	steamRoot := func(t *testing.T) string {
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, "userdata", "12345", "config"), 0755)
		return root
	}

	t.Run("given Steam is closed when exported then each ready game becomes a shortcut with grid art", func(t *testing.T) {
		// Given
		library(t)
		root := steamRoot(t)
		steamRunning = func() bool { return false }
		defer func() { steamRunning = steam.Running }()

		// When
		code, stdout, stderr := execute(t, "export", "steam", "--steam-root", root)

		// Then
		if code != kernel.ExitOK {
			t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
		}
		config := filepath.Join(root, "userdata", "12345", "config")
		data, _ := os.ReadFile(filepath.Join(config, steam.ShortcutsFile))
		vdf, err := steam.Parse(data)
		if err != nil || len(vdf.Child("shortcuts").Children) != 2 {
			t.Fatalf("shortcuts %v err %v", vdf, err)
		}
		grid, _ := filepath.Glob(filepath.Join(config, "grid", "*p.png"))
		if len(grid) != 1 {
			t.Fatalf("grid art = %v", grid)
		}
	})

	t.Run("given Steam is running then export refuses unless forced", func(t *testing.T) {
		library(t)
		root := steamRoot(t)
		steamRunning = func() bool { return true }
		defer func() { steamRunning = steam.Running }()
		if code, _, stderr := execute(t, "export", "steam", "--steam-root", root); code != kernel.ExitError || !strings.Contains(stderr, "close Steam") {
			t.Fatalf("exit %d %s", code, stderr)
		}
		if code, _, stderr := execute(t, "export", "steam", "--steam-root", root, "--force"); code != kernel.ExitOK {
			t.Fatalf("forced exit %d %s", code, stderr)
		}
	})
}

func TestExportUsage(t *testing.T) {
	library(t)
	if code, _, stderr := execute(t, "export", "phone"); code != kernel.ExitUsage || !strings.Contains(stderr, "desktop, steam or autostart") {
		t.Fatalf("exit %d %s", code, stderr)
	}
}
