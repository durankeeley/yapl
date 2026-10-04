// Package server composes a running registry: the package store, its HTTP
// API, a periodic rescan and the LAN discovery broadcast.
package server

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"yapl/internal/defaults"
	"yapl/internal/registry/api"
	"yapl/internal/registry/store"
)

// DiscoveryPrefix starts every UDP discovery datagram: "YAPL:<http port>".
const DiscoveryPrefix = "YAPL:"

// Config holds the server configuration.
type Config struct {
	PackagesDir string
	Port        int
	Auth        string
	Password    string
	// Kernel and Modules are advertised at /v1/info.
	Kernel  string
	Modules map[string]string
	// AllowPush enables uploads (yapl push).
	AllowPush bool
	// BinaryPath, when set, is offered to clients for `yapl upgrade self`.
	BinaryPath string
	// Warnings receives scan problems; nil means stderr.
	Warnings io.Writer
}

// Server is a YAPL LAN registry.
type Server struct {
	cfg     Config
	store   *store.Store
	handler http.Handler
}

// New validates the configuration, scans the packages directory and builds
// the HTTP API.
func New(cfg Config) (*Server, error) {
	if cfg.Auth != api.AuthOpen && cfg.Auth != api.AuthPassword {
		return nil, fmt.Errorf("unknown auth mode %q: must be open or password", cfg.Auth)
	}
	if cfg.Auth == api.AuthPassword && cfg.Password == "" {
		return nil, fmt.Errorf("--auth password needs --password")
	}
	if cfg.Warnings == nil {
		cfg.Warnings = os.Stderr
	}
	st := store.New(cfg.PackagesDir, cfg.Warnings)
	if err := st.Refresh(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", cfg.PackagesDir, err)
	}
	if cfg.AllowPush && cfg.Auth == api.AuthOpen {
		fmt.Fprintln(cfg.Warnings, "warning: push is enabled without a password; anyone on the LAN can publish packages")
	}
	apiCfg := api.Config{
		Store:     st,
		Auth:      cfg.Auth,
		Password:  cfg.Password,
		AllowPush: cfg.AllowPush,
		Info:      api.Info{Kernel: cfg.Kernel, Modules: cfg.Modules},
	}
	h := api.New(apiCfg)
	if cfg.BinaryPath != "" {
		var err error
		if h, err = api.NewWithBinary(apiCfg, cfg.BinaryPath); err != nil {
			return nil, err
		}
	}
	return &Server{cfg: cfg, store: st, handler: h}, nil
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

// Store returns the package index.
func (s *Server) Store() *store.Store { return s.store }

// BroadcastTo sends one discovery datagram to addr.
func (s *Server) BroadcastTo(addr *net.UDPAddr) error {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.WriteTo([]byte(fmt.Sprintf("%s%d", DiscoveryPrefix, s.cfg.Port)), addr)
	return err
}

// Start serves until the listener fails, rescanning and broadcasting in the
// background.
func (s *Server) Start() error {
	go func() {
		ticker := time.NewTicker(defaults.ManifestRefreshInterval)
		defer ticker.Stop()
		for range ticker.C {
			if err := s.store.Refresh(); err != nil {
				fmt.Fprintf(s.cfg.Warnings, "warning: rescan failed: %v\n", err)
			}
		}
	}()
	go func() {
		addr := &net.UDPAddr{IP: net.IPv4bcast, Port: s.cfg.Port}
		ticker := time.NewTicker(defaults.BroadcastInterval)
		defer ticker.Stop()
		for range ticker.C {
			s.BroadcastTo(addr)
		}
	}()

	listen := fmt.Sprintf(":%d", s.cfg.Port)
	fmt.Printf("-> Serving %d packages from %s on %s (auth: %s)\n", len(s.store.Catalog()), s.cfg.PackagesDir, listen, s.cfg.Auth)
	return http.ListenAndServe(listen, s.handler)
}
