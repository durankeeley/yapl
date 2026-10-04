// Package store provides the commands that fetch games from a YAPL server.
package store

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"yapl/internal/app"
	"yapl/internal/catalog"
	"yapl/internal/client"
	"yapl/internal/config"
	connectpkg "yapl/internal/connect"
	"yapl/internal/defaults"
	"yapl/internal/kernel"
	"yapl/internal/manifest"
	"yapl/internal/paths"
	"yapl/internal/publish"
	"yapl/internal/pull"
	"yapl/internal/upgrade"
)

// Version is bumped whenever a store command changes behaviour.
const Version = "2.1.0"

const bytesPerMegabyte = 1 << 20

// Module registers the store commands with the kernel.
type Module struct{}

func (Module) Name() string    { return "store" }
func (Module) Version() string { return Version }

func (Module) Commands() []kernel.Command {
	return []kernel.Command{pullCommand(), pushCommand(), upgradeCommand()}
}

func serverFlags(fs *flag.FlagSet) {
	fs.String("server", "", "Server `address` (host:port or URL). Auto-discovered when omitted.")
	fs.Int("port", defaults.ServerPort, "UDP `port` to listen on for discovery.")
	fs.String("auth", defaults.AuthMode, "Auth `mode`: open or password.")
	fs.String("password", "", "Password for password auth (default $"+defaults.PasswordEnv+").")
}

// connect resolves the server from --server, client.json or LAN discovery.
func connect(ctx *kernel.Context) (*client.Client, error) {
	password := ctx.String("password")
	if password == "" {
		password = os.Getenv(defaults.PasswordEnv)
	}
	return connectpkg.Connect(connectpkg.Options{
		Server:        ctx.String("server"),
		Auth:          ctx.String("auth"),
		AuthExplicit:  ctx.IsSet("auth"),
		Password:      password,
		SettingsPath:  paths.ClientConfig,
		Discover:      true,
		DiscoveryPort: ctx.Int("port"),
	}, ctx.Stdout)
}

// remember saves the server that just worked so later commands (and the
// console) skip discovery.
func remember(ctx *kernel.Context, c *client.Client) {
	if err := connectpkg.Remember(paths.ClientConfig, c, ctx.String("auth")); err != nil {
		fmt.Fprintf(ctx.Stderr, "warning: could not save %s: %v\n", paths.ClientConfig, err)
	}
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
			if err == nil {
				remember(ctx, c)
			}
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
		Usage:       "pull <list|name[:version]> [flags]",
		Summary:     "List or install packages from a YAPL server",
		Description: "Installs name (latest version) or name:version. The archive and every component it needs (Proton, runtime, DXVK, …) are downloaded, checked against their SHA-256 and moved into place only when complete. The server is remembered in client.json; without --server YAPL uses that, or listens for a server's LAN broadcast.",
		Examples: []string{
			"yapl pull list",
			`yapl pull "Doom"`,
			`yapl pull "Doom:1.2.0"`,
			`yapl pull "Doom" --server 192.168.1.10:8471`,
		},
		Flags: serverFlags,
		Run: func(ctx *kernel.Context) error {
			c, err := connect(ctx)
			if err != nil {
				return err
			}
			ref := ctx.Arg(0)
			if ref == "" || ref == "list" {
				err = printCatalog(ctx, c)
			} else {
				name, tag := manifest.ParseReference(ref)
				_, err = pull.Pull(c, name, tag, pull.Options{Out: ctx.Stdout})
			}
			if err == nil {
				remember(ctx, c)
			}
			return err
		},
	}
}

