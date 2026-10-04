package store

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/kernel"
	"yapl/internal/registry/api"
	registrystore "yapl/internal/registry/store"
)

func execute(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	k := kernel.New("yapl", &out, &errOut)
	if err := k.Register(Module{}); err != nil {
		t.Fatal(err)
	}
	return k.Execute(args), out.String(), errOut.String()
}

func TestPullList(t *testing.T) {
	t.Run("given a server with one package when pull list runs then the package is printed", func(t *testing.T) {
		// Given
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode([]map[string]any{{"name": "Doom", "type": "game", "format": "xz", "size_bytes": 5 << 20}})
		}))
		defer srv.Close()

		// When
		code, out, errOut := execute(t, "pull", "list", "--server", srv.URL)

		// Then
		if code != kernel.ExitOK || !strings.Contains(out, "Doom") || !strings.Contains(out, "5 MB") {
			t.Fatalf("exit %d out:\n%s\nerr:\n%s", code, out, errOut)
		}
	})

	t.Run("given an unreachable server when pull list runs then it fails", func(t *testing.T) {
		code, _, errOut := execute(t, "pull", "list", "--server", "127.0.0.1:1")
		if code != kernel.ExitError || !strings.Contains(errOut, "list packages") {
			t.Fatalf("exit %d err:\n%s", code, errOut)
		}
	})
}

func TestPush(t *testing.T) {
	t.Run("given a game and a push-enabled registry when push runs then the game is published", func(t *testing.T) {
		// Given
		dir := t.TempDir()
		orig, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(orig)
		os.MkdirAll(filepath.Join("games", "Doom"), 0755)
		os.WriteFile(filepath.Join("games", "Doom", "game.json"), []byte(`{"proton_version":"system","launch_method":"direct","executable":"drive_c/doom.exe"}`), 0644)
		os.WriteFile("runner.json", []byte(`{"proton_versions":{}}`), 0644)
		st := registrystore.New(t.TempDir(), io.Discard)
		st.Refresh()
		srv := httptest.NewServer(api.New(api.Config{Store: st, Auth: "open", AllowPush: true}))
		defer srv.Close()

		// When
		code, out, errOut := execute(t, "push", "Doom", "--version", "1.0.0", "--server", srv.URL)

		// Then
		if code != kernel.ExitOK {
			t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
		}
		if _, err := st.Resolve("Doom", "1.0.0"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "Published Doom:1.0.0") {
			t.Fatalf("output:\n%s", out)
		}
	})

	t.Run("given no name when push runs then it is a usage error", func(t *testing.T) {
		if code, _, _ := execute(t, "push"); code != kernel.ExitUsage {
			t.Fatalf("exit %d", code)
		}
	})
}
