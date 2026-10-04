package pull

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/archive"
	"yapl/internal/client"
	"yapl/internal/config"
	"yapl/internal/manifest"
	"yapl/internal/registry/api"
	"yapl/internal/registry/store"
)

type registryFixture struct {
	client *client.Client
	store  *store.Store
	url    string
}

func newRegistry(t *testing.T) *registryFixture {
	t.Helper()
	st := store.New(t.TempDir(), io.Discard)
	st.Refresh()
	srv := httptest.NewServer(api.New(api.Config{Store: st, Auth: "open", AllowPush: true}))
	t.Cleanup(srv.Close)
	return &registryFixture{client: client.New(client.Config{Server: srv.URL}), store: st, url: srv.URL}
}

// gameArchive builds a real .tar.gz whose top-level directory is name.
func gameArchive(t *testing.T, name string, files map[string]string) []byte {
	t.Helper()
	src := filepath.Join(t.TempDir(), name)
	for rel, content := range files {
		p := filepath.Join(src, rel)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(content), 0644)
	}
	var buf bytes.Buffer
	if err := archive.Write(&buf, src, filepath.Dir(src), "gz"); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (r *registryFixture) publishGame(t *testing.T, name, ver string, data []byte, components ...manifest.Component) manifest.Manifest {
	t.Helper()
	sum := sha256.Sum256(data)
	m := manifest.Manifest{SchemaVersion: 1, Name: name, Type: "game", Metadata: manifest.Metadata{Title: name, Version: ver},
		Archive:    manifest.Archive{File: name + ".tar.gz", Format: "gz", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(data))},
		Components: components}
	u, _ := r.store.NewUpload()
	u.WriteArchive(bytes.NewReader(data))
	if err := u.CommitPackage(name, ver, m); err != nil {
		t.Fatal(err)
	}
	return m
}

func (r *registryFixture) publishComponent(t *testing.T, kind, ver string, files map[string]string) {
	t.Helper()
	src := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(src, rel)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(content), 0755)
	}
	cm := manifest.ComponentManifest{SchemaVersion: 1, Kind: kind, Version: ver, VersionInfo: config.VersionInfo{BinPath: "files/bin"}}
	if err := r.client.PushComponent(cm, func(w io.Writer) error { return archive.Write(w, src, src, manifest.ComponentFormat) }, nil); err != nil {
		t.Fatal(err)
	}
}

func inLibrary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })
	return dir
}

var proton = manifest.Component{Kind: "proton", Version: "GE-Proton11-7"}

