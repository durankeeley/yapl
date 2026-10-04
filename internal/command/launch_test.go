package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/config"
)

// recorder writes a script that logs its arguments and selected
// environment variables to logPath, one per line.
func recorder(t *testing.T, path, logPath string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do echo \"arg:$a\" >> \"" + logPath + "\"; done\n" +
		"echo \"env:WINEPREFIX=$WINEPREFIX\" >> \"" + logPath + "\"\n" +
		"echo \"env:PROTONPATH=$PROTONPATH\" >> \"" + logPath + "\"\n" +
		"echo \"env:GAMEID=$GAMEID\" >> \"" + logPath + "\"\n" +
		"echo \"env:STORE=$STORE\" >> \"" + logPath + "\"\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("runner never ran: %v", err)
	}
	return string(data)
}

func launchLibrary(t *testing.T) (protonDir, prefix, logPath string) {
	t.Helper()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })
	protonDir = filepath.Join(dir, "proton", "ge")
	os.MkdirAll(filepath.Join(protonDir, "files", "bin"), 0755)
	os.WriteFile(filepath.Join(protonDir, "proton"), []byte("#!/bin/sh\n"), 0755)
	prefix = filepath.Join(dir, "games", "Doom", "prefix")
	os.MkdirAll(prefix, 0755)
	return protonDir, prefix, filepath.Join(dir, "runner.log")
}

func TestRunInContainer(t *testing.T) {
	t.Run("given a runtime when a game runs in the container then the entry point receives the proton verb chain and exe", func(t *testing.T) {
		// Given
		protonDir, prefix, logPath := launchLibrary(t)
		recorder(t, filepath.Join("dependencies", "runtime", "sniper", "yapl-entry-point"), logPath)
		global := config.Global{ProtonVersions: map[string]config.VersionInfo{"ge": {Path: protonDir}}}
		app := config.App{ProtonVersion: "ge", RuntimeVersion: "sniper", LaunchMethod: "container", Executable: "drive_c/doom.exe", LaunchArgs: []string{"-fullscreen"}}

		// When
		err := RunInContainer(prefix, app, global, false)

		// Then
		if err != nil {
			t.Fatal(err)
		}
		log := readLog(t, logPath)
		for _, want := range []string{"arg:--verb=waitforexitandrun", "arg:--", "arg:" + filepath.Join("dependencies", "runtime", "sniper", "yapl-shim"), "arg:" + filepath.Join(protonDir, "proton"), "arg:" + filepath.Join(prefix, "drive_c", "doom.exe"), "arg:-fullscreen", "env:WINEPREFIX=" + prefix} {
			if !strings.Contains(log, want+"\n") {
				t.Errorf("missing %q in:\n%s", want, log)
			}
		}
	})

	t.Run("given no runtime version then container mode refuses to start", func(t *testing.T) {
		_, prefix, _ := launchLibrary(t)
		if err := RunInContainer(prefix, config.App{ProtonVersion: "ge"}, config.Global{}, false); err == nil || !strings.Contains(err.Error(), "runtime_version") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given a wine-only build without a proton script then container mode explains why it cannot run", func(t *testing.T) {
		protonDir, prefix, _ := launchLibrary(t)
		os.Remove(filepath.Join(protonDir, "proton"))
		global := config.Global{ProtonVersions: map[string]config.VersionInfo{"ge": {Path: protonDir}}}
		err := RunInContainer(prefix, config.App{ProtonVersion: "ge", RuntimeVersion: "sniper", Executable: "x.exe"}, global, false)
		if err == nil || !strings.Contains(err.Error(), "full Proton build") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestRunWithUMU(t *testing.T) {
	t.Run("given a bundled umu-launcher when a game runs then umu-run receives the exe and umu environment", func(t *testing.T) {
		// Given
		protonDir, prefix, logPath := launchLibrary(t)
		recorder(t, filepath.Join("dependencies", "umu-launcher", "1.2.9", "bin", "umu-run"), logPath)
		global := config.Global{
			ProtonVersions:     map[string]config.VersionInfo{"ge": {Path: protonDir}},
			DependencyVersions: map[string]map[string]config.VersionInfo{"umu-launcher": {"1.2.9": {BinPath: "bin"}}},
		}
		app := config.App{ProtonVersion: "ge", LaunchMethod: "umu", Executable: "drive_c/doom.exe", LaunchArgs: []string{"-a"},
			UMUOptions: config.UMUOptions{Version: "1.2.9", GameID: "umu-2280", Store: "gog", LaunchArgs: []string{"-b"}}}

		// When
		err := RunWithUMU(prefix, app, global, false)

		// Then
		if err != nil {
			t.Fatal(err)
		}
		log := readLog(t, logPath)
		for _, want := range []string{"arg:" + filepath.Join(prefix, "drive_c", "doom.exe"), "arg:-a", "arg:-b", "env:PROTONPATH=" + protonDir, "env:GAMEID=umu-2280", "env:STORE=gog"} {
			if !strings.Contains(log, want+"\n") {
				t.Errorf("missing %q in:\n%s", want, log)
			}
		}
	})

	tests := []struct {
		name    string
		umu     config.UMUOptions
		wantErr string
	}{
		{"given no umu version then it asks for one", config.UMUOptions{}, "umu_options.version"},
		{"given a umu version runner.json lacks then it says so", config.UMUOptions{Version: "9.9"}, "not defined"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, prefix, _ := launchLibrary(t)
			err := RunWithUMU(prefix, config.App{ProtonVersion: "ge", UMUOptions: tc.umu}, config.Global{}, false)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestWaitForWineserver(t *testing.T) {
	t.Run("given a wineserver in the proton build then it is asked to wait for the prefix", func(t *testing.T) {
		protonDir, prefix, logPath := launchLibrary(t)
		recorder(t, filepath.Join(protonDir, "files", "bin", "wineserver"), logPath)
		global := config.Global{ProtonVersions: map[string]config.VersionInfo{"ge": {Path: protonDir}}}
		if err := WaitForWineserver(prefix, config.App{ProtonVersion: "ge"}, global); err != nil {
			t.Fatal(err)
		}
		if log := readLog(t, logPath); !strings.Contains(log, "arg:-w\n") || !strings.Contains(log, "env:WINEPREFIX="+prefix+"\n") {
			t.Fatalf("log:\n%s", log)
		}
	})

	t.Run("given no wineserver then there is nothing to wait for", func(t *testing.T) {
		protonDir, prefix, _ := launchLibrary(t)
		global := config.Global{ProtonVersions: map[string]config.VersionInfo{"ge": {Path: protonDir}}}
		if err := WaitForWineserver(prefix, config.App{ProtonVersion: "ge"}, global); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPrefixIsInitialized(t *testing.T) {
	tests := []struct {
		name  string
		setup func(dir string)
		want  bool
	}{
		{"given a flat prefix with system.reg then it is initialised", func(d string) { os.WriteFile(filepath.Join(d, "system.reg"), nil, 0644) }, true},
		{"given a Lutris pfx directory with system.reg then it is initialised", func(d string) {
			os.MkdirAll(filepath.Join(d, "pfx"), 0755)
			os.WriteFile(filepath.Join(d, "pfx", "system.reg"), nil, 0644)
		}, true},
		{"given a pfx symlink back to the prefix then it is not counted", func(d string) { os.Symlink(".", filepath.Join(d, "pfx")) }, false},
		{"given an empty directory then it is not initialised", func(string) {}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(dir)
			if got := PrefixIsInitialized(dir); got != tc.want {
				t.Fatalf("PrefixIsInitialized = %v", got)
			}
		})
	}
}
