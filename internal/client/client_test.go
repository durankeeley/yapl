package client_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yapl/internal/client"
)

type fakePackageEntry struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Format    string `json:"format"`
	SizeBytes int64  `json:"size_bytes"`
}

func newFakeServer(t *testing.T, games, apps map[string][]byte) *httptest.Server {
	t.Helper()
	packages := make(map[string]fakePackageEntry)
	content := make(map[string][]byte)

	addEntries := func(entryType string, files map[string][]byte) {
		for fname, data := range files {
			parts := strings.SplitN(fname, ".", 2)
			format := "xz"
			if len(parts) == 2 {
				switch parts[1] {
				case "tar.gz":
					format = "gz"
				case "tar.zst":
					format = "zst"
				}
			}
			name := parts[0]
			packages[name] = fakePackageEntry{Name: name, Type: entryType, Format: format, SizeBytes: int64(len(data))}
			content[name] = data
		}
	}
	addEntries("game", games)
	addEntries("app", apps)

	mux := http.NewServeMux()
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		var entries []fakePackageEntry
		for _, e := range packages {
			entries = append(entries, e)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	})
	mux.HandleFunc("/packages/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/packages/")
		data, ok := content[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(data)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestClient_ListReturnsAvailablePackagesWithType(t *testing.T) {
	// Given a server advertising a game and an app
	ts := newFakeServer(t,
		map[string][]byte{"Doom.tar.xz": []byte("xz")},
		map[string][]byte{"SteamCMD.tar.gz": []byte("gz")},
	)

	// When List is called
	c := client.New(client.Config{Server: ts.URL, Auth: "open"})
	entries, err := c.List()

	// Then both entries have correct names and types
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byName := make(map[string]client.PackageEntry)
	for _, e := range entries {
		byName[e.Name] = e
	}
	if byName["Doom"].Type != "game" {
		t.Errorf("expected Doom type=game, got %q", byName["Doom"].Type)
	}
	if byName["SteamCMD"].Type != "app" {
		t.Errorf("expected SteamCMD type=app, got %q", byName["SteamCMD"].Type)
	}
}

func TestClient_DownloadWritesFileToOutputDir(t *testing.T) {
	// Given a server with a game containing known bytes
	content := []byte("fake archive content 1234")
	ts := newFakeServer(t, map[string][]byte{"Doom.tar.xz": content}, nil)

	// When Download is called
	outDir := t.TempDir()
	c := client.New(client.Config{Server: ts.URL, Auth: "open"})
	archivePath, entryType, err := c.Download("Doom", outDir, io.Discard)

	// Then the file is written and type is "game"
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entryType != "game" {
		t.Errorf("expected entryType 'game', got %q", entryType)
	}
	got, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content mismatch: got %q, want %q", got, content)
	}
}

func TestClient_DownloadReturnsAppTypeForApps(t *testing.T) {
	// Given a server with an app
	ts := newFakeServer(t, nil, map[string][]byte{"SteamCMD.tar.xz": []byte("data")})

	// When Download is called
	c := client.New(client.Config{Server: ts.URL, Auth: "open"})
	_, entryType, err := c.Download("SteamCMD", t.TempDir(), io.Discard)

	// Then type is "app"
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entryType != "app" {
		t.Errorf("expected entryType 'app', got %q", entryType)
	}
}

func TestClient_DownloadReturnsErrorFor404(t *testing.T) {
	// Given a server with no packages
	ts := newFakeServer(t, nil, nil)

	c := client.New(client.Config{Server: ts.URL, Auth: "open"})
	_, _, err := c.Download("Ghost", t.TempDir(), io.Discard)

	if err == nil {
		t.Fatal("expected error for package not found")
	}
	if !strings.Contains(err.Error(), "Ghost") {
		t.Errorf("expected error to mention package name, got: %v", err)
	}
}

func TestClient_DownloadDeletesPartialFileOnError(t *testing.T) {
	// Given a server that aborts mid-stream
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/packages" {
			entries := []fakePackageEntry{{Name: "Doom", Type: "game", Format: "xz", SizeBytes: 1000}}
			json.NewEncoder(w).Encode(entries)
			return
		}
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial"))
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(ts.Close)

	outDir := t.TempDir()
	c := client.New(client.Config{Server: ts.URL, Auth: "open"})
	_, _, err := c.Download("Doom", outDir, io.Discard)

	if err == nil {
		t.Fatal("expected error for aborted download")
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Errorf("expected outDir to be empty after failed download, found %d files", len(entries))
	}
}

func TestClient_PasswordAuthSentCorrectly(t *testing.T) {
	// Given a server that validates basic auth
	var receivedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		_, pw, _ := r.BasicAuth()
		if pw != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode([]fakePackageEntry{})
	}))
	t.Cleanup(ts.Close)

	c := client.New(client.Config{Server: ts.URL, Auth: "password", Password: "secret"})
	_, err := c.List()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(receivedAuth, "Basic ") {
		t.Errorf("expected Basic auth header, got: %q", receivedAuth)
	}
}

func TestClient_ListReturnsErrorOnServerFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	c := client.New(client.Config{Server: ts.URL, Auth: "open"})
	_, err := c.List()

	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestClient_DownloadFilenameMatchesPackageName(t *testing.T) {
	ts := newFakeServer(t, map[string][]byte{"Doom.tar.xz": []byte("content")}, nil)

	outDir := t.TempDir()
	c := client.New(client.Config{Server: ts.URL, Auth: "open"})
	archivePath, _, err := c.Download("Doom", outDir, io.Discard)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	base := filepath.Base(archivePath)
	if !strings.HasPrefix(base, "Doom") {
		t.Errorf("expected filename to start with Doom, got %q", base)
	}
}

func TestClient_DiscoverParsesYAPLBroadcastMessage(t *testing.T) {
	// Given a UDP listener simulating the client's discovery socket
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Skip("UDP not available:", err)
	}
	listenPort := conn.LocalAddr().(*net.UDPAddr).Port

	// When a YAPL broadcast is sent to that listener before Discover is called
	go func() {
		time.Sleep(50 * time.Millisecond)
		sender, err := net.Dial("udp4", fmt.Sprintf("127.0.0.1:%d", listenPort))
		if err != nil {
			return
		}
		defer sender.Close()
		sender.Write([]byte("YAPL:8471"))
	}()

	// Then DiscoverFrom correctly parses the message and returns the sender's address with the announced port
	addr, err := client.DiscoverFrom(conn, 2*time.Second)
	if err != nil {
		t.Fatalf("DiscoverFrom returned error: %v", err)
	}
	if !strings.HasSuffix(addr, ":8471") {
		t.Errorf("expected address to end with :8471, got %q", addr)
	}
}

func TestClient_DiscoverTimesOutWhenNoServerPresent(t *testing.T) {
	// Given a UDP listener with no broadcaster
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Skip("UDP not available:", err)
	}

	// When DiscoverFrom is called with a short timeout
	_, err = client.DiscoverFrom(conn, 100*time.Millisecond)

	// Then it returns an error
	if err == nil {
		t.Fatal("expected timeout error when no server is broadcasting")
	}
}
