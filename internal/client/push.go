package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"yapl/internal/manifest"
	"yapl/internal/progress"
	"yapl/internal/registry/api"
)

// errUploadDone unblocks the multipart writer if the server answers before
// reading the whole body.
var errUploadDone = errors.New("upload finished")

// PushComponent uploads a component whose archive is produced by source.
// The archive is hashed while it streams and the checksum is sent in the
// manifest part afterwards, so no temporary file is needed.
func (c *Client) PushComponent(m manifest.ComponentManifest, source func(io.Writer) error, report progress.Func) error {
	return c.upload(componentPath(m.Kind, m.Version), func(mw *multipart.Writer) error {
		part, err := mw.CreateFormFile(api.PartArchive, manifest.ComponentArchiveFile)
		if err != nil {
			return err
		}
		hasher := sha256.New()
		counter := progress.NewWriter(io.MultiWriter(part, hasher), m.Component().String(), -1, report)
		if err := source(counter); err != nil {
			return fmt.Errorf("pack %s: %w", m.Component(), err)
		}
		counter.Finish()
		m.Archive = manifest.Archive{
			File:   manifest.ComponentArchiveFile,
			Format: manifest.ComponentFormat,
			SHA256: hex.EncodeToString(hasher.Sum(nil)),
		}
		m.Archive.SizeBytes = counter.Written()
		return writeManifestPart(mw, m)
	})
}

// PushPackage uploads a packaged archive, its optional cover and manifest.
func (c *Client) PushPackage(m manifest.Manifest, archivePath, coverPath string, report progress.Func) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return c.upload(packagePath(m.Name, m.Metadata.Version), func(mw *multipart.Writer) error {
		part, err := mw.CreateFormFile(api.PartArchive, filepath.Base(archivePath))
		if err != nil {
			return err
		}
		if _, err := io.Copy(part, progress.NewReader(f, m.Name, info.Size(), report)); err != nil {
			return err
		}
		if coverPath != "" {
			if err := copyFilePart(mw, api.PartCover, coverPath); err != nil {
				return err
			}
		}
		return writeManifestPart(mw, m)
	})
}

// upload streams a multipart PUT; write fills the parts.
func (c *Client) upload(path string, write func(*multipart.Writer) error) error {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		err := write(mw)
		if err == nil {
			err = mw.Close()
		}
		pw.CloseWithError(err)
	}()
	defer pr.CloseWithError(errUploadDone)

	req, err := http.NewRequest(http.MethodPut, c.cfg.Server+path, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c.applyAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return serverError(resp)
	}
	return nil
}

func writeManifestPart(mw *multipart.Writer, v any) error {
	part, err := mw.CreateFormFile(api.PartManifest, "manifest.json")
	if err != nil {
		return err
	}
	return json.NewEncoder(part).Encode(v)
}

func copyFilePart(mw *multipart.Writer, field, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	part, err := mw.CreateFormFile(field, filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, f)
	return err
}
