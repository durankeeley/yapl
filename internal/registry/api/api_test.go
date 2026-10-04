package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yapl/internal/manifest"
	"yapl/internal/registry/store"
)

func publish(t *testing.T, root, name, ver string, content []byte, cover bool) {
	t.Helper()
	dir := filepath.Join(root, "games", name, ver)
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, name+".tar.xz"), content, 0644)
	if cover {
		os.WriteFile(filepath.Join(dir, "cover.png"), []byte("png-bytes"), 0644)
	}
	sum := sha256.Sum256(content)
	manifest.Write(filepath.Join(dir, store.ManifestFile), manifest.Manifest{
		SchemaVersion: 1, Name: name, Type: "game",
		Metadata: manifest.Metadata{Title: name, Version: ver},
		Archive:  manifest.Archive{File: name + ".tar.xz", Format: "xz", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content))},
	})
}

type fixture struct {
	server *httptest.Server
	root   string
}

func newFixture(t *testing.T, auth, password string) fixture {
	t.Helper()
	root := t.TempDir()
	publish(t, root, "Doom", "1.0.0", []byte("doom-1.0.0"), true)
	publish(t, root, "Doom", "1.10.0", []byte("doom-1.10.0"), true)
	publish(t, root, "Need for Speed", "2.0.0", []byte("nfs"), false)
	st := store.New(root, io.Discard)
	if err := st.Refresh(); err != nil {
		t.Fatal(err)
	}
	h := New(Config{Store: st, Auth: auth, Password: password, Info: Info{Kernel: "1.0.0", Modules: map[string]string{"registry": "2.0.0"}}})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return fixture{server: srv, root: root}
}

