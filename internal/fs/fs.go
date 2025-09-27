package fs

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

func MustCreateDirectory(p string) error {
	return os.MkdirAll(p, 0755)
}

func MustGetAbsolutePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		log.Fatalf("❌ Could not get absolute path for '%s': %v", p, err)
	}
	return abs
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
