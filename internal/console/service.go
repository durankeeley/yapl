package console

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"yapl/internal/kernel"
	"yapl/internal/paths"
	"yapl/internal/progress"
	"yapl/internal/registry/api"
	"yapl/internal/version"
)

// ConsoleHeader must accompany every mutating request. A cross-origin page
// cannot add it without a CORS preflight, which this service never grants.
const ConsoleHeader = "X-Yapl-Console"

// LogFile is written into the game directory on each launch.
const LogFile = "last-run.log"

const (
	jobInstall = "install"
	jobUpgrade = "upgrade"
	confirmKey = "confirm"
)

var loopbackHosts = map[string]bool{"127.0.0.1": true, "localhost": true, "::1": true}

// Config wires the console service.
type Config struct {
	Versions     kernel.Versions
	Workdir      string
	Registry     Registry
	Launcher     Launcher
	JobRetention time.Duration
	// UI, when set, serves everything outside /api/.
	UI http.Handler
}

// System is what /api/system returns.
type System struct {
	Kernel          string            `json:"kernel"`
	Modules         map[string]string `json:"modules"`
	Platform        string            `json:"platform"`
	Workdir         string            `json:"workdir"`
	Server          string            `json:"server,omitempty"`
	ServerReachable bool              `json:"server_reachable"`
}

// StoreItem is a catalog entry annotated with local state.
type StoreItem struct {
	api.CatalogEntry
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version,omitempty"`
	Upgradable       bool   `json:"upgradable"`
	JobID            string `json:"job_id,omitempty"`
}

// Service is the console backend.
type Service struct {
	cfg      Config
	jobs     *Jobs
	sessions *Sessions
	handler  http.Handler
}

