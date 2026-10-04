package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func touch(t *testing.T, root, rel string, size int) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, size), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshot(t *testing.T) {
	t.Run("given a prefix then executables under drive_c are found except the windows directory", func(t *testing.T) {
		// Given
		prefix := t.TempDir()
		touch(t, prefix, "drive_c/Games/Doom/doom.exe", 10)
		touch(t, prefix, "drive_c/Games/Doom/README.txt", 10)
		touch(t, prefix, "drive_c/Program Files/Tool/TOOL.EXE", 5)
		touch(t, prefix, "drive_c/windows/system32/notepad.exe", 1)
		touch(t, prefix, "drive_c/WINDOWS/explorer.exe", 1)

		// When
		snap, err := Snapshot(prefix)

		// Then
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]int64{"drive_c/Games/Doom/doom.exe": 10, "drive_c/Program Files/Tool/TOOL.EXE": 5}
		if len(snap) != len(want) {
			t.Fatalf("snapshot = %v, want %v", snap, want)
		}
		for k, v := range want {
			if snap[k] != v {
				t.Errorf("snapshot[%q] = %d, want %d", k, snap[k], v)
			}
		}
	})

	t.Run("given a prefix without drive_c then the snapshot is empty", func(t *testing.T) {
		snap, err := Snapshot(t.TempDir())
		if err != nil || len(snap) != 0 {
			t.Fatalf("snapshot = %v, %v", snap, err)
		}
	})
}

func paths(exes []Executable) string {
	var out []string
	for _, e := range exes {
		out = append(out, e.Path)
	}
	return strings.Join(out, ",")
}

func TestCandidates(t *testing.T) {
	tests := []struct {
		name     string
		game     string
		before   map[string]int64
		after    map[string]int64
		wantPath string
	}{
		{
			name:     "given an installer that adds the game and its uninstaller then the uninstaller is excluded",
			game:     "Doom",
			after:    map[string]int64{"drive_c/Games/Doom/Doom.exe": 100, "drive_c/Games/Doom/unins000.exe": 900},
			wantPath: "drive_c/Games/Doom/Doom.exe",
		},
		{
			name:     "given executables that existed before then only new ones are candidates",
			game:     "Doom",
			before:   map[string]int64{"drive_c/Tools/old.exe": 1},
			after:    map[string]int64{"drive_c/Tools/old.exe": 1, "drive_c/Games/Doom/doom.exe": 2},
			wantPath: "drive_c/Games/Doom/doom.exe",
		},
		{
			name:     "given a name match and a larger unrelated file then the name match wins",
			game:     "Need for Speed Most Wanted",
			after:    map[string]int64{"drive_c/NFS/speed.exe": 10, "drive_c/NFS/tools/bigtool.exe": 9999},
			wantPath: "drive_c/NFS/speed.exe",
		},
		{
			name:     "given the game's initials as the exe name then it is preferred",
			game:     "Half-Life",
			after:    map[string]int64{"drive_c/Games/Half-Life/hl.exe": 10, "drive_c/Games/Half-Life/hlds.exe": 20},
			wantPath: "drive_c/Games/Half-Life/hl.exe",
		},
		{
			name:     "given no name match then the shallowest largest executable wins",
			game:     "Quake",
			after:    map[string]int64{"drive_c/Q/bin/a/deep.exe": 500, "drive_c/Q/main.exe": 400, "drive_c/Q/small.exe": 1},
			wantPath: "drive_c/Q/main.exe",
		},
		{
			name:     "given only redistributables then there are no candidates",
			game:     "Doom",
			after:    map[string]int64{"drive_c/_redist/vcredist_x86.exe": 1, "drive_c/_redist/DXSETUP.exe": 1, "drive_c/x/UnityCrashHandler64.exe": 1},
			wantPath: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// When
			got := Candidates(tc.before, tc.after, tc.game)

			// Then
			if tc.wantPath == "" {
				if len(got) != 0 {
					t.Fatalf("expected no candidates, got %s", paths(got))
				}
				return
			}
			if len(got) == 0 || got[0].Path != tc.wantPath {
				t.Fatalf("ranked = %s, want first %s", paths(got), tc.wantPath)
			}
		})
	}
}

func TestIgnored(t *testing.T) {
	ignored := []string{"unins000.exe", "Uninstall.exe", "setup.exe", "Install.exe", "installer.exe", "vcredist_x64.exe", "VC_redist.x86.exe", "DXSETUP.exe", "dotNetFx40_Full_setup.exe", "oalinst.exe", "CrashReporter.exe", "UnityCrashHandler64.exe", "BugReport.exe", "updater.exe", "PhysX_SystemSoftware.exe"}
	for _, name := range ignored {
		t.Run("given "+name+" then it is ignored", func(t *testing.T) {
			if !Ignored(name) {
				t.Fatalf("%s should be ignored", name)
			}
		})
	}
	for _, name := range []string{"hl.exe", "speed.exe", "Launcher.exe", "game.exe"} {
		t.Run("given "+name+" then it is kept", func(t *testing.T) {
			if Ignored(name) {
				t.Fatalf("%s should not be ignored", name)
			}
		})
	}
}
