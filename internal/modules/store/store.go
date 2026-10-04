// Package store provides the commands that fetch games from a YAPL server.
package store

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"yapl/internal/archive"
	"yapl/internal/client"
	"yapl/internal/defaults"
	"yapl/internal/kernel"
	"yapl/internal/paths"
)

// Version is bumped whenever a store command changes behaviour.
const Version = "1.0.0"

const bytesPerMegabyte = 1 << 20

// Module registers the store commands with the kernel.
type Module struct{}

func (Module) Name() string    { return "store" }
func (Module) Version() string { return Version }

func (Module) Commands() []kernel.Command {
	return []kernel.Command{pullCommand()}
}

func pullCommand() kernel.Command {
	return kernel.Command{
		Verb:        "pull",
		Usage:       "pull <list|name> [flags]",
		Summary:     "List or download packages from a YAPL server",
		Description: "Without --server, YAPL listens for a server's LAN broadcast.",
		Examples:    []string{"yapl pull list", `yapl pull "Doom"`, `yapl pull --server 192.168.1.10:8471 "Doom"`},
		Flags: func(fs *flag.FlagSet) {
			fs.String("server", "", "Server `address` (host:port or URL). Auto-discovered when omitted.")
			fs.Int("port", defaults.ServerPort, "UDP `port` to listen on for discovery.")
			fs.String("auth", defaults.AuthMode, "Auth `mode`: open or password.")
			fs.String("password", "", "Password for password auth.")
			fs.String("output-dir", "", "Keep the downloaded archive in this `path` (default: a temporary directory).")
		},
		Run: func(ctx *kernel.Context) error {
			addr := ctx.String("server")
			if addr == "" {
				fmt.Fprintln(ctx.Stdout, "-> No --server specified; discovering YAPL server on LAN...")
				found, err := client.Discover(ctx.Int("port"), defaults.DiscoveryTimeout)
				if err != nil {
					return fmt.Errorf("%w\n   Run 'yapl pull --server <host:port> ...' to specify the server", err)
				}
				addr = found
				fmt.Fprintf(ctx.Stdout, "-> Found server at %s\n", addr)
			}
			c := client.New(client.Config{Server: addr, Auth: ctx.String("auth"), Password: ctx.String("password")})

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