func (f fixture) get(t *testing.T, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(f.server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestInfo(t *testing.T) {
	t.Run("given a password protected server when info is requested anonymously then it succeeds and names the auth mode", func(t *testing.T) {
		// Given
		f := newFixture(t, "password", "secret")

		// When
		resp := f.get(t, "/v1/info")

		// Then
		var info Info
		decode(t, resp, &info)
		if resp.StatusCode != http.StatusOK || info.APIVersion != APIVersion || info.Auth != "password" || info.Modules["registry"] != "2.0.0" {
			t.Fatalf("status %d info %+v", resp.StatusCode, info)
		}
	})
}

func TestCatalog(t *testing.T) {
	t.Run("given published packages when the catalog is requested then each lists its latest version and cover url", func(t *testing.T) {
		// Given
		f := newFixture(t, "open", "")

		// When
		resp := f.get(t, "/v1/packages")

		// Then
		var cat []CatalogEntry
		decode(t, resp, &cat)
		if len(cat) != 2 || cat[0].Name != "Doom" || cat[0].Latest != "1.10.0" {
			t.Fatalf("catalog = %+v", cat)
		}
		if cat[0].CoverURL != "/v1/packages/Doom/latest/cover" || cat[1].CoverURL != "" {
			t.Fatalf("cover urls = %q, %q", cat[0].CoverURL, cat[1].CoverURL)
		}
	})

	t.Run("given password auth when the catalog is requested without credentials then it is rejected", func(t *testing.T) {
		f := newFixture(t, "password", "secret")
		if resp := f.get(t, "/v1/packages"); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given password auth when the catalog is requested with the password then it succeeds", func(t *testing.T) {
		f := newFixture(t, "password", "secret")
		req, _ := http.NewRequest("GET", f.server.URL+"/v1/packages", nil)
		req.SetBasicAuth("yapl", "secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("status %v err %v", resp.StatusCode, err)
		}
		resp.Body.Close()
	})
}

func TestPackageDetail(t *testing.T) {
	t.Run("given a package with two versions when its detail is requested then both manifests are returned newest first", func(t *testing.T) {
		f := newFixture(t, "open", "")
		var detail PackageDetail
		resp := f.get(t, "/v1/packages/Doom")
		decode(t, resp, &detail)
		if len(detail.Versions) != 2 || detail.Versions[0].Metadata.Version != "1.10.0" || detail.Type != "game" {
			t.Fatalf("detail = %+v", detail)
		}
	})
}

func TestManifestAndArchive(t *testing.T) {
	t.Run("given latest when the manifest is requested then the highest version is returned", func(t *testing.T) {
		f := newFixture(t, "open", "")
		var m manifest.Manifest
		decode(t, f.get(t, "/v1/packages/Doom/latest/manifest"), &m)
		if m.Metadata.Version != "1.10.0" {
			t.Fatalf("version = %s", m.Metadata.Version)
		}
	})

	t.Run("given a name with spaces when its archive is requested then the bytes and checksum header are served", func(t *testing.T) {
		// Given
		f := newFixture(t, "open", "")

		// When
		resp := f.get(t, "/v1/packages/"+url.PathEscape("Need for Speed")+"/2.0.0/archive")

		// Then
		body, _ := io.ReadAll(resp.Body)
		sum := sha256.Sum256([]byte("nfs"))
		if resp.StatusCode != http.StatusOK || string(body) != "nfs" || resp.Header.Get(SHA256Header) != hex.EncodeToString(sum[:]) {
			t.Fatalf("status %d body %q header %q", resp.StatusCode, body, resp.Header.Get(SHA256Header))
		}
	})

	t.Run("given a range request when the archive is downloaded then only that range is served", func(t *testing.T) {
		f := newFixture(t, "open", "")
		req, _ := http.NewRequest("GET", f.server.URL+"/v1/packages/Doom/1.0.0/archive", nil)
		req.Header.Set("Range", "bytes=5-")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusPartialContent || string(body) != "1.0.0" {
			t.Fatalf("status %d body %q", resp.StatusCode, body)
		}
	})

	t.Run("given a cover when it is requested then the image is served", func(t *testing.T) {
		f := newFixture(t, "open", "")
		resp := f.get(t, "/v1/packages/Doom/latest/cover")
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK || string(body) != "png-bytes" || resp.Header.Get("Content-Type") != "image/png" {
			t.Fatalf("status %d body %q type %q", resp.StatusCode, body, resp.Header.Get("Content-Type"))
		}
	})

	errorsCases := []struct {
		name   string
		path   string
		status int
	}{
		{"given an unknown package then 404", "/v1/packages/Quake/latest/manifest", http.StatusNotFound},
		{"given an unknown version then 404", "/v1/packages/Doom/9.9.9/archive", http.StatusNotFound},
		{"given a package without a cover then 404", "/v1/packages/" + url.PathEscape("Need for Speed") + "/latest/cover", http.StatusNotFound},
		{"given a traversal attempt then 400", "/v1/packages/..%2F..%2Fetc/latest/archive", http.StatusBadRequest},
		{"given a hidden name then 400", "/v1/packages/.staging/latest/manifest", http.StatusBadRequest},
	}
	for _, tc := range errorsCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "open", "")
			resp := f.get(t, tc.path)
			if resp.StatusCode != tc.status {
				t.Fatalf("status %d, want %d", resp.StatusCode, tc.status)
			}
			if !strings.Contains(resp.Header.Get("Content-Type"), "json") {
				t.Fatalf("error is not json: %q", resp.Header.Get("Content-Type"))
			}
		})
	}
}

func TestLegacyEndpoints(t *testing.T) {
	t.Run("given an old client when it lists packages then it gets name, type, format and size", func(t *testing.T) {
		f := newFixture(t, "open", "")
		var list []map[string]any
		decode(t, f.get(t, "/packages"), &list)
		if len(list) != 2 || list[0]["name"] != "Doom" || list[0]["type"] != "game" || list[0]["format"] != "xz" || list[0]["size_bytes"] == nil {
			t.Fatalf("legacy list = %+v", list)
		}
	})

	t.Run("given an old client when it downloads by name then the latest archive is served", func(t *testing.T) {
		f := newFixture(t, "open", "")
		body, _ := io.ReadAll(f.get(t, "/packages/Doom").Body)
		if string(body) != "doom-1.10.0" {
			t.Fatalf("body = %q", body)
		}
	})
}
