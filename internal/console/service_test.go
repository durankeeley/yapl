package console

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"yapl/internal/kernel"
	"yapl/internal/manifest"
	"yapl/internal/progress"
	"yapl/internal/registry/api"
	"yapl/internal/registry/store"
)

// fakeRegistry stands in for a YAPL server: installing a game writes a
// minimal library entry so the library service sees it.
type fakeRegistry struct {
	mu       sync.Mutex
	catalog  []api.CatalogEntry
	failWith error
	release  chan struct{}
}

func (f *fakeRegistry) Server() string  { return "http://10.0.0.5:8471" }
func (f *fakeRegistry) Reachable() bool { return true }
func (f *fakeRegistry) Catalog() ([]api.CatalogEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.catalog, nil
}
func (f *fakeRegistry) Cover(name string) (io.ReadCloser, string, error) {
	if name != "Doom" {
		return nil, "", errors.New("no cover")
	}
	return io.NopCloser(strings.NewReader("remote-png")), "image/png", nil
}
func (f *fakeRegistry) Install(name string, report progress.Func) error {
	if f.release != nil {
		<-f.release
	}
	if f.failWith != nil {
		return f.failWith
	}
	report(progress.Update{Label: name, Done: 50, Total: 100})
	dir := filepath.Join("games", name)
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "game.json"), []byte(readyConfig), 0644)
	manifest.WriteInstalled(dir, manifest.Installed{Name: name, Type: "game", Version: "1.1.0"})
	return nil
}
func (f *fakeRegistry) Upgrade(name string, report progress.Func) error {
	manifest.WriteInstalled(filepath.Join("games", name), manifest.Installed{Name: name, Type: "game", Version: "2.0.0"})
	return nil
}

func catalogEntry(name, latest string) api.CatalogEntry {
	return api.CatalogEntry{Summary: store.Summary{Name: name, Type: "game", Title: name, Latest: latest, Versions: []string{latest}, HasCover: true}}
}

type harness struct {
	server   *httptest.Server
	registry *fakeRegistry
	service  *Service
}

func newHarness(t *testing.T, registry *fakeRegistry, launcher Launcher) harness {
	t.Helper()
	inLibrary(t)
	cfg := Config{
		Versions: kernel.Versions{Kernel: "1.0.0", Modules: map[string]string{"console": "1.0.0"}},
		Workdir:  "/srv/library",
		Launcher: launcher,
	}
	if registry != nil {
		cfg.Registry = registry
	}
	svc := New(cfg)
	srv := httptest.NewServer(svc.Handler())
	t.Cleanup(func() { srv.Close(); svc.Close() })
	return harness{server: srv, registry: registry, service: svc}
}

