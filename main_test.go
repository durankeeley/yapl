package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// --- Test Helpers ---

// createTestTarball creates a temporary tarball with specified files and format.
func createTestTarball(t *testing.T, files map[string]string, format string) string {
	t.Helper()

	// Create a temporary file for the archive.
	archiveFile, err := os.CreateTemp(t.TempDir(), "test-archive-*.tar."+format)
	if err != nil {
		t.Fatalf("Failed to create temp archive file: %v", err)
	}
	defer archiveFile.Close()

	var compressor io.WriteCloser
	switch format {
	case "gz":
		compressor = gzip.NewWriter(archiveFile)
	case "xz":
		compressor, err = xz.NewWriter(archiveFile)
		if err != nil {
			t.Fatalf("Failed to create xz writer: %v", err)
		}
	case "zst":
		compressor, err = zstd.NewWriter(archiveFile)
		if err != nil {
			t.Fatalf("Failed to create zstd writer: %v", err)
		}
	}
	defer compressor.Close()

	tw := tar.NewWriter(compressor)
	defer tw.Close()

	// Add a top-level directory to mimic real-world archives.
	topLevelDir := "my-test-archive-1.0/"
	if err := tw.WriteHeader(&tar.Header{Name: topLevelDir, Typeflag: tar.TypeDir, Mode: 0755}); err != nil {
		t.Fatalf("Failed to write tar header for top-level dir: %v", err)
	}

	for name, content := range files {
		hdr := &tar.Header{
			Name:     filepath.Join(topLevelDir, name),
			Mode:     0644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("Failed to write tar header for %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("Failed to write content for %s: %v", name, err)
		}
	}

	return archiveFile.Name()
}

// assertFileContains checks if a file at a given path contains the expected content.
// func assertFileContains(t *testing.T, path, expectedContent string) {
// 	t.Helper()
// 	content, err := os.ReadFile(path)
// 	if err != nil {
// 		t.Fatalf("Failed to read file for assertion: %s, error: %v", path, err)
// 	}
// 	if string(content) != expectedContent {
// 		t.Errorf("File content mismatch for %s. Got '%s', want '%s'", path, string(content), expectedContent)
// 	}
// }

// --- Tests ---

func TestNewApp(t *testing.T) {
	// GIVEN: Test data for creating a new app.
	appType := "games"
	appName := "test-game"
	gc := GlobalConfig{}
	ac := AppConfig{}

	// WHEN: We create a new App instance.
	app := NewApp(appType, appName, false, gc, ac)

	// THEN: We verify the fields are set correctly.
	if app.Type != appType {
		t.Errorf("Expected Type to be '%s', got '%s'", appType, app.Type)
	}
	if app.Name != appName {
		t.Errorf("Expected Name to be '%s', got '%s'", appName, app.Name)
	}
	expectedAppDir := filepath.Join(appType, appName)
	if app.AppDir != expectedAppDir {
		t.Errorf("Expected AppDir to be '%s', got '%s'", expectedAppDir, app.AppDir)
	}
	expectedPrefixPath := filepath.Join(expectedAppDir, "prefix")
	if app.PrefixPath != expectedPrefixPath {
		t.Errorf("Expected PrefixPath to be '%s', got '%s'", expectedPrefixPath, app.PrefixPath)
	}
}

func TestUnpackageArchives(t *testing.T) {
	testCases := []struct {
		name          string
		archiveType   string
		archiveFormat string
		archiveFiles  map[string]string
		expectErr     bool
		expectedDir   string
		expectedFile  string
	}{
		{"Unpackage single game gz", "game", "gz", map[string]string{"info.txt": "game data"}, false, "games/test-archive", "games/test-archive/info.txt"},
		{"Unpackage single app xz", "app", "xz", map[string]string{"config.json": "{}"}, false, "apps/test-archive", "apps/test-archive/config.json"},
		{"Skip existing directory", "game", "gz", map[string]string{}, false, "games/test-archive", ""}, // No file expected, as it should skip
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN: A temporary directory and a test tarball.
			tempDir := t.TempDir()
			os.Chdir(tempDir)
			defer os.Chdir(t.TempDir()) // Isolate filesystem changes

			archiveName := fmt.Sprintf("test-archive.tar.%s", tc.archiveFormat)
			archivePath := createTestTarball(t, tc.archiveFiles, tc.archiveFormat)
			// We need to rename the generated temp file to have the expected name for parsing.
			finalArchivePath := filepath.Join(filepath.Dir(archivePath), archiveName)
			os.Rename(archivePath, finalArchivePath)

			if tc.name == "Skip existing directory" {
				os.MkdirAll(tc.expectedDir, 0755)
			}

			// WHEN: We call the unpackage function.
			err := UnpackageArchives(tc.archiveType, []string{finalArchivePath})

			// THEN: We check for errors and expected file creation.
			if (err != nil) != tc.expectErr {
				t.Errorf("Expected error: %v, got: %v", tc.expectErr, err)
			}

			if tc.expectedDir != "" {
				if _, err := os.Stat(tc.expectedDir); os.IsNotExist(err) {
					t.Errorf("Expected directory to be created: %s", tc.expectedDir)
				}
			}

			if tc.expectedFile != "" {
				if _, err := os.Stat(tc.expectedFile); os.IsNotExist(err) {
					t.Errorf("Expected file to be extracted: %s", tc.expectedFile)
				}
			}
		})
	}
}

