package upgrade

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/archive"
	"yapl/internal/catalog"
	"yapl/internal/client"
	"yapl/internal/config"
	"yapl/internal/kernel"
	"yapl/internal/manifest"
	"yapl/internal/pull"
	"yapl/internal/registry/api"
	"yapl/internal/registry/store"
)

type fixture struct {
	client *client.Client
	store  *store.Store
}

func newFixture(t *testing.T, binaryPath string, versions kernel.Versions) *fixture {
	t.Helper()
	st := store.New(t.TempDir(), io.Discard)
	st.Refresh()
	cfg := api.Config{Store: st, Auth: api.AuthOpen, Info: api.Info{Kernel: versions.Kernel, Modules: versions.Modules}}
	var h = api.New(cfg)
	if binaryPath != "" {
		var err error
		if h, err = api.NewWithBinary(cfg, binaryPath); err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &fixture{client: client.New(client.Config{Server: srv.URL}), store: st}
}

func (f *fixture) publish(t *testing.T, ver string, files map[string]string) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "Doom")
	for rel, content := range files {
		p := filepath.Join(src, rel)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(content), 0644)
	}
	var buf bytes.Buffer
	if err := archive.Write(&buf, src, filepath.Dir(src), "gz"); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	m := manifest.Manifest{SchemaVersion: 1, Name: "Doom", Type: "game", Metadata: manifest.Metadata{Title: "Doom", Version: ver},
		Archive: manifest.Archive{File: "Doom.tar.gz", Format: "gz", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(buf.Len())}}
	u, _ := f.store.NewUpload()
	u.WriteArchive(bytes.NewReader(buf.Bytes()))
	if err := u.CommitPackage("Doom", ver, m); err != nil {
		t.Fatal(err)
	}
}

func inLibrary(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })
}

const gameJSON = `{"proton_version":"system","launch_method":"direct","executable":"drive_c/doom.exe"}`

