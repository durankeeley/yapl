package dependency

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"yapl/internal/config"
)

// tarGz builds an archive whose entries all sit under one top-level
// directory, like real Proton, DXVK and runtime releases.
func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(content)), Typeflag: tar.TypeReg})
		tw.Write([]byte(content))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

type releaseServer struct {
	*httptest.Server
	mu    sync.Mutex
	hits  map[string]int
	files map[string][]byte
}

func newReleaseServer(t *testing.T, files map[string][]byte) *releaseServer {
	t.Helper()
	rs := &releaseServer{hits: map[string]int{}, files: files}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.hits[r.URL.Path]++
		data, ok := rs.files[r.URL.Path]
		rs.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *releaseServer) count(path string) int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.hits[path]
}

func (rs *releaseServer) set(path string, data []byte) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.files[path] = data
}

func inLibrary(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })
}

func TestEnsureAll(t *testing.T) {
	t.Run("given missing dxvk and vkd3d when ensured then both are downloaded without their top-level folder", func(t *testing.T) {
		// Given
		inLibrary(t)
		rs := newReleaseServer(t, map[string][]byte{
			"/dxvk.tar.gz":  tarGz(t, map[string]string{"dxvk-3.1.1/x64/d3d9.dll": "dxvk"}),
			"/vkd3d.tar.gz": tarGz(t, map[string]string{"vkd3d-proton-3.0.1/x64/d3d12.dll": "vkd3d"}),
		})
		global := config.Global{DependencyVersions: map[string]map[string]config.VersionInfo{
			"dxvk":  {"3.1.1": {URL: rs.URL + "/dxvk.tar.gz"}},
			"vkd3d": {"3.0.1": {URL: rs.URL + "/vkd3d.tar.gz"}},
		}}
		app := config.App{ProtonVersion: "system", Dependencies: config.AppDependencies{DXVKVersion: "3.1.1", VKD3DVersion: "3.0.1"}}

		// When
		err := EnsureAll(app, false, global)

		// Then
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(filepath.Join("dependencies", "dxvk", "3.1.1", "x64", "d3d9.dll")); string(got) != "dxvk" {
			t.Fatal("dxvk not installed")
		}
		if got, _ := os.ReadFile(filepath.Join("dependencies", "vkd3d", "3.0.1", "x64", "d3d12.dll")); string(got) != "vkd3d" {
			t.Fatal("vkd3d not installed")
		}

		// And a second call downloads nothing
		EnsureAll(app, false, global)
		if rs.count("/dxvk.tar.gz") != 1 {
			t.Fatalf("dxvk downloaded %d times", rs.count("/dxvk.tar.gz"))
		}
	})

	t.Run("given a dependency type runner.json does not define then it fails naming it", func(t *testing.T) {
		inLibrary(t)
		err := EnsureAll(config.App{ProtonVersion: "system", Dependencies: config.AppDependencies{DXVKVersion: "9.9"}}, false, config.Global{})
		if err == nil || !strings.Contains(err.Error(), "dxvk") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given umu with its own binary then umu-launcher is ensured too", func(t *testing.T) {
		inLibrary(t)
		rs := newReleaseServer(t, map[string][]byte{"/umu.tar.gz": tarGz(t, map[string]string{"umu/umu-run": "umu"})})
		global := config.Global{DependencyVersions: map[string]map[string]config.VersionInfo{"umu-launcher": {"1.2.9": {URL: rs.URL + "/umu.tar.gz"}}}}
		app := config.App{ProtonVersion: "system", LaunchMethod: "umu", UMUOptions: config.UMUOptions{Version: "1.2.9"}}
		if err := EnsureAll(app, false, global); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join("dependencies", "umu-launcher", "1.2.9", "umu-run")); err != nil {
			t.Fatal("umu-launcher not installed")
		}
	})

	t.Run("given a failing download then the error names the dependency", func(t *testing.T) {
		inLibrary(t)
		rs := newReleaseServer(t, map[string][]byte{})
		global := config.Global{DependencyVersions: map[string]map[string]config.VersionInfo{"dxvk": {"3.1.1": {URL: rs.URL + "/missing.tar.gz"}}}}
		err := EnsureAll(config.App{ProtonVersion: "system", Dependencies: config.AppDependencies{DXVKVersion: "3.1.1"}}, false, global)
		if err == nil || !strings.Contains(err.Error(), "dxvk") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestEnsureProtonDownload(t *testing.T) {
	t.Run("given a proton URL when ensured then it is downloaded, and forcing an upgrade replaces it", func(t *testing.T) {
		// Given
		inLibrary(t)
		rs := newReleaseServer(t, map[string][]byte{"/ge.tar.gz": tarGz(t, map[string]string{"GE-Proton11-7/proton": "v1"})})
		global := config.Global{ProtonVersions: map[string]config.VersionInfo{"GE-Proton11-7": {URL: rs.URL + "/ge.tar.gz"}}}
		app := config.App{ProtonVersion: "GE-Proton11-7"}

		// When
		if err := EnsureAll(app, false, global); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join("proton", "GE-Proton11-7", "stale"), []byte("x"), 0644)
		rs.set("/ge.tar.gz", tarGz(t, map[string]string{"GE-Proton11-7/proton": "v2"}))
		if err := EnsureAll(app, true, global); err != nil {
			t.Fatal(err)
		}

		// Then
		if got, _ := os.ReadFile(filepath.Join("proton", "GE-Proton11-7", "proton")); string(got) != "v2" {
			t.Fatalf("proton = %q", got)
		}
		if _, err := os.Stat(filepath.Join("proton", "GE-Proton11-7", "stale")); !os.IsNotExist(err) {
			t.Fatal("forced upgrade kept old files")
		}
	})

	t.Run("given a proton entry without URL or path then it explains what is missing", func(t *testing.T) {
		inLibrary(t)
		err := EnsureAll(config.App{ProtonVersion: "ge"}, false, config.Global{ProtonVersions: map[string]config.VersionInfo{"ge": {}}})
		if err == nil || !strings.Contains(err.Error(), "has no URL") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestEnsureRuntimeInstall(t *testing.T) {
	runtimeFiles := func(t *testing.T) map[string][]byte {
		return map[string][]byte{
			"/rt/SteamLinuxRuntime_sniper.tar.gz": tarGz(t, map[string]string{"SteamLinuxRuntime_sniper/_v2-entry-point": "#!/bin/sh"}),
			"/rt/BUILD_ID.txt":                    []byte("build-42\n"),
		}
	}
	app := config.App{RuntimeVersion: "sniper"}

	t.Run("given no runtime when ensured then it is installed with the yapl entry point, shim and version", func(t *testing.T) {
		// Given
		inLibrary(t)
		rs := newReleaseServer(t, runtimeFiles(t))
		global := config.Global{RuntimeVersions: map[string]config.VersionInfo{"sniper": {URL: rs.URL + "/rt/SteamLinuxRuntime_sniper.tar.gz", CheckForUpdates: true}}}

		// When
		err := EnsureRuntime(app, global)

		// Then
		dir := filepath.Join("dependencies", "runtime", "sniper")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "yapl-entry-point")); err != nil {
			t.Fatal("entry point not renamed")
		}
		if info, err := os.Stat(filepath.Join(dir, "yapl-shim")); err != nil || info.Mode()&0100 == 0 {
			t.Fatal("shim missing or not executable")
		}
		if v, _ := os.ReadFile(filepath.Join(dir, "version.txt")); strings.TrimSpace(string(v)) != "build-42" {
			t.Fatalf("version = %q", v)
		}

		// And an up-to-date runtime is not downloaded again
		EnsureRuntime(app, global)
		if n := rs.count("/rt/SteamLinuxRuntime_sniper.tar.gz"); n != 1 {
			t.Fatalf("runtime downloaded %d times", n)
		}

		// And a new build on the server triggers an update
		rs.set("/rt/BUILD_ID.txt", []byte("build-43\n"))
		if err := EnsureRuntime(app, global); err != nil {
			t.Fatal(err)
		}
		if n := rs.count("/rt/SteamLinuxRuntime_sniper.tar.gz"); n != 2 {
			t.Fatalf("runtime downloaded %d times after a new build", n)
		}
	})

	t.Run("given a runtime version runner.json does not define then it fails", func(t *testing.T) {
		inLibrary(t)
		if err := EnsureRuntime(app, config.Global{}); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("given a runtime without URL then it fails", func(t *testing.T) {
		inLibrary(t)
		if err := EnsureRuntime(app, config.Global{RuntimeVersions: map[string]config.VersionInfo{"sniper": {}}}); err == nil || !strings.Contains(err.Error(), "no URL") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given a broken runtime download then the partial install is removed", func(t *testing.T) {
		inLibrary(t)
		rs := newReleaseServer(t, map[string][]byte{"/rt/SteamLinuxRuntime_sniper.tar.gz": []byte("not a tarball")})
		global := config.Global{RuntimeVersions: map[string]config.VersionInfo{"sniper": {URL: rs.URL + "/rt/SteamLinuxRuntime_sniper.tar.gz"}}}
		if err := EnsureRuntime(app, global); err == nil {
			t.Fatal("expected an error")
		}
		if _, err := os.Stat(filepath.Join("dependencies", "runtime", "sniper")); !os.IsNotExist(err) {
			t.Fatal("partial runtime left behind")
		}
	})
}

func TestInstallCustomComponents_CopiesDLLs(t *testing.T) {
	t.Run("given custom dxvk for directx 9 and vkd3d when installed then their DLLs land next to the game", func(t *testing.T) {
		// Given
		inLibrary(t)
		for _, f := range []string{"dxvk/3.1.1/x64/d3d9.dll", "vkd3d/3.0.1/x64/d3d12.dll", "vkd3d/3.0.1/x64/d3d12core.dll"} {
			p := filepath.Join("dependencies", f)
			os.MkdirAll(filepath.Dir(p), 0755)
			os.WriteFile(p, []byte(filepath.Base(f)), 0644)
		}
		deps := config.AppDependencies{DXVKVersion: "3.1.1", DXVKDirectXVersion: "9", DXVKInstallPath: "Games/NFS", VKD3DVersion: "3.0.1", VKD3DInstallPath: "Games/NFS"}

		// When
		err := InstallCustomComponents("prefix", deps)

		// Then
		if err != nil {
			t.Fatal(err)
		}
		for _, dll := range []string{"d3d9.dll", "d3d12.dll", "d3d12core.dll"} {
			if _, err := os.Stat(filepath.Join("prefix", "drive_c", "Games", "NFS", dll)); err != nil {
				t.Errorf("%s not installed", dll)
			}
		}
	})
}
