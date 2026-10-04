package server_test

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yapl/internal/registry/api"
	"yapl/internal/server"
)

func packagesDir(t *testing.T, games map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "games"), 0755)
	for file, content := range games {
		os.WriteFile(filepath.Join(dir, "games", file), []byte(content), 0644)
	}
	return dir
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     server.Config
		wantErr string
	}{
		{"given an unknown auth mode then it is rejected", server.Config{Auth: "magic"}, "unknown auth mode"},
		{"given password auth without a password then it is rejected", server.Config{Auth: "password"}, "needs --password"},
		{"given open auth then it is accepted", server.Config{Auth: "open"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.PackagesDir = t.TempDir()
			tc.cfg.Warnings = io.Discard
			_, err := server.New(tc.cfg)
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestServerComposition(t *testing.T) {
	t.Run("given a drop-in archive when the server starts then the v1 catalog and legacy endpoints both serve it", func(t *testing.T) {
		// Given
		s, err := server.New(server.Config{PackagesDir: packagesDir(t, map[string]string{"Doom.tar.xz": "doom"}), Auth: "open", Kernel: "1.0.0", Warnings: io.Discard})
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(s.Handler())
		defer srv.Close()

		// When
		v1, err := http.Get(srv.URL + "/v1/packages")
		if err != nil {
			t.Fatal(err)
		}
		legacy, err := http.Get(srv.URL + "/packages/Doom")
		if err != nil {
			t.Fatal(err)
		}
		defer v1.Body.Close()
		defer legacy.Body.Close()

		// Then
		var cat []api.CatalogEntry
		json.NewDecoder(v1.Body).Decode(&cat)
		body, _ := io.ReadAll(legacy.Body)
		if len(cat) != 1 || cat[0].Name != "Doom" || string(body) != "doom" {
			t.Fatalf("catalog %+v legacy body %q", cat, body)
		}
	})

	t.Run("given the server's versions when info is requested then they are advertised", func(t *testing.T) {
		s, _ := server.New(server.Config{PackagesDir: t.TempDir(), Auth: "open", Kernel: "1.2.3", Modules: map[string]string{"registry": "2.0.0"}, Warnings: io.Discard})
		srv := httptest.NewServer(s.Handler())
		defer srv.Close()
		resp, err := http.Get(srv.URL + "/v1/info")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var info api.Info
		json.NewDecoder(resp.Body).Decode(&info)
		if info.Kernel != "1.2.3" || info.Modules["registry"] != "2.0.0" || info.APIVersion != api.APIVersion {
			t.Fatalf("info = %+v", info)
		}
	})

	t.Run("given a file copied in after start when the store refreshes then it is served", func(t *testing.T) {
		dir := packagesDir(t, nil)
		s, _ := server.New(server.Config{PackagesDir: dir, Auth: "open", Warnings: io.Discard})
		os.WriteFile(filepath.Join(dir, "games", "Quake.tar.gz"), []byte("q"), 0644)
		s.Store().Refresh()
		if len(s.Store().Catalog()) != 1 {
			t.Fatalf("catalog = %+v", s.Store().Catalog())
		}
	})
}

func TestBroadcastTo(t *testing.T) {
	t.Run("given a server on port 8471 when it broadcasts then the datagram is YAPL:8471", func(t *testing.T) {
		// Given
		s, err := server.New(server.Config{PackagesDir: t.TempDir(), Auth: "open", Port: 8471, Warnings: io.Discard})
		if err != nil {
			t.Fatal(err)
		}
		conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Skip("UDP not available:", err)
		}
		defer conn.Close()

		// When
		if err := s.BroadcastTo(conn.LocalAddr().(*net.UDPAddr)); err != nil {
			t.Fatal(err)
		}

		// Then
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 64)
		n, _, err := conn.ReadFrom(buf)
		if err != nil || string(buf[:n]) != "YAPL:8471" {
			t.Fatalf("got %q, %v", buf[:n], err)
		}
	})
}
