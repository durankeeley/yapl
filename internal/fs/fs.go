package fs

import (
	"io"
	"os"
	"path/filepath"
)

func MustCreateDirectory(p string) error {
	return os.MkdirAll(p, 0755)
}

// GetAbsolutePath returns the absolute path for p, or an error.
func GetAbsolutePath(p string) (string, error) {
	return filepath.Abs(p)
}

func DirExistsAndIsNotEmpty(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || !info.IsDir() {
		return false
	}

	_, err = f.Readdirnames(1)
	return err == nil
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}

// CopyDir recursively copies src to dst, preserving symlinks.
func CopyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		info, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, dstPath); err != nil {
				return err
			}
		case info.IsDir():
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		default:
			if err := CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// OverlayDir copies src (a file, symlink or directory) onto dst, replacing
// whatever is at each destination path while leaving unrelated files in dst
// alone. Upgrades use it to lay saved data over a freshly installed version.
func OverlayDir(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if existing, err := os.Lstat(dst); err == nil && (existing.IsDir() != info.IsDir() || existing.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSymlink != 0) {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.IsDir():
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := OverlayDir(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	default:
		return CopyFile(src, dst)
	}
}
