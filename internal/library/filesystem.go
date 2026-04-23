package library

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jeffWelling/commentarr/internal/title"
)

// videoExtensions matches the allow-list from DESIGN.md § 5.8.
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".mov": true,
	".ts": true, ".m2ts": true, ".mpeg": true, ".mpg": true, ".webm": true,
	".wmv": true, ".flv": true, ".iso": true, ".vob": true,
}

// movieYearPattern matches "Title (YYYY)" folder or filename style.
var movieYearPattern = regexp.MustCompile(`^(.+?)\s*\((\d{4})\)`)

// episodePattern matches "SxxEyy" anywhere in the filename.
var episodePattern = regexp.MustCompile(`[sS](\d{1,2})[eE](\d{1,2})`)

// filesystemSource walks a directory and yields titles.
type filesystemSource struct {
	name string
	root string
}

// NewFilesystemSource returns a LibrarySource that walks root and derives
// titles from directory/file naming conventions. No TMDB lookup is
// performed; this is the degraded mode referenced in DESIGN.md § 5.1.
func NewFilesystemSource(name, root string) LibrarySource {
	return &filesystemSource{name: name, root: root}
}

func (s *filesystemSource) Name() string { return s.name }

func (s *filesystemSource) List(ctx context.Context) ([]title.Title, error) {
	var out []title.Title

	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !videoExtensions[ext] {
			return nil
		}

		t := parsePath(s.root, path)
		out = append(out, t)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", s.root, err)
	}
	return out, nil
}

func (s *filesystemSource) Refresh(ctx context.Context, path string) error {
	return nil // filesystem has nothing to notify
}

// parsePath derives a Title from a video file path under root.
func parsePath(root, path string) title.Title {
	rel, _ := filepath.Rel(root, path)
	base := filepath.Base(path)
	nameNoExt := strings.TrimSuffix(base, filepath.Ext(base))
	dir := filepath.Dir(rel)

	// Episode: SxxEyy pattern anywhere in the filename.
	if m := episodePattern.FindStringSubmatch(nameNoExt); len(m) == 3 {
		season, _ := strconv.Atoi(m[1])
		episode, _ := strconv.Atoi(m[2])
		series := topLevelDir(dir)
		return title.Title{
			ID:          "fs:" + rel,
			Kind:        title.KindEpisode,
			DisplayName: fmt.Sprintf("%s - S%02dE%02d", series, season, episode),
			SeriesID:    "fs-series:" + series,
			Season:      season,
			Episode:     episode,
			FilePath:    path,
		}
	}

	// Movie: either filename "Title (YYYY)" or parent folder "Title (YYYY)".
	displayName := nameNoExt
	year := 0
	if m := movieYearPattern.FindStringSubmatch(nameNoExt); len(m) == 3 {
		displayName = strings.TrimSpace(m[1])
		year, _ = strconv.Atoi(m[2])
	} else if m := movieYearPattern.FindStringSubmatch(filepath.Base(dir)); len(m) == 3 {
		displayName = strings.TrimSpace(m[1])
		year, _ = strconv.Atoi(m[2])
	}
	return title.Title{
		ID:          "fs:" + rel,
		Kind:        title.KindMovie,
		DisplayName: displayName,
		Year:        year,
		FilePath:    path,
	}
}

// topLevelDir returns the first segment of a relative path ("a/b/c" → "a").
func topLevelDir(relDir string) string {
	parts := strings.Split(relDir, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
