// Package publish pushes a game and any components the registry is missing,
// the `docker push` of YAPL.
package publish

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"yapl/internal/app"
	"yapl/internal/archive"
	"yapl/internal/client"
	"yapl/internal/config"
	"yapl/internal/fs"
	"yapl/internal/manifest"
	"yapl/internal/paths"
	"yapl/internal/progress"
	"yapl/internal/registry/api"
)

// Options controls a publish.
type Options struct {
	Version   string
	Format    string
	CreatedBy string
}

// Result reports what was uploaded and what the server already had.
type Result struct {
	Manifest manifest.Manifest
	Uploaded []manifest.Component
	Skipped  []manifest.Component
}

// Publish packages a, uploads the components the server lacks, then the
// package itself. Progress and status lines go to w.
func Publish(a *app.App, c *client.Client, opts Options, w io.Writer) (Result, error) {
	info, err := c.Info()
	if err != nil {
		return Result{}, fmt.Errorf("server does not support publishing (needs the v1 API): %w", err)
	}
	if info.APIVersion != api.APIVersion {
		return Result{}, fmt.Errorf("server speaks API v%d, this yapl speaks v%d; upgrade the older side", info.APIVersion, api.APIVersion)
	}
	if !info.PushEnabled {
		return Result{}, fmt.Errorf("push is disabled on this server; start it with 'yapl serve --allow-push'")
	}

	staging, err := os.MkdirTemp("", "yapl-push-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(staging)

	m, err := a.Package(app.PackageOptions{Format: opts.Format, OutputDir: staging, Version: opts.Version, Yes: true, CreatedBy: opts.CreatedBy})
	if err != nil {
		return Result{}, err
	}
	res := Result{Manifest: m}
	report := progress.TerminalPrinter(w)

	for _, comp := range m.Components {
		has, err := c.HasComponent(comp.Kind, comp.Version)
		if err != nil {
			return res, err
		}
		if has {
			fmt.Fprintf(w, "-> Server already has %s\n", comp)
			res.Skipped = append(res.Skipped, comp)
			continue
		}
		if err := pushComponent(a, c, comp, report, w); err != nil {
			return res, err
		}
		res.Uploaded = append(res.Uploaded, comp)
	}

	cover := ""
	if matches, _ := filepath.Glob(manifest.CoverSidecarPath(staging, a.Name, ".*")); len(matches) > 0 {
		cover = matches[0]
	}
	fmt.Fprintf(w, "-> Uploading %s:%s\n", m.Name, m.Metadata.Version)
	if err := c.PushPackage(m, filepath.Join(staging, m.Archive.File), cover, report); err != nil {
		return res, err
	}
	fmt.Fprintf(w, "✅ Published %s:%s (%d components uploaded, %d already on the server)\n", m.Name, m.Metadata.Version, len(res.Uploaded), len(res.Skipped))
	return res, nil
}

func pushComponent(a *app.App, c *client.Client, comp manifest.Component, report progress.Func, w io.Writer) error {
	info, _ := a.GlobalConfig.VersionInfoFor(comp.Kind, comp.Version)
	dir := componentDir(comp.Kind, comp.Version, info)
	if !fs.DirExistsAndIsNotEmpty(dir) {
		return fmt.Errorf("component %s is not installed at %s; run 'yapl setup' for this game first", comp, dir)
	}
	fmt.Fprintf(w, "-> Uploading component %s from %s\n", comp, dir)
	cm := manifest.ComponentManifest{
		SchemaVersion: manifest.SchemaVersion,
		Kind:          comp.Kind,
		Version:       comp.Version,
		VersionInfo:   manifest.PortableVersionInfo(info),
	}
	return c.PushComponent(cm, func(out io.Writer) error {
		return archive.Write(out, dir, dir, manifest.ComponentFormat)
	}, report)
}

// componentDir is where a component lives on this machine: its custom path
// when runner.json gives one, otherwise the library directory.
func componentDir(kind, version string, info config.VersionInfo) string {
	if info.Path != "" {
		return info.Path
	}
	return paths.Component(kind, version)
}
