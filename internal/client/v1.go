package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"yapl/internal/registry/api"
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

func (c *Client) do(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.cfg.Server+path, body)
	if err != nil {
		return nil, err
	}
	c.applyAuth(req)
	return c.httpClient.Do(req)
}

// serverError turns a non-success response into an error carrying the
// server's own explanation when it sent one.
func serverError(resp *http.Response) error {
	var body errorBody
	if json.NewDecoder(resp.Body).Decode(&body) == nil && body.Error != "" {
		return fmt.Errorf("server returned %s: %s", resp.Status, body.Error)
	}
	return fmt.Errorf("server returned %s", resp.Status)
}

func packagePath(name, version string) string {
	return "/v1/packages/" + url.PathEscape(name) + "/" + url.PathEscape(version)
}

func componentPath(kind, version string) string {
	return "/v1/components/" + url.PathEscape(kind) + "/" + url.PathEscape(version)
}
