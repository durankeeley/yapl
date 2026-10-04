// Package desktop writes freedesktop.org .desktop entries so games appear
// in application menus and the console can start on login.
package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// DefaultIcon is the themed icon used when a game has no cover art.
const DefaultIcon = "applications-games"

const (
	fileMode      = 0644
	fallbackSlug  = "game"
	execReserved  = " \t\n\"'\\><~|&;$*?#()`"
	quoteEscapes  = "\"`$\\"
	fieldCodeMark = "%"
)

// Entry is one .desktop file.
type Entry struct {
	Name       string
	Comment    string
	Exec       []string
	Icon       string
	Categories []string
	// Extra holds additional keys, written in order after the standard ones.
	Extra [][2]string
}

// Slug turns a game name into a safe, readable file-name component.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.TrimSuffix(b.String(), "-")
	if slug == "" {
		return fallbackSlug
	}
	return slug
}

// ExecLine quotes arguments as the Desktop Entry spec requires: arguments
// with reserved characters are double-quoted with ", `, $ and \ escaped,
// and % is doubled so it is never read as a field code.
func ExecLine(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		a = strings.ReplaceAll(a, fieldCodeMark, fieldCodeMark+fieldCodeMark)
		if !strings.ContainsAny(a, execReserved) {
			out[i] = a
			continue
		}
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range a {
			if strings.ContainsRune(quoteEscapes, r) {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		b.WriteByte('"')
		out[i] = b.String()
	}
	return strings.Join(out, " ")
}

// Render produces the file contents.
func Render(e Entry) string {
	icon := e.Icon
	if icon == "" {
		icon = DefaultIcon
	}
	var b strings.Builder
	line := func(k, v string) {
		if v != "" {
			b.WriteString(k + "=" + singleLine(v) + "\n")
		}
	}
	b.WriteString("[Desktop Entry]\n")
	line("Type", "Application")
	line("Name", e.Name)
	line("Comment", e.Comment)
	line("Exec", ExecLine(e.Exec))
	line("Icon", icon)
	if len(e.Categories) > 0 {
		line("Categories", strings.Join(e.Categories, ";")+";")
	}
	line("Terminal", "false")
	for _, kv := range e.Extra {
		line(kv[0], kv[1])
	}
	return b.String()
}

// singleLine keeps a value on one line so it can never start a new key.
func singleLine(v string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(v)
}

// Write saves e as dir/fileName, creating dir if needed.
func Write(dir, fileName string, e Entry) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fileName)
	return path, os.WriteFile(path, []byte(Render(e)), fileMode)
}

// ApplicationsDir is where menu entries live ($XDG_DATA_HOME/applications).
func ApplicationsDir() string {
	return filepath.Join(xdgDir("XDG_DATA_HOME", ".local/share"), "applications")
}

// AutostartDir is where login autostart entries live.
func AutostartDir() string {
	return filepath.Join(xdgDir("XDG_CONFIG_HOME", ".config"), "autostart")
}

func xdgDir(env, homeDefault string) string {
	if d := os.Getenv(env); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), homeDefault)
}