func TestBuildDllOverridesString(t *testing.T) {
	testCases := []struct {
		name      string
		overrides map[string]string
		expected  string
	}{
		{"Empty map", map[string]string{}, ""},
		{"Single override", map[string]string{"d3d9": "n,b"}, "d3d9=n,b"},
		// Use a fixed set to handle map iteration order unpredictability.
		{"Multiple overrides", map[string]string{"a": "1", "b": "2"}, "a=1;b=2"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GIVEN: An AppConfig with specific DLL overrides.
			app := App{AppConfig: AppConfig{DLLOverrides: tc.overrides}}

			// WHEN: We build the override string.
			result := app.buildDllOverridesString()

			// THEN: The result must match our expectation (handling multiple orderings).
			if len(tc.overrides) > 1 {
				parts := strings.Split(result, ";")
				expectedParts := strings.Split(tc.expected, ";")
				if !reflect.DeepEqual(parts, expectedParts) {
					// Check reverse order
					altExpected := fmt.Sprintf("%s;%s", expectedParts[1], expectedParts[0])
					if result != altExpected {
						t.Errorf("Expected '%s' or '%s', got '%s'", tc.expected, altExpected, result)
					}
				}
			} else {
				if result != tc.expected {
					t.Errorf("Expected '%s', got '%s'", tc.expected, result)
				}
			}
		})
	}
}

func TestEnsureAppDirAndDefaultConfig(t *testing.T) {
	// GIVEN: A temporary directory.
	tempDir := t.TempDir()
	os.Chdir(tempDir)
	defer os.Chdir(t.TempDir())

	appDir := filepath.Join("games", "new-game")
	expectedConfigPath := filepath.Join(appDir, "game.json")

	// WHEN: We call the function for a non-existent directory.
	// We wrap this in a function to catch the os.Exit call.
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				// This is expected because of os.Exit
			}
		}()
		err = ensureAppDirAndDefaultConfig(appDir, "games")
	}()

	// THEN: It should not return an error, and the directory and default config should be created.
	if err != nil {
		t.Fatalf("Expected no error, but got: %v", err)
	}

	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		t.Errorf("Expected directory to be created: %s", appDir)
	}

	if _, err := os.Stat(expectedConfigPath); os.IsNotExist(err) {
		t.Errorf("Expected default config file to be created: %s", expectedConfigPath)
	}

	// Verify content of the default config
	var cfg AppConfig
	data, _ := os.ReadFile(expectedConfigPath)
	json.Unmarshal(data, &cfg)
	if cfg.Executable != "drive_c/path/to/your/app.exe" {
		t.Errorf("Default config content is incorrect.")
	}
}

func TestExtractTarStripsTopLevelDir(t *testing.T) {
	// GIVEN: A tarball created in memory with a top-level directory.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	topDir := "my-archive-1.2.3/"
	tw.WriteHeader(&tar.Header{Name: topDir, Typeflag: tar.TypeDir, Mode: 0755})
	content := "hello world"
	hdr := &tar.Header{
		Name:     filepath.Join(topDir, "file.txt"),
		Mode:     0600,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	tw.WriteHeader(hdr)
	tw.Write([]byte(content))
	tw.Close()

	// GIVEN: A temporary destination directory.
	destDir := t.TempDir()

	// WHEN: We extract the archive from the memory buffer.
	err := extractTar(&buf, destDir)

	// THEN: The extraction should succeed and the file should be in the root of the destination.
	if err != nil {
		t.Fatalf("extractTar failed: %v", err)
	}

	expectedFilePath := filepath.Join(destDir, "file.txt")
	if _, err := os.Stat(expectedFilePath); os.IsNotExist(err) {
		t.Errorf("Expected file '%s' to be extracted, but it was not found.", expectedFilePath)
	}

	// THEN: The original top-level directory should NOT exist.
	unexpectedDirPath := filepath.Join(destDir, topDir)
	if _, err := os.Stat(unexpectedDirPath); err == nil {
		t.Errorf("Expected top-level directory '%s' NOT to be extracted, but it was.", unexpectedDirPath)
	}
}
