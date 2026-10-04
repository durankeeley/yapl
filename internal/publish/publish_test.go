package publish

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"yapl/internal/app"
	"yapl/internal/client"
	"yapl/internal/config"
	"yapl/internal/registry/api"
	"yapl/internal/registry/store"
)

type registryFixture struct {
	client *client.Client
	store  *store.Store
	mu     sync.Mutex
	puts   []string
}

func newRegistry(t *testing.T, allowPush bool) *registryFixture {
	t.Helper()
	f := &registryFixture{store: store.New(t.TempDir(), io.Discard)}
	f.store.Refresh()
	h := api.New(api.Config{Store: f.store, Auth: "open", AllowPush: allowPush})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			f.mu.Lock()
			f.puts = append(f.puts, r.URL.Path)
			f.mu.Unlock()
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	f.client = client.New(client.Config{Server: srv.URL, Auth: "open"})
	return f
}

// library creates a game using a locally installed fake Proton build.
func library(t *testing.T) *app.App {
	t.Helper()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	os.MkdirAll(filepath.Join("proton", "GE-Proton11-7", "files", "bin"), 0755)
	os.WriteFile(filepath.Join("proton", "GE-Proton11-7", "files", "bin", "wine64"), []byte("wine"), 0755)
	os.MkdirAll(filepath.Join("games", "Doom"), 0755)
	os.WriteFile(filepath.Join("games", "Doom", "doom.wad"), []byte("wad"), 0644)
	cfg := config.App{ProtonVersion: "GE-Proton11-7", LaunchMethod: "direct", Executable: "drive_c/doom.exe"}
	global := config.Global{ProtonVersions: map[string]config.VersionInfo{"GE-Proton11-7": {URL: "https://example/ge.tar.gz", BinPath: "files/bin"}}}
	return app.New("games", "Doom", false, false, false, global, cfg)
}

func TestPublish(t *testing.T) {
	t.Run("given a server without the game's proton when published then proton and the game are uploaded", func(t *testing.T) {
		// Given
		reg := newRegistry(t, true)
		a := library(t)
		var out bytes.Buffer

		// When
		res, err := Publish(a, reg.client, Options{Version: "1.0.0", Format: "gz"}, &out)

		// Then
		if err != nil {
			t.Fatalf("publish: %v\n%s", err, out.String())
		}
		if len(res.Uploaded) != 1 || res.Uploaded[0].String() != "proton/GE-Proton11-7" || len(res.Skipped) != 0 {
			t.Fatalf("result = %+v", res)
		}
		c, err := reg.store.ResolveComponent("proton", "GE-Proton11-7")
		if err != nil || c.Manifest.VersionInfo.BinPath != "files/bin" || c.Manifest.VersionInfo.URL != "" {
			t.Fatalf("component = %+v, %v", c, err)
		}
		if _, err := reg.store.Resolve("Doom", "1.0.0"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("given the server already has the component when a new version is published then only the game is uploaded", func(t *testing.T) {
		// Given
		reg := newRegistry(t, true)
		a := library(t)
		if _, err := Publish(a, reg.client, Options{Version: "1.0.0", Format: "gz"}, io.Discard); err != nil {
			t.Fatal(err)
		}
		reg.puts = nil

		// When
		res, err := Publish(a, reg.client, Options{Version: "1.1.0", Format: "gz"}, io.Discard)

		// Then
		if err != nil {
			t.Fatal(err)
		}
		if len(reg.puts) != 1 || !strings.HasPrefix(reg.puts[0], "/v1/packages/Doom/1.1.0") || len(res.Skipped) != 1 {
			t.Fatalf("puts = %v result = %+v", reg.puts, res)
		}
	})

	t.Run("given push is disabled when published then it fails before packaging", func(t *testing.T) {
		reg := newRegistry(t, false)
		a := library(t)
		_, err := Publish(a, reg.client, Options{Version: "1.0.0"}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "--allow-push") {
			t.Fatalf("error = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join("games", "Doom", "metadata.json")); !os.IsNotExist(statErr) {
			t.Fatal("game was packaged although push is disabled")
		}
	})

	t.Run("given a component that is not installed locally when published then it explains how to fix it", func(t *testing.T) {
		reg := newRegistry(t, true)
		a := library(t)
		os.RemoveAll(filepath.Join("proton", "GE-Proton11-7"))
		_, err := Publish(a, reg.client, Options{Version: "1.0.0", Format: "gz"}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "proton/GE-Proton11-7") || !strings.Contains(err.Error(), "yapl setup") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given a proton build at a custom path when published then that directory is packed", func(t *testing.T) {
		reg := newRegistry(t, true)
		a := library(t)
		custom := t.TempDir()
		os.Rename(filepath.Join("proton", "GE-Proton11-7", "files"), filepath.Join(custom, "files"))
		a.GlobalConfig.ProtonVersions["GE-Proton11-7"] = config.VersionInfo{Path: custom, BinPath: "files/bin"}
		if _, err := Publish(a, reg.client, Options{Version: "1.0.0", Format: "gz"}, io.Discard); err != nil {
			t.Fatal(err)
		}
		c, _ := reg.store.ResolveComponent("proton", "GE-Proton11-7")
		if c.Manifest.VersionInfo.Path != "" {
			t.Fatal("machine-specific path leaked into the registry")
		}
	})
}

func TestComponentDir(t *testing.T) {
	tests := []struct {
		name string
		info config.VersionInfo
		want string
	}{
		{"given a downloaded component then it lives in the library", config.VersionInfo{URL: "u"}, filepath.Join("proton", "ge")},
		{"given a component with a local path then that path is used", config.VersionInfo{Path: "/opt/ge"}, "/opt/ge"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := componentDir("proton", "ge", tc.info); got != tc.want {
				t.Fatalf("got %q", got)
			}
		})
	}
}
