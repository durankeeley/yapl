// Package console provides `yapl console`, the full-screen, gamepad-driven
// launcher.
package console

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"yapl/internal/connect"
	consolesvc "yapl/internal/console"
	"yapl/internal/defaults"
	"yapl/internal/kernel"
	"yapl/internal/paths"
)

// Version is bumped whenever the console's behaviour or UI changes.
const Version = "1.0.1"

const (
	loopback        = "127.0.0.1"
	shutdownTimeout = 5 * time.Second
	headerTimeout   = 10 * time.Second
)

// Module registers the console command.
type Module struct{}

func (Module) Name() string    { return "console" }
func (Module) Version() string { return Version }

func (Module) Commands() []kernel.Command {
	return []kernel.Command{{
		Verb:    "console",
		Usage:   "console [flags]",
		Summary: "Open the full-screen, gamepad-friendly launcher",
		Description: "Serves the console on " + loopback + " and opens it full-screen in the first available browser (Chromium, Chrome, Brave, Edge, Firefox). " +
			"Browse the library, install and update games from the LAN server, and play — all with a controller. " +
			"The server is --server, else the one in client.json, else one found on the LAN; without one the console runs offline with the local library. " +
			"Closing a kiosk window YAPL opened quits the console.",
		Examples: []string{
			"yapl console",
			"yapl --workdir /opt/lan-games console",
			"yapl console --windowed",
			"yapl console --no-browser --port 9000",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.Int("port", defaults.ConsolePort, "Loopback `port` to serve the console on.")
			fs.Bool("no-browser", false, "Only print the console's URL.")
			fs.Bool("windowed", false, "Open an app window instead of full-screen kiosk mode.")
			fs.String("server", "", "Server `address` (host:port or URL).")
			fs.String("auth", defaults.AuthMode, "Auth `mode`: open or password.")
			fs.String("password", "", "Password for password auth (default $"+defaults.PasswordEnv+").")
		},
		Run: run,
	}}
}

// stopSignal ends the console on Ctrl+C or SIGTERM; tests replace it.
var stopSignal = func() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

type options struct {
	Port         int
	Server       string
	Auth         string
	AuthExplicit bool
	Password     string
	NoDiscovery  bool
	Versions     kernel.Versions
}

func run(ctx *kernel.Context) error {
	password := ctx.String("password")
	if password == "" {
		password = os.Getenv(defaults.PasswordEnv)
	}
	inst, err := start(options{
		Port: ctx.Int("port"), Server: ctx.String("server"), Auth: ctx.String("auth"), AuthExplicit: ctx.IsSet("auth"),
		Password: password, Versions: ctx.Kernel.Versions(),
	}, ctx.Stdout)
	if err != nil {
		return err
	}
	defer inst.Close()

	sig, stop := stopSignal()
	defer stop()
	if !ctx.Bool("no-browser") {
		openBrowser(inst, !ctx.Bool("windowed"), stop, ctx.Stdout)
	}
	fmt.Fprintf(ctx.Stdout, "-> Console running at %s (Ctrl+C to quit)\n", inst.URL)
	<-sig.Done()
	return nil
}

// openBrowser shows the console. When YAPL owns a kiosk window, closing it
// ends the console.
func openBrowser(inst *instance, kiosk bool, quit func(), out io.Writer) {
	cmd, dedicated, err := consolesvc.BrowserCommand(consolesvc.BrowserOptions{
		URL: inst.URL, ProfileDir: filepath.Join(inst.workdir, paths.ConsoleBrowserDir), Kiosk: kiosk,
	}, exec.LookPath)
	if err != nil {
		fmt.Fprintf(out, "-> %v\n", err)
		return
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(out, "-> Could not start %s: %v; open %s yourself\n", cmd.Path, err, inst.URL)
		return
	}
	if dedicated && kiosk {
		go func() {
			cmd.Wait()
			quit()
		}()
	}
}

type instance struct {
	URL     string
	Port    int
	workdir string
	server  *http.Server
	service *consolesvc.Service
}

// Close stops serving and the job worker.
func (i *instance) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	i.server.Shutdown(ctx)
	i.service.Close()
}

// start resolves the server, builds the service and begins serving.
func start(opts options, out io.Writer) (*instance, error) {
	workdir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cfg := consolesvc.Config{Versions: opts.Versions, Workdir: workdir, Launcher: launcher(workdir), UI: consolesvc.UI()}
	c, err := connect.Connect(connect.Options{
		Server: opts.Server, Auth: opts.Auth, AuthExplicit: opts.AuthExplicit, Password: opts.Password,
		SettingsPath: paths.ClientConfig, Discover: !opts.NoDiscovery, DiscoveryPort: defaults.ServerPort,
	}, out)
	switch {
	case errors.Is(err, connect.ErrNoServer):
		fmt.Fprintln(out, "-> No YAPL server found; the console is offline (library only)")
	case err != nil:
		return nil, err
	default:
		cfg.Registry = consolesvc.NewRegistry(c)
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(loopback, strconv.Itoa(opts.Port)))
	if err != nil {
		return nil, fmt.Errorf("listen on port %d: %w", opts.Port, err)
	}
	svc := consolesvc.New(cfg)
	port := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: svc.Handler(), ReadHeaderTimeout: headerTimeout}
	go srv.Serve(ln)
	return &instance{URL: fmt.Sprintf("http://%s:%d/", loopback, port), Port: port, workdir: workdir, server: srv, service: svc}, nil
}

// launcher runs games as a separate `yapl run` process, so a crash in a
// game or in Wine never takes the console down.
func launcher(workdir string) consolesvc.Launcher {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	return func(name string) *exec.Cmd {
		return exec.Command(exe, "--workdir", workdir, "run", name)
	}
}