func printCatalog(ctx *kernel.Context, c *client.Client) error {
	catalog, err := c.Catalog()
	if errors.Is(err, client.ErrNotFound) {
		return printLegacyCatalog(ctx, c)
	}
	if err != nil {
		return fmt.Errorf("list packages: %w", err)
	}
	tw := tabwriter.NewWriter(ctx.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTYPE\tLATEST\tVERSIONS\tSIZE\tTITLE")
	for _, e := range catalog {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d MB\t%s\n", e.Name, e.Type, e.Latest, len(e.Versions), e.SizeBytes/bytesPerMegabyte, e.Title)
	}
	return tw.Flush()
}

func printLegacyCatalog(ctx *kernel.Context, c *client.Client) error {
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

// Arguments to upgrade that name a part rather than a game.
const (
	upgradeSelf    = "self"
	upgradeCatalog = "catalog"
)

func upgradeCommand() kernel.Command {
	return kernel.Command{
		Verb:    "upgrade",
		Usage:   "upgrade [name[:version] | self | catalog] [flags]",
		Summary: "Upgrade only the parts that changed: games, yapl itself, or the defaults catalog",
		Description: "Without arguments, lists what is out of date. With a name, installs the server's newer version of that game and keeps its saves (prefix/drive_c/users, prefix/user.reg and metadata.json \"preserve\" paths); an explicit :version can roll back. " +
			"'self' replaces this yapl binary with the server's when the server's is newer in some module and older in none. 'catalog' adds the newer pinned Proton, runtime, DXVK and VKD3D versions to runner.json. --all upgrades every outdated game plus the catalog.",
		Examples: []string{
			"yapl upgrade",
			`yapl upgrade "Doom"`,
			`yapl upgrade "Doom:1.0.0"`,
			"yapl upgrade --all",
			"yapl upgrade self",
			"yapl upgrade catalog",
		},
		Flags: func(fs *flag.FlagSet) {
			serverFlags(fs)
			fs.Bool("all", false, "Upgrade every outdated game and the catalog.")
		},
		Run: func(ctx *kernel.Context) error {
			target := ctx.Arg(0)
			opts := upgrade.Options{Out: ctx.Stdout}
			if target == upgradeCatalog {
				return upgradeCatalogPart(ctx)
			}
			c, err := connect(ctx)
			if err != nil {
				return err
			}
			switch {
			case target == upgradeSelf:
				exe, err := os.Executable()
				if err != nil {
					return fmt.Errorf("locate this yapl binary: %w", err)
				}
				_, err = upgrade.Self(c, ctx.Kernel.Versions(), exe, opts)
				return err
			case target != "":
				name, tag := manifest.ParseReference(target)
				_, _, err := upgrade.Game(c, name, tag, opts)
				return err
			}
			items, err := plan(ctx, c)
			if err != nil {
				return err
			}
			if !ctx.Bool("all") {
				printPlan(ctx, items)
				return nil
			}
			for _, item := range items {
				switch item.Part {
				case upgrade.PartYAPL:
					fmt.Fprintln(ctx.Stdout, "-> A newer yapl is available; run 'yapl upgrade self' to install it")
				case upgrade.PartCatalog:
					err = upgradeCatalogPart(ctx)
				default:
					_, _, err = upgrade.Game(c, item.Name, manifest.LatestTag, opts)
				}
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func plan(ctx *kernel.Context, c *client.Client) ([]upgrade.Item, error) {
	installed, err := upgrade.Installed()
	if err != nil {
		return nil, err
	}
	runner, _ := config.LoadGlobal(paths.RunnerConfig)
	local := upgrade.LocalState{Installed: installed, Binary: ctx.Kernel.Versions(), RunnerCatalog: runner.CatalogVersion, BinaryCatalog: catalog.Version}
	var remote upgrade.RemoteState
	if info, err := c.Info(); err == nil {
		remote.Binary = kernel.Versions{Kernel: info.Kernel, Modules: info.Modules}
	}
	if entries, err := c.Catalog(); err == nil {
		remote.Catalog = entries
	} else {
		fmt.Fprintf(ctx.Stderr, "warning: could not read the server catalog: %v\n", err)
	}
	return upgrade.Plan(local, remote), nil
}

func printPlan(ctx *kernel.Context, items []upgrade.Item) {
	if len(items) == 0 {
		fmt.Fprintln(ctx.Stdout, "Everything is up to date.")
		return
	}
	tw := tabwriter.NewWriter(ctx.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PART\tNAME\tCURRENT\tAVAILABLE")
	for _, i := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", i.Part, i.Name, i.From, i.To)
	}
	tw.Flush()
	fmt.Fprintln(ctx.Stdout, "\nRun 'yapl upgrade <name>', 'yapl upgrade self', 'yapl upgrade catalog' or 'yapl upgrade --all'.")
}

func upgradeCatalogPart(ctx *kernel.Context) error {
	added, err := upgrade.Catalog(paths.RunnerConfig)
	if err != nil {
		return err
	}
	for _, a := range added {
		fmt.Fprintf(ctx.Stdout, "-> Added %s\n", a)
	}
	fmt.Fprintf(ctx.Stdout, "✅ runner.json now uses catalog %s\n", catalog.Version)
	return nil
}
