// Package pull installs games and their components from a registry, the
// `docker pull` of YAPL. Everything is downloaded into the library's
// staging directory, verified, and moved into place in one rename, so a
// failed pull leaves nothing half-installed.
package pull

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"yapl/internal/archive"
	"yapl/internal/client"
	"yapl/internal/config"
	"yapl/internal/fs"
	"yapl/internal/manifest"
	"yapl/internal/paths"
	"yapl/internal/progress"
	"yapl/internal/registry/api"
)

// ErrAlreadyInstalled is returned when the target directory exists.
var ErrAlreadyInstalled = errors.New("already installed")

const (
	packageStagingPrefix   = "package-"
	componentStagingPrefix = "component-"
)

// Options controls output. Report defaults to a terminal printer on Out.
type Options struct {
	Out    io.Writer
	Report progress.Func
}

func (o Options) withDefaults() Options {
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.Report == nil {
		o.Report = progress.TerminalPrinter(o.Out)
	}
	return o
}

// Result describes a finished pull.
type Result struct {
	Manifest   manifest.Manifest
	Dir        string
	Components []manifest.Component
}

// Pull installs name:tag into the library in the working directory.
func Pull(c *client.Client, name, tag string, opts Options) (Result, error) {
	opts = opts.withDefaults()
	if err := checkServer(c); err != nil {
		if errors.Is(err, errLegacyServer) {
			return pullLegacy(c, name, opts)
		}
		return Result{}, err
	}
	m, err := c.Manifest(name, tag)
	if err != nil {
		return Result{}, err
	}
	dir := paths.AppDir(paths.TypeDir(m.Type), m.Name)
	if _, err := os.Stat(dir); err == nil {
		return Result{}, alreadyInstalled(dir, m.Name)
	}

	comps, err := EnsureComponents(c, m, opts)
	if err != nil {
		return Result{}, err
	}
	staged, cleanup, err := Fetch(c, m, opts)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return Result{}, err
	}
	if err := os.Rename(staged, dir); err != nil {
		return Result{}, fmt.Errorf("install %s: %w", dir, err)
	}
	if err := RecordInstall(dir, m, c); err != nil {
		return Result{}, err
	}
	fmt.Fprintf(opts.Out, "✅ Installed %s:%s into %s\n", m.Name, m.Metadata.Version, dir)
	return Result{Manifest: m, Dir: dir, Components: comps}, nil
}

// RecordInstall writes installed.json for a package pulled from c.
func RecordInstall(dir string, m manifest.Manifest, c *client.Client) error {
	return manifest.WriteInstalled(dir, manifest.Installed{
		Name: m.Name, Type: m.Type, Version: m.Metadata.Version, Server: c.Server(), SHA256: m.Archive.SHA256,
	})
}

var errLegacyServer = errors.New("server predates the v1 API")

func checkServer(c *client.Client) error {
	info, err := c.Info()
	switch {
	case errors.Is(err, client.ErrNotFound), errors.Is(err, client.ErrUnauthorized):
		// Servers before API v1 answer /v1/info with 404 (or 401 when
		// password protected, because they guard every path).
		return errLegacyServer
	case err != nil:
		return err
	case info.APIVersion != api.APIVersion:
		return fmt.Errorf("server speaks API v%d, this yapl speaks v%d; run 'yapl upgrade self' or upgrade the server", info.APIVersion, api.APIVersion)
	}
	return nil
}

func alreadyInstalled(dir, name string) error {
	return fmt.Errorf("%w: %s exists; run 'yapl upgrade %s' to update it", ErrAlreadyInstalled, dir, name)
}

// EnsureComponents installs every component m needs that this machine
// lacks, registering new ones in runner.json. Bundled packages carry their
// own components and are skipped.
func EnsureComponents(c *client.Client, m manifest.Manifest, opts Options) ([]manifest.Component, error) {
	if m.Bundled || len(m.Components) == 0 {
		return nil, nil
	}
	opts = opts.withDefaults()
	global, err := config.LoadGlobal(paths.RunnerConfig)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", paths.RunnerConfig, err)
	}
	var installed []manifest.Component
	changed := false
	for _, comp := range m.Components {
		added, registered, err := ensureComponent(c, &global, comp, opts)
		if err != nil {
			return installed, err
		}
		if added {
			installed = append(installed, comp)
		}
		changed = changed || registered
	}
	if changed {
		if err := config.SaveGlobal(paths.RunnerConfig, global); err != nil {
			return installed, fmt.Errorf("write %s: %w", paths.RunnerConfig, err)
		}
	}
	return installed, nil
}