// New builds the service and starts its job worker.
func New(cfg Config) *Service {
	if cfg.JobRetention == 0 {
		cfg.JobRetention = DefaultJobRetention
	}
	s := &Service{cfg: cfg, jobs: NewJobs(cfg.JobRetention), sessions: NewSessions(cfg.Launcher)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/system", s.system)
	mux.HandleFunc("GET /api/library", s.library)
	mux.HandleFunc("GET /api/library/{name}", s.libraryGame)
	mux.HandleFunc("GET /api/library/{name}/cover", s.libraryCover)
	mux.HandleFunc("POST /api/library/{name}/launch", s.launch)
	mux.HandleFunc("POST /api/library/{name}/upgrade", s.upgrade)
	mux.HandleFunc("DELETE /api/library/{name}", s.uninstall)
	mux.HandleFunc("GET /api/store", s.store)
	mux.HandleFunc("GET /api/store/{name}/cover", s.storeCover)
	mux.HandleFunc("POST /api/store/{name}/install", s.install)
	mux.HandleFunc("GET /api/jobs", s.listJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.getJob)
	mux.HandleFunc("GET /api/session", s.session)
	mux.HandleFunc("POST /api/session/stop", s.stop)
	if cfg.UI != nil {
		mux.Handle("/", cfg.UI)
	}
	s.handler = guard(mux)
	return s
}

// Handler is the HTTP entry point.
func (s *Service) Handler() http.Handler { return s.handler }

// Close stops the job worker after the current job.
func (s *Service) Close() { s.jobs.Close() }

// guard rejects non-loopback Host headers (DNS rebinding) and mutating
// requests without ConsoleHeader (cross-site requests).
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if !loopbackHosts[host] {
			writeError(w, http.StatusForbidden, errors.New("the console only answers on loopback addresses"))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(ConsoleHeader) == "" {
			writeError(w, http.StatusForbidden, errors.New("missing "+ConsoleHeader+" header"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) system(w http.ResponseWriter, r *http.Request) {
	sys := System{Kernel: s.cfg.Versions.Kernel, Modules: s.cfg.Versions.Modules, Platform: api.Platform(), Workdir: s.cfg.Workdir}
	if s.cfg.Registry != nil {
		sys.Server = s.cfg.Registry.Server()
		sys.ServerReachable = s.cfg.Registry.Reachable()
	}
	writeJSON(w, http.StatusOK, sys)
}

func (s *Service) library(w http.ResponseWriter, r *http.Request) {
	games, err := ListLibrary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if games == nil {
		games = []Game{}
	}
	writeJSON(w, http.StatusOK, games)
}

func (s *Service) libraryGame(w http.ResponseWriter, r *http.Request) {
	g, _, err := FindGame(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Service) libraryCover(w http.ResponseWriter, r *http.Request) {
	_, cover, err := FindGame(r.PathValue("name"))
	if err != nil || cover == "" {
		writeError(w, http.StatusNotFound, errors.New("no cover"))
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(cover)))
	http.ServeFile(w, r, cover)
}

func (s *Service) launch(w http.ResponseWriter, r *http.Request) {
	g, _, err := FindGame(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if !g.Ready {
		writeError(w, http.StatusConflict, errors.New(g.Name+" has no executable set yet; install it first"))
		return
	}
	if s.cfg.Launcher == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("launching is not available"))
		return
	}
	logPath := filepath.Join(paths.AppDir(paths.TypeDir(g.Type), g.Name), LogFile)
	if err := s.sessions.Launch(g.Name, logPath); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrSessionActive) {
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusAccepted, s.sessions.Status())
}

func (s *Service) uninstall(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if r.URL.Query().Get(confirmKey) != name {
		writeError(w, http.StatusBadRequest, errors.New("add ?confirm=<name> to uninstall"))
		return
	}
	if st := s.sessions.Status(); st.Running && st.Name == name {
		writeError(w, http.StatusConflict, errors.New(name+" is running"))
		return
	}
	if err := Uninstall(name); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) store(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Registry == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("no YAPL server configured"))
		return
	}
	catalog, err := s.cfg.Registry.Catalog()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	active := map[string]string{}
	for _, j := range s.jobs.List() {
		if j.State == JobQueued || j.State == JobRunning {
			active[j.Name] = j.ID
		}
	}
	items := make([]StoreItem, 0, len(catalog))
	for _, e := range catalog {
		item := StoreItem{CatalogEntry: e, JobID: active[e.Name]}
		if g, _, err := FindGame(e.Name); err == nil {
			item.Installed, item.InstalledVersion = true, g.Version
			item.Upgradable = version.Compare(e.Latest, g.Version) > 0
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Service) storeCover(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Registry == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("no YAPL server configured"))
		return
	}
	body, contentType, err := s.cfg.Registry.Cover(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", contentType)
	io.Copy(w, body)
}

func (s *Service) install(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, _, err := FindGame(name); err == nil {
		writeError(w, http.StatusConflict, errors.New(name+" is already installed"))
		return
	}
	s.submit(w, jobInstall, name, func(reg Registry, report progress.Func) error { return reg.Install(name, report) })
}

func (s *Service) upgrade(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, _, err := FindGame(name); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	s.submit(w, jobUpgrade, name, func(reg Registry, report progress.Func) error { return reg.Upgrade(name, report) })
}

func (s *Service) submit(w http.ResponseWriter, kind, name string, run func(Registry, progress.Func) error) {
	reg := s.cfg.Registry
	if reg == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("no YAPL server configured"))
		return
	}
	job, err := s.jobs.Submit(kind, name, func(report progress.Func) error { return run(reg, report) })
	switch {
	case errors.Is(err, ErrJobActive):
		writeError(w, http.StatusConflict, err)
	case err != nil:
		writeError(w, http.StatusServiceUnavailable, err)
	default:
		writeJSON(w, http.StatusAccepted, job)
	}
}

func (s *Service) listJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.jobs.List())
}

func (s *Service) getJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.jobs.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("no such job"))
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Service) session(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.sessions.Status())
}

func (s *Service) stop(w http.ResponseWriter, r *http.Request) {
	if err := s.sessions.Stop(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
