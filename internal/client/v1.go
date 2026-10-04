package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"yapl/internal/manifest"
	"yapl/internal/progress"
	"yapl/internal/registry/api"
)

// ErrNotFound wraps every 404 and ErrUnauthorized every 401 from the
// registry, so callers can react without parsing messages.
var (
	ErrNotFound     = errors.New("not found on server")
	ErrUnauthorized = errors.New("unauthorized")
)

// errorBody is the JSON error shape the registry API returns.
type errorBody struct {
	Error string `json:"error"`
}

// Info fetches /v1/info. Older servers without the v1 API return an error.
func (c *Client) Info() (api.Info, error) {
	var info api.Info
	err := c.getJSON("/v1/info", &info)
	return info, err
}

// HasComponent reports whether the server stores kind/version.
func (c *Client) HasComponent(kind, version string) (bool, error) {
	resp, err := c.do(http.MethodHead, componentPath(kind, version)+"/manifest", nil)
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("check component %s/%s: server returned %s", kind, version, resp.Status)
	}
}

func (c *Client) getJSON(path string, v any) error {
	resp, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return serverError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// do sends a small metadata request; it uses a short timeout so an
// unreachable server fails fast, unlike archive transfers.
func (c *Client) do(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.cfg.Server+path, body)
	if err != nil {
		return nil, err
	}
	c.applyAuth(req)
	return c.metadataClient.Do(req)
}

// serverError turns a non-success response into an error carrying the
// server's own explanation when it sent one.
func serverError(resp *http.Response) error {
	msg := fmt.Sprintf("server returned %s", resp.Status)
	var body errorBody
	if json.NewDecoder(resp.Body).Decode(&body) == nil && body.Error != "" {
		msg += ": " + body.Error
	}
	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, msg)
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %s (check --auth and --password)", ErrUnauthorized, msg)
	}
	return errors.New(msg)
}

func packagePath(name, version string) string {
	return "/v1/packages/" + url.PathEscape(name) + "/" + url.PathEscape(version)
}

func componentPath(kind, version string) string {
	return "/v1/components/" + url.PathEscape(kind) + "/" + url.PathEscape(version)
}

// Server is the normalised base URL this client talks to.
func (c *Client) Server() string { return c.cfg.Server }

// Catalog fetches /v1/packages.
func (c *Client) Catalog() ([]api.CatalogEntry, error) {
	var out []api.CatalogEntry
	err := c.getJSON("/v1/packages", &out)
	return out, err
}

// Manifest fetches one package version's manifest; tag may be "latest".
func (c *Client) Manifest(name, tag string) (manifest.Manifest, error) {
	var m manifest.Manifest
	if err := c.getJSON(packagePath(name, tag)+"/manifest", &m); err != nil {
		return m, fmt.Errorf("%s:%s: %w", name, tag, err)
	}
	return m, m.Validate()
}

// ComponentManifest fetches a component's manifest.
func (c *Client) ComponentManifest(kind, version string) (manifest.ComponentManifest, error) {
	var m manifest.ComponentManifest
	if err := c.getJSON(componentPath(kind, version)+"/manifest", &m); err != nil {
		return m, fmt.Errorf("component %s/%s: %w", kind, version, err)
	}
	return m, m.Validate()
}

// OpenArchive starts downloading a package archive. The caller closes it.
func (c *Client) OpenArchive(name, version string) (io.ReadCloser, int64, error) {
	return c.open(packagePath(name, version) + "/archive")
}

// OpenComponentArchive starts downloading a component archive.
func (c *Client) OpenComponentArchive(kind, version string) (io.ReadCloser, int64, error) {
	return c.open(componentPath(kind, version) + "/archive")
}

// ComponentURL is the absolute download URL of a component, recorded in
// runner.json so the component can be fetched again later.
func (c *Client) ComponentURL(kind, version string) string {
	return c.cfg.Server + componentPath(kind, version) + "/archive"
}

func (c *Client) open(path string) (io.ReadCloser, int64, error) {
	req, err := http.NewRequest(http.MethodGet, c.cfg.Server+path, nil)
	if err != nil {
		return nil, 0, err
	}
	c.applyAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, 0, serverError(resp)
	}
	return resp.Body, resp.ContentLength, nil
}

// DownloadBinary saves the server's yapl binary to dest, verifying it
// against the checksum header. dest is removed on any failure.
func (c *Client) DownloadBinary(dest string, report progress.Func) error {
	req, err := http.NewRequest(http.MethodGet, c.cfg.Server+api.BinaryPath, nil)
	if err != nil {
		return err
	}
	c.applyAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return serverError(resp)
	}
	want := resp.Header.Get(api.SHA256Header)
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, executableMode)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, h), progress.NewReader(resp.Body, "yapl", resp.ContentLength, report))
	if closeErr := f.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if got := hex.EncodeToString(h.Sum(nil)); copyErr == nil && (want == "" || got != want) {
		copyErr = fmt.Errorf("binary checksum mismatch: got %s, server says %q", got, want)
	}
	if copyErr != nil {
		os.Remove(dest)
		return copyErr
	}
	return nil
}

// executableMode is applied to a downloaded yapl binary.
const executableMode = 0755
