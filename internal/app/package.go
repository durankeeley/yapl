package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"yapl/internal/archive"
	"yapl/internal/config"
	"yapl/internal/defaults"
	"yapl/internal/fs"
	"yapl/internal/manifest"
	"yapl/internal/paths"
	"yapl/internal/version"
)

// steamCoverURL is the Steam CDN's portrait library art; a var so tests can
// point it at a local server.
var steamCoverURL = "https://cdn.cloudflare.steamstatic.com/steam/apps/%s/library_600x900.jpg"

const (
	coverDownloadTimeout = 30 * time.Second
	fetchedCoverName     = "cover.jpg"
	bytesPerGigabyte     = 1 << 30
)

// PackageOptions controls `yapl package`.
type PackageOptions struct {
	Format     string
	BundleDeps bool
	Yes        bool
	OutputDir  string
	Version    string
	FetchCover bool
	CreatedBy  string
}

// Package archives the game directory and writes a manifest (and cover art
// when present) next to the archive. With BundleDeps, Proton and the
// configured dependencies travel inside the archive under _bundle/.
func (a *App) Package(opts PackageOptions) (manifest.Manifest, error) {
	if err := config.ValidateApp(a.AppConfig); err != nil {
		return manifest.Manifest{}, err
	}
	meta, _, err := manifest.LoadMetadata(a.AppDir, a.Name)
	if err != nil {
		return manifest.Manifest{}, err
	}
	if opts.Version != "" {
		meta.Version = opts.Version
	}
	if !version.Valid(meta.Version) {
		return manifest.Manifest{}, fmt.Errorf("invalid package version %q: use MAJOR.MINOR.PATCH, for example 1.2.0", meta.Version)
	}
	if _, err := os.Stat(a.AppDir); err != nil {
		return manifest.Manifest{}, fmt.Errorf("application directory '%s' not found", a.AppDir)
	}
	if opts.FetchCover && meta.CoverPath(a.AppDir) == "" {
		if err := a.fetchSteamCover(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not download cover art: %v\n", err)
		}
	}
	if err := manifest.SaveMetadata(a.AppDir, meta); err != nil {
		return manifest.Manifest{}, fmt.Errorf("save metadata: %w", err)
	}

	format := opts.Format
	if format == "" {
		format = defaults.PackageFormat
	}
	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = "."
	}

	fmt.Println("📦 Starting packaging process...")
	var res archive.Result
	if opts.BundleDeps {
		res, err = a.packageBundled(format, outputDir, opts.Yes)
	} else {
		res, err = archive.Package(a.AppDir, outputDir, format)
	}
	if err != nil {
		return manifest.Manifest{}, err
	}

	m := manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		Name:          a.Name,
		Type:          paths.KindOf(a.Type),
		Metadata:      meta,
		Archive:       manifest.Archive{File: filepath.Base(res.Path), Format: res.Format, SHA256: res.SHA256, SizeBytes: res.SizeBytes},
		Components:    manifest.ComponentsFor(a.AppConfig),
		Bundled:       opts.BundleDeps,
		CreatedBy:     opts.CreatedBy,
	}
	if cover := meta.CoverPath(a.AppDir); cover != "" {
		if err := fs.CopyFile(cover, manifest.CoverSidecarPath(outputDir, a.Name, filepath.Ext(cover))); err != nil {
			return manifest.Manifest{}, fmt.Errorf("copy cover: %w", err)
		}
	}
	sidecar := manifest.SidecarPath(outputDir, a.Name)
	if err := manifest.Write(sidecar, m); err != nil {
		return manifest.Manifest{}, fmt.Errorf("write manifest: %w", err)
	}
	fmt.Printf("-> Wrote %s (version %s, sha256 %s)\n", sidecar, meta.Version, res.SHA256)
	return m, nil
}

