// Package detect finds the executable a game installer created, so
// `yapl install` can fill in game.json without the user hunting for it.
package detect

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Ranking weights. A full name match beats initials, which beat matching
// words; every directory level costs a little so the top-level exe wins ties.
const (
	FullNameWeight = 1000
	AcronymWeight  = 800
	TokenWeight    = 100
	DepthPenalty   = 10

	minTokenLength = 3
	exeSuffix      = ".exe"
	driveC         = "drive_c"
	windowsDir     = "windows"
)

var stopWords = map[string]bool{"the": true, "of": true, "for": true, "and": true}

// ignoredPrefixes and ignoredSubstrings match helper executables that are
// never the game itself: uninstallers, redistributables, crash reporters.
var (
	ignoredPrefixes   = []string{"unins", "setup", "install", "vcredist", "vc_redist", "dxsetup", "dxwebsetup", "dotnet", "ndp", "oalinst", "physx", "update", "directx"}
	ignoredSubstrings = []string{"crash", "report", "redist"}
)

// Executable is a candidate, with Path relative to the prefix in slash form.
type Executable struct {
	Path string
	Size int64
}

// Snapshot lists every .exe under drive_c (excluding drive_c/windows) with
// its size, keyed by slash-separated path relative to prefix.
func Snapshot(prefix string) (map[string]int64, error) {
	found := map[string]int64{}
	root := filepath.Join(prefix, driveC)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return found, nil
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root && filepath.Dir(path) == root && strings.EqualFold(d.Name(), windowsDir) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), exeSuffix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(prefix, path)
		if err != nil {
			return err
		}
		found[filepath.ToSlash(rel)] = info.Size()
		return nil
	})
	return found, err
}

// Candidates returns executables present in after but not before, excluding
// helpers, ranked best first for a game called gameName.
func Candidates(before, after map[string]int64, gameName string) []Executable {
	var out []Executable
	for path, size := range after {
		if _, existed := before[path]; existed || Ignored(filepath.Base(path)) {
			continue
		}
		out = append(out, Executable{Path: path, Size: size})
	}
	scores := make(map[string]int, len(out))
	for _, e := range out {
		scores[e.Path] = score(e.Path, gameName)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if scores[a.Path] != scores[b.Path] {
			return scores[a.Path] > scores[b.Path]
		}
		if a.Size != b.Size {
			return a.Size > b.Size
		}
		return a.Path < b.Path
	})
	return out
}

// Ignored reports whether an executable's file name marks it as a helper.
func Ignored(fileName string) bool {
	base := strings.ToLower(strings.TrimSuffix(strings.ToLower(fileName), exeSuffix))
	for _, p := range ignoredPrefixes {
		if strings.HasPrefix(base, p) {
			return true
		}
	}
	for _, s := range ignoredSubstrings {
		if strings.Contains(base, s) {
			return true
		}
	}
	return false
}

func score(path, gameName string) int {
	base := normalize(strings.TrimSuffix(strings.ToLower(filepath.Base(path)), exeSuffix))
	words := words(gameName)
	s := -DepthPenalty * strings.Count(path, "/")

	full := strings.Join(words, "")
	switch {
	case full != "" && strings.Contains(base, full):
		s += FullNameWeight
	case len(words) > 1 && base == acronym(words):
		s += AcronymWeight
	}
	for _, w := range words {
		if len(w) >= minTokenLength && !stopWords[w] && strings.Contains(base, w) {
			s += TokenWeight
		}
	}
	return s
}

func words(name string) []string {
	return strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func acronym(words []string) string {
	var b strings.Builder
	for _, w := range words {
		b.WriteByte(w[0])
	}
	return b.String()
}

func normalize(s string) string {
	return strings.Join(words(s), "")
}
