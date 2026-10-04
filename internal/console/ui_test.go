package console

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestUI(t *testing.T) {
	srv := httptest.NewServer(UI())
	defer srv.Close()
	get := func(t *testing.T, path string) (*http.Response, string) {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}

	t.Run("given the console root then the html page is served", func(t *testing.T) {
		resp, body := get(t, "/")
		if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") || !strings.Contains(body, "YAPL Console") {
			t.Fatalf("status %d type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	})

	t.Run("given the page then every asset it references is served with a browser-friendly type", func(t *testing.T) {
		_, page := get(t, "/")
		refs := regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllStringSubmatch(page, -1)
		if len(refs) < 2 {
			t.Fatalf("page references %d assets", len(refs))
		}
		for _, ref := range refs {
			resp, body := get(t, "/"+ref[1])
			if resp.StatusCode != http.StatusOK || body == "" {
				t.Errorf("%s: status %d", ref[1], resp.StatusCode)
			}
			ct := resp.Header.Get("Content-Type")
			if strings.HasSuffix(ref[1], ".js") && !strings.Contains(ct, "javascript") || strings.HasSuffix(ref[1], ".css") && !strings.Contains(ct, "text/css") {
				t.Errorf("%s served as %q", ref[1], ct)
			}
		}
	})

	t.Run("given app.js then every module it imports is embedded", func(t *testing.T) {
		_, app := get(t, "/app.js")
		for _, imp := range regexp.MustCompile(`from '\./([^']+)'`).FindAllStringSubmatch(app, -1) {
			if resp, _ := get(t, "/"+imp[1]); resp.StatusCode != http.StatusOK {
				t.Errorf("import %s not embedded", imp[1])
			}
		}
	})

	t.Run("given the test file then it is not shipped", func(t *testing.T) {
		if resp, _ := get(t, "/nav.test.mjs"); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("test file served with status %d", resp.StatusCode)
		}
	})

	t.Run("given the ui source then server data is never written with innerHTML", func(t *testing.T) {
		_, app := get(t, "/app.js")
		if strings.Contains(app, "innerHTML") {
			t.Fatal("app.js uses innerHTML; build nodes with el() so titles can't inject markup")
		}
	})
}

func TestUILogicWithNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; skipping the UI logic tests")
	}
	out, err := exec.Command(node, "--test", "web/nav.test.mjs").CombinedOutput()
	if err != nil {
		t.Fatalf("node tests failed:\n%s", out)
	}
}

func TestUIStylesRespectHidden(t *testing.T) {
	t.Run("given elements styled with display rules then the hidden attribute still hides them", func(t *testing.T) {
		css, err := webFiles.ReadFile("web/app.css")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(css), "[hidden] { display: none !important; }") {
			t.Fatal("app.css must force [hidden] to display:none, or overlays cover the screen")
		}
	})
}
