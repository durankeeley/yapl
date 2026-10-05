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

	"yapl/internal/manifest"
)

// publish writes a versioned package the way push (PRD-43) lays it out.
func publish(t *testing.T, root, typeDir, name, ver string, content []byte, cover bool) {
	t.Helper()
	dir := filepath.Join(root, typeDir, name, ver)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, name+".tar.xz"), content, 0644)
	if cover {
		os.WriteFile(filepath.Join(dir, "cover.png"), []byte("png"), 0644)
	}
	sum := sha256.Sum256(content)
	kind := strings.TrimSuffix(typeDir, "s")
	m := manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion, Name: name, Type: kind,
		Metadata: manifest.Metadata{Title: name + " Deluxe", Version: ver, Description: "desc", Tags: []string{"fps"}},
		Archive:  manifest.Archive{File: name + ".tar.xz", Format: "xz", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content))},
	}
	if err := manifest.Write(filepath.Join(dir, ManifestFile), m); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T, root string) (*Store, *bytes.Buffer) {
	t.Helper()
	var warnings bytes.Buffer
	s := New(root, &warnings)
	if err := s.Refresh(); err != nil {
		t.Fatal(err)
	}
	return s, &warnings
}

func TestResolve(t *testing.T) {
	t.Run("given several versions when latest is resolved then the highest semantic version wins", func(t *testing.T) {
		// Given
		root := t.TempDir()
		for _, v := range []string{"1.0.0", "1.10.0", "1.2.0"} {
			publish(t, root, "games", "Doom", v, []byte(v), false)
		}
		s, _ := open(t, root)

		// When
		v, err := s.Resolve("Doom", LatestTag)

		// Then
		if err != nil || v.Manifest.Metadata.Version != "1.10.0" {
			t.Fatalf("resolved %+v, %v", v, err)
		}
		if v.ArchivePath != filepath.Join(root, "games", "Doom", "1.10.0", "Doom.tar.xz") {
			t.Fatalf("archive path = %s", v.ArchivePath)
		}
	})

	t.Run("given a specific tag when resolved then that version is returned", func(t *testing.T) {
		root := t.TempDir()
		publish(t, root, "games", "Doom", "1.0.0", []byte("a"), false)
		publish(t, root, "games", "Doom", "2.0.0", []byte("b"), false)
		s, _ := open(t, root)
		if v, err := s.Resolve("Doom", "1.0.0"); err != nil || v.Manifest.Metadata.Version != "1.0.0" {
			t.Fatalf("resolved %+v, %v", v, err)
		}
	})

	t.Run("given an unknown package or tag when resolved then ErrNotFound is returned", func(t *testing.T) {
		root := t.TempDir()
		publish(t, root, "games", "Doom", "1.0.0", []byte("a"), false)
		s, _ := open(t, root)
		for _, ref := range [][2]string{{"Quake", LatestTag}, {"Doom", "9.9.9"}} {
			if _, err := s.Resolve(ref[0], ref[1]); !errors.Is(err, ErrNotFound) {
				t.Errorf("Resolve(%s:%s) error = %v", ref[0], ref[1], err)
			}
		}
	})

	t.Run("given an unsafe name when resolved then ErrInvalidName is returned", func(t *testing.T) {
		s, _ := open(t, t.TempDir())
		if _, err := s.Resolve("../etc", LatestTag); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestLegacyArchives(t *testing.T) {
	t.Run("given a flat archive with no sidecar then it is listed as version 0.0.0 without a checksum", func(t *testing.T) {
		// Given
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, "games"), 0755)
		os.WriteFile(filepath.Join(root, "games", "Quake.tar.gz"), []byte("quake"), 0644)

		// When
		s, _ := open(t, root)
		v, err := s.Resolve("Quake", LatestTag)

		// Then
		if err != nil || v.Manifest.Metadata.Version != LegacyVersion || v.Manifest.Archive.SHA256 != "" || v.Manifest.Archive.Format != "gz" || v.Manifest.Archive.SizeBytes != 5 || v.Manifest.Type != "game" {
			t.Fatalf("legacy version = %+v, %v", v, err)
		}
	})

	t.Run("given a flat archive with a sidecar manifest and cover then both are used", func(t *testing.T) {
		// Given
		root := t.TempDir()
		games := filepath.Join(root, "games")
		os.MkdirAll(games, 0755)
		os.WriteFile(filepath.Join(games, "Doom.tar.xz"), []byte("doom"), 0644)
		os.WriteFile(filepath.Join(games, "Doom.cover.jpg"), []byte("jpg"), 0644)
		sum := sha256.Sum256([]byte("doom"))
		manifest.Write(manifest.SidecarPath(games, "Doom"), manifest.Manifest{
			SchemaVersion: 1, Name: "Doom", Type: "game",
			Metadata: manifest.Metadata{Title: "DOOM", Version: "1.9.0"},
			Archive:  manifest.Archive{File: "Doom.tar.xz", Format: "xz", SHA256: hex.EncodeToString(sum[:]), SizeBytes: 4},
		})

		// When
		s, _ := open(t, root)
		v, err := s.Resolve("Doom", LatestTag)

		// Then
		if err != nil || v.Manifest.Metadata.Version != "1.9.0" || v.Manifest.Metadata.Title != "DOOM" || v.CoverPath != filepath.Join(games, "Doom.cover.jpg") {
			t.Fatalf("version = %+v, %v", v, err)
		}
	})

	t.Run("given non-archive files then they are ignored", func(t *testing.T) {
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, "games"), 0755)
		os.WriteFile(filepath.Join(root, "games", "notes.txt"), []byte("x"), 0644)
		s, _ := open(t, root)
		if len(s.Catalog()) != 0 {
			t.Fatalf("catalog = %+v", s.Catalog())
		}
	})
}

