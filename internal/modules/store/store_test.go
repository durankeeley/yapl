package store

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yapl/internal/kernel"
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
