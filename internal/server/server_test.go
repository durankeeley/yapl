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

	"yapl/internal/server"
)

// makePackagesDir creates the directory layout the server expects:
//
//	<root>/games/<archives>
//	<root>/apps/<archives>
func makePackagesDir(t *testing.T, games, apps map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range games {
		sub := filepath.Join(dir, "games")
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, name), content, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range apps {
		sub := filepath.Join(dir, "apps")
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, name), content, 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func newTestServer(t *testing.T, cfg server.Config) *httptest.Server {
	t.Helper()
	s, err := server.New(cfg)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestServer_ManifestScanFindsGamesAndApps(t *testing.T) {
	// Given a packages dir with games and apps in separate subdirectories
	dir := makePackagesDir(t,
		map[string][]byte{"Doom.tar.xz": []byte("x"), "Quake.tar.gz": []byte("y")},
		map[string][]byte{"SteamCMD.tar.xz": []byte("z")},
	)

	// When the server builds its manifest
	s, err := server.New(server.Config{PackagesDir: dir, Auth: "open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries := s.Manifest()

	// Then all archives appear with correct types
	if e, ok := entries["Doom"]; !ok || e.Type != "game" {
		t.Errorf("expected Doom with type game, got %+v", entries["Doom"])
	}
	if e, ok := entries["Quake"]; !ok || e.Type != "game" {
		t.Errorf("expected Quake with type game, got %+v", entries["Quake"])
	}
	if e, ok := entries["SteamCMD"]; !ok || e.Type != "app" {
		t.Errorf("expected SteamCMD with type app, got %+v", entries["SteamCMD"])
	}
}

func TestServer_ManifestIgnoresNonArchiveFiles(t *testing.T) {
	// Given a games dir that also contains a non-archive file
	dir := makePackagesDir(t,
		map[string][]byte{"Doom.tar.xz": []byte("x"), "readme.txt": []byte("ignored")},
		nil,
	)

	s, err := server.New(server.Config{PackagesDir: dir, Auth: "open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries := s.Manifest()

	if _, ok := entries["readme"]; ok {
		t.Error("readme.txt should not be in manifest")
	}
	if _, ok := entries["Doom"]; !ok {
		t.Error("expected Doom in manifest")
	}
}

func TestServer_ListEndpointReturnsTypeField(t *testing.T) {
	// Given a server with a game and an app
	dir := makePackagesDir(t,
		map[string][]byte{"Doom.tar.xz": []byte("x")},
		map[string][]byte{"SteamCMD.tar.gz": []byte("y")},
	)
	ts := newTestServer(t, server.Config{PackagesDir: dir, Auth: "open"})

	// When GET /packages is called
	resp, err := http.Get(ts.URL + "/packages")
	if err != nil {
		t.Fatalf("GET /packages: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var entries []server.PackageEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}

	// Then each entry carries the correct type
	byName := make(map[string]server.PackageEntry)
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

func TestServer_ListEndpointIncludesSizeBytes(t *testing.T) {
	// Given a game archive with known content
	content := []byte("hello world archive")
	dir := makePackagesDir(t, map[string][]byte{"Doom.tar.xz": content}, nil)
	ts := newTestServer(t, server.Config{PackagesDir: dir, Auth: "open"})

	resp, err := http.Get(ts.URL + "/packages")
	if err != nil {
		t.Fatalf("GET /packages: %v", err)
	}
	defer resp.Body.Close()

	var entries []server.PackageEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].SizeBytes != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), entries[0].SizeBytes)
	}
}

func TestServer_DownloadEndpointStreamsCorrectContent(t *testing.T) {
	// Given a game archive with known content
	content := []byte("fake archive bytes")
	dir := makePackagesDir(t, map[string][]byte{"Doom.tar.xz": content}, nil)
	ts := newTestServer(t, server.Config{PackagesDir: dir, Auth: "open"})

	resp, err := http.Get(ts.URL + "/packages/Doom")
	if err != nil {
		t.Fatalf("GET /packages/Doom: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != string(content) {
		t.Errorf("body mismatch: got %q, want %q", body, content)
	}
}

func TestServer_DownloadEndpoint404ForUnknownPackage(t *testing.T) {
	dir := makePackagesDir(t, nil, nil)
	ts := newTestServer(t, server.Config{PackagesDir: dir, Auth: "open"})

	resp, err := http.Get(ts.URL + "/packages/Ghost")
	if err != nil {
		t.Fatalf("GET /packages/Ghost: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestServer_OpenAuthAllowsUnauthenticatedRequests(t *testing.T) {
	dir := makePackagesDir(t, map[string][]byte{"Doom.tar.xz": []byte("x")}, nil)
	ts := newTestServer(t, server.Config{PackagesDir: dir, Auth: "open"})

	resp, err := http.Get(ts.URL + "/packages")
	if err != nil {
		t.Fatalf("GET /packages: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for open auth, got %d", resp.StatusCode)
	}
}

func TestServer_PasswordAuthRejectsWrongPassword(t *testing.T) {
	dir := makePackagesDir(t, map[string][]byte{"Doom.tar.xz": []byte("x")}, nil)
	ts := newTestServer(t, server.Config{PackagesDir: dir, Auth: "password", Password: "secret"})

	req, _ := http.NewRequest("GET", ts.URL+"/packages", nil)
	req.SetBasicAuth("user", "wrong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestServer_PasswordAuthAllowsCorrectPassword(t *testing.T) {
	dir := makePackagesDir(t, map[string][]byte{"Doom.tar.xz": []byte("x")}, nil)
	ts := newTestServer(t, server.Config{PackagesDir: dir, Auth: "password", Password: "secret"})

	req, _ := http.NewRequest("GET", ts.URL+"/packages", nil)
	req.SetBasicAuth("user", "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServer_NewReturnsErrorForInvalidAuth(t *testing.T) {
	dir := makePackagesDir(t, nil, nil)

	_, err := server.New(server.Config{PackagesDir: dir, Auth: "token"})

	if err == nil {
		t.Fatal("expected error for unknown auth mode 'token'")
	}
	if !strings.Contains(err.Error(), "auth") {
		t.Errorf("expected error to mention 'auth', got: %v", err)
	}
}

func TestServer_ManifestRefreshPicksUpNewFiles(t *testing.T) {
	// Given a server with one game
	dir := makePackagesDir(t, map[string][]byte{"Doom.tar.xz": []byte("x")}, nil)
	s, err := server.New(server.Config{PackagesDir: dir, Auth: "open"})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	// When a new game is added and the manifest is refreshed
	if err := os.WriteFile(filepath.Join(dir, "games", "Quake.tar.gz"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	s.RefreshManifest()

	entries := s.Manifest()
	if _, ok := entries["Doom"]; !ok {
		t.Error("expected Doom in manifest after refresh")
	}
	if _, ok := entries["Quake"]; !ok {
		t.Error("expected Quake in manifest after refresh")
	}
}

func TestServer_BroadcastMessageFormat(t *testing.T) {
	// Given a server configured on port 8471
	dir := makePackagesDir(t, nil, nil)
	s, err := server.New(server.Config{PackagesDir: dir, Auth: "open", Port: 8471})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	// When a discovery listener is set up and the server broadcasts to it
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Skip("UDP not available:", err)
	}
	defer conn.Close()
	listenPort := conn.LocalAddr().(*net.UDPAddr).Port

	// Have the server send one broadcast to our test listener
	done := make(chan string, 1)
	go func() {
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 256)
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			done <- ""
			return
		}
		done <- string(buf[:n])
	}()

	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: listenPort}
	if err := s.BroadcastTo(addr); err != nil {
		t.Fatalf("BroadcastTo: %v", err)
	}

	msg := <-done
	// Then the message is "YAPL:8471"
	if msg != "YAPL:8471" {
		t.Errorf("expected broadcast message 'YAPL:8471', got %q", msg)
	}
}
