package placer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Mode picks how the new file lands in the library.
type Mode string

const (
	ModeReplace         Mode = "replace"
	ModeSidecar         Mode = "sidecar"
	ModeSeparateLibrary Mode = "separate-library"
)

// Config configures the Placer.
type Config struct {
	Mode             Mode
	FilenameTemplate string
	TrashDir         string
	SeparateRoot     string
}

// PlaceRequest is one placement job.
type PlaceRequest struct {
	NewFilePath      string
	OriginalFilePath string
	Title            string
	Year             string
	Edition          string
	Container        string
}

// Result describes what Place did.
type Result struct {
	FinalPath   string
	TrashedPath string
	Mode        Mode
}

// Placer executes placements.
type Placer struct {
	cfg Config
}

// New returns a Placer.
func New(cfg Config) *Placer { return &Placer{cfg: cfg} }

// Place carries out the configured placement strategy.
func (p *Placer) Place(r PlaceRequest) (Result, error) {
	filename, err := Render(p.cfg.FilenameTemplate, map[string]string{
		"title": r.Title, "year": r.Year, "edition": r.Edition, "ext": r.Container,
	})
	if err != nil {
		return Result{}, fmt.Errorf("render filename: %w", err)
	}
	filename = strings.TrimSpace(filename)
	res := Result{Mode: p.cfg.Mode}

	switch p.cfg.Mode {
	case ModeReplace:
		if r.OriginalFilePath == "" {
			return Result{}, fmt.Errorf("replace mode requires OriginalFilePath")
		}
		dest := r.OriginalFilePath
		trashed, err := moveToTrash(r.OriginalFilePath, p.cfg.TrashDir)
		if err != nil {
			return Result{}, fmt.Errorf("trash original: %w", err)
		}
		res.TrashedPath = trashed
		if err := moveFile(r.NewFilePath, dest); err != nil {
			return Result{}, fmt.Errorf("move new file: %w", err)
		}
		res.FinalPath = dest
		return res, nil

	case ModeSidecar:
		if r.OriginalFilePath == "" {
			return Result{}, fmt.Errorf("sidecar mode requires OriginalFilePath")
		}
		dest := filepath.Join(filepath.Dir(r.OriginalFilePath), filename)
		if err := moveFile(r.NewFilePath, dest); err != nil {
			return Result{}, fmt.Errorf("move new file: %w", err)
		}
		res.FinalPath = dest
		return res, nil

	case ModeSeparateLibrary:
		if p.cfg.SeparateRoot == "" {
			return Result{}, fmt.Errorf("separate-library mode requires SeparateRoot")
		}
		dest := filepath.Join(p.cfg.SeparateRoot, filename)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return Result{}, err
		}
		if err := moveFile(r.NewFilePath, dest); err != nil {
			return Result{}, fmt.Errorf("move new file: %w", err)
		}
		res.FinalPath = dest
		return res, nil

	default:
		return Result{}, fmt.Errorf("unknown mode %q", p.cfg.Mode)
	}
}

// moveFile tries hardlink → rename → copy+remove.
func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Link(src, dst); err == nil {
		return os.Remove(src)
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	return copyAndRemove(src, dst)
}

func copyAndRemove(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

// moveToTrash moves src into trashDir with a timestamped name.
func moveToTrash(src, trashDir string) (string, error) {
	if trashDir == "" {
		return "", fmt.Errorf("trash dir not configured")
	}
	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().UTC().Format("20060102-150405")
	dst := filepath.Join(trashDir, fmt.Sprintf("%s.%s", ts, filepath.Base(src)))
	if err := moveFile(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}
