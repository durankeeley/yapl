package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/config"
	"yapl/internal/manifest"
)

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func packageManifest(name, ver string, content []byte) manifest.Manifest {
	return manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion, Name: name, Type: "game",
		Metadata: manifest.Metadata{Title: name, Version: ver},
		Archive:  manifest.Archive{File: name + ".tar.zst", Format: "zst", SHA256: sha(content), SizeBytes: int64(len(content))},
	}
}

func componentManifest(kind, ver string, content []byte) manifest.ComponentManifest {
	return manifest.ComponentManifest{
		SchemaVersion: manifest.SchemaVersion, Kind: kind, Version: ver,
		Archive:     manifest.Archive{File: manifest.ComponentArchiveFile, Format: manifest.ComponentFormat, SHA256: sha(content), SizeBytes: int64(len(content))},
		VersionInfo: config.VersionInfo{BinPath: "files/bin"},
	}
}

func TestPackageUpload(t *testing.T) {
	t.Run("given a complete upload when committed then the version is stored and listed", func(t *testing.T) {
		// Given
		root := t.TempDir()
		s, _ := open(t, root)
		content := []byte("doom archive")
		u, err := s.NewUpload()
		if err != nil {
			t.Fatal(err)
		}

		// When
		if err := u.WriteArchive(bytes.NewReader(content)); err != nil {
			t.Fatal(err)
		}
		if err := u.WriteCover(".png", strings.NewReader("png")); err != nil {
			t.Fatal(err)
		}
		err = u.CommitPackage("Doom", "1.0.0", packageManifest("Doom", "1.0.0", content))

		// Then
		if err != nil {
			t.Fatal(err)
		}
		v, err := s.Resolve("Doom", LatestTag)
		if err != nil || v.CoverPath == "" {
			t.Fatalf("resolve = %+v, %v", v, err)
		}
		if data, _ := os.ReadFile(filepath.Join(root, "games", "Doom", "1.0.0", "Doom.tar.zst")); string(data) != "doom archive" {
			t.Fatalf("stored archive = %q", data)
		}
		if leftovers, _ := os.ReadDir(filepath.Join(root, uploadsDir)); len(leftovers) != 0 {
			t.Fatalf("staging not cleaned up: %v", leftovers)
		}
	})

	t.Run("given bytes that don't match the manifest checksum when committed then nothing is stored", func(t *testing.T) {
		// Given
		root := t.TempDir()
		s, _ := open(t, root)
		u, _ := s.NewUpload()
		u.WriteArchive(strings.NewReader("corrupted"))

		// When
		err := u.CommitPackage("Doom", "1.0.0", packageManifest("Doom", "1.0.0", []byte("original")))

		// Then
		if !errors.Is(err, ErrChecksum) {
			t.Fatalf("error = %v", err)
		}
		if _, err := s.Resolve("Doom", LatestTag); !errors.Is(err, ErrNotFound) {
			t.Fatal("corrupted upload was indexed")
		}
		if leftovers, _ := os.ReadDir(filepath.Join(root, uploadsDir)); len(leftovers) != 0 {
			t.Fatalf("staging not cleaned up after failure: %v", leftovers)
		}
	})

	t.Run("given an existing version when the same version is committed then it conflicts", func(t *testing.T) {
		root := t.TempDir()
		publish(t, root, "games", "Doom", "1.0.0", []byte("a"), false)
		s, _ := open(t, root)
		u, _ := s.NewUpload()
		u.WriteArchive(strings.NewReader("b"))
		if err := u.CommitPackage("Doom", "1.0.0", packageManifest("Doom", "1.0.0", []byte("b"))); !errors.Is(err, ErrConflict) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given an app with the name of an existing game when committed then it conflicts", func(t *testing.T) {
		root := t.TempDir()
		publish(t, root, "games", "Tool", "1.0.0", []byte("a"), false)
		s, _ := open(t, root)
		u, _ := s.NewUpload()
		u.WriteArchive(strings.NewReader("b"))
		m := packageManifest("Tool", "2.0.0", []byte("b"))
		m.Type = "app"
		if err := u.CommitPackage("Tool", "2.0.0", m); !errors.Is(err, ErrConflict) {
			t.Fatalf("error = %v", err)
		}
	})

	invalid := []struct {
		name    string
		urlName string
		urlVer  string
		archive bool
		mutate  func(*manifest.Manifest)
	}{
		{"given a manifest for a different name than the url then it is rejected", "Quake", "1.0.0", true, func(*manifest.Manifest) {}},
		{"given a manifest for a different version than the url then it is rejected", "Doom", "2.0.0", true, func(*manifest.Manifest) {}},
		{"given no archive part then it is rejected", "Doom", "1.0.0", false, func(*manifest.Manifest) {}},
		{"given an unsafe archive file name then it is rejected", "Doom", "1.0.0", true, func(m *manifest.Manifest) { m.Archive.File = "../x.tar.zst" }},
		{"given an archive file with an unknown extension then it is rejected", "Doom", "1.0.0", true, func(m *manifest.Manifest) { m.Archive.File = "Doom.zip" }},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := open(t, t.TempDir())
			u, _ := s.NewUpload()
			content := []byte("x")
			if tc.archive {
				u.WriteArchive(bytes.NewReader(content))
			}
			m := packageManifest("Doom", "1.0.0", content)
			tc.mutate(&m)
			if err := u.CommitPackage(tc.urlName, tc.urlVer, m); !errors.Is(err, ErrInvalidUpload) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	t.Run("given an unsupported cover type then it is rejected", func(t *testing.T) {
		s, _ := open(t, t.TempDir())
		u, _ := s.NewUpload()
		if err := u.WriteCover(".exe", strings.NewReader("x")); !errors.Is(err, ErrInvalidUpload) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given an oversized cover then it is rejected", func(t *testing.T) {
		s, _ := open(t, t.TempDir())
		u, _ := s.NewUpload()
		if err := u.WriteCover(".png", bytes.NewReader(make([]byte, MaxCoverBytes+1))); !errors.Is(err, ErrInvalidUpload) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given an aborted upload then its staging directory is removed", func(t *testing.T) {
		root := t.TempDir()
		s, _ := open(t, root)
		u, _ := s.NewUpload()
		u.WriteArchive(strings.NewReader("x"))
		u.Abort()
		if leftovers, _ := os.ReadDir(filepath.Join(root, uploadsDir)); len(leftovers) != 0 {
			t.Fatalf("staging left behind: %v", leftovers)
		}
	})
}

func TestComponents(t *testing.T) {
	t.Run("given an uploaded component when resolved then its manifest and archive are found", func(t *testing.T) {
		// Given
		root := t.TempDir()
		s, _ := open(t, root)
		content := []byte("proton build")
		u, _ := s.NewUpload()
		u.WriteArchive(bytes.NewReader(content))

		// When
		err := u.CommitComponent("proton", "GE-Proton11-7", componentManifest("proton", "GE-Proton11-7", content))

		// Then
		if err != nil {
			t.Fatal(err)
		}
		c, err := s.ResolveComponent("proton", "GE-Proton11-7")
		if err != nil || c.Manifest.VersionInfo.BinPath != "files/bin" {
			t.Fatalf("component = %+v, %v", c, err)
		}
		if data, _ := os.ReadFile(c.ArchivePath); string(data) != "proton build" {
			t.Fatalf("archive = %q", data)
		}
		if list := s.Components(); len(list) != 1 || list[0].Kind != "proton" {
			t.Fatalf("components = %+v", list)
		}
	})

	t.Run("given a stored component when it is uploaded again then it conflicts", func(t *testing.T) {
		s, _ := open(t, t.TempDir())
		for i := 0; i < 2; i++ {
			u, _ := s.NewUpload()
			u.WriteArchive(strings.NewReader("p"))
			err := u.CommitComponent("dxvk", "3.1.1", componentManifest("dxvk", "3.1.1", []byte("p")))
			if i == 1 && !errors.Is(err, ErrConflict) {
				t.Fatalf("second upload error = %v", err)
			}
		}
	})

	t.Run("given a component manifest for another kind than the url then it is rejected", func(t *testing.T) {
		s, _ := open(t, t.TempDir())
		u, _ := s.NewUpload()
		u.WriteArchive(strings.NewReader("p"))
		if err := u.CommitComponent("dxvk", "3.1.1", componentManifest("vkd3d", "3.1.1", []byte("p"))); !errors.Is(err, ErrInvalidUpload) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given an unknown component when resolved then ErrNotFound is returned", func(t *testing.T) {
		s, _ := open(t, t.TempDir())
		if _, err := s.ResolveComponent("proton", "nope"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("error = %v", err)
		}
		if _, err := s.ResolveComponent("drivers", "1"); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given a component directory with a broken manifest then it is skipped with a warning", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, ComponentsDir, "proton", "x")
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, ManifestFile), []byte("{"), 0644)
		s, warnings := open(t, root)
		if len(s.Components()) != 0 || !strings.Contains(warnings.String(), "skipping") {
			t.Fatalf("components %+v warnings %q", s.Components(), warnings.String())
		}
	})
}
