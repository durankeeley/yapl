package steam

import (
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"yapl/internal/fs"
)

const (
	// ShortcutsFile lives in userdata/<user>/config.
	ShortcutsFile = "shortcuts.vdf"
	shortcutsKey  = "shortcuts"
	gridDir       = "grid"
	// gridPortraitSuffix names the 600x900 library capsule Big Picture shows.
	gridPortraitSuffix = "p"
	// shortcutIDBit marks an app ID as a non-Steam shortcut.
	shortcutIDBit = 0x80000000
	fileMode      = 0644
	enabled       = 1
)

// Placeholder user directories Steam creates that are not real accounts.
var ignoredUsers = map[string]bool{"0": true, "ac": true, "anonymous": true}

// Shortcut is one non-Steam game entry.
type Shortcut struct {
	AppName       string
	Exe           string
	StartDir      string
	Icon          string
	LaunchOptions string
}

// AppID is the ID Steam derives for a non-Steam shortcut; grid art is
// stored under it.
func (s Shortcut) AppID() uint32 {
	return crc32.ChecksumIEEE([]byte(s.Exe+s.AppName)) | shortcutIDBit
}

// Upsert adds s, or updates the existing YAPL shortcut with the same Exe
// and LaunchOptions. Other shortcuts are never touched. It reports whether
// a new entry was added.
func Upsert(root *Node, s Shortcut) bool {
	list := root.Child(shortcutsKey)
	if list == nil {
		list = &Node{Type: TypeMap, Key: shortcutsKey}
		root.Children = append(root.Children, list)
	}
	for _, entry := range list.Children {
		if entry.String("Exe") == s.Exe && entry.String("LaunchOptions") == s.LaunchOptions {
			entry.SetUint32("appid", s.AppID())
			entry.SetString("AppName", s.AppName)
			entry.SetString("StartDir", s.StartDir)
			entry.SetString("icon", s.Icon)
			return false
		}
	}
	entry := &Node{Type: TypeMap, Key: strconv.Itoa(nextIndex(list))}
	entry.SetUint32("appid", s.AppID())
	entry.SetString("AppName", s.AppName)
	entry.SetString("Exe", s.Exe)
	entry.SetString("StartDir", s.StartDir)
	entry.SetString("icon", s.Icon)
	entry.SetString("ShortcutPath", "")
	entry.SetString("LaunchOptions", s.LaunchOptions)
	entry.SetUint32("IsHidden", 0)
	entry.SetUint32("AllowDesktopConfig", enabled)
	entry.SetUint32("AllowOverlay", enabled)
	entry.SetUint32("OpenVR", 0)
	entry.SetUint32("Devkit", 0)
	entry.SetString("DevkitGameID", "")
	entry.SetUint32("DevkitOverrideAppID", 0)
	entry.SetUint32("LastPlayTime", 0)
	entry.SetString("FlatpakAppID", "")
	entry.Children = append(entry.Children, &Node{Type: TypeMap, Key: "tags"})
	list.Children = append(list.Children, entry)
	return true
}

func nextIndex(list *Node) int {
	next := 0
	for _, c := range list.Children {
		if i, err := strconv.Atoi(c.Key); err == nil && i >= next {
			next = i + 1
		}
	}
	return next
}

// Item is a shortcut plus optional cover art for the Steam library grid.
type Item struct {
	Shortcut Shortcut
	Cover    string
}

// Export merges items into configDir/shortcuts.vdf and copies cover art to
// the grid folder. A file Steam can't parse is left alone.
func Export(configDir string, items []Item) (added int, err error) {
	path := filepath.Join(configDir, ShortcutsFile)
	root := &Node{Type: TypeMap}
	if data, err := os.ReadFile(path); err == nil {
		if root, err = Parse(data); err != nil {
			return 0, fmt.Errorf("%s is not a valid shortcuts file, leaving it untouched: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	for _, it := range items {
		if Upsert(root, it.Shortcut) {
			added++
		}
		if it.Cover != "" {
			dst := GridPath(configDir, it.Shortcut.AppID(), filepath.Ext(it.Cover))
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return added, err
			}
			if err := fs.CopyFile(it.Cover, dst); err != nil {
				return added, fmt.Errorf("copy cover: %w", err)
			}
		}
	}
	tmp := path + ".yapl-tmp"
	if err := os.WriteFile(tmp, root.Encode(), fileMode); err != nil {
		return added, err
	}
	return added, os.Rename(tmp, path)
}

// GridPath is where Steam looks for a shortcut's portrait library art.
func GridPath(configDir string, appID uint32, ext string) string {
	return filepath.Join(configDir, gridDir, strconv.FormatUint(uint64(appID), 10)+gridPortraitSuffix+ext)
}

// UserConfigDirs lists userdata/<id>/config for every real Steam user, or
// only for user when it is set.
func UserConfigDirs(steamRoot, user string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(steamRoot, "userdata"))
	if err != nil {
		return nil, fmt.Errorf("read Steam users: %w", err)
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() || ignoredUsers[e.Name()] || (user != "" && e.Name() != user) {
			continue
		}
		dirs = append(dirs, filepath.Join(steamRoot, "userdata", e.Name(), "config"))
	}
	sort.Strings(dirs)
	if len(dirs) == 0 {
		return nil, errors.New("no Steam user found; start Steam and log in once first")
	}
	return dirs, nil
}

// rootCandidates are tried in order: the native package, then the legacy
// symlink location.
var rootCandidates = []string{".local/share/Steam", ".steam/steam"}

// FindRoot locates the Steam installation in the user's home directory.
func FindRoot() (string, error) {
	home := os.Getenv("HOME")
	for _, c := range rootCandidates {
		p := filepath.Join(home, c)
		if info, err := os.Stat(filepath.Join(p, "userdata")); err == nil && info.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("Steam is not installed for this user (looked in ~/" + strings.Join(rootCandidates, " and ~/") + "); pass --steam-root")
}

// procDir is where running processes are listed.
const procDir = "/proc"

// Running reports whether Steam is running. Steam rewrites shortcuts.vdf
// when it exits, which would discard YAPL's changes.
func Running() bool { return runningIn(procDir) }

func runningIn(proc string) bool {
	entries, err := os.ReadDir(proc)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join(proc, e.Name(), "comm"))
		if err == nil && strings.TrimSpace(string(comm)) == "steam" {
			return true
		}
	}
	return false
}
