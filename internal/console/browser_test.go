package console

import (
	"errors"
	"strings"
	"testing"
)

func onPath(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, n := range names {
			if n == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestBrowserCommand(t *testing.T) {
	opts := BrowserOptions{URL: "http://127.0.0.1:8472/", ProfileDir: "/lib/.console-browser", Kiosk: true}
	tests := []struct {
		name          string
		path          []string
		opts          BrowserOptions
		wantBin       string
		wantArgs      []string
		wantDedicated bool
	}{
		{
			name: "given chromium then it opens the console as a kiosk app with its own profile",
			path: []string{"chromium", "firefox", "xdg-open"}, opts: opts,
			wantBin: "/usr/bin/chromium", wantDedicated: true,
			wantArgs: []string{"--user-data-dir=/lib/.console-browser", "--no-first-run", "--no-default-browser-check", "--noerrdialogs", "--kiosk", "--app=http://127.0.0.1:8472/"},
		},
		{
			name: "given chrome but no chromium then chrome is used",
			path: []string{"google-chrome", "xdg-open"}, opts: opts,
			wantBin: "/usr/bin/google-chrome", wantDedicated: true,
		},
		{
			name: "given only firefox then it runs a separate kiosk instance",
			path: []string{"firefox", "xdg-open"}, opts: opts,
			wantBin: "/usr/bin/firefox", wantDedicated: true,
			wantArgs: []string{"--new-instance", "--profile", "/lib/.console-browser", "--kiosk", "http://127.0.0.1:8472/"},
		},
		{
			name: "given windowed mode with chromium then it opens an app window without kiosk",
			path: []string{"chromium"}, opts: BrowserOptions{URL: opts.URL, ProfileDir: opts.ProfileDir},
			wantBin: "/usr/bin/chromium", wantDedicated: true,
			wantArgs: []string{"--user-data-dir=/lib/.console-browser", "--no-first-run", "--no-default-browser-check", "--noerrdialogs", "--app=http://127.0.0.1:8472/"},
		},
		{
			name: "given only xdg-open then the default browser is asked to open the url",
			path: []string{"xdg-open"}, opts: opts,
			wantBin: "/usr/bin/xdg-open", wantArgs: []string{"http://127.0.0.1:8472/"}, wantDedicated: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// When
			cmd, dedicated, err := BrowserCommand(tc.opts, onPath(tc.path...))

			// Then
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Path != tc.wantBin || dedicated != tc.wantDedicated {
				t.Fatalf("path %q dedicated %v", cmd.Path, dedicated)
			}
			if tc.wantArgs != nil && strings.Join(cmd.Args[1:], " ") != strings.Join(tc.wantArgs, " ") {
				t.Fatalf("args = %q", cmd.Args[1:])
			}
		})
	}

	t.Run("given no browser at all then it explains how to open the console manually", func(t *testing.T) {
		_, _, err := BrowserCommand(opts, onPath())
		if err == nil || !strings.Contains(err.Error(), opts.URL) {
			t.Fatalf("error = %v", err)
		}
	})
}