func TestGame(t *testing.T) {
	t.Run("given an installed game with saves when a newer version is published then upgrading keeps the saves", func(t *testing.T) {
		// Given
		f := newFixture(t, "", kernel.Versions{})
		f.publish(t, "1.0.0", map[string]string{"game.json": gameJSON, "old-only.txt": "gone in 1.1", "metadata.json": `{"preserve":["prefix/drive_c/Games/Doom/base/config.cfg"]}`})
		inLibrary(t)
		if _, err := pull.Pull(f.client, "Doom", "latest", pull.Options{}); err != nil {
			t.Fatal(err)
		}
		save := filepath.Join("games", "Doom", "prefix", "drive_c", "users", "steamuser", "Saved Games", "doom.sav")
		os.MkdirAll(filepath.Dir(save), 0755)
		os.WriteFile(save, []byte("level 9"), 0644)
		cfg := filepath.Join("games", "Doom", "prefix", "drive_c", "Games", "Doom", "base", "config.cfg")
		os.MkdirAll(filepath.Dir(cfg), 0755)
		os.WriteFile(cfg, []byte("bind w forward"), 0644)
		f.publish(t, "1.1.0", map[string]string{"game.json": gameJSON, "new-in-1.1.txt": "hi", "prefix/drive_c/Games/Doom/base/config.cfg": "default binds", "metadata.json": `{"preserve":["prefix/drive_c/Games/Doom/base/config.cfg"]}`})

		// When
		from, to, err := Game(f.client, "Doom", manifest.LatestTag, Options{})

		// Then
		if err != nil || from != "1.0.0" || to != "1.1.0" {
			t.Fatalf("upgrade %s -> %s: %v", from, to, err)
		}
		if got, _ := os.ReadFile(save); string(got) != "level 9" {
			t.Fatalf("save = %q", got)
		}
		if got, _ := os.ReadFile(cfg); string(got) != "bind w forward" {
			t.Fatalf("preserved config = %q", got)
		}
		if _, err := os.Stat(filepath.Join("games", "Doom", "new-in-1.1.txt")); err != nil {
			t.Fatal("new version's files missing")
		}
		if _, err := os.Stat(filepath.Join("games", "Doom", "old-only.txt")); !os.IsNotExist(err) {
			t.Fatal("old version's files survived the upgrade")
		}
		if in, _ := manifest.ReadInstalled(filepath.Join("games", "Doom")); in.Version != "1.1.0" {
			t.Fatalf("installed = %+v", in)
		}
		if leftovers, _ := os.ReadDir(".staging"); len(leftovers) != 0 {
			t.Fatalf("staging not cleaned: %v", leftovers)
		}
	})

	t.Run("given the latest version is installed when upgrading then nothing changes", func(t *testing.T) {
		f := newFixture(t, "", kernel.Versions{})
		f.publish(t, "1.0.0", map[string]string{"game.json": gameJSON})
		inLibrary(t)
		pull.Pull(f.client, "Doom", "latest", pull.Options{})
		var out bytes.Buffer
		from, to, err := Game(f.client, "Doom", manifest.LatestTag, Options{Out: &out})
		if err != nil || from != to || !strings.Contains(out.String(), "up to date") {
			t.Fatalf("%s -> %s err %v out %q", from, to, err, out.String())
		}
	})

	t.Run("given an explicit older version when upgrading then the game is rolled back", func(t *testing.T) {
		f := newFixture(t, "", kernel.Versions{})
		f.publish(t, "1.0.0", map[string]string{"game.json": gameJSON, "v": "1"})
		f.publish(t, "2.0.0", map[string]string{"game.json": gameJSON, "v": "2"})
		inLibrary(t)
		pull.Pull(f.client, "Doom", "latest", pull.Options{})
		if _, _, err := Game(f.client, "Doom", "1.0.0", Options{}); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(filepath.Join("games", "Doom", "v")); string(got) != "1" {
			t.Fatalf("v = %q", got)
		}
	})

	t.Run("given a corrupted new version when upgrading then the old install is untouched", func(t *testing.T) {
		f := newFixture(t, "", kernel.Versions{})
		f.publish(t, "1.0.0", map[string]string{"game.json": gameJSON, "v": "1"})
		inLibrary(t)
		pull.Pull(f.client, "Doom", "latest", pull.Options{})
		f.publish(t, "1.1.0", map[string]string{"game.json": gameJSON, "v": "2"})
		v, _ := f.store.Resolve("Doom", "1.1.0")
		os.WriteFile(v.ArchivePath, []byte("garbage"), 0644)
		if _, _, err := Game(f.client, "Doom", manifest.LatestTag, Options{}); err == nil {
			t.Fatal("expected the upgrade to fail")
		}
		if got, _ := os.ReadFile(filepath.Join("games", "Doom", "v")); string(got) != "1" {
			t.Fatalf("old install damaged: v = %q", got)
		}
	})

	t.Run("given a game that is not installed when upgrading then it says so", func(t *testing.T) {
		f := newFixture(t, "", kernel.Versions{})
		inLibrary(t)
		if _, _, err := Game(f.client, "Doom", manifest.LatestTag, Options{}); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestSelf(t *testing.T) {
	local := kernel.Versions{Kernel: "1.0.0", Modules: map[string]string{"store": "2.0.0"}}
	newer := kernel.Versions{Kernel: "1.0.0", Modules: map[string]string{"store": "2.1.0"}}

	serverBinary := func(t *testing.T) string {
		p := filepath.Join(t.TempDir(), "yapl-server")
		os.WriteFile(p, []byte("NEW BINARY"), 0755)
		return p
	}
	localBinary := func(t *testing.T) string {
		p := filepath.Join(t.TempDir(), "yapl")
		os.WriteFile(p, []byte("OLD BINARY"), 0755)
		return p
	}

	t.Run("given a newer server binary when upgrading self then the local binary is replaced and stays executable", func(t *testing.T) {
		// Given
		f := newFixture(t, serverBinary(t), newer)
		exe := localBinary(t)

		// When
		upgraded, err := Self(f.client, local, exe, Options{})

		// Then
		info, _ := os.Stat(exe)
		got, _ := os.ReadFile(exe)
		if err != nil || !upgraded || string(got) != "NEW BINARY" || info.Mode()&0111 == 0 {
			t.Fatalf("upgraded %v err %v content %q mode %v", upgraded, err, got, info.Mode())
		}
	})

	t.Run("given the server binary is not newer when upgrading self then nothing is downloaded", func(t *testing.T) {
		f := newFixture(t, serverBinary(t), local)
		exe := localBinary(t)
		upgraded, err := Self(f.client, local, exe, Options{})
		got, _ := os.ReadFile(exe)
		if err != nil || upgraded || string(got) != "OLD BINARY" {
			t.Fatalf("upgraded %v err %v content %q", upgraded, err, got)
		}
	})

	t.Run("given a server without a binary when upgrading self then it fails and the local binary is untouched", func(t *testing.T) {
		f := newFixture(t, "", newer)
		exe := localBinary(t)
		if _, err := Self(f.client, local, exe, Options{}); err == nil {
			t.Fatal("expected an error")
		}
		if got, _ := os.ReadFile(exe); string(got) != "OLD BINARY" {
			t.Fatal("local binary changed")
		}
		if matches, _ := filepath.Glob(exe + ".*"); len(matches) != 0 {
			t.Fatalf("temporary files left: %v", matches)
		}
	})
}

func TestCatalog(t *testing.T) {
	t.Run("given an old runner.json when the catalog is upgraded then new pins and defaults are added and local entries kept", func(t *testing.T) {
		// Given
		path := filepath.Join(t.TempDir(), "runner.json")
		config.SaveGlobal(path, config.Global{
			ProtonVersions: map[string]config.VersionInfo{"GE-Proton9-20": {URL: "https://old"}},
			Defaults:       config.Defaults{Proton: "GE-Proton9-20"},
		})

		// When
		added, err := Catalog(path)

		// Then
		if err != nil || len(added) == 0 {
			t.Fatalf("added %v err %v", added, err)
		}
		g, _ := config.LoadGlobal(path)
		if _, ok := g.ProtonVersions["GE-Proton9-20"]; !ok {
			t.Fatal("local proton entry removed")
		}
		if g.Defaults.Proton != catalog.ProtonVersion || g.CatalogVersion != catalog.Version {
			t.Fatalf("defaults %+v catalog %s", g.Defaults, g.CatalogVersion)
		}
	})

	t.Run("given no runner.json when the catalog is upgraded then it is created", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "runner.json")
		if _, err := Catalog(path); err != nil {
			t.Fatal(err)
		}
		if g, err := config.LoadGlobal(path); err != nil || g.CatalogVersion != catalog.Version {
			t.Fatalf("g %+v err %v", g, err)
		}
	})
}
