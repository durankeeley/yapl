package archive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
	"yapl/internal/fs"
)

// downloadTimeout caps how long a single HTTP download may take.
const downloadTimeout = 30 * time.Minute

var httpClient = &http.Client{Timeout: downloadTimeout}

// Archive represents a local or remote compressed tarball.
type Archive struct {
	Source string
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

// Package creates a new compressed bundle from a source directory.
func Package(sourceDir, format string) error {
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("application directory '%s' not found", sourceDir)
	}

	extension, err := getExtensionForFormat(format)
	if err != nil {
		return err
	}

	packageName := filepath.Base(sourceDir) + extension
	fmt.Printf("-> Creating %s bundle '%s'...\n", strings.ToUpper(format), packageName)
	if err := createBundle(packageName, sourceDir, filepath.Dir(sourceDir), format); err != nil {
		return fmt.Errorf("failed to create package: %w", err)
	}
	fmt.Println("\n✅ Packaging complete!")
	fmt.Printf("➡️ Distribute '%s' to other machines.\n", packageName)
	return nil
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

// PackageFromDir creates a compressed bundle from all contents of dir,
// using archive entry names relative to dir itself (not its parent).
func PackageFromDir(dir, archiveName, format string) error {
	extension, err := getExtensionForFormat(format)
	if err != nil {
		return err
	}
	packageName := archiveName + extension
	fmt.Printf("-> Creating %s bundle '%s'...\n", strings.ToUpper(format), packageName)
	if err := createBundle(packageName, dir, dir, format); err != nil {
		return fmt.Errorf("failed to create package: %w", err)
	}
	fmt.Println("\n✅ Packaging complete!")
	fmt.Printf("➡️ Distribute '%s' to other machines.\n", packageName)
	return nil
}

// installBundledDeps processes a _bundle/ directory that may have been extracted
// into targetDir, installing its contents into the deployment root.
func installBundledDeps(targetDir string) error {
	bundleDir := filepath.Join(targetDir, "_bundle")
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
	protonSrc := filepath.Join(bundleDir, "proton")
	if entries, err := os.ReadDir(protonSrc); err == nil {
		for _, e := range entries {
			if err := installDir(filepath.Join(protonSrc, e.Name()), filepath.Join("proton", e.Name())); err != nil {
				return fmt.Errorf("install bundled proton: %w", err)
			}
		}
	}

	// Install dependencies (dxvk, vkd3d, runtime, etc.).
	depsSrc := filepath.Join(bundleDir, "dependencies")
	if depTypes, err := os.ReadDir(depsSrc); err == nil {
		for _, depType := range depTypes {
			versions, _ := os.ReadDir(filepath.Join(depsSrc, depType.Name()))
			for _, ver := range versions {
				src := filepath.Join(depsSrc, depType.Name(), ver.Name())
				dst := filepath.Join("dependencies", depType.Name(), ver.Name())
				if err := installDir(src, dst); err != nil {
					return fmt.Errorf("install bundled %s: %w", depType.Name(), err)
				}
			}
		}
	}

	// Install runner.json only if one does not already exist.
	bundledRunner := filepath.Join(bundleDir, "runner.json")
	if _, err := os.Stat(bundledRunner); err == nil {
		if _, err := os.Stat("runner.json"); os.IsNotExist(err) {
			fmt.Println("-> Installing bundled runner.json...")
			if err := fs.CopyFile(bundledRunner, "runner.json"); err != nil {
				return fmt.Errorf("install bundled runner.json: %w", err)
			}
		}
	}

	return os.RemoveAll(bundleDir)
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
		return resp.Body, nil
	}
	fmt.Printf(" Reading local file %s...\n", a.Source)
	return os.Open(a.Source)
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

func createBundle(bundleName, walkDir, baseDir, format string) error {
	f, err := os.Create(bundleName)
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}
	defer f.Close()

	var compressor io.WriteCloser
	switch format {
	case "gz":
		compressor = gzip.NewWriter(f)
	case "xz":
		compressor, err = xz.NewWriter(f)
	case "zst":
		compressor, err = zstd.NewWriter(f)
	}
	if err != nil {
		return fmt.Errorf("create %s writer: %w", format, err)
	}
	defer compressor.Close()

	tw := tar.NewWriter(compressor)
	defer tw.Close()

	return filepath.Walk(walkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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
		header.Uid, header.Gid = 65534, 65534
		header.Uname, header.Gname = "nobody", "nobody"

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
