package steam

import (
	"bytes"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func doom() Shortcut {
	return Shortcut{AppName: "Doom", Exe: `"/usr/bin/yapl"`, StartDir: `"/lib"`, LaunchOptions: `--workdir /lib run Doom`, Icon: "/lib/games/Doom/cover.png"}
}

func TestShortcutAppID(t *testing.T) {
	t.Run("given a shortcut then its id is the crc32 of exe and name with the high bit set", func(t *testing.T) {
		s := doom()
		want := crc32.ChecksumIEEE([]byte(s.Exe+s.AppName)) | shortcutIDBit
		if got := s.AppID(); got != want || got&shortcutIDBit == 0 {
			t.Fatalf("AppID = %d, want %d", got, want)
		}
	})
}

func TestUpsert(t *testing.T) {
	t.Run("given an empty file when a game is added then a complete shortcut is created", func(t *testing.T) {
		// Given
		root := &Node{Type: TypeMap}

		// When
		added := Upsert(root, doom())

		// Then
		entry := root.Child("shortcuts").Child("0")
		if !added || entry.String("AppName") != "Doom" || entry.String("LaunchOptions") != doom().LaunchOptions || entry.Uint32("appid") != doom().AppID() || entry.Uint32("AllowOverlay") != 1 || entry.Child("tags") == nil {
			t.Fatalf("entry = %+v", entry)
		}
	})

	t.Run("given a user's own shortcut when games are added then it is byte-identical afterwards", func(t *testing.T) {
		// Given
		root, _ := Parse(fixture())
		before := root.Child("shortcuts").Child("0").Encode()

		// When
		Upsert(root, doom())

		// Then
		shortcuts := root.Child("shortcuts")
		if !bytes.Equal(shortcuts.Child("0").Encode(), before) {
			t.Fatal("user shortcut changed")
		}
		if shortcuts.Child("1").String("AppName") != "Doom" {
			t.Fatalf("new shortcut missing: %+v", shortcuts.Children)
		}
	})

	t.Run("given a game exported twice then there is still one shortcut for it", func(t *testing.T) {
		root := &Node{Type: TypeMap}
		Upsert(root, doom())
		if added := Upsert(root, doom()); added || len(root.Child("shortcuts").Children) != 1 {
			t.Fatalf("added=%v count=%d", added, len(root.Child("shortcuts").Children))
		}
	})

	t.Run("given a renamed game then its existing shortcut is updated", func(t *testing.T) {
		root := &Node{Type: TypeMap}
		Upsert(root, doom())
		renamed := doom()
		renamed.AppName = "DOOM (1993)"
		Upsert(root, renamed)
		entry := root.Child("shortcuts").Child("0")
		if entry.String("AppName") != "DOOM (1993)" || entry.Uint32("appid") != renamed.AppID() {
			t.Fatalf("entry = %+v", entry)
		}
	})
}

func steamTree(t *testing.T, users ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, u := range users {
		os.MkdirAll(filepath.Join(root, "userdata", u, "config"), 0755)
	}
	return root
}

func TestUserConfigDirs(t *testing.T) {
	t.Run("given real and placeholder user directories then only real users are returned", func(t *testing.T) {
		root := steamTree(t, "12345", "67890", "0", "ac")
		dirs, err := UserConfigDirs(root, "")
		if err != nil || len(dirs) != 2 || filepath.Base(filepath.Dir(dirs[0])) != "12345" {
			t.Fatalf("dirs %v err %v", dirs, err)
		}
	})

	t.Run("given a chosen user then only that user is returned", func(t *testing.T) {
		root := steamTree(t, "12345", "67890")
		dirs, _ := UserConfigDirs(root, "67890")
		if len(dirs) != 1 || filepath.Base(filepath.Dir(dirs[0])) != "67890" {
			t.Fatalf("dirs %v", dirs)
		}
	})

	t.Run("given no users then it explains that Steam has to be run once", func(t *testing.T) {
		if _, err := UserConfigDirs(steamTree(t), ""); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestFindRoot(t *testing.T) {
	t.Run("given a native Steam install then its directory is found", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		os.MkdirAll(filepath.Join(home, ".local", "share", "Steam", "userdata"), 0755)
		if root, err := FindRoot(); err != nil || root != filepath.Join(home, ".local", "share", "Steam") {
			t.Fatalf("root %q err %v", root, err)
		}
	})
	t.Run("given no Steam install then it fails", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		if _, err := FindRoot(); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestRunning(t *testing.T) {
	t.Run("given a process named steam then Steam is running", func(t *testing.T) {
		proc := t.TempDir()
		os.MkdirAll(filepath.Join(proc, "4242"), 0755)
		os.WriteFile(filepath.Join(proc, "4242", "comm"), []byte("steam\n"), 0644)
		os.MkdirAll(filepath.Join(proc, "self"), 0755)
		if !runningIn(proc) {
			t.Fatal("steam not detected")
		}
	})
	t.Run("given no steam process then Steam is not running", func(t *testing.T) {
		proc := t.TempDir()
		os.MkdirAll(filepath.Join(proc, "1"), 0755)
		os.WriteFile(filepath.Join(proc, "1", "comm"), []byte("systemd\n"), 0644)
		if runningIn(proc) {
			t.Fatal("steam falsely detected")
		}
	})
}

func TestExport(t *testing.T) {
	t.Run("given games with covers when exported then shortcuts.vdf and grid art are written", func(t *testing.T) {
		// Given
		config := filepath.Join(steamTree(t, "12345"), "userdata", "12345", "config")
		cover := filepath.Join(t.TempDir(), "cover.png")
		os.WriteFile(cover, []byte("png"), 0644)
		s := doom()

		// When
		added, err := Export(config, []Item{{Shortcut: s, Cover: cover}})

		// Then
		if err != nil || added != 1 {
			t.Fatalf("added %d err %v", added, err)
		}
		data, _ := os.ReadFile(filepath.Join(config, ShortcutsFile))
		root, err := Parse(data)
		if err != nil || root.Child("shortcuts").Child("0").String("AppName") != "Doom" {
			t.Fatalf("shortcuts.vdf not readable: %v", err)
		}
		if art, err := os.ReadFile(GridPath(config, s.AppID(), ".png")); err != nil || string(art) != "png" {
			t.Fatalf("grid art missing: %v", err)
		}
	})

	t.Run("given an existing shortcuts file when exported then user shortcuts are kept", func(t *testing.T) {
		config := filepath.Join(steamTree(t, "12345"), "userdata", "12345", "config")
		os.WriteFile(filepath.Join(config, ShortcutsFile), fixture(), 0644)
		if _, err := Export(config, []Item{{Shortcut: doom()}}); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(config, ShortcutsFile))
		root, _ := Parse(data)
		if len(root.Child("shortcuts").Children) != 2 || root.Child("shortcuts").Child("0").String("AppName") != "My Emulator" {
			t.Fatalf("shortcuts = %+v", root.Child("shortcuts").Children)
		}
	})

	t.Run("given a corrupt shortcuts file then export refuses rather than overwriting it", func(t *testing.T) {
		config := filepath.Join(steamTree(t, "12345"), "userdata", "12345", "config")
		os.WriteFile(filepath.Join(config, ShortcutsFile), []byte{0x05}, 0644)
		if _, err := Export(config, []Item{{Shortcut: doom()}}); err == nil {
			t.Fatal("expected an error")
		}
		if data, _ := os.ReadFile(filepath.Join(config, ShortcutsFile)); !bytes.Equal(data, []byte{0x05}) {
			t.Fatal("corrupt file was overwritten")
		}
	})
}
