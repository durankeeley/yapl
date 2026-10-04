package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"yapl/internal/registry/store"
)

func TestBinary(t *testing.T) {
	t.Run("given a server binary when it is requested then the bytes and checksum are served", func(t *testing.T) {
		// Given
		bin := filepath.Join(t.TempDir(), "yapl")
		os.WriteFile(bin, []byte("ELF-yapl"), 0755)
		st := store.New(t.TempDir(), io.Discard)
		h, err := NewWithBinary(Config{Store: st, Auth: AuthOpen}, bin)
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(h)
		defer srv.Close()

		// When
		resp, err := http.Get(srv.URL + BinaryPath)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		// Then
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK || string(body) != "ELF-yapl" || resp.Header.Get(SHA256Header) != shaHex("ELF-yapl") {
			t.Fatalf("status %d body %q sha %q", resp.StatusCode, body, resp.Header.Get(SHA256Header))
		}
	})

	t.Run("given no binary is configured when it is requested then 404", func(t *testing.T) {
		srv := httptest.NewServer(New(Config{Store: store.New(t.TempDir(), io.Discard), Auth: AuthOpen}))
		defer srv.Close()
		resp, err := http.Get(srv.URL + BinaryPath)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})

	t.Run("given a binary path that does not exist then building the handler fails", func(t *testing.T) {
		if _, err := NewWithBinary(Config{Store: store.New(t.TempDir(), io.Discard)}, "/nonexistent/yapl"); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("given any server then info reports its platform", func(t *testing.T) {
		srv := httptest.NewServer(New(Config{Store: store.New(t.TempDir(), io.Discard), Auth: AuthOpen}))
		defer srv.Close()
		resp, err := http.Get(srv.URL + "/v1/info")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var info Info
		json.NewDecoder(resp.Body).Decode(&info)
		if info.Platform != runtime.GOOS+"/"+runtime.GOARCH {
			t.Fatalf("platform = %q", info.Platform)
		}
	})
}
