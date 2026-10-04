// Package api serves a registry store over HTTP: the versioned v1 API used
// by current clients and the console, plus the original /packages endpoints
// for older YAPL builds.
package api

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"yapl/internal/manifest"
	"yapl/internal/registry/store"
)

// APIVersion is the major version of the HTTP API. Clients refuse servers
// with a different one.
const APIVersion = 1

// SHA256Header carries an archive's expected checksum on downloads.
const SHA256Header = "X-Yapl-SHA256"

// Auth modes.
const (
	AuthOpen     = "open"
	AuthPassword = "password"
)

const authRealm = `Basic realm="yapl"`

// Info is served at /v1/info without authentication, so clients can learn
// the API version and auth mode before asking for credentials.
type Info struct {
	APIVersion  int               `json:"api_version"`
	Kernel      string            `json:"kernel"`
	Modules     map[string]string `json:"modules"`
	Auth        string            `json:"auth"`
	PushEnabled bool              `json:"push_enabled"`
}

// CatalogEntry is a store summary plus the URL of its cover, if any.
type CatalogEntry struct {
	store.Summary
	CoverURL string `json:"cover_url,omitempty"`
}

// PackageDetail lists every version of one package, newest first.
type PackageDetail struct {
	Name     string              `json:"name"`
	Type     string              `json:"type"`
	Versions []manifest.Manifest `json:"versions"`
}

// legacyEntry is the pre-v1 /packages row.
type legacyEntry struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Format    string `json:"format"`
	SizeBytes int64  `json:"size_bytes"`
}

// Config wires a store to the HTTP layer.
type Config struct {
	Store    *store.Store
	Info     Info
	Auth     string
	Password string
}

type handler struct {
	cfg Config
}

// New returns the registry's HTTP handler.
func New(cfg Config) http.Handler {
	cfg.Info.APIVersion = APIVersion
	cfg.Info.Auth = cfg.Auth
	h := &handler{cfg: cfg}

	protected := http.NewServeMux()
	protected.HandleFunc("GET /v1/packages", h.catalog)
	protected.HandleFunc("GET /v1/packages/{name}", h.detail)
	protected.HandleFunc("GET /v1/packages/{name}/{version}/manifest", h.manifest)
	protected.HandleFunc("GET /v1/packages/{name}/{version}/archive", h.archive)
	protected.HandleFunc("GET /v1/packages/{name}/{version}/cover", h.cover)
	protected.HandleFunc("GET /packages", h.legacyList)
	protected.HandleFunc("GET /packages/{name}", h.legacyDownload)

	root := http.NewServeMux()
	root.HandleFunc("GET /v1/info", h.info)
	root.Handle("/", h.requireAuth(protected))
	return root
}

func (h *handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.cfg.Auth == AuthPassword {
			_, pw, ok := r.BasicAuth()
			if !ok || pw != h.cfg.Password {
				w.Header().Set("WWW-Authenticate", authRealm)
				writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *handler) info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.cfg.Info)
}

func (h *handler) catalog(w http.ResponseWriter, r *http.Request) {
	summaries := h.cfg.Store.Catalog()
	out := make([]CatalogEntry, 0, len(summaries))
	for _, s := range summaries {
		e := CatalogEntry{Summary: s}
		if s.HasCover {
			e.CoverURL = "/v1/packages/" + url.PathEscape(s.Name) + "/" + store.LatestTag + "/cover"
		}
		out = append(out, e)
	}
	writeJSON(w, out)
}

func (h *handler) detail(w http.ResponseWriter, r *http.Request) {
	versions, err := h.cfg.Store.Versions(r.PathValue("name"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	d := PackageDetail{Name: r.PathValue("name"), Type: versions[0].Manifest.Type}
	for _, v := range versions {
		d.Versions = append(d.Versions, v.Manifest)
	}
	writeJSON(w, d)
}

func (h *handler) resolve(w http.ResponseWriter, r *http.Request) (*store.Version, bool) {
	v, err := h.cfg.Store.Resolve(r.PathValue("name"), r.PathValue("version"))
	if err != nil {
		writeStoreError(w, err)
		return nil, false
	}
	return v, true
}

func (h *handler) manifest(w http.ResponseWriter, r *http.Request) {
	if v, ok := h.resolve(w, r); ok {
		writeJSON(w, v.Manifest)
	}
}

func (h *handler) archive(w http.ResponseWriter, r *http.Request) {
	if v, ok := h.resolve(w, r); ok {
		serveArchive(w, r, v)
	}
}

func (h *handler) cover(w http.ResponseWriter, r *http.Request) {
	v, ok := h.resolve(w, r)
	if !ok {
		return
	}
	if v.CoverPath == "" {
		writeError(w, http.StatusNotFound, errors.New("package has no cover"))
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(v.CoverPath)))
	serveFile(w, r, v.CoverPath)
}

func (h *handler) legacyList(w http.ResponseWriter, r *http.Request) {
	summaries := h.cfg.Store.Catalog()
	out := make([]legacyEntry, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, legacyEntry{Name: s.Name, Type: s.Type, Format: s.Format, SizeBytes: s.SizeBytes})
	}
	writeJSON(w, out)
}

func (h *handler) legacyDownload(w http.ResponseWriter, r *http.Request) {
	v, err := h.cfg.Store.Resolve(r.PathValue("name"), store.LatestTag)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	serveArchive(w, r, v)
}

func serveArchive(w http.ResponseWriter, r *http.Request, v *store.Version) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": v.Manifest.Archive.File}))
	if v.Manifest.Archive.SHA256 != "" {
		w.Header().Set(SHA256Header, v.Manifest.Archive.SHA256)
	}
	serveFile(w, r, v.ArchivePath)
}

// serveFile uses ServeContent so Range requests (resumed downloads) work.
func serveFile(w http.ResponseWriter, r *http.Request, path string) {
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not open file"))
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not stat file"))
		return
	}
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInvalidName):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
