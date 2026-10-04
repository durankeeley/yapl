package console

import (
	"fmt"
	"os/exec"
)

const (
	familyChromium = "chromium"
	familyFirefox  = "firefox"
	familyOpener   = "opener"
)

// browsers are tried in order. Chromium-family browsers give the best
// console experience (true app windows, Gamepad API without prompts).
var browsers = []struct{ name, family string }{
	{"chromium", familyChromium},
	{"chromium-browser", familyChromium},
	{"google-chrome", familyChromium},
	{"google-chrome-stable", familyChromium},
	{"brave-browser", familyChromium},
	{"microsoft-edge", familyChromium},
	{"firefox", familyFirefox},
	{"xdg-open", familyOpener},
}

// BrowserOptions describes how to show the console.
type BrowserOptions struct {
	URL string
	// ProfileDir isolates the console browser from the user's own profile
	// and lets kiosk mode work even when their browser is already open.
	ProfileDir string
	Kiosk      bool
}

// BrowserCommand picks the first available browser. dedicated is true when
// the process is the console window itself, so its exit can end the console.
func BrowserCommand(opts BrowserOptions, lookPath func(string) (string, error)) (cmd *exec.Cmd, dedicated bool, err error) {
	for _, b := range browsers {
		path, err := lookPath(b.name)
		if err != nil {
			continue
		}
		switch b.family {
		case familyChromium:
			args := []string{"--user-data-dir=" + opts.ProfileDir, "--no-first-run", "--no-default-browser-check", "--noerrdialogs"}
			if opts.Kiosk {
				args = append(args, "--kiosk")
			}
			return exec.Command(path, append(args, "--app="+opts.URL)...), true, nil
		case familyFirefox:
			args := []string{"--new-instance", "--profile", opts.ProfileDir}
			if opts.Kiosk {
				args = append(args, "--kiosk")
			} else {
				args = append(args, "--new-window")
			}
			return exec.Command(path, append(args, opts.URL)...), true, nil
		default:
			return exec.Command(path, opts.URL), false, nil
		}
	}
	return nil, false, fmt.Errorf("no browser found; open %s yourself", opts.URL)
}
