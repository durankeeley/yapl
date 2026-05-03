package dependency

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"yapl/internal/config"
)

// --- PRD-6: httpClient timeout ---

func TestHttpClient_HasNonZeroTimeout(t *testing.T) {
	// Given the package-level httpClient defined in runtime.go
	// Then it must have a non-zero timeout — bare http.Get with no timeout hangs indefinitely
	if httpClient.Timeout == 0 {
		t.Fatal("httpClient must have a non-zero timeout configured in runtime.go")
	}
}

func TestRuntimeNeedsUpdate_RespectsHttpClientTimeout(t *testing.T) {
	// Given a local version.txt
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "version.txt"), []byte("old-build"), 0644); err != nil {
		t.Fatal(err)
	}

	// And a server that deliberately delays longer than our injected timeout
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Write([]byte("build-id"))
	}))
	defer srv.Close()

	// When httpClient is overridden with a very short timeout for the test
	orig := httpClient
	httpClient = &http.Client{Timeout: 50 * time.Millisecond}
	defer func() { httpClient = orig }()

	// Then runtimeNeedsUpdate returns a timeout error
	_, err := runtimeNeedsUpdate(d, srv.URL+"/SteamLinuxRuntime.tar.xz")
	if err == nil {
		t.Fatal("expected a timeout error but got nil — httpClient.Get must be used, not bare http.Get")
	}
}

// --- PRD-17: HTTP status validation ---

func TestRuntimeNeedsUpdate_ReturnsErrorOnNon200Status(t *testing.T) {
	// Given a local version.txt
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "version.txt"), []byte("old-build"), 0644); err != nil {
		t.Fatal(err)
	}

	// And a server that returns 404
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// When runtimeNeedsUpdate is called
	_, err := runtimeNeedsUpdate(d, srv.URL+"/SteamLinuxRuntime.tar.xz")

	// Then it returns an error — a 404 body must not be silently treated as a valid version response
	if err == nil {
		t.Fatal("expected error for 404 response but got nil")
	}
}

func TestPostInstallRuntimeFixup_ReturnsErrorOnNon200Status(t *testing.T) {
	// Given a runtime directory and a server that returns 503
	d := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// When postInstallRuntimeFixup is called
	err := postInstallRuntimeFixup(d, srv.URL+"/SteamLinuxRuntime.tar.xz")

	// Then it returns an error — a 503 body must not be written as version.txt
	if err == nil {
		t.Fatal("expected error for 503 response but got nil")
	}
}

// --- PRD-24: EnsureRuntime propagates version-check errors ---

func TestEnsureRuntime_ReturnsErrorWhenVersionCheckFails(t *testing.T) {
	// Given a working directory with a runtime already present (version.txt exists)
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	runtimeDir := filepath.Join(d, "dependencies", "runtime", "sniper")
	os.MkdirAll(runtimeDir, 0755)
	os.WriteFile(filepath.Join(runtimeDir, "version.txt"), []byte("current-build"), 0644)

	// And a config with CheckForUpdates=true pointing to an unreachable URL (port 0 = connection refused)
	globalCfg := config.Global{
		RuntimeVersions: map[string]config.VersionInfo{
			"sniper": {
				URL:             "http://127.0.0.1:0/SteamLinuxRuntime_sniper.tar.xz",
				CheckForUpdates: true,
			},
		},
	}
	appCfg := config.App{RuntimeVersion: "sniper"}

	// When EnsureRuntime is called
	err := EnsureRuntime(appCfg, globalCfg)

	// Then it returns an error — swallowing the version-check failure hides network problems from the caller
	if err == nil {
		t.Fatal("expected error when runtime version check fails due to network error, but got nil")
	}
}
