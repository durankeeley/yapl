package lifecycle

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/kernel"
)

// inLibrary runs the test with the working directory set to a fresh library.
func inLibrary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	return dir
}

func execute(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	k := kernel.New("yapl", &out, &errOut)
	if err := k.Register(Module{}); err != nil {
		t.Fatal(err)
	}
	code := k.Execute(args)
	return code, out.String(), errOut.String()
}

func writeGame(t *testing.T, name, cfg string) {
	t.Helper()
	dir := filepath.Join("games", name)
	if err := os.MkdirAll(filepath.Join(dir, "prefix"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.json"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
}

const validGame = `{"proton_version":"ge","launch_method":"direct","executable":"drive_c/doom.exe"}`

func TestModule(t *testing.T) {
	t.Run("given the module then it declares a semantic version and every lifecycle verb", func(t *testing.T) {
		var verbs []string
		for _, c := range (Module{}).Commands() {
			verbs = append(verbs, c.Verb)
		}
		if got := strings.Join(verbs, ","); got != "setup,install,run,package,unpackage,info,list,clean" {
			t.Fatalf("verbs = %s", got)
		}
	})
}

func TestList(t *testing.T) {
	t.Run("given an installed game when list runs then it is printed", func(t *testing.T) {
		// Given
		inLibrary(t)
		writeGame(t, "Doom", validGame)

		// When
		code, out, _ := execute(t, "list")

		// Then
		if code != kernel.ExitOK || !strings.Contains(out, "Doom") || !strings.Contains(out, "direct") {
			t.Fatalf("exit %d out:\n%s", code, out)
		}
	})
}

func TestInfo(t *testing.T) {
	t.Run("given an installed game when info runs then its config is summarised", func(t *testing.T) {
		// Given
		inLibrary(t)
		writeGame(t, "Doom", validGame)

		// When
		code, out, errOut := execute(t, "info", "Doom")

		// Then
		if code != kernel.ExitOK || !strings.Contains(out, "drive_c/doom.exe") {
			t.Fatalf("exit %d out:\n%s\nerr:\n%s", code, out, errOut)
		}
	})

	t.Run("given an unknown game when info runs then it fails naming the game", func(t *testing.T) {
		inLibrary(t)
		code, _, errOut := execute(t, "info", "Quake")
		if code != kernel.ExitError || !strings.Contains(errOut, "'Quake' not found") {
			t.Fatalf("exit %d err:\n%s", code, errOut)
		}
	})

	t.Run("given no name when info runs then it is a usage error", func(t *testing.T) {
		inLibrary(t)
		if code, _, _ := execute(t, "info"); code != kernel.ExitUsage {
			t.Fatalf("exit %d", code)
		}
	})
}

func TestClean(t *testing.T) {
	t.Run("given --all --yes when clean runs then the prefix is removed", func(t *testing.T) {
		// Given
		inLibrary(t)
		writeGame(t, "Doom", validGame)

		// When
		code, _, errOut := execute(t, "clean", "Doom", "--all", "--yes")

		// Then
		if code != kernel.ExitOK {
			t.Fatalf("exit %d err:\n%s", code, errOut)
		}
		if _, err := os.Stat(filepath.Join("games", "Doom", "prefix")); !os.IsNotExist(err) {
			t.Fatal("prefix not removed")
		}
	})

	t.Run("given no target flag when clean runs then it is a usage error", func(t *testing.T) {
		inLibrary(t)
		writeGame(t, "Doom", validGame)
		if code, _, errOut := execute(t, "clean", "Doom"); code != kernel.ExitUsage || !strings.Contains(errOut, "--prefix") {
			t.Fatalf("exit %d err:\n%s", code, errOut)
		}
	})
}

func TestUnpackage(t *testing.T) {
	t.Run("given no archive files when unpackage runs then it is a usage error", func(t *testing.T) {
		inLibrary(t)
		if code, _, errOut := execute(t, "unpackage", "game"); code != kernel.ExitUsage || !strings.Contains(errOut, "archive") {
			t.Fatalf("exit %d err:\n%s", code, errOut)
		}
	})

	t.Run("given a wrong kind when unpackage runs then it is a usage error", func(t *testing.T) {
		inLibrary(t)
		if code, _, _ := execute(t, "unpackage", "movie", "x.tar.xz"); code != kernel.ExitUsage {
			t.Fatalf("exit %d", code)
		}
	})
}

func TestPackageAndRunValidation(t *testing.T) {
	t.Run("given an invalid config when package runs then validation fails before any archive is written", func(t *testing.T) {
		// Given
		dir := inLibrary(t)
		writeGame(t, "Doom", `{"launch_method":"direct"}`)

		// When
		code, _, errOut := execute(t, "package", "Doom")

		// Then
		if code != kernel.ExitError || !strings.Contains(errOut, "proton_version is required") {
			t.Fatalf("exit %d err:\n%s", code, errOut)
		}
		if matches, _ := filepath.Glob(filepath.Join(dir, "*.tar.*")); len(matches) > 0 {
			t.Fatalf("archive written despite invalid config: %v", matches)
		}
	})

	t.Run("given an invalid config when run starts then validation fails", func(t *testing.T) {
		inLibrary(t)
		writeGame(t, "Doom", `{"launch_method":"direct"}`)
		if code, _, errOut := execute(t, "run", "Doom"); code != kernel.ExitError || !strings.Contains(errOut, "executable is required") {
			t.Fatalf("exit %d err:\n%s", code, errOut)
		}
	})
}

func TestInstall(t *testing.T) {
	t.Run("given a portable folder when install runs then the game is copied and the executable recorded", func(t *testing.T) {
		// Given
		inLibrary(t)
		os.WriteFile("runner.json", []byte(`{"proton_versions":{}}`), 0644)
		writeGame(t, "HalfLife", `{"proton_version":"system","launch_method":"direct","executable":"drive_c/windows/explorer.exe"}`)
		os.WriteFile(filepath.Join("games", "HalfLife", "prefix", "system.reg"), nil, 0644)
		src := t.TempDir()
		os.WriteFile(filepath.Join(src, "hl.exe"), []byte("exe"), 0755)

		// When
		code, out, errOut := execute(t, "install", "game", "HalfLife", "--from", src)

		// Then
		if code != kernel.ExitOK {
			t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
		}
		cfg, _ := os.ReadFile(filepath.Join("games", "HalfLife", "game.json"))
		if !strings.Contains(string(cfg), "drive_c/Games/HalfLife/hl.exe") {
			t.Fatalf("config not updated: %s", cfg)
		}
	})

	t.Run("given a custom config file when install runs then that file is the one updated", func(t *testing.T) {
		// Given
		inLibrary(t)
		os.WriteFile("runner.json", []byte(`{"proton_versions":{}}`), 0644)
		writeGame(t, "HalfLife", validGame)
		os.WriteFile(filepath.Join("games", "HalfLife", "cstrike.json"), []byte(`{"proton_version":"system","launch_method":"direct","executable":"drive_c/windows/explorer.exe"}`), 0644)
		os.WriteFile(filepath.Join("games", "HalfLife", "prefix", "system.reg"), nil, 0644)
		src := t.TempDir()
		os.WriteFile(filepath.Join(src, "hl.exe"), []byte("exe"), 0755)

		// When
		code, _, errOut := execute(t, "install", "game", "HalfLife", "--from", src, "--config", "cstrike.json")

		// Then
		if code != kernel.ExitOK {
			t.Fatalf("exit %d\n%s", code, errOut)
		}
		custom, _ := os.ReadFile(filepath.Join("games", "HalfLife", "cstrike.json"))
		if !strings.Contains(string(custom), "hl.exe") {
			t.Fatalf("cstrike.json not updated: %s", custom)
		}
	})

	usage := []struct {
		name string
		args []string
		want string
	}{
		{"given neither --installer nor --from then it is a usage error", []string{"install", "game", "Doom"}, "exactly one"},
		{"given both --installer and --from then it is a usage error", []string{"install", "game", "Doom", "--installer", "a.exe", "--from", "dir"}, "exactly one"},
		{"given a wrong kind then it is a usage error", []string{"install", "movie", "Doom", "--from", "dir"}, "game or app"},
		{"given no name then it is a usage error", []string{"install", "game", "--from", "dir"}, "name is required"},
	}
	for _, tc := range usage {
		t.Run(tc.name, func(t *testing.T) {
			inLibrary(t)
			code, _, errOut := execute(t, tc.args...)
			if code != kernel.ExitUsage || !strings.Contains(errOut, tc.want) {
				t.Fatalf("exit %d err:\n%s", code, errOut)
			}
		})
	}
}
