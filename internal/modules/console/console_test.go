package console

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	consolesvc "yapl/internal/console"
	"yapl/internal/kernel"
	"yapl/internal/registry/api"
	"yapl/internal/registry/store"
)

func inLibrary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })
	return dir
}

func TestStart(t *testing.T) {
	t.Run("given a server when the console starts then the UI and API are served on loopback", func(t *testing.T) {
		// Given
		inLibrary(t)
		registry := httptest.NewServer(api.New(api.Config{Store: store.New(t.TempDir(), io.Discard), Auth: api.AuthOpen}))
		defer registry.Close()
		var out bytes.Buffer

		// When
		inst, err := start(options{Port: 0, Server: registry.URL, Versions: kernel.Versions{Kernel: "1.0.0"}}, &out)
		if err != nil {
			t.Fatal(err)
		}
		defer inst.Close()

		// Then
		if !strings.HasPrefix(inst.URL, "http://127.0.0.1:") {
			t.Fatalf("url = %q", inst.URL)
		}
		page, err := http.Get(inst.URL)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(page.Body)
		page.Body.Close()
		if !strings.Contains(string(body), "YAPL Console") {
			t.Fatalf("page = %.200s", body)
		}
		resp, err := http.Get(inst.URL + "api/system")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var sys consolesvc.System
		json.NewDecoder(resp.Body).Decode(&sys)
		if sys.Server != registry.URL || !sys.ServerReachable || sys.Kernel != "1.0.0" {
			t.Fatalf("system = %+v", sys)
		}
	})

	t.Run("given no server can be found then the console starts offline and says so", func(t *testing.T) {
		inLibrary(t)
		var out bytes.Buffer
		inst, err := start(options{Port: 0, NoDiscovery: true}, &out)
		if err != nil {
			t.Fatal(err)
		}
		defer inst.Close()
		resp, err := http.Get(inst.URL + "api/store")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(out.String(), "offline") {
			t.Fatalf("status %d out %q", resp.StatusCode, out.String())
		}
	})

	t.Run("given the port is taken then start fails", func(t *testing.T) {
		inLibrary(t)
		first, err := start(options{Port: 0, NoDiscovery: true}, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		defer first.Close()
		if _, err := start(options{Port: first.Port, NoDiscovery: true}, io.Discard); err == nil {
			t.Fatal("expected a port conflict")
		}
	})
}

func TestModule(t *testing.T) {
	t.Run("given the module then it provides the console command with scoped flags", func(t *testing.T) {
		cmds := (Module{}).Commands()
		if len(cmds) != 1 || cmds[0].Verb != "console" {
			t.Fatalf("commands = %+v", cmds)
		}
	})
}

func fakePath(t *testing.T, browsers map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, script := range browsers {
		os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script+"\n"), 0755)
	}
	t.Setenv("PATH", dir)
}

func TestOpenBrowser(t *testing.T) {
	t.Run("given a kiosk browser when its window closes then the console is told to quit", func(t *testing.T) {
		// Given a "chromium" that records its arguments and exits at once
		argsFile := filepath.Join(t.TempDir(), "args")
		fakePath(t, map[string]string{"chromium": `echo "$@" > ` + argsFile})
		inst := &instance{URL: "http://127.0.0.1:8472/", workdir: t.TempDir()}
		quit := make(chan struct{})

		// When
		openBrowser(inst, true, func() { close(quit) }, io.Discard)

		// Then
		select {
		case <-quit:
		case <-time.After(3 * time.Second):
			t.Fatal("console was not told to quit when the kiosk window closed")
		}
		args, _ := os.ReadFile(argsFile)
		if !strings.Contains(string(args), "--kiosk") || !strings.Contains(string(args), "--app=http://127.0.0.1:8472/") {
			t.Fatalf("args = %s", args)
		}
	})

	t.Run("given a windowed browser when it closes then the console keeps running", func(t *testing.T) {
		fakePath(t, map[string]string{"chromium": "exit 0"})
		quit := make(chan struct{})
		openBrowser(&instance{URL: "http://x/", workdir: t.TempDir()}, false, func() { close(quit) }, io.Discard)
		select {
		case <-quit:
			t.Fatal("windowed browser exit must not quit the console")
		case <-time.After(300 * time.Millisecond):
		}
	})

	t.Run("given no browser then the URL is printed for the user", func(t *testing.T) {
		fakePath(t, nil)
		var out bytes.Buffer
		openBrowser(&instance{URL: "http://127.0.0.1:8472/", workdir: t.TempDir()}, true, func() {}, &out)
		if !strings.Contains(out.String(), "http://127.0.0.1:8472/") {
			t.Fatalf("out = %q", out.String())
		}
	})
}

func TestLauncher(t *testing.T) {
	t.Run("given a library then games are launched as yapl run with that workdir", func(t *testing.T) {
		cmd := launcher("/srv/library")("Doom")
		args := strings.Join(cmd.Args[1:], " ")
		if args != "--workdir /srv/library run Doom" {
			t.Fatalf("args = %q", args)
		}
	})
}

func TestConsoleCommand(t *testing.T) {
	t.Run("given --no-browser when the console command runs then it serves and stops on the quit signal", func(t *testing.T) {
		// Given a quit signal that has already fired
		inLibrary(t)
		registry := httptest.NewServer(api.New(api.Config{Store: store.New(t.TempDir(), io.Discard), Auth: api.AuthOpen}))
		defer registry.Close()
		stopSignal = func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}
		defer func() { stopSignal = defaultStopSignal }()
		var out, errOut bytes.Buffer
		k := kernel.New("yapl", &out, &errOut)
		k.Register(Module{})

		// When
		code := k.Execute([]string{"console", "--no-browser", "--port", "0", "--server", registry.URL})

		// Then
		if code != kernel.ExitOK || !strings.Contains(out.String(), "Console running at http://127.0.0.1:") {
			t.Fatalf("exit %d\n%s\n%s", code, out.String(), errOut.String())
		}
	})
}

var defaultStopSignal = stopSignal
