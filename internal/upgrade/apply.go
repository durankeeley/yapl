package upgrade

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"yapl/internal/app"
	"yapl/internal/catalog"
	"yapl/internal/client"
	"yapl/internal/config"
	"yapl/internal/fs"
	"yapl/internal/kernel"
	"yapl/internal/manifest"
	"yapl/internal/paths"
	"yapl/internal/progress"
	"yapl/internal/pull"
	"yapl/internal/registry/api"
	"yapl/internal/version"
)

// alwaysPreserved are kept across every game upgrade: Wine's per-user
// folders (saves, settings) and the per-user registry hive.
var alwaysPreserved = []string{"prefix/drive_c/users", "prefix/user.reg"}

const (
	previousPrefix   = "previous-"
	downloadSuffix   = ".download"
	unknownVersion   = "unknown"
	previousInstallD = "install"
)

// Options controls output.
type Options struct {
	Out    io.Writer
	Report progress.Func
}

func (o Options) pullOptions() pull.Options {
	return pull.Options{Out: o.Out, Report: o.Report}
}

func (o Options) out() io.Writer {
	if o.Out == nil {
		return io.Discard
	}
	return o.Out
}

// Game replaces an installed game with name:tag from the server, keeping
// saves and preserved files. With tag "latest" it does nothing unless the
// server's version is newer; an explicit tag may also roll back.
func Game(c *client.Client, name, tag string, opts Options) (from, to string, err error) {
	appType, appName, err := app.Find(name)
	if err != nil {
		return "", "", err
	}
	dir := paths.AppDir(appType, appName)
	from = unknownVersion
	if in, err := manifest.ReadInstalled(dir); err == nil {
		from = in.Version
	}
	m, err := c.Manifest(appName, tag)
	if err != nil {
		return from, from, err
	}
	to = m.Metadata.Version
	if from == to || (tag == manifest.LatestTag && from != unknownVersion && version.Compare(to, from) <= 0) {
		fmt.Fprintf(opts.out(), "-> %s is up to date (%s)\n", appName, from)
		return from, from, nil
	}

	fmt.Fprintf(opts.out(), "-> Upgrading %s %s -> %s\n", appName, from, to)
	if _, err := pull.EnsureComponents(c, m, opts.pullOptions()); err != nil {
		return from, from, err
	}
	staged, cleanup, err := pull.Fetch(c, m, opts.pullOptions())
	if err != nil {
		return from, from, err
	}
	defer cleanup()
	if err := carryOver(dir, staged); err != nil {
		return from, from, fmt.Errorf("keep saves: %w", err)
	}
	if err := swap(dir, staged); err != nil {
		return from, from, err
	}
	if err := pull.RecordInstall(dir, m, c); err != nil {
		return from, to, err
	}
	fmt.Fprintf(opts.out(), "✅ Upgraded %s to %s\n", appName, to)
	return from, to, nil
}

// carryOver copies preserved paths from the old install onto the new one.
func carryOver(oldDir, newDir string) error {
	patterns := append([]string{}, alwaysPreserved...)
	for _, dir := range []string{oldDir, newDir} {
		if md, _, err := manifest.LoadMetadata(dir, ""); err == nil {
			patterns = append(patterns, md.Preserve...)
		}
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(oldDir, filepath.FromSlash(pattern)))
		if err != nil {
			return fmt.Errorf("preserve pattern %q: %w", pattern, err)
		}
		for _, match := range matches {
			rel, err := filepath.Rel(oldDir, match)
			if err != nil {
				return err
			}
			if err := fs.OverlayDir(match, filepath.Join(newDir, rel)); err != nil {
				return err
			}
		}
	}
	return nil
}

// swap moves the old install aside, puts the new one in place and only
// then deletes the old one, restoring it if the move fails.
func swap(dir, staged string) error {
	if err := os.MkdirAll(paths.StagingDir, 0755); err != nil {
		return err
	}
	holder, err := os.MkdirTemp(paths.StagingDir, previousPrefix)
	if err != nil {
		return err
	}
	defer os.RemoveAll(holder)
	previous := filepath.Join(holder, previousInstallD)
	if err := os.Rename(dir, previous); err != nil {
		return fmt.Errorf("move old install aside: %w", err)
	}
	if err := os.Rename(staged, dir); err != nil {
		if restoreErr := os.Rename(previous, dir); restoreErr != nil {
			return fmt.Errorf("install new version: %v; restoring the old one also failed: %v (it is in %s)", err, restoreErr, previous)
		}
		return fmt.Errorf("install new version: %w", err)
	}
	return nil
}

// Self replaces exePath with the server's yapl binary when the server's is
// newer in at least one part and older in none.
func Self(c *client.Client, local kernel.Versions, exePath string, opts Options) (bool, error) {
	info, err := c.Info()
	if err != nil {
		return false, err
	}
	if info.Platform != "" && info.Platform != api.Platform() {
		return false, fmt.Errorf("the server's yapl is built for %s, this machine is %s", info.Platform, api.Platform())
	}
	newer, msg := NewerBinary(local, kernel.Versions{Kernel: info.Kernel, Modules: info.Modules})
	if !newer {
		fmt.Fprintf(opts.out(), "-> %s\n", msg)
		return false, nil
	}
	tmp := exePath + downloadSuffix
	if err := c.DownloadBinary(tmp, opts.Report); err != nil {
		return false, err
	}
	// Renaming over a running executable is safe on Linux: the running
	// process keeps the old inode.
	if err := os.Rename(tmp, exePath); err != nil {
		os.Remove(tmp)
		return false, fmt.Errorf("replace %s: %w", exePath, err)
	}
	fmt.Fprintf(opts.out(), "✅ Upgraded yapl: %s\n", msg)
	return true, nil
}

// Catalog adds the compiled-in catalog's pins to runner.json (existing
// entries win) and adopts its defaults and version.
func Catalog(runnerPath string) ([]string, error) {
	local, err := config.LoadGlobal(runnerPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	pinned := catalog.Default()
	merged, added := config.MergeGlobal(local, pinned)
	merged.Defaults = pinned.Defaults
	merged.CatalogVersion = pinned.CatalogVersion
	return added, config.SaveGlobal(runnerPath, merged)
}
