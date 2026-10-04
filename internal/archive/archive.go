package archive

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"yapl/internal/config"
	"yapl/internal/defaults"
	"yapl/internal/fs"
	"yapl/internal/paths"
	"yapl/internal/progress"
)

var httpClient = &http.Client{Timeout: defaults.DownloadTimeout}

// Archive represents a local or remote compressed tarball.
type Archive struct {
	Source string
	// Progress receives download updates for HTTP sources; nil prints them
	// to stdout.
	Progress progress.Func
}

// Extract unpacks the archive to a destination path.
func (a *Archive) Extract(destPath string, stripTopLevelDir bool) error {
	if a.Source == "" {
		return errors.New("archive source cannot be empty")
	}

	stream, err := a.open()
	if err != nil {
		return err
	}
	defer stream.Close()

	decompressedReader, err := getDecompressedReader(stream, a.Source)
	if err != nil {
		return err
	}
	return extractTar(decompressedReader, destPath, stripTopLevelDir)
}

// Result describes an archive written by Package or PackageFromDir.
type Result struct {
	Path      string
	Format    string
	SHA256    string
	SizeBytes int64
}

// Package compresses sourceDir (keeping its name as the top-level directory)
// into outputDir/<name>.tar.<format>.
func Package(sourceDir, outputDir, format string) (Result, error) {
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return Result{}, fmt.Errorf("application directory '%s' not found", sourceDir)
	}
	return writeArchive(filepath.Base(sourceDir), sourceDir, filepath.Dir(sourceDir), outputDir, format)
}

// PackageFromDir compresses every entry of dir, with archive names relative
// to dir itself, into outputDir/<archiveName>.tar.<format>.
func PackageFromDir(dir, archiveName, outputDir, format string) (Result, error) {
	return writeArchive(archiveName, dir, dir, outputDir, format)
}

func writeArchive(name, walkDir, baseDir, outputDir, format string) (Result, error) {
	extension, err := getExtensionForFormat(format)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return Result{}, fmt.Errorf("create output dir: %w", err)
	}
	res := Result{Path: filepath.Join(outputDir, name+extension), Format: format}
	fmt.Printf("-> Creating %s bundle '%s'...\n", strings.ToUpper(format), res.Path)
	res.SHA256, res.SizeBytes, err = createBundle(res.Path, walkDir, baseDir, format)
	if err != nil {
		return Result{}, fmt.Errorf("failed to create package: %w", err)
	}
	fmt.Println("\n✅ Packaging complete!")
	fmt.Printf("➡️ Distribute '%s' to other machines.\n", res.Path)
	return res, nil
}

// Unpackage extracts one or more archives into a target directory.
// If an extracted archive contains a _bundle/ directory, its contents are installed
// into the deployment root (proton/, dependencies/) and the _bundle/ dir is removed.
func Unpackage(targetDir string, archivePaths []string) error {
	if len(archivePaths) == 0 {
		return errors.New("no archive files provided")
	}
	fmt.Println("📦 Starting unpackaging process...")
	for _, archivePath := range archivePaths {
		fmt.Printf("-> Unpackaging '%s'...\n", archivePath)
		nameWithoutExt, ok := trimArchiveSuffix(filepath.Base(archivePath))
		if !ok {
			fmt.Fprintf(os.Stderr, "⚠️  Skipping '%s': unrecognized archive extension.\n", archivePath)
			continue
		}

		// Guard: skip if the destination folder already exists.
		destPath := filepath.Join(targetDir, nameWithoutExt)
		if _, err := os.Stat(destPath); err == nil {
			fmt.Fprintf(os.Stderr, "⚠️  Skipping '%s': destination '%s' already exists.\n", archivePath, destPath)
			continue
		}

		// Extract to the parent targetDir; the archive already contains the named subdirectory.
		// Cleanup of partial extraction on failure is the caller's responsibility.
		ar := &Archive{Source: archivePath}
		if err := ar.Extract(targetDir, false); err != nil {
			return fmt.Errorf("failed to unpackage '%s': %w", archivePath, err)
		}
		fmt.Printf("✅ Successfully unpackaged to '%s'\n", destPath)

		// Install bundled dependencies if the archive contained a _bundle/ dir.
		if err := installBundledDeps(targetDir); err != nil {
			return fmt.Errorf("bundle install failed: %w", err)
		}
	}
	fmt.Println("\n✨ Unpackaging complete!")
	return nil
}

