package console

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	consolesvc "yapl/internal/console"
	"yapl/internal/kernel"
	"yapl/internal/registry/api"
	"yapl/internal/registry/store"
)

func inLibrary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })
	return dir
}

func TestStart(t *testing.T) {
	t.Run("given a server when the console starts then the UI and API are served on loopback", func(t *testing.T) {
		// Given
		inLibrary(t)
		registry := httptest.NewServer(api.New(api.Config{Store: store.New(t.TempDir(), io.Discard), Auth: api.AuthOpen}))
		defer registry.Close()
		var out bytes.Buffer

		// When
		inst, err := start(options{Port: 0, Server: registry.URL, Versions: kernel.Versions{Kernel: "1.0.0"}}, &out)
		if err != nil {
			t.Fatal(err)
		}
		defer inst.Close()

		// Then
		if !strings.HasPrefix(inst.URL, "http://127.0.0.1:") {
			t.Fatalf("url = %q", inst.URL)
		}
		page, err := http.Get(inst.URL)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(page.Body)
		page.Body.Close()
		if !strings.Contains(string(body), "YAPL Console") {
			t.Fatalf("page = %.200s", body)
		}
		resp, err := http.Get(inst.URL + "api/system")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var sys consolesvc.System
		json.NewDecoder(resp.Body).Decode(&sys)
		if sys.Server != registry.URL || !sys.ServerReachable || sys.Kernel != "1.0.0" {
			t.Fatalf("system = %+v", sys)
		}
	})

	t.Run("given no server can be found then the console starts offline and says so", func(t *testing.T) {
		inLibrary(t)
		var out bytes.Buffer
		inst, err := start(options{Port: 0, NoDiscovery: true}, &out)
		if err != nil {
			t.Fatal(err)
		}
		defer inst.Close()
		resp, err := http.Get(inst.URL + "api/store")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(out.String(), "offline") {
			t.Fatalf("status %d out %q", resp.StatusCode, out.String())
		}
	})

	t.Run("given the port is taken then start fails", func(t *testing.T) {
		inLibrary(t)
		first, err := start(options{Port: 0, NoDiscovery: true}, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		defer first.Close()
		if _, err := start(options{Port: first.Port, NoDiscovery: true}, io.Discard); err == nil {
			t.Fatal("expected a port conflict")
		}
	})
}

func TestModule(t *testing.T) {
	t.Run("given the module then it provides the console command with scoped flags", func(t *testing.T) {
		cmds := (Module{}).Commands()
		if len(cmds) != 1 || cmds[0].Verb != "console" {
			t.Fatalf("commands = %+v", cmds)
		}
	})
}
