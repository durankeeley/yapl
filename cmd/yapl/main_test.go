package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const runMainEnv = "YAPL_TEST_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		os.Args = []string{"yapl"}
		if raw := os.Getenv("YAPL_TEST_ARGS"); raw != "" {
			os.Args = append(os.Args, strings.Split(raw, "\x1f")...)
		}
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type cliResult struct {
	output   string
	exitCode int
}

// runCLI executes the real yapl entry point in a subprocess rooted at workDir.
func runCLI(t *testing.T, workDir string, args ...string) cliResult {
	t.Helper()
	return runCLIWithEnv(t, workDir, nil, args...)
}

func runCLIWithEnv(t *testing.T, workDir string, env []string, args ...string) cliResult {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Dir = workDir
	cmd.Env = append(append(os.Environ(), env...), runMainEnv+"=1", "YAPL_TEST_ARGS="+strings.Join(args, "\x1f"))
	out, err := cmd.CombinedOutput()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("could not run cli: %v", err)
	}
	return cliResult{output: string(out), exitCode: code}
}

func writeGame(t *testing.T, workDir, name string) string {
	t.Helper()
	gameDir := filepath.Join(workDir, "games", name)
	if err := os.MkdirAll(filepath.Join(gameDir, "prefix", "drive_c"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := `{"proton_version":"p","launch_method":"direct","executable":"drive_c/game.exe"}`
	if err := os.WriteFile(filepath.Join(gameDir, "game.json"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "runner.json"), []byte(`{"proton_versions":{"p":{"url":"http://example/p.tar.gz"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	return gameDir
}

func TestClean(t *testing.T) {
	t.Run("given an unknown game when clean --prefix runs then the flag is accepted and the game lookup fails", func(t *testing.T) {
		// Given
		workDir := t.TempDir()

		// When
		res := runCLI(t, workDir, "clean", "Doom", "--prefix")

		// Then
		if strings.Contains(res.output, "flag provided but not defined") {
			t.Fatalf("clean flag rejected:\n%s", res.output)
		}
		if res.exitCode == 0 || !strings.Contains(res.output, "not found in games/ or apps/") {
			t.Fatalf("expected lookup failure, got exit %d:\n%s", res.exitCode, res.output)
		}
	})

	t.Run("given an installed game when clean --prefix --yes runs then the prefix is deleted", func(t *testing.T) {
		// Given
		workDir := t.TempDir()
		gameDir := writeGame(t, workDir, "Doom")

		// When
		res := runCLI(t, workDir, "clean", "Doom", "--prefix", "--yes")

		// Then
		if res.exitCode != 0 {
			t.Fatalf("expected success, got exit %d:\n%s", res.exitCode, res.output)
		}
		if _, err := os.Stat(filepath.Join(gameDir, "prefix")); !os.IsNotExist(err) {
			t.Fatalf("prefix still exists after clean")
		}
	})

	t.Run("given no clean target flag when clean runs then it explains which flags are required", func(t *testing.T) {
		// Given
		workDir := t.TempDir()
		writeGame(t, workDir, "Doom")

		// When
		res := runCLI(t, workDir, "clean", "Doom")

		// Then
		if res.exitCode == 0 || !strings.Contains(res.output, "--prefix") {
			t.Fatalf("expected usage error mentioning --prefix, got exit %d:\n%s", res.exitCode, res.output)
		}
	})
}

func TestHelpAndVersion(t *testing.T) {
	t.Run("given no arguments when yapl runs then the command overview is printed with a usage exit code", func(t *testing.T) {
		// When
		res := runCLI(t, t.TempDir())

		// Then
		if res.exitCode != 2 {
			t.Fatalf("exit %d, want 2", res.exitCode)
		}
		for _, verb := range []string{"setup", "run", "package", "unpackage", "info", "list", "clean", "serve", "pull", "help", "version"} {
			if !strings.Contains(res.output, "  "+verb+" ") {
				t.Errorf("overview missing %q:\n%s", verb, res.output)
			}
		}
	})

	t.Run("given help for clean then its scoped flags are listed with double dashes", func(t *testing.T) {
		// When
		res := runCLI(t, t.TempDir(), "help", "clean")

		// Then
		for _, want := range []string{"Usage: yapl clean <name> [flags]", "--prefix", "--proton", "--deps", "--all", "--yes"} {
			if !strings.Contains(res.output, want) {
				t.Errorf("clean help missing %q:\n%s", want, res.output)
			}
		}
	})

	t.Run("given version then every module is reported", func(t *testing.T) {
		// When
		res := runCLI(t, t.TempDir(), "version")

		// Then
		for _, want := range []string{"kernel", "lifecycle", "registry", "store"} {
			if !strings.Contains(res.output, want) {
				t.Errorf("version missing %q:\n%s", want, res.output)
			}
		}
	})
}

func TestFlagScoping(t *testing.T) {
	t.Run("given a clean flag passed to serve when yapl runs then it is rejected as unknown", func(t *testing.T) {
		// When
		res := runCLI(t, t.TempDir(), "serve", "--packages-dir", ".", "--prefix")

		// Then
		if res.exitCode != 2 || !strings.Contains(res.output, "--prefix") {
			t.Fatalf("exit %d:\n%s", res.exitCode, res.output)
		}
	})

	t.Run("given an unknown command then the error names it", func(t *testing.T) {
		res := runCLI(t, t.TempDir(), "teleport")
		if res.exitCode != 2 || !strings.Contains(res.output, `unknown command "teleport"`) {
			t.Fatalf("exit %d:\n%s", res.exitCode, res.output)
		}
	})
}

func TestSetupValidation(t *testing.T) {
	t.Run("given an invalid launch method when setup runs then it is a usage error and nothing is created", func(t *testing.T) {
		// Given
		workDir := t.TempDir()

		// When
		res := runCLI(t, workDir, "setup", "game", "Doom", "--method", "teleport")

		// Then
		if res.exitCode != 2 || !strings.Contains(res.output, "invalid --method") {
			t.Fatalf("exit %d:\n%s", res.exitCode, res.output)
		}
		if _, err := os.Stat(filepath.Join(workDir, "games", "Doom")); !os.IsNotExist(err) {
			t.Fatal("setup created a game directory despite the invalid method")
		}
	})

	t.Run("given a wrong kind when setup runs then it explains game or app", func(t *testing.T) {
		res := runCLI(t, t.TempDir(), "setup", "movie", "Doom")
		if res.exitCode != 2 || !strings.Contains(res.output, "game or app") {
			t.Fatalf("exit %d:\n%s", res.exitCode, res.output)
		}
	})
}

func TestWorkdir(t *testing.T) {
	t.Run("given --workdir before the command when list runs elsewhere then the library at workdir is listed", func(t *testing.T) {
		// Given
		library := t.TempDir()
		writeGame(t, library, "Doom")

		// When
		res := runCLI(t, t.TempDir(), "--workdir", library, "list")

		// Then
		if res.exitCode != 0 || !strings.Contains(res.output, "Doom") {
			t.Fatalf("exit %d:\n%s", res.exitCode, res.output)
		}
	})

	t.Run("given YAPL_HOME and no flag when list runs then the library at YAPL_HOME is listed", func(t *testing.T) {
		// Given
		library := t.TempDir()
		writeGame(t, library, "Quake")

		// When
		res := runCLIWithEnv(t, t.TempDir(), []string{"YAPL_HOME=" + library}, "list")

		// Then
		if res.exitCode != 0 || !strings.Contains(res.output, "Quake") {
			t.Fatalf("exit %d:\n%s", res.exitCode, res.output)
		}
	})

	t.Run("given both --workdir and YAPL_HOME then the flag wins", func(t *testing.T) {
		// Given
		flagLibrary, envLibrary := t.TempDir(), t.TempDir()
		writeGame(t, flagLibrary, "FromFlag")
		writeGame(t, envLibrary, "FromEnv")

		// When
		res := runCLIWithEnv(t, t.TempDir(), []string{"YAPL_HOME=" + envLibrary}, "--workdir", flagLibrary, "list")

		// Then
		if !strings.Contains(res.output, "FromFlag") || strings.Contains(res.output, "FromEnv") {
			t.Fatalf("flag did not take precedence:\n%s", res.output)
		}
	})

	t.Run("given a workdir that does not exist when a command runs then it fails clearly", func(t *testing.T) {
		// When
		res := runCLI(t, t.TempDir(), "--workdir", "/nonexistent/yapl-library", "list")

		// Then
		if res.exitCode != 1 || !strings.Contains(res.output, "workdir") {
			t.Fatalf("exit %d:\n%s", res.exitCode, res.output)
		}
	})
}

func TestFreshLibraryGetsWorkingDefaults(t *testing.T) {
	t.Run("given a game but no runner.json when a command loads it then runner.json is created from the pinned catalog", func(t *testing.T) {
		// Given
		workDir := t.TempDir()
		gameDir := filepath.Join(workDir, "games", "Doom")
		os.MkdirAll(gameDir, 0755)
		os.WriteFile(filepath.Join(gameDir, "game.json"), []byte(`{"proton_version":"GE-Proton11-7","runtime_version":"sniper","executable":"drive_c/doom.exe"}`), 0644)

		// When
		res := runCLI(t, workDir, "info", "Doom")

		// Then
		if res.exitCode != 0 {
			t.Fatalf("exit %d:\n%s", res.exitCode, res.output)
		}
		runner, err := os.ReadFile(filepath.Join(workDir, "runner.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"GE-Proton11-7-x86_64.tar.gz", "SteamLinuxRuntime_sniper", "dxvk-3.1.1", "vkd3d-proton-3.0.1", `"defaults"`} {
			if !strings.Contains(string(runner), want) {
				t.Errorf("runner.json missing %q", want)
			}
		}
		if strings.Contains(string(runner), "EDIT_ME") {
			t.Error("runner.json still contains placeholders")
		}
	})
}
