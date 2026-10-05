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

const (
	// noAutoUpdateUntil tells Chrome's outdated-build detector that
	// auto-update is disabled until then, so "Can't update Chrome" never
	// covers the console.
	noAutoUpdateUntil = "Tue, 31 Dec 2099 23:59:59 GMT"
	// updateCheckIntervalSeconds (one year) keeps a session from checking
	// for updates at all.
	updateCheckIntervalSeconds = "31536000"
)

// quietChromiumArgs keep a console window free of first-run, update,
// crash-restore and translate prompts; there is no mouse on the couch to
// dismiss them.
var quietChromiumArgs = []string{
	"--no-first-run",
	"--no-default-browser-check",
	"--noerrdialogs",
	"--simulate-outdated-no-au=" + noAutoUpdateUntil,
	"--check-for-update-interval=" + updateCheckIntervalSeconds,
	"--hide-crash-restore-bubble",
	"--disable-features=Translate",
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
			args := append([]string{"--user-data-dir=" + opts.ProfileDir}, quietChromiumArgs...)
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
