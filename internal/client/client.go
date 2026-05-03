package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PackageEntry describes a package available on a YAPL server.
type PackageEntry struct {
	Name      string `json:"name"`
	Type      string `json:"type"` // "game" or "app"
	Format    string `json:"format"`
	SizeBytes int64  `json:"size_bytes"`
}

// Config holds client connection settings.
type Config struct {
	Server   string // base URL or host:port
	Auth     string // "open" or "password"
	Password string
}

// Client communicates with a YAPL package server.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// New creates a Client.
func New(cfg Config) *Client {
	baseURL := cfg.Server
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	cfg.Server = strings.TrimRight(baseURL, "/")
	return &Client{cfg: cfg, httpClient: &http.Client{}}
}

// List fetches the package manifest from the server.
func (c *Client) List() ([]PackageEntry, error) {
	req, err := http.NewRequest("GET", c.cfg.Server+"/packages", nil)
	if err != nil {
		return nil, err
	}
	c.applyAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var entries []PackageEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return entries, nil
}

// Download fetches the named package, writes it to outputDir, and returns the
// downloaded file path and the entry type ("game" or "app").
// Progress is written to progressW. The partial file is removed on failure.
func (c *Client) Download(name, outputDir string, progressW io.Writer) (archivePath, entryType string, err error) {
	entries, err := c.List()
	if err != nil {
		return "", "", fmt.Errorf("list packages: %w", err)
	}
	var entry *PackageEntry
	for i := range entries {
		if entries[i].Name == name {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return "", "", fmt.Errorf("package %q not found on server", name)
	}

	ext := formatExtension(entry.Format)
	destPath := filepath.Join(outputDir, name+ext)

	req, err := http.NewRequest("GET", c.cfg.Server+"/packages/"+name, nil)
	if err != nil {
		return "", "", err
	}
	c.applyAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("server returned %d for package %q", resp.StatusCode, name)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return "", "", fmt.Errorf("create output file: %w", err)
	}

	total := entry.SizeBytes
	var written int64
	buf := make([]byte, 32*1024)
	var copyErr error
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			wn, writeErr := f.Write(buf[:n])
			written += int64(wn)
			if writeErr != nil {
				copyErr = writeErr
				break
			}
			if total > 0 && progressW != nil {
				pct := written * 100 / total
				fmt.Fprintf(progressW, "\rDownloading %s: %d%% (%d MB / %d MB)",
					name, pct, written>>20, total>>20)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			copyErr = readErr
			break
		}
	}
	f.Close()

	if copyErr != nil {
		os.Remove(destPath)
		return "", "", fmt.Errorf("download %q: %w", name, copyErr)
	}

	if progressW != nil {
		fmt.Fprintln(progressW)
	}
	return destPath, entry.Type, nil
}

// Discover listens on UDP port for a YAPL broadcast and returns the server address.
func Discover(port int, timeout time.Duration) (string, error) {
	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return "", fmt.Errorf("open discovery socket: %w", err)
	}
	return DiscoverFrom(conn, timeout)
}

// DiscoverFrom reads a YAPL discovery datagram from conn and returns the server address.
// Exported so tests can inject a pre-bound connection.
func DiscoverFrom(conn net.PacketConn, timeout time.Duration) (string, error) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	buf := make([]byte, 256)
	n, addr, err := conn.ReadFrom(buf)
	if err != nil {
		return "", fmt.Errorf("no YAPL server found on LAN (timed out): %w", err)
	}

	msg := string(buf[:n])
	var httpPort int
	if _, scanErr := fmt.Sscanf(msg, "YAPL:%d", &httpPort); scanErr != nil || httpPort == 0 {
		return "", fmt.Errorf("unrecognised discovery message: %q", msg)
	}

	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return "", fmt.Errorf("unexpected address type from discovery")
	}
	return fmt.Sprintf("%s:%d", udpAddr.IP.String(), httpPort), nil
}

func (c *Client) applyAuth(req *http.Request) {
	if c.cfg.Auth == "password" {
		req.SetBasicAuth("yapl", c.cfg.Password)
	}
}

func formatExtension(format string) string {
	switch format {
	case "gz":
		return ".tar.gz"
	case "zst":
		return ".tar.zst"
	default:
		return ".tar.xz"
	}
}
