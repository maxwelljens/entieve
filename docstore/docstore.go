// Package docstore reads and writes the JSON sidecar documents beside a
// save (layout, settings, keys, the save itself): one tolerant read
// contract and one atomic write contract, so no sidecar hand-rolls its own
// os.WriteFile.
//
// Read is tolerant by agreement: a missing, corrupt or out-of-range
// document leaves the caller's defaults in place and reports which kind of
// failure it was — sidecar callers ignore the error, the save path reports
// it. Write is atomic: exact MarshalIndent bytes into a dot-hidden temp
// file in the same directory, fsynced, chmod 0644, then renamed over the
// document, so an interrupted write can never truncate what the player had.
// The dot-hidden name keeps an in-flight temp out of the save manager's
// *.json listing.
package docstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The read failure kinds. Every Load error wraps one of them (except I/O
// errors that are neither missing nor a format problem).
var (
	ErrMissing = errors.New("document missing")
	ErrFormat  = errors.New("document format")
	ErrVersion = errors.New("document version")
)

// Doc is one sidecar document: its path plus, for versioned documents, the
// version range the reader accepts. The zero Doc is a plain document with
// no path (Load reports ErrMissing, Save fails).
type Doc struct {
	path      string
	version   int // written by the caller; never changed by the store
	readFrom  int // lowest accepted version on read
	versioned bool
}

// Versioned is a document carrying a top-level "version" field: Load
// accepts versions readFrom..version (inclusive) and rejects the rest with
// ErrVersion. The caller owns the number (its save format's schema); the
// store only gates on it.
func Versioned(path string, version, readFrom int) Doc {
	return Doc{path: path, version: version, readFrom: readFrom, versioned: true}
}

// Plain is a document with no version field to gate on (settings, the sim
// save): every well-formed file is read as-is.
func Plain(path string) Doc { return Doc{path: path} }

// Path is the document's location on disk.
func (d Doc) Path() string { return d.path }

// Load decodes the document into v. A versioned document is decoded only
// when its version is in range, so a rejected file leaves v untouched (the
// caller's defaults win). Errors wrap ErrMissing, ErrFormat or ErrVersion;
// other I/O failures are returned as they are.
func (d Doc) Load(v any) error {
	data, err := os.ReadFile(d.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", d.path, ErrMissing)
		}
		return err
	}
	if d.versioned {
		var head struct {
			Version int `json:"version"`
		}
		if err := json.Unmarshal(data, &head); err != nil {
			return fmt.Errorf("%s: %w: %v", d.path, ErrFormat, err)
		}
		if head.Version < d.readFrom || head.Version > d.version {
			return fmt.Errorf("%s: %w %d (accepts %d..%d)",
				d.path, ErrVersion, head.Version, d.readFrom, d.version)
		}
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w: %v", d.path, ErrFormat, err)
	}
	return nil
}

// Save writes v as MarshalIndent(v, "", "  ") — the exact bytes every
// sidecar has always held — atomically: a temp file in the same directory
// (so the rename stays on one filesystem), chmod 0644, fsync, rename. A
// failure leaves the previous document intact and removes the temp.
func (d Doc) Save(v any) (err error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(d.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(d.path)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	// CreateTemp opens 0600; the sidecars have always been 0644.
	if err = f.Chmod(0o644); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// shortcut: the directory entry is not fsynced after the rename, so a
	// power loss can lose the last write; add when save durability must
	// survive power loss (not just process death).
	return os.Rename(f.Name(), d.path)
}

// Remove deletes the document. A missing file is not an error (the empty
// sidecar's own removal is the common caller).
func (d Doc) Remove() error {
	if err := os.Remove(d.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
