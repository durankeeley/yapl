// Package store provides the commands that fetch games from a YAPL server.
package store

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"yapl/internal/app"
	"yapl/internal/archive"
	"yapl/internal/catalog"
	"yapl/internal/client"
	"yapl/internal/config"
	"yapl/internal/defaults"
	"yapl/internal/kernel"
	"yapl/internal/paths"
	"yapl/internal/publish"
)

// Version is bumped whenever a store command changes behaviour.
const Version = "1.1.0"

const bytesPerMegabyte = 1 << 20

// Module registers the store commands with the kernel.
type Module struct{}

func (Module) Name() string    { return "store" }
func (Module) Version() string { return Version }

func (Module) Commands() []kernel.Command {
	return []kernel.Command{pullCommand(), pushCommand()}
}

func serverFlags(fs *flag.FlagSet) {
	fs.String("server", "", "Server `address` (host:port or URL). Auto-discovered when omitted.")
	fs.Int("port", defaults.ServerPort, "UDP `port` to listen on for discovery.")
	fs.String("auth", defaults.AuthMode, "Auth `mode`: open or password.")
	fs.String("password", "", "Password for password auth.")
}

// connect builds a client for --server, discovering one on the LAN when
// the flag is empty.
func connect(ctx *kernel.Context) (*client.Client, error) {
	addr := ctx.String("server")
	if addr == "" {
		fmt.Fprintln(ctx.Stdout, "-> No --server specified; discovering YAPL server on LAN...")
		found, err := client.Discover(ctx.Int("port"), defaults.DiscoveryTimeout)
		if err != nil {
			return nil, fmt.Errorf("%w\n   Pass --server <host:port> to name the server", err)
		}
		addr = found
		fmt.Fprintf(ctx.Stdout, "-> Found server at %s\n", addr)
	}
	return client.New(client.Config{Server: addr, Auth: ctx.String("auth"), Password: ctx.String("password")}), nil
}

// pushFormat is zstd: a little larger than xz but many times faster to
// compress, which matters when publishing multi-GB games.
const pushFormat = "zst"

func pushCommand() kernel.Command {
	return kernel.Command{
		Verb:        "push",
		Usage:       "push <name> [flags]",
		Summary:     "Publish a game or app to a YAPL server",
		Description: "Packages the game, uploads every component (Proton, runtime, DXVK, VKD3D, umu-launcher) the server doesn't have yet, then uploads the package as <name>:<version>. Published versions are immutable; bump --version to publish again. The server must run with --allow-push.",
		Examples: []string{
			`yapl push "Doom" --version 1.0.0`,
			`yapl push "Doom" --version 1.1.0 --server 192.168.1.10:8471 --auth password --password lanparty`,
		},
		Flags: func(fs *flag.FlagSet) {
			serverFlags(fs)
			fs.String("version", "", "Package `version` (MAJOR.MINOR.PATCH); defaults to metadata.json's.")
			fs.String("format", pushFormat, "Compression `format` for the package: "+strings.Join(defaults.PackageFormats, ", ")+".")
			fs.String("config", "", "Config `file` name inside the game directory (default game.json or app.json).")
		},
		Run: func(ctx *kernel.Context) error {
			name := ctx.Arg(0)
			if name == "" {
				return kernel.UsageError("a name is required")
			}
			a, err := loadApp(name, ctx.String("config"))
			if err != nil {
				return err
			}
			c, err := connect(ctx)
			if err != nil {
				return err
			}
			_, err = publish.Publish(a, c, publish.Options{
				Version:   ctx.String("version"),
				Format:    ctx.String("format"),
				CreatedBy: "yapl store/" + Version,
			}, ctx.Stdout)
			return err
		},
	}
}

// loadApp finds an existing game or app in the library.
func loadApp(name, configName string) (*app.App, error) {
	appType, appName, err := app.Find(name)
	if err != nil {
		return nil, err
	}
	globalCfg, err := config.LoadOrCreateGlobal(paths.RunnerConfig, catalog.Default())
	if err != nil {
		return nil, err
	}
	appCfg, err := config.LoadOrCreateApp(appType, appName, configName, globalCfg, "")
	if err != nil {
		return nil, err
	}
	a := app.New(appType, appName, false, false, false, globalCfg, appCfg)
	if configName != "" {
		a.ConfigFile = filepath.Join(a.AppDir, configName)
	}
	return a, nil
}

func pullCommand() kernel.Command {
	return kernel.Command{
		Verb:        "pull",
		Usage:       "pull <list|name> [flags]",
		Summary:     "List or download packages from a YAPL server",
		Description: "Without --server, YAPL listens for a server's LAN broadcast.",
		Examples:    []string{"yapl pull list", `yapl pull "Doom"`, `yapl pull --server 192.168.1.10:8471 "Doom"`},
		Flags: func(fs *flag.FlagSet) {
			serverFlags(fs)
			fs.String("output-dir", "", "Keep the downloaded archive in this `path` (default: a temporary directory).")
		},
		Run: func(ctx *kernel.Context) error {
			c, err := connect(ctx)
			if err != nil {
				return err
			}

			name := ctx.Arg(0)
			if name == "" || name == "list" {
				return printCatalog(ctx, c)
			}
			return pull(ctx, c, name)
		},
	}
}

func printCatalog(ctx *kernel.Context, c *client.Client) error {
	entries, err := c.List()
	if err != nil {
		return fmt.Errorf("list packages: %w", err)
	}
	tw := tabwriter.NewWriter(ctx.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTYPE\tFORMAT\tSIZE")
	for _, e := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d MB\n", e.Name, e.Type, e.Format, e.SizeBytes/bytesPerMegabyte)
	}
	return tw.Flush()
}

func pull(ctx *kernel.Context, c *client.Client, name string) error {
	dlDir := ctx.String("output-dir")
	if dlDir == "" {
		tmp, err := os.MkdirTemp("", "yapl-pull-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tmp)
		dlDir = tmp
	}
	archivePath, entryType, err := c.Download(name, dlDir, ctx.Stderr)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	fmt.Fprintf(ctx.Stdout, "-> Downloaded to %s\n", archivePath)

	targetDir := paths.TypeDir(entryType)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", targetDir, err)
	}
	return archive.Unpackage(targetDir, []string{archivePath})
}
