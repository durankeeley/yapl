// Package connect decides which YAPL server a command talks to: the one
// named on the command line, else the one remembered in client.json (if it
// answers), else one found by LAN discovery.
package connect

import (
	"errors"
	"fmt"
	"io"

	"yapl/internal/client"
	"yapl/internal/config"
	"yapl/internal/defaults"
)

// ErrNoServer means no server was named, remembered or discovered.
var ErrNoServer = errors.New("no YAPL server found")

// Options are the user's choices; zero values mean "not given".
type Options struct {
	Server   string
	Auth     string
	Password string
	// AuthExplicit is true when --auth was passed, so it overrides the
	// remembered auth mode.
	AuthExplicit  bool
	SettingsPath  string
	Discover      bool
	DiscoveryPort int
}

// Connect returns a client for the chosen server. Progress notes go to out.
func Connect(opts Options, out io.Writer) (*client.Client, error) {
	auth := opts.Auth
	if auth == "" {
		auth = defaults.AuthMode
	}
	newClient := func(server, auth string) *client.Client {
		return client.New(client.Config{Server: server, Auth: auth, Password: opts.Password})
	}
	if opts.Server != "" {
		return newClient(opts.Server, auth), nil
	}

	saved, _ := config.LoadClientSettings(opts.SettingsPath)
	if saved.Server != "" {
		savedAuth := auth
		if !opts.AuthExplicit && saved.Auth != "" {
			savedAuth = saved.Auth
		}
		c := newClient(saved.Server, savedAuth)
		if Reachable(c) {
			return c, nil
		}
		fmt.Fprintf(out, "-> Saved server %s is not answering\n", saved.Server)
	}
	if !opts.Discover {
		return nil, fmt.Errorf("%w: pass --server <host:port>", ErrNoServer)
	}
	fmt.Fprintln(out, "-> Discovering YAPL server on LAN...")
	found, err := client.Discover(opts.DiscoveryPort, defaults.DiscoveryTimeout)
	if err != nil {
		return nil, fmt.Errorf("%w: %v; pass --server <host:port>", ErrNoServer, err)
	}
	fmt.Fprintf(out, "-> Found server at %s\n", found)
	return newClient(found, auth), nil
}

// Reachable reports whether a server answers at all; servers that predate
// /v1/info answer it with 404 or 401, which still proves they are there.
func Reachable(c *client.Client) bool {
	_, err := c.Info()
	return err == nil || errors.Is(err, client.ErrNotFound) || errors.Is(err, client.ErrUnauthorized)
}

// Remember saves the server that just worked. The password is never written.
func Remember(settingsPath string, c *client.Client, auth string) error {
	return config.SaveClientSettings(settingsPath, config.ClientSettings{Server: c.Server(), Auth: auth})
}
