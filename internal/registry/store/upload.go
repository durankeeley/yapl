package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"yapl/internal/archive"
	"yapl/internal/manifest"
	"yapl/internal/paths"
)

const (
	// uploadsDir holds in-flight uploads under the root; it is never indexed.
	uploadsDir      = ".uploads"
	stagedArchive   = "archive.part"
	MaxCoverBytes   = 10 << 20
	uploadDirPrefix = "upload-"
)

var (
	ErrConflict      = errors.New("already exists")
	ErrChecksum      = errors.New("checksum mismatch")
	ErrInvalidUpload = errors.New("invalid upload")
)

var coverExtensions = []string{".jpg", ".png", ".webp"}

// Upload stages one package or component. Nothing becomes visible until a
// Commit succeeds; the staging directory is then renamed into place in one
// step, so readers never see a half-written version.
type Upload struct {
	store       *Store
	staging     string
	archiveSHA  string
	archiveSize int64
	hasArchive  bool
	coverExt    string
}

// NewUpload creates a staging directory for one upload.
func (s *Store) NewUpload() (*Upload, error) {
	base := filepath.Join(s.root, uploadsDir)
	if err := os.MkdirAll(base, 0755); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(base, uploadDirPrefix)
	if err != nil {
		return nil, err
	}
	return &Upload{store: s, staging: dir}, nil
}

// WriteArchive streams the archive into staging, hashing it on the way.
func (u *Upload) WriteArchive(r io.Reader) error {
	f, err := os.Create(filepath.Join(u.staging, stagedArchive))
	if err != nil {
		return err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), r)
	if closeErr := f.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return fmt.Errorf("receive archive: %w", copyErr)
	}
	u.archiveSHA, u.archiveSize, u.hasArchive = hex.EncodeToString(h.Sum(nil)), n, true
	return nil
}

// WriteCover stores the cover image; ext is ".jpg", ".png" or ".webp".
func (u *Upload) WriteCover(ext string, r io.Reader) error {
	if !slices.Contains(coverExtensions, ext) {
		return fmt.Errorf("%w: cover must be one of %v", ErrInvalidUpload, coverExtensions)
	}
	f, err := os.Create(filepath.Join(u.staging, coverPrefix+ext))
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, io.LimitReader(r, MaxCoverBytes+1))
	if err != nil {
		return err
	}
	if n > MaxCoverBytes {
		return fmt.Errorf("%w: cover is larger than %d bytes", ErrInvalidUpload, MaxCoverBytes)
	}
	u.coverExt = ext
	return nil
}

// Abort discards everything staged.
func (u *Upload) Abort() {
	os.RemoveAll(u.staging)
}

// CommitPackage verifies the staged archive against m and publishes it as
// name:version. The upload is discarded whatever the outcome.
func (u *Upload) CommitPackage(name, version string, m manifest.Manifest) error {
	defer u.Abort()
	if err := m.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidUpload, err)
	}
	if m.Name != name || m.Metadata.Version != version {
		return fmt.Errorf("%w: manifest is for %s:%s, not %s:%s", ErrInvalidUpload, m.Name, m.Metadata.Version, name, version)
	}
	if _, _, ok := archive.ParseName(m.Archive.File); !ok || !manifest.ValidName(m.Archive.File) {
		return fmt.Errorf("%w: archive file %q must be a .tar.gz, .tar.xz or .tar.zst name", ErrInvalidUpload, m.Archive.File)
	}
	if err := u.verify(m.Archive); err != nil {
		return err
	}
	if existing, err := u.store.Versions(name); err == nil && existing[0].Manifest.Type != m.Type {
		return fmt.Errorf("%w: %q is already published as a %s", ErrConflict, name, existing[0].Manifest.Type)
	}
	if err := os.Rename(filepath.Join(u.staging, stagedArchive), filepath.Join(u.staging, m.Archive.File)); err != nil {
		return err
	}
	if err := manifest.Write(filepath.Join(u.staging, ManifestFile), m); err != nil {
		return err
	}
	return u.publish(filepath.Join(u.store.root, paths.TypeDir(m.Type), name, version))
}

// CommitComponent verifies the staged archive against m and publishes it as
// kind/version.
func (u *Upload) CommitComponent(kind, version string, m manifest.ComponentManifest) error {
	defer u.Abort()
	if err := m.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidUpload, err)
	}
	if m.Kind != kind || m.Version != version {
		return fmt.Errorf("%w: manifest is for %s, not %s/%s", ErrInvalidUpload, m.Component(), kind, version)
	}
	m.Archive.File = manifest.ComponentArchiveFile
	if err := u.verify(m.Archive); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(u.staging, stagedArchive), filepath.Join(u.staging, manifest.ComponentArchiveFile)); err != nil {
		return err
	}
	if err := manifest.WriteComponent(filepath.Join(u.staging, ManifestFile), m); err != nil {
		return err
	}
	return u.publish(filepath.Join(u.store.root, ComponentsDir, kind, version))
}

func (u *Upload) verify(a manifest.Archive) error {
	if !u.hasArchive {
		return fmt.Errorf("%w: no archive was uploaded", ErrInvalidUpload)
	}
	if u.archiveSHA != a.SHA256 || u.archiveSize != a.SizeBytes {
		return fmt.Errorf("%w: received %d bytes with sha256 %s, manifest says %d bytes with %s", ErrChecksum, u.archiveSize, u.archiveSHA, a.SizeBytes, a.SHA256)
	}
	return nil
}

// publish renames staging to target. A rename onto an existing version
// fails, which is what makes published versions immutable.
func (u *Upload) publish(target string) error {
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%w: %s", ErrConflict, target)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	if err := os.Rename(u.staging, target); err != nil {
		if _, statErr := os.Stat(target); statErr == nil {
			return fmt.Errorf("%w: %s", ErrConflict, target)
		}
		return err
	}
	return u.store.Refresh()
}
