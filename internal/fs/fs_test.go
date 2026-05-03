package fs

import (
	"os"
	"path/filepath"
	"testing"
)

// --- DirExistsAndIsNotEmpty ---

func TestDirExistsAndIsNotEmpty_MissingDirReturnsFalse(t *testing.T) {
	// Given a path that does not exist on the filesystem
	// When DirExistsAndIsNotEmpty is called
	// Then it returns false
	if DirExistsAndIsNotEmpty("/does/not/exist/ever") {
		t.Fatal("expected false for a path that does not exist")
	}
}

func TestDirExistsAndIsNotEmpty_EmptyDirReturnsFalse(t *testing.T) {
	// Given an existing directory that contains no files or subdirectories
	d := t.TempDir()
	// When DirExistsAndIsNotEmpty is called
	// Then it returns false
	if DirExistsAndIsNotEmpty(d) {
		t.Fatal("expected false for an empty directory")
	}
}

func TestDirExistsAndIsNotEmpty_NonEmptyDirReturnsTrue(t *testing.T) {
	// Given a directory that contains at least one file
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// When DirExistsAndIsNotEmpty is called
	// Then it returns true
	if !DirExistsAndIsNotEmpty(d) {
		t.Fatal("expected true for a non-empty directory")
	}
}

func TestDirExistsAndIsNotEmpty_FilePathReturnsFalse(t *testing.T) {
	// Given a path that points to a regular file, not a directory
	d := t.TempDir()
	f := filepath.Join(d, "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// When DirExistsAndIsNotEmpty is called on the file path
	// Then it returns false (only directories qualify)
	if DirExistsAndIsNotEmpty(f) {
		t.Fatal("expected false when the path points to a file, not a directory")
	}
}

// --- CopyFile ---

func TestCopyFile_CopiesContentAndPreservesPermissions(t *testing.T) {
	// Given a source file with known content and executable permissions
	d := t.TempDir()
	src := filepath.Join(d, "src.txt")
	dst := filepath.Join(d, "dst.txt")
	content := []byte("hello yapl")
	if err := os.WriteFile(src, content, 0755); err != nil {
		t.Fatal(err)
	}

	// When CopyFile is called
	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile returned an unexpected error: %v", err)
	}

	// Then the destination file has the same content
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %q want %q", got, content)
	}

	// And the destination file has the same permissions as the source
	srcInfo, _ := os.Stat(src)
	dstInfo, _ := os.Stat(dst)
	if srcInfo.Mode() != dstInfo.Mode() {
		t.Fatalf("permission mismatch: src=%v dst=%v", srcInfo.Mode(), dstInfo.Mode())
	}
}

func TestCopyFile_MissingSourceReturnsError(t *testing.T) {
	// Given a source path that does not exist
	d := t.TempDir()

	// When CopyFile is called
	err := CopyFile(filepath.Join(d, "nope"), filepath.Join(d, "dst"))

	// Then it returns an error
	if err == nil {
		t.Fatal("expected an error when the source file does not exist but got nil")
	}
}

// --- GetAbsolutePath ---

func TestGetAbsolutePath_ReturnsAbsolutePathWithNoError(t *testing.T) {
	// Given an existing directory path
	d := t.TempDir()

	// When GetAbsolutePath is called
	abs, err := GetAbsolutePath(d)

	// Then it returns the absolute path with no error
	if err != nil {
		t.Fatalf("GetAbsolutePath returned an unexpected error: %v", err)
	}
	if abs != d {
		t.Fatalf("path mismatch: got %q want %q", abs, d)
	}
}

func TestGetAbsolutePath_ReturnsAbsolutePathForRelativeInput(t *testing.T) {
	// Given a relative path segment
	// When GetAbsolutePath is called
	abs, err := GetAbsolutePath("some/relative/path")

	// Then it returns an absolute path with no error — GetAbsolutePath is the only authoritative function
	if err != nil {
		t.Fatalf("GetAbsolutePath returned an unexpected error: %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Fatalf("expected absolute path for relative input, got: %q", abs)
	}
}

// --- CopyDir ---

func TestCopyDir_PreservesSymlinks(t *testing.T) {
	// Given a source directory containing a regular file and a symlink pointing to it
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "real.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(src, "link.txt")); err != nil {
		t.Fatal(err)
	}

	// When CopyDir is called
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir returned an unexpected error: %v", err)
	}

	// Then the destination contains a symlink (the pointer is preserved, not the file content duplicated)
	info, err := os.Lstat(filepath.Join(dst, "link.txt"))
	if err != nil {
		t.Fatalf("could not stat the destination entry: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected a symlink at the destination, got mode %v", info.Mode())
	}

	// And the symlink points to the same target as the original
	target, err := os.Readlink(filepath.Join(dst, "link.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if target != "real.txt" {
		t.Fatalf("symlink target mismatch: got %q want %q", target, "real.txt")
	}
}

func TestCopyDir_DoesNotRecurseIntoDirectorySymlinks(t *testing.T) {
	// Given a source directory containing a symlink that points to another directory
	src := t.TempDir()
	dst := t.TempDir()
	subDir := filepath.Join(src, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a symlink to the subdirectory itself
	if err := os.Symlink("subdir", filepath.Join(src, "link-to-dir")); err != nil {
		t.Fatal(err)
	}

	// When CopyDir is called
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir returned an unexpected error: %v", err)
	}

	// Then the directory symlink is recreated as a symlink, not expanded into a copy of the directory
	info, err := os.Lstat(filepath.Join(dst, "link-to-dir"))
	if err != nil {
		t.Fatalf("could not stat the destination entry: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected a symlink for the directory link, got mode %v — directory was incorrectly expanded", info.Mode())
	}
}

func TestCopyDir_CopiesNestedStructure(t *testing.T) {
	// Given a source directory with a nested subdirectory and file
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	sub := filepath.Join(src, "subdir")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.txt"), []byte("nested"), 0644); err != nil {
		t.Fatal(err)
	}

	// When CopyDir is called
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir returned an unexpected error: %v", err)
	}

	// Then the nested file appears at the correct path in the destination
	got, err := os.ReadFile(filepath.Join(dst, "subdir", "nested.txt"))
	if err != nil {
		t.Fatalf("could not read the nested file in the destination: %v", err)
	}
	if string(got) != "nested" {
		t.Fatalf("content mismatch: got %q", got)
	}
}