func TestPull(t *testing.T) {
	t.Run("given a game and its proton on the server when pulled then both are installed and registered", func(t *testing.T) {
		// Given
		reg := newRegistry(t)
		reg.publishComponent(t, "proton", "GE-Proton11-7", map[string]string{"files/bin/wine64": "wine"})
		reg.publishGame(t, "Doom", "1.2.0", gameArchive(t, "Doom", map[string]string{"game.json": "{}", "base/doom.wad": "wad"}), proton)
		inLibrary(t)

		// When
		res, err := Pull(reg.client, "Doom", "latest", Options{Out: io.Discard})

		// Then
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(filepath.Join("games", "Doom", "base", "doom.wad")); string(got) != "wad" {
			t.Fatal("game files not installed")
		}
		if got, _ := os.ReadFile(filepath.Join("proton", "GE-Proton11-7", "files", "bin", "wine64")); string(got) != "wine" {
			t.Fatal("proton not installed")
		}
		installed, err := manifest.ReadInstalled(filepath.Join("games", "Doom"))
		if err != nil || installed.Version != "1.2.0" || installed.Server != reg.url {
			t.Fatalf("installed = %+v, %v", installed, err)
		}
		runner, _ := config.LoadGlobal("runner.json")
		if v := runner.ProtonVersions["GE-Proton11-7"]; v.BinPath != "files/bin" || !strings.HasPrefix(v.URL, reg.url) {
			t.Fatalf("runner entry = %+v", v)
		}
		if len(res.Components) != 1 || res.Dir != filepath.Join("games", "Doom") {
			t.Fatalf("result = %+v", res)
		}
		if leftovers, _ := os.ReadDir(".staging"); len(leftovers) != 0 {
			t.Fatalf("staging not cleaned: %v", leftovers)
		}
	})

	t.Run("given several versions when a tag is pulled then that version is installed", func(t *testing.T) {
		reg := newRegistry(t)
		reg.publishGame(t, "Doom", "1.0.0", gameArchive(t, "Doom", map[string]string{"v.txt": "one"}))
		reg.publishGame(t, "Doom", "2.0.0", gameArchive(t, "Doom", map[string]string{"v.txt": "two"}))
		inLibrary(t)
		if _, err := Pull(reg.client, "Doom", "1.0.0", Options{Out: io.Discard}); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(filepath.Join("games", "Doom", "v.txt")); string(got) != "one" {
			t.Fatalf("installed %q", got)
		}
	})

	t.Run("given a corrupted archive on the server when pulled then nothing is installed", func(t *testing.T) {
		// Given
		reg := newRegistry(t)
		reg.publishGame(t, "Doom", "1.0.0", gameArchive(t, "Doom", map[string]string{"a": "b"}))
		v, _ := reg.store.Resolve("Doom", "1.0.0")
		os.WriteFile(v.ArchivePath, gameArchive(t, "Doom", map[string]string{"a": "tampered"}), 0644)
		inLibrary(t)

		// When
		_, err := Pull(reg.client, "Doom", "latest", Options{Out: io.Discard})

		// Then
		if !errors.Is(err, archive.ErrChecksum) {
			t.Fatalf("error = %v", err)
		}
		if _, err := os.Stat(filepath.Join("games", "Doom")); !os.IsNotExist(err) {
			t.Fatal("corrupted game was installed")
		}
		if leftovers, _ := os.ReadDir(".staging"); len(leftovers) != 0 {
			t.Fatalf("staging not cleaned: %v", leftovers)
		}
	})

	t.Run("given the game is already installed when pulled then it points to upgrade", func(t *testing.T) {
		reg := newRegistry(t)
		reg.publishGame(t, "Doom", "1.0.0", gameArchive(t, "Doom", map[string]string{"a": "b"}))
		inLibrary(t)
		os.MkdirAll(filepath.Join("games", "Doom"), 0755)
		_, err := Pull(reg.client, "Doom", "latest", Options{Out: io.Discard})
		if !errors.Is(err, ErrAlreadyInstalled) || !strings.Contains(err.Error(), "yapl upgrade Doom") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("given proton is already installed locally when pulled then it is not downloaded again", func(t *testing.T) {
		reg := newRegistry(t)
		reg.publishComponent(t, "proton", "GE-Proton11-7", map[string]string{"files/bin/wine64": "server"})
		reg.publishGame(t, "Doom", "1.0.0", gameArchive(t, "Doom", map[string]string{"a": "b"}), proton)
		inLibrary(t)
		os.MkdirAll(filepath.Join("proton", "GE-Proton11-7"), 0755)
		os.WriteFile(filepath.Join("proton", "GE-Proton11-7", "local"), []byte("local"), 0644)
		config.SaveGlobal("runner.json", config.Global{ProtonVersions: map[string]config.VersionInfo{"GE-Proton11-7": {URL: "https://upstream"}}})

		res, err := Pull(reg.client, "Doom", "latest", Options{Out: io.Discard})

		if err != nil || len(res.Components) != 0 {
			t.Fatalf("result %+v err %v", res, err)
		}
		if _, err := os.Stat(filepath.Join("proton", "GE-Proton11-7", "files")); !os.IsNotExist(err) {
			t.Fatal("local proton was overwritten")
		}
		runner, _ := config.LoadGlobal("runner.json")
		if runner.ProtonVersions["GE-Proton11-7"].URL != "https://upstream" {
			t.Fatal("local runner entry was replaced")
		}
	})

	t.Run("given a component the server lacks but runner.json can download when pulled then it is deferred to first run", func(t *testing.T) {
		reg := newRegistry(t)
		reg.publishGame(t, "Doom", "1.0.0", gameArchive(t, "Doom", map[string]string{"a": "b"}), proton)
		inLibrary(t)
		config.SaveGlobal("runner.json", config.Global{ProtonVersions: map[string]config.VersionInfo{"GE-Proton11-7": {URL: "https://upstream"}}})
		var out bytes.Buffer
		if _, err := Pull(reg.client, "Doom", "latest", Options{Out: &out}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "first run") {
			t.Fatalf("output:\n%s", out.String())
		}
	})

	t.Run("given a component nobody can provide when pulled then it fails before installing the game", func(t *testing.T) {
		reg := newRegistry(t)
		reg.publishGame(t, "Doom", "1.0.0", gameArchive(t, "Doom", map[string]string{"a": "b"}), proton)
		inLibrary(t)
		_, err := Pull(reg.client, "Doom", "latest", Options{Out: io.Discard})
		if err == nil || !strings.Contains(err.Error(), "proton/GE-Proton11-7") {
			t.Fatalf("error = %v", err)
		}
		if _, err := os.Stat(filepath.Join("games", "Doom")); !os.IsNotExist(err) {
			t.Fatal("game installed without its component")
		}
	})

	t.Run("given a bundled package when pulled then its bundle is installed and server components are not needed", func(t *testing.T) {
		reg := newRegistry(t)
		data := gameArchive(t, "Doom", map[string]string{"a": "b"})
		staging := t.TempDir()
		os.MkdirAll(filepath.Join(staging, "Doom"), 0755)
		os.WriteFile(filepath.Join(staging, "Doom", "a"), []byte("b"), 0644)
		os.MkdirAll(filepath.Join(staging, "_bundle", "proton", "GE-Proton11-7"), 0755)
		os.WriteFile(filepath.Join(staging, "_bundle", "proton", "GE-Proton11-7", "proton"), []byte("p"), 0755)
		var buf bytes.Buffer
		archive.Write(&buf, staging, staging, "gz")
		data = buf.Bytes()
		m := reg.publishGame(t, "Doom", "1.0.0", data, proton)
		_ = m
		v, _ := reg.store.Resolve("Doom", "1.0.0")
		v.Manifest.Bundled = true
		manifest.Write(filepath.Join(filepath.Dir(v.ArchivePath), store.ManifestFile), v.Manifest)
		reg.store.Refresh()
		inLibrary(t)

		if _, err := Pull(reg.client, "Doom", "latest", Options{Out: io.Discard}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join("proton", "GE-Proton11-7", "proton")); err != nil {
			t.Fatal("bundled proton not installed")
		}
	})
}

func TestPullLegacyServer(t *testing.T) {
	t.Run("given an older server without the v1 api when pulled then the legacy download is used", func(t *testing.T) {
		// Given
		data := gameArchive(t, "Quake", map[string]string{"pak0.pak": "pak"})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/packages":
				json.NewEncoder(w).Encode([]map[string]any{{"name": "Quake", "type": "game", "format": "gz", "size_bytes": len(data)}})
			case "/packages/Quake":
				w.Write(data)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()
		inLibrary(t)
		var out bytes.Buffer

		// When
		res, err := Pull(client.New(client.Config{Server: srv.URL}), "Quake", "latest", Options{Out: &out})

		// Then
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		if got, _ := os.ReadFile(filepath.Join("games", "Quake", "pak0.pak")); string(got) != "pak" {
			t.Fatal("legacy game not installed")
		}
		if installed, _ := manifest.ReadInstalled(res.Dir); installed.Version != store.LegacyVersion {
			t.Fatalf("installed = %+v", installed)
		}
		if !strings.Contains(out.String(), "older YAPL server") {
			t.Fatalf("no legacy notice:\n%s", out.String())
		}
	})
}