// installBundledDeps processes a _bundle/ directory that may have been extracted
// into targetDir, installing its contents into the deployment root.
func installBundledDeps(targetDir string) error {
	bundleDir := filepath.Join(targetDir, paths.BundleDir)
	if _, err := os.Stat(bundleDir); os.IsNotExist(err) {
		return nil
	}

	installDir := func(src, dst string) error {
		if fs.DirExistsAndIsNotEmpty(dst) {
			fmt.Printf("-> Skipping bundled '%s': already present\n", filepath.Base(dst))
			return nil
		}
		fmt.Printf("-> Installing bundled '%s'...\n", filepath.Base(dst))
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		return fs.CopyDir(src, dst)
	}

	// Install proton versions.
	protonSrc := filepath.Join(bundleDir, paths.ProtonDir)
	if entries, err := os.ReadDir(protonSrc); err == nil {
		for _, e := range entries {
			if err := installDir(filepath.Join(protonSrc, e.Name()), paths.Proton(e.Name())); err != nil {
				return fmt.Errorf("install bundled proton: %w", err)
			}
		}
	}

	// Install dependencies (dxvk, vkd3d, runtime, etc.).
	depsSrc := filepath.Join(bundleDir, paths.DependenciesDir)
	if depTypes, err := os.ReadDir(depsSrc); err == nil {
		for _, depType := range depTypes {
			versions, _ := os.ReadDir(filepath.Join(depsSrc, depType.Name()))
			for _, ver := range versions {
				src := filepath.Join(depsSrc, depType.Name(), ver.Name())
				dst := paths.Dependency(depType.Name(), ver.Name())
				if err := installDir(src, dst); err != nil {
					return fmt.Errorf("install bundled %s: %w", depType.Name(), err)
				}
			}
		}
	}

	bundledRunner := filepath.Join(bundleDir, paths.RunnerConfig)
	if _, err := os.Stat(bundledRunner); err == nil {
		if err := mergeBundledRunner(bundledRunner, paths.RunnerConfig); err != nil {
			return err
		}
	}

	return os.RemoveAll(bundleDir)
}

// mergeBundledRunner adds the bundle's version entries to the local runner
// config so later pulls can reference the components they installed.
func mergeBundledRunner(bundledPath, localPath string) error {
	bundled, err := config.LoadGlobal(bundledPath)
	if err != nil {
		return fmt.Errorf("read bundled runner.json: %w", err)
	}
	local, err := config.LoadGlobal(localPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read runner.json: %w", err)
	}
	merged, added := config.MergeGlobal(local, bundled)
	for _, key := range added {
		fmt.Printf("-> Registered bundled %s in runner.json\n", key)
	}
	if err := config.SaveGlobal(localPath, merged); err != nil {
		return fmt.Errorf("write runner.json: %w", err)
	}
	return nil
}

