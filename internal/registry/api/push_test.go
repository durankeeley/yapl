package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yapl/internal/config"
	"yapl/internal/manifest"
	"yapl/internal/registry/store"
)

type part struct{ field, filename, body string }

func multipartBody(t *testing.T, parts ...part) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, p := range parts {
		fw, err := w.CreateFormFile(p.field, p.filename)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(fw, p.body)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

func manifestPart(t *testing.T, v any) part {
	t.Helper()
	data, _ := json.Marshal(v)
	return part{PartManifest, "manifest.json", string(data)}
}

func pushServer(t *testing.T, allowPush bool, auth, password string) (*httptest.Server, *store.Store) {
	t.Helper()
	root := t.TempDir()
	publish(t, root, "Doom", "1.0.0", []byte("doom"), false)
	st := store.New(root, io.Discard)
	st.Refresh()
	srv := httptest.NewServer(New(Config{Store: st, AllowPush: allowPush, Auth: auth, Password: password}))
	t.Cleanup(srv.Close)
	return srv, st
}

func put(t *testing.T, url string, body io.Reader, contentType string, password string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url, body)
	req.Header.Set("Content-Type", contentType)
	if password != "" {
		req.SetBasicAuth("yapl", password)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func doomManifest(ver string, content string) manifest.Manifest {
	return manifest.Manifest{
		SchemaVersion: 1, Name: "Doom", Type: "game",
		Metadata: manifest.Metadata{Title: "Doom", Version: ver},
		Archive:  manifest.Archive{File: "Doom.tar.zst", Format: "zst", SHA256: shaHex(content), SizeBytes: int64(len(content))},
	}
}

func TestPushPackage(t *testing.T) {
	t.Run("given push is enabled when a valid package is uploaded then it is created and listed", func(t *testing.T) {
		// Given
		srv, st := pushServer(t, true, "open", "")
		body, ct := multipartBody(t, part{PartArchive, "Doom.tar.zst", "doom-2"}, part{PartCover, "cover.PNG", "png"}, manifestPart(t, doomManifest("2.0.0", "doom-2")))

		// When
		resp := put(t, srv.URL+"/v1/packages/Doom/2.0.0", body, ct, "")

		// Then
		if resp.StatusCode != http.StatusCreated {
			msg, _ := io.ReadAll(resp.Body)
			t.Fatalf("status %d: %s", resp.StatusCode, msg)
		}
		v, err := st.Resolve("Doom", "latest")
		if err != nil || v.Manifest.Metadata.Version != "2.0.0" || v.CoverPath == "" {
			t.Fatalf("latest = %+v, %v", v, err)
		}
	})

	t.Run("given push is disabled when a package is uploaded then it is forbidden with an explanation", func(t *testing.T) {
		srv, _ := pushServer(t, false, "open", "")
		body, ct := multipartBody(t, part{PartArchive, "a", "x"})
		resp := put(t, srv.URL+"/v1/packages/Doom/2.0.0", body, ct, "")
		msg, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(msg), "push is disabled") {
			t.Fatalf("status %d: %s", resp.StatusCode, msg)
		}
	})

	t.Run("given a version that already exists when uploaded then it conflicts before the body is stored", func(t *testing.T) {
		srv, _ := pushServer(t, true, "open", "")
		body, ct := multipartBody(t, part{PartArchive, "a", "x"}, manifestPart(t, doomManifest("1.0.0", "x")))
		if resp := put(t, srv.URL+"/v1/packages/Doom/1.0.0", body, ct, ""); resp.StatusCode != http.StatusConflict {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given corrupted bytes when uploaded then it is rejected and not listed", func(t *testing.T) {
		srv, st := pushServer(t, true, "open", "")
		body, ct := multipartBody(t, part{PartArchive, "Doom.tar.zst", "corrupt"}, manifestPart(t, doomManifest("2.0.0", "original")))
		resp := put(t, srv.URL+"/v1/packages/Doom/2.0.0", body, ct, "")
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status %d", resp.StatusCode)
		}
		if v, _ := st.Resolve("Doom", "latest"); v.Manifest.Metadata.Version != "1.0.0" {
			t.Fatal("corrupt upload became latest")
		}
	})

	t.Run("given no manifest part when uploaded then it is rejected", func(t *testing.T) {
		srv, _ := pushServer(t, true, "open", "")
		body, ct := multipartBody(t, part{PartArchive, "Doom.tar.zst", "x"})
		if resp := put(t, srv.URL+"/v1/packages/Doom/2.0.0", body, ct, ""); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given a body that is not multipart when uploaded then it is rejected", func(t *testing.T) {
		srv, _ := pushServer(t, true, "open", "")
		if resp := put(t, srv.URL+"/v1/packages/Doom/2.0.0", strings.NewReader("raw"), "application/octet-stream", ""); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given password auth when uploading without credentials then it is unauthorized", func(t *testing.T) {
		srv, _ := pushServer(t, true, "password", "secret")
		body, ct := multipartBody(t, part{PartArchive, "a", "x"})
		if resp := put(t, srv.URL+"/v1/packages/Doom/2.0.0", body, ct, ""); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given push enabled then info advertises it", func(t *testing.T) {
		srv, _ := pushServer(t, true, "open", "")
		resp, err := http.Get(srv.URL + "/v1/info")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var info Info
		json.NewDecoder(resp.Body).Decode(&info)
		if !info.PushEnabled {
			t.Fatal("push_enabled not advertised")
		}
	})
}

func protonManifest(content string) manifest.ComponentManifest {
	return manifest.ComponentManifest{
		SchemaVersion: 1, Kind: "proton", Version: "GE-Proton11-7",
		Archive:     manifest.Archive{File: manifest.ComponentArchiveFile, Format: "zst", SHA256: shaHex(content), SizeBytes: int64(len(content))},
		VersionInfo: config.VersionInfo{BinPath: "files/bin"},
	}
}

func TestComponentsAPI(t *testing.T) {
	t.Run("given an uploaded component when fetched then manifest, archive and listing all serve it", func(t *testing.T) {
		// Given
		srv, _ := pushServer(t, true, "open", "")
		body, ct := multipartBody(t, part{PartArchive, "component.tar.zst", "proton-bytes"}, manifestPart(t, protonManifest("proton-bytes")))
		if resp := put(t, srv.URL+"/v1/components/proton/GE-Proton11-7", body, ct, ""); resp.StatusCode != http.StatusCreated {
			msg, _ := io.ReadAll(resp.Body)
			t.Fatalf("upload status %d: %s", resp.StatusCode, msg)
		}

		// When
		head, err := http.Head(srv.URL + "/v1/components/proton/GE-Proton11-7/manifest")
		if err != nil {
			t.Fatal(err)
		}
		head.Body.Close()
		archiveResp, err := http.Get(srv.URL + "/v1/components/proton/GE-Proton11-7/archive")
		if err != nil {
			t.Fatal(err)
		}
		defer archiveResp.Body.Close()
		listResp, err := http.Get(srv.URL + "/v1/components")
		if err != nil {
			t.Fatal(err)
		}
		defer listResp.Body.Close()

		// Then
		data, _ := io.ReadAll(archiveResp.Body)
		var list []manifest.ComponentManifest
		json.NewDecoder(listResp.Body).Decode(&list)
		if head.StatusCode != http.StatusOK || string(data) != "proton-bytes" || archiveResp.Header.Get(SHA256Header) != shaHex("proton-bytes") || len(list) != 1 {
			t.Fatalf("head %d archive %q list %+v", head.StatusCode, data, list)
		}
	})

	t.Run("given a missing component when its manifest is checked then 404", func(t *testing.T) {
		srv, _ := pushServer(t, true, "open", "")
		resp, err := http.Head(srv.URL + "/v1/components/proton/nope/manifest")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given an unknown component kind then 400", func(t *testing.T) {
		srv, _ := pushServer(t, true, "open", "")
		resp, err := http.Get(srv.URL + "/v1/components/drivers/1/manifest")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given an existing component when uploaded again then it conflicts", func(t *testing.T) {
		srv, _ := pushServer(t, true, "open", "")
		for i, want := range []int{http.StatusCreated, http.StatusConflict} {
			body, ct := multipartBody(t, part{PartArchive, "c", "p"}, manifestPart(t, protonManifest("p")))
			if resp := put(t, srv.URL+"/v1/components/proton/GE-Proton11-7", body, ct, ""); resp.StatusCode != want {
				t.Fatalf("upload %d status %d, want %d", i, resp.StatusCode, want)
			}
		}
	})
}
