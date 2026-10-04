// Package version parses and orders the version strings used across YAPL:
// module versions, package versions and component versions.
package version

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Semantic is a parsed MAJOR.MINOR.PATCH[-PRERELEASE] version.
type Semantic struct {
	Major, Minor, Patch int
	PreRelease          string
}

const maxSemanticParts = 3

// Parse reads a semantic version. A leading "v" is accepted and missing
// minor or patch parts count as zero.
func Parse(s string) (Semantic, error) {
	core, pre, _ := strings.Cut(strings.TrimPrefix(s, "v"), "-")
	parts := strings.Split(core, ".")
	if core == "" || len(parts) > maxSemanticParts {
		return Semantic{}, fmt.Errorf("invalid version %q: want MAJOR.MINOR.PATCH", s)
	}
	nums := make([]int, maxSemanticParts)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Semantic{}, fmt.Errorf("invalid version %q: %q is not a number", s, p)
		}
		nums[i] = n
	}
	return Semantic{Major: nums[0], Minor: nums[1], Patch: nums[2], PreRelease: pre}, nil
}

// Valid reports whether s is a semantic version.
func Valid(s string) bool {
	_, err := Parse(s)
	return err == nil
}

// Compare returns -1, 0 or 1. Semantic versions compare numerically and sort
// before non-semantic ones; non-semantic strings (such as "GE-Proton11-7")
// use natural order so embedded numbers compare by value.
func Compare(a, b string) int {
	va, errA := Parse(a)
	vb, errB := Parse(b)
	switch {
	case errA == nil && errB == nil:
		return compareSemantic(va, vb)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	default:
		return compareNatural(a, b)
	}
}

// IsCompatible reports whether two semantic versions share a major version.
func IsCompatible(a, b string) bool {
	va, errA := Parse(a)
	vb, errB := Parse(b)
	return errA == nil && errB == nil && va.Major == vb.Major
}

// Latest returns the highest version in versions, or "" when empty.
func Latest(versions []string) string {
	latest := ""
	for _, v := range versions {
		if latest == "" || Compare(v, latest) > 0 {
			latest = v
		}
	}
	return latest
}

func compareSemantic(a, b Semantic) int {
	for _, d := range []int{a.Major - b.Major, a.Minor - b.Minor, a.Patch - b.Patch} {
		if d != 0 {
			return sign(d)
		}
	}
	switch {
	case a.PreRelease == b.PreRelease:
		return 0
	case a.PreRelease == "":
		return 1
	case b.PreRelease == "":
		return -1
	default:
		return compareNatural(a.PreRelease, b.PreRelease)
	}
}

func compareNatural(a, b string) int {
	ca, cb := chunks(a), chunks(b)
	for i := 0; i < len(ca) && i < len(cb); i++ {
		na, errA := strconv.Atoi(ca[i])
		nb, errB := strconv.Atoi(cb[i])
		if errA == nil && errB == nil {
			if na != nb {
				return sign(na - nb)
			}
			continue
		}
		if c := strings.Compare(ca[i], cb[i]); c != 0 {
			return c
		}
	}
	return sign(len(ca) - len(cb))
}

// chunks splits s into alternating runs of digits and non-digits.
func chunks(s string) []string {
	var out []string
	start := 0
	for i := 1; i <= len(s); i++ {
		if i == len(s) || unicode.IsDigit(rune(s[i])) != unicode.IsDigit(rune(s[i-1])) {
			out = append(out, s[start:i])
			start = i
		}
	}
	return out
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}
