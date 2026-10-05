// Package upgrade moves only the parts of a YAPL install whose versions
// changed: individual games, the yapl binary (per module) and the pinned
// defaults catalog.
package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"yapl/internal/fs"
	"yapl/internal/kernel"
	"yapl/internal/manifest"
	"yapl/internal/paths"
	"yapl/internal/registry/api"
	"yapl/internal/version"
)

// Parts that can be upgraded besides games and apps.
const (
	PartYAPL    = "yapl"
	PartCatalog = "catalog"

	kernelPart  = "kernel"
	missingPart = "new"
	noneVersion = "none"
)

// Item is one outdated part.
type Item struct {
	Part string
	Name string
	From string
	To   string
}

// LocalState is what is installed on this machine.
type LocalState struct {
	Installed     []manifest.Installed
	Binary        kernel.Versions
	RunnerCatalog string
	BinaryCatalog string
}

// RemoteState is what the server offers.
type RemoteState struct {
	Catalog []api.CatalogEntry
	Binary  kernel.Versions
}

// Plan lists every outdated part. It is pure, so every rule is table-tested.
func Plan(local LocalState, remote RemoteState) []Item {
	var items []Item
	latest := map[string]string{}
	for _, e := range remote.Catalog {
		latest[e.Name] = e.Latest
	}
	for _, in := range local.Installed {
		if to, ok := latest[in.Name]; ok && version.Compare(to, in.Version) > 0 {
			items = append(items, Item{Part: in.Type, Name: in.Name, From: in.Version, To: to})
		}
	}
	if changes, ok := binaryChanges(local.Binary, remote.Binary); ok && len(changes.newer) > 0 && len(changes.older) == 0 {
		items = append(items, Item{Part: PartYAPL, Name: PartYAPL, From: changes.from(), To: changes.to()})
	}
	if local.RunnerCatalog != local.BinaryCatalog && (local.RunnerCatalog == "" || version.Compare(local.RunnerCatalog, local.BinaryCatalog) < 0) {
		from := local.RunnerCatalog
		if from == "" {
			from = noneVersion
		}
		items = append(items, Item{Part: PartCatalog, Name: paths.RunnerConfig, From: from, To: local.BinaryCatalog})
	}
	return items
}

// NewerBinary decides whether the server's binary should replace this one.
// It must be newer in at least one part and older in none; anything else is
// a downgrade or a divergent build.
func NewerBinary(local, remote kernel.Versions) (bool, string) {
	changes, ok := binaryChanges(local, remote)
	switch {
	case !ok:
		return false, "the server did not report its version"
	case len(changes.older) > 0:
		return false, "the server's yapl has older parts (" + strings.Join(changes.older, ", ") + "); upgrade the server instead"
	case len(changes.newer) == 0:
		return false, "yapl is already up to date"
	}
	return true, changes.from() + " -> " + changes.to()
}

type change struct{ part, from, to string }

type binaryDiff struct {
	newer []change
	older []string
}

func (d binaryDiff) from() string {
	var out []string
	for _, c := range d.newer {
		out = append(out, c.part+" "+c.from)
	}
	return strings.Join(out, ", ")
}

func (d binaryDiff) to() string {
	var out []string
	for _, c := range d.newer {
		out = append(out, c.part+" "+c.to)
	}
	return strings.Join(out, ", ")
}

func binaryChanges(local, remote kernel.Versions) (binaryDiff, bool) {
	if remote.Kernel == "" {
		return binaryDiff{}, false
	}
	var d binaryDiff
	compare := func(part, l, r string) {
		switch {
		case r == "":
			d.older = append(d.older, part+" missing")
		case l == "":
			d.newer = append(d.newer, change{part: part, from: missingPart, to: r})
		case version.Compare(r, l) > 0:
			d.newer = append(d.newer, change{part: part, from: l, to: r})
		case version.Compare(r, l) < 0:
			d.older = append(d.older, fmt.Sprintf("%s %s < %s", part, r, l))
		}
	}
	compare(kernelPart, local.Kernel, remote.Kernel)
	for _, name := range moduleNames(local, remote) {
		compare(name, local.Modules[name], remote.Modules[name])
	}
	return d, true
}

func moduleNames(a, b kernel.Versions) []string {
	set := map[string]bool{}
	for k := range a.Modules {
		set[k] = true
	}
	for k := range b.Modules {
		set[k] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Installed lists every package in the library that was pulled from a
// registry (has installed.json), games first, each sorted by name.
func Installed() ([]manifest.Installed, error) {
	var out []manifest.Installed
	for _, typeDir := range []string{paths.GamesDir, paths.AppsDir} {
		entries, err := os.ReadDir(typeDir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !fs.IsDir(filepath.Join(typeDir, e.Name())) {
				continue
			}
			if in, err := manifest.ReadInstalled(filepath.Join(typeDir, e.Name())); err == nil {
				out = append(out, in)
			}
		}
	}
	return out, nil
}
