// Package server composes a running registry: the package store, its HTTP
// API, a periodic rescan and the LAN discovery broadcast.
package server

import (
	"context"
	"errors"
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
	// BinaryPath, when set, is offered to clients for `yapl upgrade --self`.
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

// shutdownTimeout bounds how long in-flight downloads get when the server
// stops.
const shutdownTimeout = 5 * time.Second

// Start listens on the configured port and serves until the process exits.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.Port))
	if err != nil {
		return err
	}
	return s.Serve(context.Background(), ln)
}

// Serve answers on ln, rescanning and broadcasting in the background, until
// ctx is cancelled (nil error) or the listener fails.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	go s.every(ctx, defaults.ManifestRefreshInterval, func() {
		if err := s.store.Refresh(); err != nil {
			fmt.Fprintf(s.cfg.Warnings, "warning: rescan failed: %v\n", err)
		}
	})
	bcast := &net.UDPAddr{IP: net.IPv4bcast, Port: s.cfg.Port}
	go s.every(ctx, defaults.BroadcastInterval, func() { s.BroadcastTo(bcast) })

	srv := &http.Server{Handler: s.handler, ReadHeaderTimeout: readHeaderTimeout}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	fmt.Printf("-> Serving %d packages from %s on %s (auth: %s)\n", len(s.store.Catalog()), s.cfg.PackagesDir, ln.Addr(), s.cfg.Auth)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// readHeaderTimeout stops slow clients from holding connections open.
const readHeaderTimeout = 30 * time.Second

func (s *Server) every(ctx context.Context, interval time.Duration, f func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f()
		}
	}
}
