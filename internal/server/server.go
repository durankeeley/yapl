package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PackageEntry describes a single package available on the server.
type PackageEntry struct {
	Name      string `json:"name"`
	Type      string `json:"type"` // "game" or "app"
	Format    string `json:"format"`
	SizeBytes int64  `json:"size_bytes"`
}

// Config holds the server configuration.
type Config struct {
	PackagesDir string
	Port        int
	Auth        string // "open" or "password"
	Password    string
}

type manifestEntry struct {
	PackageEntry
	filePath string
	fileInfo os.FileInfo
}

// Server is a YAPL LAN package server.
type Server struct {
	cfg     Config
	mu      sync.RWMutex
	entries map[string]manifestEntry
	mux     *http.ServeMux
}

var supportedExts = map[string]string{
	".tar.xz":  "xz",
	".tar.gz":  "gz",
	".tar.zst": "zst",
}

// subdirTypes maps subdirectory names to the package type they contain.
var subdirTypes = map[string]string{
	"games": "game",
	"apps":  "app",
}

// New creates a Server and performs an initial manifest scan.
// Returns an error if the auth mode is unrecognised.
func New(cfg Config) (*Server, error) {
	if cfg.Auth != "open" && cfg.Auth != "password" {
		return nil, fmt.Errorf("unknown auth mode %q: must be open or password", cfg.Auth)
	}
	s := &Server{cfg: cfg}
	s.RefreshManifest()
	s.buildMux()
	return s, nil
}

// Handler returns the http.Handler for this server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Manifest returns a snapshot of the current package manifest keyed by bare name.
func (s *Server) Manifest() map[string]PackageEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]PackageEntry, len(s.entries))
	for k, v := range s.entries {
		out[k] = v.PackageEntry
	}
	return out
}

// RefreshManifest re-scans the packages directory (games/ and apps/ subdirs) and
// updates the in-memory manifest.
func (s *Server) RefreshManifest() {
	newMap := make(map[string]manifestEntry)

	for subdir, entryType := range subdirTypes {
		dir := filepath.Join(s.cfg.PackagesDir, subdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			bareName, format, ok := archiveInfo(e.Name())
			if !ok {
				continue
			}
			if _, exists := newMap[bareName]; exists {
				// games/ takes precedence; log collision but don't block.
				fmt.Fprintf(os.Stderr, "warning: package %q exists in both games/ and apps/; keeping games/ entry\n", bareName)
				continue
			}
			fullPath := filepath.Join(dir, e.Name())
			info, err := e.Info()
			if err != nil {
				continue
			}
			newMap[bareName] = manifestEntry{
				PackageEntry: PackageEntry{Name: bareName, Type: entryType, Format: format, SizeBytes: info.Size()},
				filePath:     fullPath,
				fileInfo:     info,
			}
		}
	}

	s.mu.Lock()
	s.entries = newMap
	s.mu.Unlock()
}

// BroadcastTo sends a single YAPL discovery datagram to the given UDP address.
// Used internally and exposed for testing.
func (s *Server) BroadcastTo(addr *net.UDPAddr) error {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return err
	}
	defer conn.Close()
	msg := []byte(fmt.Sprintf("YAPL:%d", s.cfg.Port))
	_, err = conn.WriteTo(msg, addr)
	return err
}

// Start begins serving on cfg.Port, refreshing the manifest every 30 seconds
// and broadcasting the server's presence every 2 seconds.
func (s *Server) Start() error {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.RefreshManifest()
		}
	}()

	go func() {
		bcastAddr := &net.UDPAddr{IP: net.IPv4bcast, Port: s.cfg.Port}
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.BroadcastTo(bcastAddr)
		}
	}()

	addr := fmt.Sprintf(":%d", s.cfg.Port)
	fmt.Printf("-> Serving packages from %s on %s (auth: %s)\n", s.cfg.PackagesDir, addr, s.cfg.Auth)
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) buildMux() {
	inner := http.NewServeMux()
	inner.HandleFunc("/packages", s.handleList)
	inner.HandleFunc("/packages/", s.handleDownload)
	s.mux = http.NewServeMux()
	s.mux.Handle("/", s.authMiddleware(inner))
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Auth == "password" {
			_, pw, ok := r.BasicAuth()
			if !ok || pw != s.cfg.Password {
				w.Header().Set("WWW-Authenticate", `Basic realm="yapl"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	list := make([]PackageEntry, 0, len(s.entries))
	for _, e := range s.entries {
		list = append(list, e.PackageEntry)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/packages/")
	if name == "" {
		s.handleList(w, r)
		return
	}

	s.mu.RLock()
	entry, ok := s.entries[name]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, fmt.Sprintf("package %q not found", name), http.StatusNotFound)
		return
	}

	f, err := os.Open(entry.filePath)
	if err != nil {
		http.Error(w, "could not open package", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, filepath.Base(entry.filePath), entry.fileInfo.ModTime(), f)
}

// archiveInfo returns the bare name and format string for a supported archive filename.
func archiveInfo(filename string) (bareName, format string, ok bool) {
	for ext, fmt := range supportedExts {
		if strings.HasSuffix(filename, ext) {
			return strings.TrimSuffix(filename, ext), fmt, true
		}
	}
	return "", "", false
}