func (a *Archive) open() (io.ReadCloser, error) {
	if strings.HasPrefix(a.Source, "http") {
		fmt.Printf(" Downloading from %s...\n", a.Source)
		resp, err := httpClient.Get(a.Source)
		if err != nil {
			return nil, fmt.Errorf("http get: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("download failed: %s", resp.Status)
		}
		report := a.Progress
		if report == nil {
			report = progress.TerminalPrinter(os.Stdout)
		}
		label := path.Base(resp.Request.URL.Path)
		return readCloser{progress.NewReader(resp.Body, label, resp.ContentLength, report), resp.Body}, nil
	}
	fmt.Printf(" Reading local file %s...\n", a.Source)
	return os.Open(a.Source)
}

type readCloser struct {
	io.Reader
	io.Closer
}

func getDecompressedReader(r io.Reader, sourceFilename string) (io.Reader, error) {
	switch {
	case strings.HasSuffix(sourceFilename, ".tar.gz"):
		return gzip.NewReader(r)
	case strings.HasSuffix(sourceFilename, ".tar.xz"):
		return xz.NewReader(r)
	case strings.HasSuffix(sourceFilename, ".tar.zst"):
		return zstd.NewReader(r)
	case strings.HasSuffix(sourceFilename, ".tar"):
		return r, nil
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", sourceFilename)
	}
}

func extractTar(r io.Reader, destPath string, stripTopLevelDir bool) error {
	tr := tar.NewReader(r)
	cleanDest := filepath.Clean(destPath)
	fmt.Println(" Extracting archive...")
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		var target string
		if stripTopLevelDir {
			parts := strings.Split(hdr.Name, string(filepath.Separator))
			if len(parts) <= 1 {
				continue
			}
			relativePath := strings.Join(parts[1:], string(filepath.Separator))
			target = filepath.Join(cleanDest, relativePath)
		} else {
			target = filepath.Join(cleanDest, hdr.Name)
		}

		target = filepath.Clean(target)

		// Reject path traversal: target must be the dest dir itself or a child of it.
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(filepath.Separator)) {
			return fmt.Errorf("archive contains invalid path: %s", hdr.Name)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("mkdirAll failed for %s: %w", filepath.Dir(target), err)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("mkdir dir: %w", err)
			}
		case tar.TypeReg:
			out, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("create file: %w", err)
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return fmt.Errorf("copy file: %w", err)
			}
		case tar.TypeSymlink:
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return fmt.Errorf("create symlink: %w", err)
			}
		}
	}
}

// createBundle writes the archive, hashing and counting the compressed bytes
// as they are written so the file never needs re-reading.
func createBundle(bundlePath, walkDir, baseDir, format string) (sha string, size int64, err error) {
	f, err := os.Create(bundlePath)
	if err != nil {
		return "", 0, fmt.Errorf("create bundle: %w", err)
	}
	hasher := sha256.New()
	counter := &countingWriter{}
	sink := io.MultiWriter(f, hasher, counter)

	var compressor io.WriteCloser
	switch format {
	case "gz":
		compressor = gzip.NewWriter(sink)
	case "xz":
		compressor, err = xz.NewWriter(sink)
	case "zst":
		compressor, err = zstd.NewWriter(sink)
	}
	if err != nil {
		f.Close()
		return "", 0, fmt.Errorf("create %s writer: %w", format, err)
	}

	tw := tar.NewWriter(compressor)
	walkErr := writeTree(tw, walkDir, baseDir, bundlePath)
	for _, closeErr := range []error{tw.Close(), compressor.Close(), f.Close()} {
		if walkErr == nil {
			walkErr = closeErr
		}
	}
	if walkErr != nil {
		os.Remove(bundlePath)
		return "", 0, walkErr
	}
	return hex.EncodeToString(hasher.Sum(nil)), counter.n, nil
}

// Archive entries are owned by "nobody" so extracting on another machine
// never recreates the packager's user and group IDs.
const (
	anonymousID   = 65534
	anonymousName = "nobody"
)

type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

// writeTree adds walkDir to tw with names relative to baseDir, skipping the
// archive being written in case it lives inside walkDir.
func writeTree(tw *tar.Writer, walkDir, baseDir, skip string) error {
	skipAbs, _ := filepath.Abs(skip)
	return filepath.Walk(walkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if abs, _ := filepath.Abs(path); abs == skipAbs {
			return nil
		}
		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}
		header.Name, err = filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			header.Linkname, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		header.Uid, header.Gid = anonymousID, anonymousID
		header.Uname, header.Gname = anonymousName, anonymousName

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			if _, err := io.Copy(tw, file); err != nil {
				return err
			}
		}
		return nil
	})
}

func getExtensionForFormat(format string) (string, error) {
	switch format {
	case "gz":
		return ".tar.gz", nil
	case "xz":
		return ".tar.xz", nil
	case "zst":
		return ".tar.zst", nil
	default:
		return "", fmt.Errorf("unsupported package format: %s. Use 'gz', 'xz', or 'zst'", format)
	}
}

func trimArchiveSuffix(filename string) (string, bool) {
	suffixes := []string{".tar.gz", ".tar.xz", ".tar.zst"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(filename, suffix) {
			return strings.TrimSuffix(filename, suffix), true
		}
	}
	return "", false
}
