package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"yapl/internal/manifest"
	"yapl/internal/registry/store"
)

// Multipart field names for uploads. Clients send archive, then cover, then
// manifest, so a streamed archive's checksum can go in the manifest.
const (
	PartArchive  = "archive"
	PartCover    = "cover"
	PartManifest = "manifest"

	maxManifestBytes = 1 << 20
	jpegAlias        = ".jpeg"
	jpegExtension    = ".jpg"
)

var errPushDisabled = errors.New("push is disabled on this server; start it with 'yapl serve --allow-push'")

func (h *handler) components(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.cfg.Store.Components())
}

func (h *handler) resolveComponent(w http.ResponseWriter, r *http.Request) (*store.ComponentVersion, bool) {
	c, err := h.cfg.Store.ResolveComponent(r.PathValue("kind"), r.PathValue("version"))
	if err != nil {
		writeStoreError(w, err)
		return nil, false
	}
	return c, true
}

func (h *handler) componentManifest(w http.ResponseWriter, r *http.Request) {
	if c, ok := h.resolveComponent(w, r); ok {
		writeJSON(w, c.Manifest)
	}
}

func (h *handler) componentArchive(w http.ResponseWriter, r *http.Request) {
	c, ok := h.resolveComponent(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set(SHA256Header, c.Manifest.Archive.SHA256)
	serveFile(w, r, c.ArchivePath)
}

func (h *handler) pushPackage(w http.ResponseWriter, r *http.Request) {
	name, version := r.PathValue("name"), r.PathValue("version")
	if _, err := h.cfg.Store.Resolve(name, version); err == nil {
		writeError(w, http.StatusConflict, fmt.Errorf("%s:%s already exists; bump the version", name, version))
		return
	}
	h.receive(w, r, func(u *store.Upload, raw []byte) error {
		var m manifest.Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("%w: manifest: %v", store.ErrInvalidUpload, err)
		}
		return u.CommitPackage(name, version, m)
	})
}

func (h *handler) pushComponent(w http.ResponseWriter, r *http.Request) {
	kind, version := r.PathValue("kind"), r.PathValue("version")
	if _, err := h.cfg.Store.ResolveComponent(kind, version); err == nil {
		writeError(w, http.StatusConflict, fmt.Errorf("component %s/%s already exists", kind, version))
		return
	}
	h.receive(w, r, func(u *store.Upload, raw []byte) error {
		var m manifest.ComponentManifest
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("%w: manifest: %v", store.ErrInvalidUpload, err)
		}
		return u.CommitComponent(kind, version, m)
	})
}

// receive streams a multipart upload into a store.Upload and hands the
// manifest bytes to commit once every part has arrived.
func (h *handler) receive(w http.ResponseWriter, r *http.Request, commit func(*store.Upload, []byte) error) {
	if !h.cfg.AllowPush {
		writeError(w, http.StatusForbidden, errPushDisabled)
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("expected a multipart upload: %w", err))
		return
	}
	upload, err := h.cfg.Store.NewUpload()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer upload.Abort()

	rawManifest, err := readParts(reader, upload)
	if err == nil && rawManifest == nil {
		err = fmt.Errorf("%w: no manifest part", store.ErrInvalidUpload)
	}
	if err == nil {
		err = commit(upload, rawManifest)
	}
	if err != nil {
		writeUploadError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func readParts(reader *multipart.Reader, upload *store.Upload) ([]byte, error) {
	var rawManifest []byte
	for {
		p, err := reader.NextPart()
		if err == io.EOF {
			return rawManifest, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", store.ErrInvalidUpload, err)
		}
		switch p.FormName() {
		case PartArchive:
			err = upload.WriteArchive(p)
		case PartCover:
			err = upload.WriteCover(coverExtension(p.FileName()), p)
		case PartManifest:
			rawManifest, err = io.ReadAll(io.LimitReader(p, maxManifestBytes))
		}
		p.Close()
		if err != nil {
			return nil, err
		}
	}
}

func coverExtension(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == jpegAlias {
		return jpegExtension
	}
	return ext
}

func writeUploadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, store.ErrChecksum), errors.Is(err, store.ErrInvalidUpload):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