func ensureComponent(c *client.Client, global *config.Global, comp manifest.Component, opts Options) (added, registered bool, err error) {
	info, known := global.VersionInfoFor(comp.Kind, comp.Version)
	dir := paths.Component(comp.Kind, comp.Version)
	if known && info.Path != "" {
		dir = info.Path
	}
	if fs.DirExistsAndIsNotEmpty(dir) {
		if known {
			return false, false, nil
		}
		cm, err := c.ComponentManifest(comp.Kind, comp.Version)
		if err != nil {
			return false, false, nil
		}
		return false, register(c, global, cm), nil
	}

	cm, err := c.ComponentManifest(comp.Kind, comp.Version)
	if errors.Is(err, client.ErrNotFound) {
		if known && info.URL != "" {
			fmt.Fprintf(opts.Out, "-> Server doesn't have %s; it will be downloaded from %s on first run\n", comp, info.URL)
			return false, false, nil
		}
		return false, false, fmt.Errorf("component %s is not on the server and runner.json has no download URL for it", comp)
	}
	if err != nil {
		return false, false, err
	}
	fmt.Fprintf(opts.Out, "-> Installing component %s\n", comp)
	if err := installComponent(c, cm, dir, opts); err != nil {
		return false, false, err
	}
	return true, !known && register(c, global, cm), nil
}

func register(c *client.Client, global *config.Global, cm manifest.ComponentManifest) bool {
	info := cm.VersionInfo
	info.URL = c.ComponentURL(cm.Kind, cm.Version)
	global.Register(cm.Kind, cm.Version, info)
	return true
}

func installComponent(c *client.Client, cm manifest.ComponentManifest, dir string, opts Options) error {
	staging, err := newStaging(componentStagingPrefix)
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	body, size, err := c.OpenComponentArchive(cm.Kind, cm.Version)
	if err != nil {
		return err
	}
	defer body.Close()
	reader := progress.NewReader(body, cm.Component().String(), size, opts.Report)
	if err := archive.ExtractVerified(reader, cm.Archive.Format, staging, cm.Archive.SHA256, cm.Archive.SizeBytes); err != nil {
		return fmt.Errorf("component %s: %w", cm.Component(), err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return err
	}
	return os.Rename(staging, dir)
}

// Fetch downloads and verifies m into the staging directory and returns
// the extracted <name> directory plus a cleanup function. Bundled
// dependencies are installed into the library on the way.
func Fetch(c *client.Client, m manifest.Manifest, opts Options) (string, func(), error) {
	opts = opts.withDefaults()
	staging, err := newStaging(packageStagingPrefix)
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(staging) }

	body, size, err := c.OpenArchive(m.Name, m.Metadata.Version)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	defer body.Close()
	wantSize := m.Archive.SizeBytes
	if m.Archive.SHA256 == "" {
		fmt.Fprintf(opts.Out, "warning: %s:%s has no checksum (copied into the server by hand); installing it unverified\n", m.Name, m.Metadata.Version)
		wantSize = -1
	}
	reader := progress.NewReader(body, m.Name, size, opts.Report)
	if err := archive.ExtractVerified(reader, m.Archive.Format, staging, m.Archive.SHA256, wantSize); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("%s:%s: %w", m.Name, m.Metadata.Version, err)
	}
	if err := archive.InstallBundle(staging); err != nil {
		cleanup()
		return "", func() {}, err
	}
	staged := filepath.Join(staging, m.Name)
	if info, err := os.Stat(staged); err != nil || !info.IsDir() {
		cleanup()
		return "", func() {}, fmt.Errorf("%s:%s: archive does not contain a %s/ directory", m.Name, m.Metadata.Version, m.Name)
	}
	return staged, cleanup, nil
}

func newStaging(prefix string) (string, error) {
	if err := os.MkdirAll(paths.StagingDir, 0755); err != nil {
		return "", err
	}
	return os.MkdirTemp(paths.StagingDir, prefix)
}

func pullLegacy(c *client.Client, name string, opts Options) (Result, error) {
	fmt.Fprintln(opts.Out, "-> This is an older YAPL server without the v1 API; installing without checksum verification")
	entries, err := c.List()
	if err != nil {
		return Result{}, err
	}
	var entry *client.PackageEntry
	for i := range entries {
		if entries[i].Name == name {
			entry = &entries[i]
		}
	}
	if entry == nil {
		return Result{}, fmt.Errorf("%w: package %q", client.ErrNotFound, name)
	}
	typeDir := paths.TypeDir(entry.Type)
	dir := paths.AppDir(typeDir, name)
	if _, err := os.Stat(dir); err == nil {
		return Result{}, alreadyInstalled(dir, name)
	}
	staging, err := newStaging(packageStagingPrefix)
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(staging)
	archivePath, _, err := c.Download(name, staging, opts.Out)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(typeDir, 0755); err != nil {
		return Result{}, err
	}
	if err := archive.Unpackage(typeDir, []string{archivePath}); err != nil {
		return Result{}, err
	}
	m := manifest.Manifest{Name: name, Type: entry.Type, Metadata: manifest.Metadata{Title: name, Version: manifest.LegacyVersion}}
	if err := RecordInstall(dir, m, c); err != nil {
		return Result{}, err
	}
	return Result{Manifest: m, Dir: dir}, nil
}
