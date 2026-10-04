// Package registry provides the commands that run a YAPL package server.
package registry

import (
	"flag"

	"yapl/internal/defaults"
	"yapl/internal/kernel"
	"yapl/internal/server"
)

// Version is bumped whenever the server's behaviour or API changes.
const Version = "2.1.0"

// Module registers the registry commands with the kernel.
type Module struct{}

func (Module) Name() string    { return "registry" }
func (Module) Version() string { return Version }

func (Module) Commands() []kernel.Command {
	return []kernel.Command{serveCommand()}
}

func serveCommand() kernel.Command {
	return kernel.Command{
		Verb:        "serve",
		Usage:       "serve --packages-dir <path> [flags]",
		Summary:     "Start a LAN package server",
		Description: "Serves packages from <path>/games and <path>/apps over the v1 HTTP API (plus the legacy /packages endpoints) and announces itself on the LAN by UDP broadcast. Versioned packages live in <path>/games/<name>/<version>/; flat <name>.tar.* files copied in by hand are served too, and the directory is rescanned every " + defaults.ManifestRefreshInterval.String() + ".",
		Examples: []string{
			"yapl serve --packages-dir /srv/games",
			"yapl serve --packages-dir /srv/games --auth password --password lanparty --allow-push",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.String("packages-dir", "", "Directory containing packaged archives (`path`, required).")
			fs.Int("port", defaults.ServerPort, "TCP and UDP `port` to listen and broadcast on.")
			fs.String("auth", defaults.AuthMode, "Auth `mode`: open or password.")
			fs.String("password", "", "Password for password auth.")
			fs.Bool("allow-push", false, "Accept uploads from 'yapl push'. Combine with --auth password on shared networks.")
		},
		Run: func(ctx *kernel.Context) error {
			dir := ctx.String("packages-dir")
			if dir == "" {
				return kernel.UsageError("--packages-dir is required")
			}
			versions := ctx.Kernel.Versions()
			s, err := server.New(server.Config{
				PackagesDir: dir,
				Port:        ctx.Int("port"),
				Auth:        ctx.String("auth"),
				Password:    ctx.String("password"),
				Kernel:      versions.Kernel,
				Modules:     versions.Modules,
				AllowPush:   ctx.Bool("allow-push"),
			})
			if err != nil {
				return err
			}
			return s.Start()
		},
	}
}
