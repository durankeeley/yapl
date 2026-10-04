package console

import (
	"errors"
	"io"

	"yapl/internal/client"
	"yapl/internal/manifest"
	"yapl/internal/progress"
	"yapl/internal/pull"
	"yapl/internal/registry/api"
	"yapl/internal/upgrade"
)

// Registry is the console's view of a YAPL server. The production
// implementation wraps the client; tests use an in-memory fake.
type Registry interface {
	Server() string
	Reachable() bool
	Catalog() ([]api.CatalogEntry, error)
	Cover(name string) (io.ReadCloser, string, error)
	Install(name string, report progress.Func) error
	Upgrade(name string, report progress.Func) error
}

// NewRegistry adapts a client to Registry.
func NewRegistry(c *client.Client) Registry { return remote{c: c} }

type remote struct{ c *client.Client }

func (r remote) Server() string { return r.c.Server() }

func (r remote) Reachable() bool {
	_, err := r.c.Info()
	return err == nil || errors.Is(err, client.ErrNotFound) || errors.Is(err, client.ErrUnauthorized)
}

func (r remote) Catalog() ([]api.CatalogEntry, error) { return r.c.Catalog() }

func (r remote) Cover(name string) (io.ReadCloser, string, error) {
	return r.c.OpenCover(name, manifest.LatestTag)
}

func (r remote) Install(name string, report progress.Func) error {
	_, err := pull.Pull(r.c, name, manifest.LatestTag, pull.Options{Report: report})
	return err
}

func (r remote) Upgrade(name string, report progress.Func) error {
	_, _, err := upgrade.Game(r.c, name, manifest.LatestTag, upgrade.Options{Report: report})
	return err
}