func (h harness) do(t *testing.T, method, path string, consoleHeader bool) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, h.server.URL+path, nil)
	if consoleHeader {
		req.Header.Set(ConsoleHeader, "1")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func waitJob(t *testing.T, h harness, id string, state JobState) Job {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var job Job
	for time.Now().Before(deadline) {
		decodeJSON(t, h.do(t, "GET", "/api/jobs/"+id, false), &job)
		if job.State == state {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s never reached %s: %+v", id, state, job)
	return job
}

func TestSystem(t *testing.T) {
	t.Run("given a console with a server then system reports versions, workdir and server", func(t *testing.T) {
		h := newHarness(t, &fakeRegistry{}, nil)
		var sys System
		decodeJSON(t, h.do(t, "GET", "/api/system", false), &sys)
		if sys.Kernel != "1.0.0" || sys.Modules["console"] != "1.0.0" || sys.Workdir != "/srv/library" || sys.Server != "http://10.0.0.5:8471" || !sys.ServerReachable || sys.Platform == "" {
			t.Fatalf("system = %+v", sys)
		}
	})
}

func TestLibraryAPI(t *testing.T) {
	t.Run("given an installed game then the library lists it and serves its cover", func(t *testing.T) {
		h := newHarness(t, nil, nil)
		dir := addGame(t, "games", "Doom", readyConfig)
		os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("jpg"), 0644)

		var games []Game
		decodeJSON(t, h.do(t, "GET", "/api/library", false), &games)
		cover := h.do(t, "GET", "/api/library/Doom/cover", false)
		body, _ := io.ReadAll(cover.Body)
		if len(games) != 1 || games[0].Name != "Doom" || string(body) != "jpg" || cover.Header.Get("Content-Type") != "image/jpeg" {
			t.Fatalf("games %+v cover %q %q", games, body, cover.Header.Get("Content-Type"))
		}
	})

	t.Run("given an unknown game then its detail is 404", func(t *testing.T) {
		h := newHarness(t, nil, nil)
		if resp := h.do(t, "GET", "/api/library/Quake", false); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given uninstall without confirmation then nothing is deleted", func(t *testing.T) {
		h := newHarness(t, nil, nil)
		addGame(t, "games", "Doom", readyConfig)
		if resp := h.do(t, "DELETE", "/api/library/Doom", true); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status %d", resp.StatusCode)
		}
		if _, err := os.Stat(filepath.Join("games", "Doom")); err != nil {
			t.Fatal("game deleted without confirmation")
		}
	})

	t.Run("given uninstall with confirmation then the game is deleted", func(t *testing.T) {
		h := newHarness(t, nil, nil)
		addGame(t, "games", "Doom", readyConfig)
		if resp := h.do(t, "DELETE", "/api/library/Doom?confirm=Doom", true); resp.StatusCode != http.StatusNoContent {
			t.Fatalf("status %d", resp.StatusCode)
		}
		if _, err := os.Stat(filepath.Join("games", "Doom")); !os.IsNotExist(err) {
			t.Fatal("game not deleted")
		}
	})
}

func TestBrowserSafety(t *testing.T) {
	t.Run("given a mutating request without the console header then it is forbidden", func(t *testing.T) {
		h := newHarness(t, nil, nil)
		addGame(t, "games", "Doom", readyConfig)
		if resp := h.do(t, "DELETE", "/api/library/Doom?confirm=Doom", false); resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given a request for a non-loopback host then it is rejected", func(t *testing.T) {
		h := newHarness(t, nil, nil)
		req, _ := http.NewRequest("GET", h.server.URL+"/api/library", nil)
		req.Host = "evil.example:8472"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given localhost as the host then it is accepted", func(t *testing.T) {
		h := newHarness(t, nil, nil)
		req, _ := http.NewRequest("GET", h.server.URL+"/api/library", nil)
		req.Host = "localhost:8472"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})
}

func TestStoreAPI(t *testing.T) {
	t.Run("given an installed older version then the store marks it upgradable", func(t *testing.T) {
		reg := &fakeRegistry{catalog: []api.CatalogEntry{catalogEntry("Doom", "1.1.0"), catalogEntry("Quake", "1.0.0")}}
		h := newHarness(t, reg, nil)
		dir := addGame(t, "games", "Doom", readyConfig)
		manifest.WriteInstalled(dir, manifest.Installed{Name: "Doom", Type: "game", Version: "1.0.0"})

		var items []StoreItem
		decodeJSON(t, h.do(t, "GET", "/api/store", false), &items)
		if len(items) != 2 || !items[0].Installed || !items[0].Upgradable || items[0].InstalledVersion != "1.0.0" || items[1].Installed {
			t.Fatalf("items = %+v", items)
		}
	})

	t.Run("given no server then the store is unavailable", func(t *testing.T) {
		h := newHarness(t, nil, nil)
		if resp := h.do(t, "GET", "/api/store", false); resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given install is requested then a job runs and the game appears in the library", func(t *testing.T) {
		// Given
		reg := &fakeRegistry{catalog: []api.CatalogEntry{catalogEntry("Doom", "1.1.0")}}
		h := newHarness(t, reg, nil)

		// When
		resp := h.do(t, "POST", "/api/store/Doom/install", true)
		var job Job
		decodeJSON(t, resp, &job)

		// Then
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("status %d", resp.StatusCode)
		}
		done := waitJob(t, h, job.ID, JobDone)
		if done.Kind != "install" || done.Done != 50 {
			t.Fatalf("job = %+v", done)
		}
		var games []Game
		decodeJSON(t, h.do(t, "GET", "/api/library", false), &games)
		if len(games) != 1 || games[0].Version != "1.1.0" {
			t.Fatalf("library = %+v", games)
		}
	})

	t.Run("given an install in progress then the store shows its job", func(t *testing.T) {
		reg := &fakeRegistry{catalog: []api.CatalogEntry{catalogEntry("Doom", "1.1.0")}, release: make(chan struct{})}
		h := newHarness(t, reg, nil)
		var job Job
		decodeJSON(t, h.do(t, "POST", "/api/store/Doom/install", true), &job)
		var items []StoreItem
		decodeJSON(t, h.do(t, "GET", "/api/store", false), &items)
		close(reg.release)
		if items[0].JobID != job.ID {
			t.Fatalf("items = %+v", items)
		}
		waitJob(t, h, job.ID, JobDone)
	})

	t.Run("given a failing install then the job reports the error", func(t *testing.T) {
		reg := &fakeRegistry{failWith: errors.New("checksum mismatch")}
		h := newHarness(t, reg, nil)
		var job Job
		decodeJSON(t, h.do(t, "POST", "/api/store/Doom/install", true), &job)
		if failed := waitJob(t, h, job.ID, JobFailed); failed.Error != "checksum mismatch" {
			t.Fatalf("job = %+v", failed)
		}
	})

	t.Run("given an already installed game then install conflicts", func(t *testing.T) {
		h := newHarness(t, &fakeRegistry{}, nil)
		addGame(t, "games", "Doom", readyConfig)
		if resp := h.do(t, "POST", "/api/store/Doom/install", true); resp.StatusCode != http.StatusConflict {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given an installed game then upgrade runs as a job", func(t *testing.T) {
		h := newHarness(t, &fakeRegistry{}, nil)
		addGame(t, "games", "Doom", readyConfig)
		var job Job
		decodeJSON(t, h.do(t, "POST", "/api/library/Doom/upgrade", true), &job)
		waitJob(t, h, job.ID, JobDone)
		if in, _ := manifest.ReadInstalled(filepath.Join("games", "Doom")); in.Version != "2.0.0" {
			t.Fatalf("installed = %+v", in)
		}
	})

	t.Run("given a remote cover then the store proxies it", func(t *testing.T) {
		h := newHarness(t, &fakeRegistry{}, nil)
		resp := h.do(t, "GET", "/api/store/Doom/cover", false)
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "remote-png" || resp.Header.Get("Content-Type") != "image/png" {
			t.Fatalf("body %q type %q", body, resp.Header.Get("Content-Type"))
		}
		if resp := h.do(t, "GET", "/api/store/Quake/cover", false); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("missing cover status %d", resp.StatusCode)
		}
	})

	t.Run("given an unknown job then 404", func(t *testing.T) {
		h := newHarness(t, &fakeRegistry{}, nil)
		if resp := h.do(t, "GET", "/api/jobs/job-999", false); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})
}

