package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"yapl/internal/config"
	"yapl/internal/progress"
)

// --- Helpers ---

func makeTarGz(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	archPath := filepath.Join(dir, "test.tar.gz")
	f, err := os.Create(archPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	return archPath
}

// --- Extract ---

func TestExtract_LocalTarGz(t *testing.T) {
	// Given a valid local .tar.gz archive containing a single file
	src := t.TempDir()
	dst := t.TempDir()
	archPath := makeTarGz(t, src, map[string]string{"file.txt": "hello"})

	// When Extract is called without stripping the top-level directory
	ar := &Archive{Source: archPath}
	if err := ar.Extract(dst, false); err != nil {
		t.Fatalf("Extract returned an unexpected error: %v", err)
	}

	// Then the file exists at the expected path with correct content
	got, err := os.ReadFile(filepath.Join(dst, "file.txt"))
	if err != nil {
		t.Fatalf("expected file not found after extraction: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("content mismatch: got %q want %q", got, "hello")
	}
}

func TestExtract_StripTopLevelDir(t *testing.T) {
	// Given a .tar.gz archive where all entries share a top-level directory prefix
	src := t.TempDir()
	dst := t.TempDir()
	archPath := makeTarGz(t, src, map[string]string{"topdir/file.txt": "stripped"})

	// When Extract is called with stripTopLevelDir=true
	ar := &Archive{Source: archPath}
	if err := ar.Extract(dst, true); err != nil {
		t.Fatalf("Extract returned an unexpected error: %v", err)
	}

	// Then the file is placed directly in the destination without the top-level folder
	got, err := os.ReadFile(filepath.Join(dst, "file.txt"))
	if err != nil {
		t.Fatalf("stripped file not found: %v", err)
	}
	if string(got) != "stripped" {
		t.Fatalf("content mismatch: got %q want %q", got, "stripped")
	}
}

func TestExtract_PathTraversalRejected(t *testing.T) {
	// Given a malicious archive containing a path-traversal entry (../../escape.txt)
	src := t.TempDir()
	dst := t.TempDir()

	archPath := filepath.Join(src, "evil.tar.gz")
	f, err := os.Create(archPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	content := "evil"
	hdr := &tar.Header{Name: "../../escape.txt", Mode: 0644, Size: int64(len(content))}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte(content))
	tw.Close()
	gw.Close()
	f.Close()

	// When Extract is called
	ar := &Archive{Source: archPath}
	err = ar.Extract(dst, false)

	// Then an error is returned and no files are written outside the destination
	if err == nil {
		t.Fatal("expected a path traversal error but got nil")
	}
	if !strings.Contains(err.Error(), "invalid path") {
		t.Fatalf("expected 'invalid path' in error, got: %v", err)
	}
}

func TestExtract_EmptySource(t *testing.T) {
	// Given an Archive with no source configured
	ar := &Archive{Source: ""}

	// When Extract is called
	err := ar.Extract(t.TempDir(), false)

	// Then it returns an error describing the missing source
	if err == nil {
		t.Fatal("expected an error for an empty source but got nil")
	}
}

func TestExtract_UnsupportedFormat(t *testing.T) {
	// Given an archive file with an unsupported extension
	d := t.TempDir()
	f := filepath.Join(d, "test.rar")
	os.WriteFile(f, []byte("data"), 0644)

	// When Extract is called
	ar := &Archive{Source: f}
	err := ar.Extract(t.TempDir(), false)

	// Then it returns an error about the unsupported format
	if err == nil {
		t.Fatal("expected an error for an unsupported format but got nil")
	}
}

// --- Package and Unpackage ---

func TestPackageAndUnpackage_RoundTrip(t *testing.T) {
	// Given a game directory containing an executable
	src := t.TempDir()
	unpackDst := t.TempDir()

	gameDir := filepath.Join(src, "mygame")
	if err := os.Mkdir(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.exe"), []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	if err := os.Chdir(src); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	// When Package is called
	if _, err := Package("mygame", ".", "gz"); err != nil {
		t.Fatalf("Package returned an unexpected error: %v", err)
	}
	archivePath := filepath.Join(src, "mygame.tar.gz")
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("expected archive file was not created: %v", err)
	}

	// And Unpackage is called with the resulting archive
	if err := Unpackage(unpackDst, []string{archivePath}); err != nil {
		t.Fatalf("Unpackage returned an unexpected error: %v", err)
	}

	// Then the game directory is recreated with the original content intact
	got, err := os.ReadFile(filepath.Join(unpackDst, "mygame", "game.exe"))
	if err != nil {
		t.Fatalf("expected game.exe not found after unpackage: %v", err)
	}
	if string(got) != "binary" {
		t.Fatalf("content mismatch after round-trip: got %q", got)
	}
}

func TestUnpackage_SkipsExistingDestination(t *testing.T) {
	// Given an archive named 'mygame.tar.gz' and a destination that already contains 'mygame'
	src := t.TempDir()
	dst := t.TempDir()

	// Build the archive manually so it is named 'mygame.tar.gz'
	archPath := filepath.Join(src, "mygame.tar.gz")
	f, err := os.Create(archPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	content := "original"
	hdr := &tar.Header{Name: "mygame/file.txt", Mode: 0644, Size: int64(len(content))}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte(content))
	tw.Close()
	gw.Close()
	f.Close()

	// And the destination already contains the 'mygame' directory
	existingDir := filepath.Join(dst, "mygame")
	if err := os.Mkdir(existingDir, 0755); err != nil {
		t.Fatal(err)
	}

	// When Unpackage is called
	if err := Unpackage(dst, []string{archPath}); err != nil {
		t.Fatalf("Unpackage returned an unexpected error: %v", err)
	}

	// Then the existing directory is left untouched (no files extracted into it)
	entries, _ := os.ReadDir(existingDir)
	if len(entries) != 0 {
		t.Fatalf("expected the existing directory to remain empty (not overwritten), but found %d entries", len(entries))
	}
}

// --- PackageFromDir (PRD-14 bundle) ---

func TestPackageFromDir_ArchiveContainsAllStagedItems(t *testing.T) {
	// Given a staging directory containing a game dir and a _bundle/ dir
	staging := t.TempDir()
	os.MkdirAll(filepath.Join(staging, "Doom"), 0755)
	os.WriteFile(filepath.Join(staging, "Doom", "game.json"), []byte(`{}`), 0644)
	os.MkdirAll(filepath.Join(staging, "_bundle", "proton", "ge9"), 0755)
	os.WriteFile(filepath.Join(staging, "_bundle", "proton", "ge9", "proton"), []byte("#!/bin/sh"), 0755)

	orig, _ := os.Getwd()
	work := t.TempDir()
	os.Chdir(work)
	defer os.Chdir(orig)

	// When PackageFromDir is called
	if _, err := PackageFromDir(staging, "Doom", ".", "gz"); err != nil {
		t.Fatalf("PackageFromDir returned unexpected error: %v", err)
	}

	// Then the archive exists and contains both Doom/ and _bundle/ paths
	archPath := filepath.Join(work, "Doom.tar.gz")
	if _, err := os.Stat(archPath); err != nil {
		t.Fatalf("archive not created: %v", err)
	}
	extractDst := t.TempDir()
	ar := &Archive{Source: archPath}
	if err := ar.Extract(extractDst, false); err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	for _, want := range []string{
		filepath.Join(extractDst, "Doom", "game.json"),
		filepath.Join(extractDst, "_bundle", "proton", "ge9", "proton"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("expected %s in archive but it is missing", want)
		}
	}
}

// --- installBundledDeps ---

func TestInstallBundledDeps_MovesProtonToProtonDir(t *testing.T) {
	// Given a games/ targetDir containing _bundle/proton/ge9/
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	bundledProton := filepath.Join(d, "games", "_bundle", "proton", "ge9")
	os.MkdirAll(bundledProton, 0755)
	os.WriteFile(filepath.Join(bundledProton, "proton"), []byte("#!/bin/sh"), 0755)

	// When installBundledDeps is called
	if err := InstallBundle("games"); err != nil {
		t.Fatalf("installBundledDeps returned unexpected error: %v", err)
	}

	// Then proton/ge9/proton exists at the deployment root
	if _, err := os.Stat(filepath.Join(d, "proton", "ge9", "proton")); err != nil {
		t.Errorf("expected proton/ge9/proton to be installed at deployment root: %v", err)
	}
	// And the _bundle/ directory is removed
	if _, err := os.Stat(filepath.Join(d, "games", "_bundle")); !os.IsNotExist(err) {
		t.Errorf("expected games/_bundle/ to be removed after install")
	}
}

func TestInstallBundledDeps_SkipsAlreadyPresentDependency(t *testing.T) {
	// Given proton/ge9/ already exists AND _bundle/proton/ge9/ is in the archive
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	existingProton := filepath.Join(d, "proton", "ge9")
	os.MkdirAll(existingProton, 0755)
	os.WriteFile(filepath.Join(existingProton, "existing-file"), []byte("existing"), 0644)

	bundledProton := filepath.Join(d, "games", "_bundle", "proton", "ge9")
	os.MkdirAll(bundledProton, 0755)
	os.WriteFile(filepath.Join(bundledProton, "proton"), []byte("#!/bin/sh"), 0755)

	// When installBundledDeps is called
	if err := InstallBundle("games"); err != nil {
		t.Fatalf("installBundledDeps returned unexpected error: %v", err)
	}

	// Then the existing proton dir is untouched (existing-file is still there)
	if _, err := os.Stat(filepath.Join(d, "proton", "ge9", "existing-file")); err != nil {
		t.Errorf("existing proton installation was overwritten: %v", err)
	}
}

func TestInstallBundledDeps_NoopWhenNoBundleDir(t *testing.T) {
	// Given a games/ dir with no _bundle/ subdirectory
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	os.MkdirAll(filepath.Join(d, "games", "Doom"), 0755)

	// When installBundledDeps is called
	err := InstallBundle("games")

	// Then it returns no error and makes no changes
	if err != nil {
		t.Fatalf("expected no error when no bundle dir exists, got: %v", err)
	}
}

func TestUnpackage_InstallsBundledProtonToProtonDir(t *testing.T) {
	// Given an archive that contains a game dir AND a _bundle/proton/ dir
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	archPath := filepath.Join(d, "Doom.tar.gz")
	f, err := os.Create(archPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	for name, content := range map[string]string{
		"Doom/game.json":            `{}`,
		"_bundle/proton/ge9/proton": "#!/bin/sh",
		"_bundle/runner.json":       `{}`,
	} {
		hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(content))}
		tw.WriteHeader(hdr)
		tw.Write([]byte(content))
	}
	tw.Close()
	gw.Close()
	f.Close()

	os.MkdirAll("games", 0755)

	// When Unpackage is called
	if err := Unpackage("games", []string{archPath}); err != nil {
		t.Fatalf("Unpackage returned unexpected error: %v", err)
	}

	// Then the game was extracted
	if _, err := os.Stat("games/Doom/game.json"); err != nil {
		t.Errorf("game.json not found after unpackage: %v", err)
	}
	// And the bundled proton was installed to the deployment root
	if _, err := os.Stat("proton/ge9/proton"); err != nil {
		t.Errorf("bundled proton not installed to deployment root: %v", err)
	}
	// And the _bundle/ dir is gone from games/
	if _, err := os.Stat("games/_bundle"); !os.IsNotExist(err) {
		t.Errorf("_bundle/ dir should be removed after install")
	}
}

func TestUnpackage_LeavesNoBundleDirAfterExtraction(t *testing.T) {
	// Given a normal (non-bundled) archive with no _bundle/ dir
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)

	archPath := filepath.Join(d, "Quake.tar.gz")
	f, err := os.Create(archPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	content := `{}`
	hdr := &tar.Header{Name: "Quake/game.json", Mode: 0644, Size: int64(len(content))}
	tw.WriteHeader(hdr)
	tw.Write([]byte(content))
	tw.Close()
	gw.Close()
	f.Close()

	os.MkdirAll("games", 0755)

	// When Unpackage is called on a normal archive
	if err := Unpackage("games", []string{archPath}); err != nil {
		t.Fatalf("Unpackage returned unexpected error: %v", err)
	}

	// Then no _bundle/ directory exists in games/
	if _, err := os.Stat("games/_bundle"); !os.IsNotExist(err) {
		t.Errorf("_bundle/ dir should not exist for a non-bundled archive")
	}
}

// --- PRD-18: Unpackage error propagation ---

func TestUnpackage_ReturnsErrorWhenExtractionFails(t *testing.T) {
	// Given a corrupt (non-tar) file with a recognised archive extension
	d := t.TempDir()
	corruptFile := filepath.Join(d, "corrupt.tar.gz")
	if err := os.WriteFile(corruptFile, []byte("this is not a valid archive"), 0644); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(d, "output")
	os.MkdirAll(targetDir, 0755)

	// When Unpackage is called
	err := Unpackage(targetDir, []string{corruptFile})

	// Then it returns an error — extraction failures must not be silently swallowed
	if err == nil {
		t.Fatal("expected error when extraction fails, but got nil")
	}
}

// --- trimArchiveSuffix ---

func TestParseName(t *testing.T) {
	cases := []struct {
		input      string
		wantName   string
		wantFormat string
		wantOk     bool
	}{
		{"game.tar.gz", "game", "gz", true},
		{"game.tar.xz", "game", "xz", true},
		{"Need for Speed.tar.zst", "Need for Speed", "zst", true},
		{"game.zip", "", "", false},
		{"game.tar", "", "", false},
		{"bare", "", "", false},
	}
	for _, tc := range cases {
		t.Run("given "+tc.input+" then the name and format are parsed", func(t *testing.T) {
			// When
			name, format, ok := ParseName(tc.input)

			// Then
			if ok != tc.wantOk || name != tc.wantName || format != tc.wantFormat {
				t.Fatalf("ParseName(%q) = %q, %q, %v", tc.input, name, format, ok)
			}
		})
	}
}

// --- getExtensionForFormat ---

func TestGetExtensionForFormat_KnownFormats(t *testing.T) {
	cases := []struct {
		// Given a format name
		format string
		// Then the extension should match
		wantExt string
	}{
		{"gz", ".tar.gz"},
		{"xz", ".tar.xz"},
		{"zst", ".tar.zst"},
	}
	for _, tc := range cases {
		// When getExtensionForFormat is called
		ext, err := getExtensionForFormat(tc.format)
		// Then it returns the correct extension with no error
		if err != nil {
			t.Errorf("Given format %q: unexpected error: %v", tc.format, err)
		}
		if ext != tc.wantExt {
			t.Errorf("Given format %q: got %q want %q", tc.format, ext, tc.wantExt)
		}
	}
}

func TestGetExtensionForFormat_UnknownFormat(t *testing.T) {
	// Given an unsupported compression format name
	// When getExtensionForFormat is called
	_, err := getExtensionForFormat("bz2")
	// Then it returns an error
	if err == nil {
		t.Fatal("expected an error for an unsupported format but got nil")
	}
}

func TestInstallBundledDeps_MergesRunnerJSONIntoExistingFile(t *testing.T) {
	// Given a machine that already pulled one game and has its own runner.json
	d := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(d)
	defer os.Chdir(orig)
	os.WriteFile("runner.json", []byte(`{"proton_versions":{"ge8":{"url":"local-ge8"}}}`), 0644)

	// And a second bundle that needs a different Proton version
	bundleDir := filepath.Join(d, "games", "_bundle")
	os.MkdirAll(bundleDir, 0755)
	os.WriteFile(filepath.Join(bundleDir, "runner.json"),
		[]byte(`{"proton_versions":{"ge8":{"url":"bundled-ge8"},"ge9":{"url":"bundled-ge9"}}}`), 0644)

	// When the bundle is installed
	if err := InstallBundle("games"); err != nil {
		t.Fatalf("installBundledDeps: %v", err)
	}

	// Then the new version is added and the existing one is untouched
	g, err := config.LoadGlobal("runner.json")
	if err != nil {
		t.Fatal(err)
	}
	if g.ProtonVersions["ge9"].URL != "bundled-ge9" {
		t.Errorf("ge9 not merged: %+v", g.ProtonVersions)
	}
	if g.ProtonVersions["ge8"].URL != "local-ge8" {
		t.Errorf("local ge8 overwritten: %+v", g.ProtonVersions["ge8"])
	}
}

func TestExtract_HTTPDownloadReportsProgress(t *testing.T) {
	// Given an archive served over HTTP
	src := makeTarGz(t, t.TempDir(), map[string]string{"top/file.txt": "hello"})
	data, _ := os.ReadFile(src)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Write(data)
	}))
	defer srv.Close()
	var updates []progress.Update
	ar := &Archive{Source: srv.URL + "/dxvk.tar.gz", Progress: func(u progress.Update) { updates = append(updates, u) }}

	// When it is extracted
	if err := ar.Extract(t.TempDir(), true); err != nil {
		t.Fatal(err)
	}

	// Then the final update is complete with the content length as total
	if len(updates) == 0 {
		t.Fatal("no progress reported")
	}
	last := updates[len(updates)-1]
	if !last.Complete || last.Total != int64(len(data)) || last.Label != "dxvk.tar.gz" {
		t.Fatalf("last update = %+v", last)
	}
}

