package command

import (
	"os"
	"path/filepath"
	"testing"

	"yapl/internal/config"
)

// --- prefixIsInitialized ---

func TestPrefixIsInitialized_FlatLayout(t *testing.T) {
	// Given a prefix directory with system.reg at the root (standard YAPL layout)
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "system.reg"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// When prefixIsInitialized is called
	// Then it returns true
	if !prefixIsInitialized(d) {
		t.Fatal("expected true for a flat-layout prefix with system.reg at root")
	}
}

func TestPrefixIsInitialized_PfxSubdirLayout(t *testing.T) {
	// Given a prefix directory with system.reg inside pfx/ (Lutris/raw Proton layout)
	d := t.TempDir()
	pfx := filepath.Join(d, "pfx")
	if err := os.Mkdir(pfx, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pfx, "system.reg"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// When prefixIsInitialized is called
	// Then it returns true (Lutris layout is detected)
	if !prefixIsInitialized(d) {
		t.Fatal("expected true for a Lutris-style pfx/ layout with system.reg inside pfx/")
	}
}

func TestPrefixIsInitialized_EmptyDir(t *testing.T) {
	// Given an empty directory with no system.reg anywhere
	d := t.TempDir()

	// When prefixIsInitialized is called
	// Then it returns false
	if prefixIsInitialized(d) {
		t.Fatal("expected false for an empty directory")
	}
}

func TestPrefixIsInitialized_PfxIsSymlink(t *testing.T) {
	// Given a prefix that is already restructured (pfx/ is a symlink to .)
	// and system.reg is at the root
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "system.reg"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".", filepath.Join(d, "pfx")); err != nil {
		t.Fatal(err)
	}

	// When prefixIsInitialized is called
	// Then it returns true without infinite-looping through the symlink
	if !prefixIsInitialized(d) {
		t.Fatal("expected true for an already-restructured prefix")
	}
}

// --- restructureProtonPrefix (idempotency) ---

func TestRestructureProtonPrefix_IsNoopWhenPfxIsAlreadySymlink(t *testing.T) {
	// Given a prefix directory where pfx/ is already a symlink to . (already restructured)
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "system.reg"), []byte("reg"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".", filepath.Join(d, "pfx")); err != nil {
		t.Fatal(err)
	}

	// When restructureProtonPrefix is called a second time
	if err := restructureProtonPrefix(d); err != nil {
		t.Fatalf("restructureProtonPrefix should be a no-op on already-restructured prefix, got: %v", err)
	}

	// Then system.reg is still in place and pfx/ is still a symlink
	if _, err := os.Stat(filepath.Join(d, "system.reg")); err != nil {
		t.Fatal("system.reg was removed during no-op restructure")
	}
	info, _ := os.Lstat(filepath.Join(d, "pfx"))
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("pfx/ should still be a symlink after no-op restructure")
	}
}

// --- system Wine prefix initialization ---

func TestInitializePrefix_SystemWineUsesExecLookPath(t *testing.T) {
	// Given a config with proton_version set to "system"
	d := t.TempDir()
	appCfg := config.App{
		ProtonVersion: "system",
		LaunchMethod:  "direct",
		Executable:    "drive_c/windows/explorer.exe",
	}
	globalCfg := config.Global{
		ProtonVersions: map[string]config.VersionInfo{
			"system": {},
		},
	}

	// When InitializePrefix is called with system Wine
	// Then it should NOT error with "proton version not defined" (system is handled specially)
	// and NOT error looking for a proton script that doesn't exist
	// (It may fail finding wine64 on this machine, but that's a different error)
	err := InitializePrefix(d, appCfg, globalCfg, false)

	// Then the error (if any) is about the wine binary, not about proton config
	if err != nil {
		errStr := err.Error()
		if contains(errStr, "proton version") || contains(errStr, "proton script") {
			t.Fatalf("system Wine path should not fail on proton-specific errors, got: %v", err)
		}
		// Other errors (wine not found, exec failed) are acceptable in CI
		t.Logf("system Wine init returned (expected on CI with no wine): %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