func TestCatalog(t *testing.T) {
	t.Run("given games and apps then the catalog lists each once, sorted, with latest metadata", func(t *testing.T) {
		// Given
		root := t.TempDir()
		publish(t, root, "games", "Doom", "1.0.0", []byte("a"), true)
		publish(t, root, "games", "Doom", "1.1.0", []byte("bb"), true)
		publish(t, root, "apps", "Notepad", "8.0.0", []byte("c"), false)

		// When
		s, _ := open(t, root)
		cat := s.Catalog()

		// Then
		if len(cat) != 2 || cat[0].Name != "Doom" || cat[1].Name != "Notepad" {
			t.Fatalf("catalog = %+v", cat)
		}
		doom := cat[0]
		if doom.Latest != "1.1.0" || strings.Join(doom.Versions, ",") != "1.1.0,1.0.0" || doom.Title != "Doom Deluxe" || !doom.HasCover || doom.SizeBytes != 2 || doom.Type != "game" {
			t.Fatalf("doom = %+v", doom)
		}
		if cat[1].Type != "app" || cat[1].HasCover {
			t.Fatalf("notepad = %+v", cat[1])
		}
	})

	t.Run("given the same name in games and apps then games wins and a warning is written", func(t *testing.T) {
		root := t.TempDir()
		publish(t, root, "games", "Tool", "1.0.0", []byte("g"), false)
		publish(t, root, "apps", "Tool", "1.0.0", []byte("a"), false)
		s, warnings := open(t, root)
		if cat := s.Catalog(); len(cat) != 1 || cat[0].Type != "game" {
			t.Fatalf("catalog = %+v", cat)
		}
		if !strings.Contains(warnings.String(), "Tool") {
			t.Fatalf("no collision warning: %q", warnings.String())
		}
	})

	t.Run("given an invalid or mismatched manifest then that version is skipped with a warning", func(t *testing.T) {
		// Given
		root := t.TempDir()
		publish(t, root, "games", "Doom", "1.0.0", []byte("a"), false)
		publish(t, root, "games", "Doom", "2.0.0", []byte("b"), false)
		os.Rename(filepath.Join(root, "games", "Doom", "2.0.0"), filepath.Join(root, "games", "Doom", "3.0.0"))
		bad := filepath.Join(root, "games", "Doom", "4.0.0")
		os.MkdirAll(bad, 0755)
		os.WriteFile(filepath.Join(bad, ManifestFile), []byte("{"), 0644)

		// When
		s, warnings := open(t, root)

		// Then
		if cat := s.Catalog(); len(cat) != 1 || strings.Join(cat[0].Versions, ",") != "1.0.0" {
			t.Fatalf("catalog = %+v", cat)
		}
		if strings.Count(warnings.String(), "skipping") != 2 {
			t.Fatalf("warnings = %q", warnings.String())
		}
	})

	t.Run("given a hidden staging directory then it is ignored", func(t *testing.T) {
		root := t.TempDir()
		publish(t, root, "games", ".staging-Doom", "1.0.0", []byte("a"), false)
		publish(t, root, "games", "Doom", ".upload-123", []byte("a"), false)
		s, _ := open(t, root)
		if len(s.Catalog()) != 0 {
			t.Fatalf("catalog = %+v", s.Catalog())
		}
	})

	t.Run("given a new version appears when refreshed then it is listed", func(t *testing.T) {
		root := t.TempDir()
		publish(t, root, "games", "Doom", "1.0.0", []byte("a"), false)
		s, _ := open(t, root)
		publish(t, root, "games", "Doom", "1.1.0", []byte("b"), false)
		s.Refresh()
		if v, _ := s.Resolve("Doom", LatestTag); v.Manifest.Metadata.Version != "1.1.0" {
			t.Fatalf("latest after refresh = %s", v.Manifest.Metadata.Version)
		}
	})

	t.Run("given a missing root then the catalog is empty and refresh succeeds", func(t *testing.T) {
		s := New(filepath.Join(t.TempDir(), "missing"), &bytes.Buffer{})
		if err := s.Refresh(); err != nil || len(s.Catalog()) != 0 {
			t.Fatalf("err %v catalog %+v", err, s.Catalog())
		}
	})
}

func TestVersions(t *testing.T) {
	t.Run("given a package then all versions are returned newest first", func(t *testing.T) {
		root := t.TempDir()
		publish(t, root, "games", "Doom", "1.0.0", []byte("a"), false)
		publish(t, root, "games", "Doom", "1.0.1", []byte("b"), false)
		s, _ := open(t, root)
		vs, err := s.Versions("Doom")
		if err != nil || len(vs) != 2 || vs[0].Manifest.Metadata.Version != "1.0.1" {
			t.Fatalf("versions = %+v, %v", vs, err)
		}
	})
}

func TestSymlinkedPackageDirectory(t *testing.T) {
	t.Run("given a package directory symlinked from another disk then it is served", func(t *testing.T) {
		other := t.TempDir()
		publish(t, other, "games", "Doom", "1.0.0", []byte("a"), false)
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, "games"), 0755)
		os.Symlink(filepath.Join(other, "games", "Doom"), filepath.Join(root, "games", "Doom"))
		s, _ := open(t, root)
		if _, err := s.Resolve("Doom", LatestTag); err != nil {
			t.Fatal(err)
		}
	})
}