func TestPackage_ReportsChecksumAndWritesToOutputDir(t *testing.T) {
	// Given a game directory and a separate output directory
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "Doom"), 0755)
	os.WriteFile(filepath.Join(src, "Doom", "doom.exe"), []byte("binary"), 0755)
	out := filepath.Join(t.TempDir(), "dist")

	// When the game is packaged
	res, err := Package(filepath.Join(src, "Doom"), out, "zst")

	// Then the archive is in the output dir and the reported facts match the file
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "Doom.tar.zst"))
	if err != nil {
		t.Fatalf("archive not in output dir: %v", err)
	}
	sum := sha256.Sum256(data)
	if res.Path != filepath.Join(out, "Doom.tar.zst") || res.Format != "zst" || res.SizeBytes != int64(len(data)) || res.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("result = %+v, want sha %x size %d", res, sum, len(data))
	}
}

func TestPackage_UnknownFormatWritesNothing(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "Doom"), 0755)
	out := t.TempDir()
	if _, err := Package(filepath.Join(src, "Doom"), out, "rar"); err == nil {
		t.Fatal("expected an error for an unknown format")
	}
	if entries, _ := os.ReadDir(out); len(entries) != 0 {
		t.Fatalf("output dir not empty: %v", entries)
	}
}

func TestWrite_StreamsAnExtractableArchive(t *testing.T) {
	// Given a component directory
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "files", "bin"), 0755)
	os.WriteFile(filepath.Join(src, "files", "bin", "wine64"), []byte("wine"), 0755)
	os.Symlink("wine64", filepath.Join(src, "files", "bin", "wine"))

	// When it is streamed with names relative to itself
	var buf bytes.Buffer
	if err := Write(&buf, src, src, "zst"); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "component.tar.zst")
	os.WriteFile(archivePath, buf.Bytes(), 0644)
	dest := t.TempDir()
	if err := (&Archive{Source: archivePath}).Extract(dest, false); err != nil {
		t.Fatal(err)
	}

	// Then the tree, file modes and symlinks are reproduced at the destination root
	info, err := os.Stat(filepath.Join(dest, "files", "bin", "wine64"))
	if err != nil || info.Mode()&0100 == 0 {
		t.Fatalf("wine64 missing or not executable: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(dest, "files", "bin", "wine")); err != nil || target != "wine64" {
		t.Fatalf("symlink = %q, %v", target, err)
	}
}

