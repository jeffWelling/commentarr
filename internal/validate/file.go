// Package validate holds file-level checks that run between download
// completion and import: which file is the main video, is its
// extension in the allow-list, do its magic bytes match.
package validate

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/h2non/filetype"
)

// Reason enumerates non-compliance causes.
type Reason string

const (
	ReasonUnexpectedExtension Reason = "unexpected_extension"
	ReasonMagicMismatch       Reason = "magic_mismatch"
	ReasonEmptyFile           Reason = "empty_file"
	ReasonNoVideoFound        Reason = "no_video_found"
)

// ValidationError is returned by ValidateFile with structured reason info.
type ValidationError struct {
	Path   string
	Reason Reason
	Detail string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validate %s: %s (%s)", e.Path, e.Reason, e.Detail)
}

// asValidationError is the test-friendly errors.As wrapper. Factored
// out so production callers can use errors.As directly.
func asValidationError(err error, out **ValidationError) bool {
	return errors.As(err, out)
}

// AllowList is a set of extensions (lower-cased, leading dot).
type AllowList struct {
	exts map[string]bool
}

// NewAllowList returns a fresh AllowList seeded with the given extensions.
func NewAllowList(exts ...string) *AllowList {
	a := &AllowList{exts: map[string]bool{}}
	for _, e := range exts {
		a.exts[strings.ToLower(e)] = true
	}
	return a
}

// DefaultAllowList matches DESIGN.md § 5.8 — video containers only.
func DefaultAllowList() *AllowList {
	return NewAllowList(
		".mkv", ".mp4", ".m4v", ".avi", ".mov", ".ts", ".m2ts",
		".mpeg", ".mpg", ".webm", ".wmv", ".flv", ".iso", ".vob",
	)
}

// Allows reports whether ext is in the set.
func (a *AllowList) Allows(ext string) bool { return a.exts[strings.ToLower(ext)] }

// Extensions returns every allowed extension.
func (a *AllowList) Extensions() []string {
	out := make([]string, 0, len(a.exts))
	for e := range a.exts {
		out = append(out, e)
	}
	return out
}

// Result describes a validated file.
type Result struct {
	Path      string
	Container string
	SizeBytes int64
	Magic     string
}

// FindMainVideo walks dir and returns the path of the largest file
// whose extension is in DefaultAllowList(). Returns a ValidationError
// with ReasonNoVideoFound if nothing matches.
func FindMainVideo(dir string) (string, error) {
	al := DefaultAllowList()
	var best string
	var bestSize int64

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		if !al.Allows(strings.ToLower(filepath.Ext(path))) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > bestSize {
			bestSize = info.Size()
			best = path
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk %s: %w", dir, err)
	}
	if best == "" {
		return "", &ValidationError{Path: dir, Reason: ReasonNoVideoFound}
	}
	return best, nil
}

// ValidateFile checks extension + magic bytes + non-empty.
func ValidateFile(path string, allow *AllowList) (Result, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if !allow.Allows(ext) {
		return Result{}, &ValidationError{Path: path, Reason: ReasonUnexpectedExtension, Detail: ext}
	}

	info, err := os.Stat(path)
	if err != nil {
		return Result{}, fmt.Errorf("stat: %w", err)
	}
	if info.Size() == 0 {
		return Result{}, &ValidationError{Path: path, Reason: ReasonEmptyFile}
	}

	f, err := os.Open(path)
	if err != nil {
		return Result{}, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	head := make([]byte, 262)
	n, err := io.ReadAtLeast(f, head, 1)
	if err != nil {
		return Result{}, fmt.Errorf("read: %w", err)
	}
	kind, err := filetype.Match(head[:n])
	if err != nil {
		return Result{}, fmt.Errorf("detect: %w", err)
	}
	if kind == filetype.Unknown {
		return Result{}, &ValidationError{Path: path, Reason: ReasonMagicMismatch, Detail: "unknown magic"}
	}
	extNoDot := strings.TrimPrefix(ext, ".")
	if !extMatchesKind(extNoDot, kind.Extension) {
		return Result{}, &ValidationError{
			Path: path, Reason: ReasonMagicMismatch,
			Detail: fmt.Sprintf("ext=%s kind=%s", extNoDot, kind.Extension),
		}
	}

	return Result{
		Path: path, Container: extNoDot, SizeBytes: info.Size(), Magic: kind.Extension,
	}, nil
}

// extMatchesKind accepts known aliases.
func extMatchesKind(ext, kind string) bool {
	if ext == kind {
		return true
	}
	switch ext {
	case "m4v":
		return kind == "mp4"
	case "mpg":
		return kind == "mpeg"
	}
	return false
}