func (a *App) fetchSteamCover() error {
	if a.AppConfig.SteamAppID == "" || a.AppConfig.SteamAppID == "0" {
		return fmt.Errorf("no steam_app_id set")
	}
	client := &http.Client{Timeout: coverDownloadTimeout}
	resp, err := client.Get(fmt.Sprintf(steamCoverURL, a.AppConfig.SteamAppID))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("steam returned %s", resp.Status)
	}
	f, err := os.Create(filepath.Join(a.AppDir, fetchedCoverName))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func (a *App) packageBundled(format, outputDir string, yes bool) (archive.Result, error) {
	totalBytes, err := bundleSize(a)
	if err != nil {
		return archive.Result{}, fmt.Errorf("could not estimate bundle size: %w", err)
	}
	sizeGB := float64(totalBytes) / bytesPerGigabyte
	if !yes {
		fmt.Printf("Warning: bundled package will be approximately %.1f GB. Continue? [y/N]: ", sizeGB)
		var resp string
		fmt.Scanln(&resp)
		if resp != "y" && resp != "Y" {
			return archive.Result{}, fmt.Errorf("packaging cancelled")
		}
	} else {
		fmt.Printf("-> Bundling approximately %.1f GB of dependencies...\n", sizeGB)
	}

	stagingDir, err := os.MkdirTemp("", "yapl-bundle-*")
	if err != nil {
		return archive.Result{}, fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	// Copy game dir into staging as <name>/
	if err := fs.CopyDir(a.AppDir, filepath.Join(stagingDir, a.Name)); err != nil {
		return archive.Result{}, fmt.Errorf("copy game dir to staging: %w", err)
	}

	// Copy Proton.
	protonSrc := paths.Proton(a.AppConfig.ProtonVersion)
	if !fs.DirExistsAndIsNotEmpty(protonSrc) {
		return archive.Result{}, fmt.Errorf("proton '%s' not downloaded; run 'setup' before bundling", a.AppConfig.ProtonVersion)
	}
	protonDst := filepath.Join(stagingDir, paths.BundleDir, protonSrc)
	if err := fs.CopyDir(protonSrc, protonDst); err != nil {
		return archive.Result{}, fmt.Errorf("copy proton to bundle: %w", err)
	}

	// Copy DXVK if configured.
	if v := a.AppConfig.Dependencies.DXVKVersion; v != "" {
		src := paths.Dependency(paths.KindDXVK, v)
		if fs.DirExistsAndIsNotEmpty(src) {
			if err := fs.CopyDir(src, filepath.Join(stagingDir, paths.BundleDir, src)); err != nil {
				return archive.Result{}, fmt.Errorf("copy dxvk to bundle: %w", err)
			}
		}
	}

	// Copy VKD3D if configured.
	if v := a.AppConfig.Dependencies.VKD3DVersion; v != "" {
		src := paths.Dependency(paths.KindVKD3D, v)
		if fs.DirExistsAndIsNotEmpty(src) {
			if err := fs.CopyDir(src, filepath.Join(stagingDir, paths.BundleDir, src)); err != nil {
				return archive.Result{}, fmt.Errorf("copy vkd3d to bundle: %w", err)
			}
		}
	}

	// Copy runtime if configured.
	if v := a.AppConfig.RuntimeVersion; v != "" {
		src := paths.Runtime(v)
		if fs.DirExistsAndIsNotEmpty(src) {
			if err := fs.CopyDir(src, filepath.Join(stagingDir, paths.BundleDir, src)); err != nil {
				return archive.Result{}, fmt.Errorf("copy runtime to bundle: %w", err)
			}
		}
	}

	// Copy runner.json so the target machine knows which versions to reference.
	if _, err := os.Stat(paths.RunnerConfig); err == nil {
		if err := fs.CopyFile(paths.RunnerConfig, filepath.Join(stagingDir, paths.BundleDir, paths.RunnerConfig)); err != nil {
			return archive.Result{}, fmt.Errorf("copy runner.json to bundle: %w", err)
		}
	}

	return archive.PackageFromDir(stagingDir, a.Name, outputDir, format)
}

func bundleSize(a *App) (int64, error) {
	var total int64
	dirs := []string{a.AppDir, paths.Proton(a.AppConfig.ProtonVersion)}
	if v := a.AppConfig.Dependencies.DXVKVersion; v != "" {
		dirs = append(dirs, paths.Dependency(paths.KindDXVK, v))
	}
	if v := a.AppConfig.Dependencies.VKD3DVersion; v != "" {
		dirs = append(dirs, paths.Dependency(paths.KindVKD3D, v))
	}
	if v := a.AppConfig.RuntimeVersion; v != "" {
		dirs = append(dirs, paths.Runtime(v))
	}
	for _, p := range dirs {
		_ = filepath.Walk(p, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
	}
	return total, nil
}