func TestSessionAPI(t *testing.T) {
	t.Run("given a ready game when launched then the session shows it and stop ends it", func(t *testing.T) {
		// Given
		h := newHarness(t, nil, scriptLauncher(`sleep 30`))
		addGame(t, "games", "Doom", readyConfig)

		// When
		launch := h.do(t, "POST", "/api/library/Doom/launch", true)
		var st SessionStatus
		decodeJSON(t, h.do(t, "GET", "/api/session", false), &st)

		// Then
		if launch.StatusCode != http.StatusAccepted || !st.Running || st.Name != "Doom" {
			t.Fatalf("launch %d status %+v", launch.StatusCode, st)
		}
		if _, err := os.Stat(filepath.Join("games", "Doom", LogFile)); err != nil {
			t.Fatal("no log file")
		}
		if again := h.do(t, "POST", "/api/library/Doom/launch", true); again.StatusCode != http.StatusConflict {
			t.Fatalf("second launch status %d", again.StatusCode)
		}
		if stop := h.do(t, "POST", "/api/session/stop", true); stop.StatusCode != http.StatusNoContent {
			t.Fatalf("stop status %d", stop.StatusCode)
		}
		decodeJSON(t, h.do(t, "GET", "/api/session", false), &st)
		if st.Running {
			t.Fatalf("still running: %+v", st)
		}
	})

	t.Run("given a game that is not ready when launched then it is refused", func(t *testing.T) {
		h := newHarness(t, nil, scriptLauncher(`true`))
		addGame(t, "games", "Doom", `{"proton_version":"ge","launch_method":"direct","executable":"drive_c/windows/explorer.exe"}`)
		if resp := h.do(t, "POST", "/api/library/Doom/launch", true); resp.StatusCode != http.StatusConflict {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given nothing running when stop is requested then 409", func(t *testing.T) {
		h := newHarness(t, nil, scriptLauncher(`true`))
		if resp := h.do(t, "POST", "/api/session/stop", true); resp.StatusCode != http.StatusConflict {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})
}
