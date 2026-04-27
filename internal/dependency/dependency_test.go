package dependency

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/config"
)

// --- ensureProton ---

func TestEnsureProton_UsesLocalPathWithoutDownloading(t *testing.T) {
	// Given a proton version that points to a local path (no URL download needed)
	d := t.TempDir()
	protonDir := filepath.Join(d, "my-proton")
	if err := os.MkdirAll(protonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protonDir, "proton"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	appCfg := config.App{ProtonVersion: "my-proton"}
	globalCfg := config.Global{
		ProtonVersions: map[string]config.VersionInfo{
			"my-proton": {Path: protonDir},
		},
	}

	// When ensureProton is called
	err := ensureProton(appCfg, false, globalCfg)

	// Then it succeeds without downloading anything
	if err != nil {
		t.Fatalf("ensureProton returned an unexpected error: %v", err)
	}
}

func TestEnsureProton_ErrorsWhenLocalPathMissing(t *testing.T) {
	// Given a proton version that references a local path that does not exist
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	appCfg := config.App{ProtonVersion: "my-proton"}
	globalCfg := config.Global{
		ProtonVersions: map[string]config.VersionInfo{
			"my-proton": {Path: "/path/does/not/exist"},
		},
	}

	// When ensureProton is called
	err := ensureProton(appCfg, false, globalCfg)

	// Then it returns an error about the missing path
	if err == nil {
		t.Fatal("expected an error for a missing local proton path but got nil")
	}
}

func TestEnsureProton_DoesNotCreateWin32PatchedDirectory(t *testing.T) {
	// Given a proton version with a local path (simulating a real setup)
	d := t.TempDir()
	protonDir := filepath.Join(d, "proton", "ge9")
	if err := os.MkdirAll(protonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protonDir, "proton"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	// When ensureProton is called with any config (no wine_arch field after PRD-2)
	appCfg := config.App{ProtonVersion: "ge9"}
	globalCfg := config.Global{
		ProtonVersions: map[string]config.VersionInfo{
			"ge9": {Path: protonDir},
		},
	}
	if err := ensureProton(appCfg, false, globalCfg); err != nil {
		t.Fatalf("ensureProton: %v", err)
	}

	// Then no -win32 patched directory is ever created on disk
	win32Dir := filepath.Join(d, "proton", "ge9-win32")
	if _, err := os.Stat(win32Dir); !os.IsNotExist(err) {
		t.Fatalf("a win32 patched directory was created at %s — patchProtonForWin32 should be removed", win32Dir)
	}
}

func TestEnsureProton_ErrorsWhenVersionNotInGlobalConfig(t *testing.T) {
	// Given an app config that references a proton version not defined in runner.json
	appCfg := config.App{ProtonVersion: "ghost"}
	globalCfg := config.Global{ProtonVersions: map[string]config.VersionInfo{}}

	// When ensureProton is called
	err := ensureProton(appCfg, false, globalCfg)

	// Then it returns an error
	if err == nil {
		t.Fatal("expected an error for an undefined proton version but got nil")
	}
}

func TestEnsureProton_SystemProtonVersionIsNoop(t *testing.T) {
	// Given proton_version is "system" with no matching entry in runner.json
	appCfg := config.App{ProtonVersion: "system"}
	globalCfg := config.Global{ProtonVersions: map[string]config.VersionInfo{}}

	// When ensureProton is called
	err := ensureProton(appCfg, false, globalCfg)

	// Then it returns nil without erroring about a missing version
	if err != nil {
		t.Fatalf("expected nil for system proton version but got: %v", err)
	}
}

// --- EnsureWinetricks ---

func TestEnsureWinetricks_SkipsWhenListEmpty(t *testing.T) {
	// Given an empty winetricks package list
	prefixDir := t.TempDir()

	// When EnsureWinetricks is called with no packages
	err := EnsureWinetricks(prefixDir, nil)

	// Then it returns nil without doing anything
	if err != nil {
		t.Fatalf("expected nil for empty package list but got: %v", err)
	}
}

func TestEnsureWinetricks_ErrorsWhenWinetricksNotInPath(t *testing.T) {
	// Given winetricks is not in PATH
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", origPath)

	// When EnsureWinetricks is called with packages
	err := EnsureWinetricks(t.TempDir(), []string{"vcrun2022"})

	// Then it returns an error mentioning winetricks not found
	if err == nil {
		t.Fatal("expected an error when winetricks is not in PATH but got nil")
	}
	if !strings.Contains(err.Error(), "winetricks not found") {
		t.Fatalf("expected error to mention 'winetricks not found', got: %v", err)
	}
}

func TestEnsureWinetricks_RunsSingleInvocationWithAllPackages(t *testing.T) {
	// Given a fake winetricks script that records its arguments
	d := t.TempDir()
	argsFile := filepath.Join(d, "winetricks.args")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\nexit 0\n"
	scriptPath := filepath.Join(d, "winetricks")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", d+":"+origPath)
	defer os.Setenv("PATH", origPath)

	// When EnsureWinetricks is called with multiple packages
	if err := EnsureWinetricks(t.TempDir(), []string{"vcrun2022", "dotnet48"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then winetricks was called once with all packages in a single invocation
	argsBytes, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal("winetricks was not called: args file not created")
	}
	argsStr := strings.TrimSpace(string(argsBytes))
	if !strings.Contains(argsStr, "vcrun2022") || !strings.Contains(argsStr, "dotnet48") {
		t.Fatalf("expected both packages in single invocation, got: %q", argsStr)
	}
	if strings.Count(argsStr, "\n") > 0 {
		t.Fatalf("expected single invocation but got multiple lines: %q", argsStr)
	}
}

// --- getInfo ---

func TestGetInfo_ReturnsVersionInfoForKnownDependency(t *testing.T) {
	// Given a global config that defines a DXVK version
	globalCfg := config.Global{
		DependencyVersions: map[string]map[string]config.VersionInfo{
			"dxvk": {
				"2.3": {URL: "http://example.com/dxvk-2.3.tar.gz"},
			},
		},
	}

	// When getInfo is called
	vinfo, err := getInfo("dxvk", "2.3", globalCfg)

	// Then it returns the correct info with no error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vinfo.URL != "http://example.com/dxvk-2.3.tar.gz" {
		t.Fatalf("URL mismatch: got %q", vinfo.URL)
	}
}

func TestGetInfo_ErrorsForUnknownDependencyType(t *testing.T) {
	// Given a global config with no entry for "vkd3d"
	globalCfg := config.Global{DependencyVersions: map[string]map[string]config.VersionInfo{}}

	// When getInfo is called for an unknown dependency type
	_, err := getInfo("vkd3d", "2.0", globalCfg)

	// Then it returns an error
	if err == nil {
		t.Fatal("expected an error for an unknown dependency type but got nil")
	}
}

func TestGetInfo_ErrorsForUnknownVersion(t *testing.T) {
	// Given a global config that has the dependency type but not the requested version
	globalCfg := config.Global{
		DependencyVersions: map[string]map[string]config.VersionInfo{
			"dxvk": {"2.3": {URL: "http://example.com"}},
		},
	}

	// When getInfo is called for a version that doesn't exist
	_, err := getInfo("dxvk", "9.9", globalCfg)

	// Then it returns an error
	if err == nil {
		t.Fatal("expected an error for an unknown version but got nil")
	}
}