func TestWrite_UnknownFormat(t *testing.T) {
	if err := Write(&bytes.Buffer{}, t.TempDir(), t.TempDir(), "rar"); err == nil {
		t.Fatal("expected an error for an unknown format")
	}
}

func gzipTar(t *testing.T, files map[string]string) []byte {
	t.Helper()
	path := makeTarGz(t, t.TempDir(), files)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExtractVerified(t *testing.T) {
	t.Run("given an archive and its checksum when extracted then files appear and verification passes", func(t *testing.T) {
		// Given
		data := gzipTar(t, map[string]string{"Doom/doom.wad": "wad"})
		sum := sha256.Sum256(data)
		dest := t.TempDir()

		// When
		err := ExtractVerified(bytes.NewReader(data), "gz", dest, hex.EncodeToString(sum[:]), int64(len(data)))

		// Then
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(filepath.Join(dest, "Doom", "doom.wad")); string(got) != "wad" {
			t.Fatalf("extracted %q", got)
		}
	})

	t.Run("given a checksum that does not match when extracted then ErrChecksum is returned", func(t *testing.T) {
		data := gzipTar(t, map[string]string{"Doom/doom.wad": "wad"})
		err := ExtractVerified(bytes.NewReader(data), "gz", t.TempDir(), strings.Repeat("0", 64), int64(len(data)))
		if !errors.Is(err, ErrChecksum) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given a size that does not match when extracted then ErrChecksum is returned", func(t *testing.T) {
		data := gzipTar(t, map[string]string{"Doom/doom.wad": "wad"})
		sum := sha256.Sum256(data)
		err := ExtractVerified(bytes.NewReader(data), "gz", t.TempDir(), hex.EncodeToString(sum[:]), int64(len(data)+1))
		if !errors.Is(err, ErrChecksum) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given no checksum when extracted then verification is skipped", func(t *testing.T) {
		data := gzipTar(t, map[string]string{"Doom/doom.wad": "wad"})
		if err := ExtractVerified(bytes.NewReader(data), "gz", t.TempDir(), "", -1); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("given an unknown format then it fails", func(t *testing.T) {
		if err := ExtractVerified(strings.NewReader("x"), "rar", t.TempDir(), "", -1); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestExtract_OverwritesLongerFilesCompletely(t *testing.T) {
	// Given a destination file longer than the archived one
	dest := t.TempDir()
	os.MkdirAll(filepath.Join(dest, "Doom"), 0755)
	os.WriteFile(filepath.Join(dest, "Doom", "config.cfg"), []byte("a much longer old config"), 0644)
	data := gzipTar(t, map[string]string{"Doom/config.cfg": "new"})

	// When the archive is extracted over it
	if err := ExtractVerified(bytes.NewReader(data), "gz", dest, "", -1); err != nil {
		t.Fatal(err)
	}

	// Then no stale bytes remain
	if got, _ := os.ReadFile(filepath.Join(dest, "Doom", "config.cfg")); string(got) != "new" {
		t.Fatalf("file = %q", got)
	}
}

func TestExtractVerified_Zstd(t *testing.T) {
	t.Run("given a zstd archive when extracted and verified then the checksum covers the whole stream", func(t *testing.T) {
		// Given a zstd archive, whose decoder reads ahead in a background goroutine
		src := t.TempDir()
		os.MkdirAll(filepath.Join(src, "Doom"), 0755)
		os.WriteFile(filepath.Join(src, "Doom", "doom.wad"), bytes.Repeat([]byte("wad"), 50000), 0644)
		var buf bytes.Buffer
		if err := Write(&buf, filepath.Join(src, "Doom"), src, "zst"); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(buf.Bytes())

		// When
		err := ExtractVerified(bytes.NewReader(buf.Bytes()), "zst", t.TempDir(), hex.EncodeToString(sum[:]), int64(buf.Len()))

		// Then
		if err != nil {
			t.Fatal(err)
		}
	})
}
