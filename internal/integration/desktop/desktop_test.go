package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	tests := map[string]string{
		"Need for Speed: Most Wanted": "need-for-speed-most-wanted",
		"Half-Life":                   "half-life",
		"  Doom  ":                    "doom",
		"C&C Red Alert 2":             "c-c-red-alert-2",
		"!!!":                         "game",
	}
	for in, want := range tests {
		t.Run("given "+in+" then the slug is "+want, func(t *testing.T) {
			if got := Slug(in); got != want {
				t.Fatalf("Slug(%q) = %q", in, got)
			}
		})
	}
}

func TestExecLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"given plain arguments then they are joined unquoted", []string{"/usr/bin/yapl", "run", "Doom"}, "/usr/bin/yapl run Doom"},
		{"given an argument with spaces then it is double quoted", []string{"/usr/bin/yapl", "run", "Need for Speed"}, `/usr/bin/yapl run "Need for Speed"`},
		{"given quotes, dollars and backslashes then they are escaped inside quotes", []string{`a "b" $c \d`}, `"a \"b\" \$c \\d"`},
		{"given a percent sign then it is doubled for field codes", []string{"100%"}, "100%%"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExecLine(tc.args); got != tc.want {
				t.Fatalf("ExecLine = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	t.Run("given an entry then a valid desktop file is produced", func(t *testing.T) {
		// When
		out := Render(Entry{Name: "Need for Speed", Comment: "Play with YAPL", Exec: []string{"/usr/bin/yapl", "run", "Need for Speed"}, Icon: "/lib/games/NFS/cover.jpg", Categories: []string{"Game"}})

		// Then
		for _, want := range []string{"[Desktop Entry]\n", "Type=Application\n", "Name=Need for Speed\n", `Exec=/usr/bin/yapl run "Need for Speed"` + "\n", "Icon=/lib/games/NFS/cover.jpg\n", "Categories=Game;\n", "Terminal=false\n"} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q in:\n%s", want, out)
			}
		}
	})

	t.Run("given extra keys then they are written in order", func(t *testing.T) {
		out := Render(Entry{Name: "YAPL Console", Exec: []string{"yapl", "console"}, Extra: [][2]string{{"X-GNOME-Autostart-enabled", "true"}}})
		if !strings.HasSuffix(out, "X-GNOME-Autostart-enabled=true\n") {
			t.Fatalf("out:\n%s", out)
		}
	})

	t.Run("given no icon then the generic games icon is used", func(t *testing.T) {
		if out := Render(Entry{Name: "Doom", Exec: []string{"yapl"}}); !strings.Contains(out, "Icon="+DefaultIcon+"\n") {
			t.Fatalf("out:\n%s", out)
		}
	})

	t.Run("given a name with a newline then it cannot inject keys", func(t *testing.T) {
		out := Render(Entry{Name: "Doom\nExec=rm -rf ~", Exec: []string{"yapl"}})
		if strings.Contains(out, "\nExec=rm") {
			t.Fatalf("newline injected a key:\n%s", out)
		}
	})
}

func TestWrite(t *testing.T) {
	t.Run("given an output directory that does not exist then the file is created inside it", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "applications")
		path, err := Write(dir, "yapl-doom.desktop", Entry{Name: "Doom", Exec: []string{"yapl", "run", "Doom"}})
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		if path != filepath.Join(dir, "yapl-doom.desktop") || !strings.Contains(string(data), "Name=Doom") {
			t.Fatalf("path %s data %s", path, data)
		}
	})
}

func TestDirectories(t *testing.T) {
	t.Run("given XDG variables then they decide the target directories", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "/data")
		t.Setenv("XDG_CONFIG_HOME", "/config")
		if ApplicationsDir() != "/data/applications" || AutostartDir() != "/config/autostart" {
			t.Fatalf("%s %s", ApplicationsDir(), AutostartDir())
		}
	})
	t.Run("given no XDG variables then the home defaults are used", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "/home/player")
		if ApplicationsDir() != "/home/player/.local/share/applications" || AutostartDir() != "/home/player/.config/autostart" {
			t.Fatalf("%s %s", ApplicationsDir(), AutostartDir())
		}
	})
}
