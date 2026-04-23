package placer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPlace_SidecarMode(t *testing.T) {
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "library", "Movie (2020)")
	trash := filepath.Join(tmp, "trash")
	originalPath := filepath.Join(lib, "Movie (2020).mkv")
	newPath := filepath.Join(tmp, "download", "Movie.2020.Criterion.mkv")
	writeFile(t, originalPath, []byte("original"))
	writeFile(t, newPath, []byte("new"))

	p := New(Config{
		Mode:             ModeSidecar,
		FilenameTemplate: "{title} ({year}) - {edition}.{ext}",
		TrashDir:         trash,
	})
	res, err := p.Place(PlaceRequest{
		NewFilePath:      newPath,
		OriginalFilePath: originalPath,
		Title:            "Movie",
		Year:             "2020",
		Edition:          "Criterion",
		Container:        "mkv",
	})
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if _, err := os.Stat(res.FinalPath); err != nil {
		t.Fatalf("final file missing: %v", err)
	}
	if res.FinalPath == originalPath {
		t.Fatal("sidecar mode must NOT overwrite original")
	}
	if _, err := os.Stat(originalPath); err != nil {
		t.Fatal("sidecar mode must preserve original")
	}
	if res.TrashedPath != "" {
		t.Fatal("sidecar mode must not trash anything")
	}
}

func TestPlace_ReplaceMode_MovesOriginalToTrash(t *testing.T) {
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "library")
	trash := filepath.Join(tmp, "trash")
	originalPath := filepath.Join(lib, "Movie (2020).mkv")
	newPath := filepath.Join(tmp, "download", "Movie.2020.Criterion.mkv")
	writeFile(t, originalPath, []byte("original"))
	writeFile(t, newPath, []byte("new"))

	p := New(Config{
		Mode:             ModeReplace,
		FilenameTemplate: "{title} ({year}).{ext}",
		TrashDir:         trash,
	})
	res, err := p.Place(PlaceRequest{
		NewFilePath:      newPath,
		OriginalFilePath: originalPath,
		Title:            "Movie",
		Year:             "2020",
		Container:        "mkv",
	})
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if res.FinalPath != originalPath {
		t.Fatalf("replace should land at original path, got %s", res.FinalPath)
	}
	if res.TrashedPath == "" {
		t.Fatal("replace should trash original")
	}
	if _, err := os.Stat(res.TrashedPath); err != nil {
		t.Fatalf("trashed file missing: %v", err)
	}
}

func TestPlace_SeparateLibraryMode(t *testing.T) {
	tmp := t.TempDir()
	altRoot := filepath.Join(tmp, "commentary-library")
	newPath := filepath.Join(tmp, "download", "Movie.2020.Criterion.mkv")
	writeFile(t, newPath, []byte("new"))

	p := New(Config{
		Mode:             ModeSeparateLibrary,
		FilenameTemplate: "{title} ({year}).{ext}",
		SeparateRoot:     altRoot,
	})
	res, err := p.Place(PlaceRequest{
		NewFilePath: newPath,
		Title:       "Movie",
		Year:        "2020",
		Container:   "mkv",
	})
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if !strings.HasPrefix(res.FinalPath, altRoot) {
		t.Fatalf("separate-library mode should land under altRoot; got %s", res.FinalPath)
	}
}

func TestPlace_ReplaceRequiresOriginal(t *testing.T) {
	p := New(Config{Mode: ModeReplace, FilenameTemplate: "{title}.{ext}", TrashDir: t.TempDir()})
	_, err := p.Place(PlaceRequest{NewFilePath: "/x", Title: "x", Container: "mkv"})
	if err == nil {
		t.Fatal("expected error without OriginalFilePath")
	}
}
