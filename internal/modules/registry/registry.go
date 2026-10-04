// Package registry provides the commands that run a YAPL package server.
package registry

import (
	"flag"

	"yapl/internal/defaults"
	"yapl/internal/kernel"
	"yapl/internal/server"
)

// Version is bumped whenever the server's behaviour or API changes.
const Version = "1.0.0"

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
		Description: "Serves archives from <path>/games and <path>/apps over HTTP and announces itself on the LAN by UDP broadcast.",
		Examples:    []string{"yapl serve --packages-dir /srv/games", "yapl serve --packages-dir /srv/games --auth password --password lanparty"},
		Flags: func(fs *flag.FlagSet) {
			fs.String("packages-dir", "", "Directory containing packaged archives (`path`, required).")
			fs.Int("port", defaults.ServerPort, "TCP and UDP `port` to listen and broadcast on.")
			fs.String("auth", defaults.AuthMode, "Auth `mode`: open or password.")
			fs.String("password", "", "Password for password auth.")
		},
		Run: func(ctx *kernel.Context) error {
			dir := ctx.String("packages-dir")
			if dir == "" {
				return kernel.UsageError("--packages-dir is required")
			}
			s, err := server.New(server.Config{
				PackagesDir: dir,
				Port:        ctx.Int("port"),
				Auth:        ctx.String("auth"),
				Password:    ctx.String("password"),
			})
			if err != nil {
				return err
			}
			return s.Start()
		},
	}
}
