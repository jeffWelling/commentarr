package validate

import (
	"os"
	"path/filepath"
	"testing"
)

// mkvMagic is enough of an EBML header to pass h2non/filetype's MKV detector.
var mkvMagic = []byte{
	0x1A, 0x45, 0xDF, 0xA3,
	0xA3, 0x42, 0x82, 0x88,
	'm', 'a', 't', 'r', 'o', 's', 'k', 'a',
}

// peMagic is a minimal DOS/PE header.
var peMagic = []byte{'M', 'Z', 0x90, 0x00}

func writeBlob(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestValidate_PicksLargestVideoFile(t *testing.T) {
	dir := t.TempDir()
	writeBlob(t, dir, "movie.mkv", append(mkvMagic, make([]byte, 5000)...))
	writeBlob(t, dir, "sample.mkv", append(mkvMagic, make([]byte, 200)...))
	writeBlob(t, dir, "readme.txt", []byte("hi"))

	path, err := FindMainVideo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "movie.mkv" {
		t.Fatalf("expected movie.mkv, got %s", path)
	}
}

func TestValidate_NoVideoFoundError(t *testing.T) {
	dir := t.TempDir()
	writeBlob(t, dir, "readme.txt", []byte("hi"))
	_, err := FindMainVideo(dir)
	if err == nil {
		t.Fatal("expected error")
	}
	var ve *ValidationError
	if !asValidationError(err, &ve) || ve.Reason != ReasonNoVideoFound {
		t.Fatalf("expected ReasonNoVideoFound, got %+v", err)
	}
}

func TestValidate_RejectsUnexpectedExtension(t *testing.T) {
	dir := t.TempDir()
	p := writeBlob(t, dir, "sketchy.exe", peMagic)
	_, err := ValidateFile(p, DefaultAllowList())
	if err == nil {
		t.Fatal("expected error for disallowed extension")
	}
	var vErr *ValidationError
	if !asValidationError(err, &vErr) || vErr.Reason != ReasonUnexpectedExtension {
		t.Fatalf("expected ReasonUnexpectedExtension, got %+v", err)
	}
}

func TestValidate_RejectsMagicByteMismatch(t *testing.T) {
	dir := t.TempDir()
	p := writeBlob(t, dir, "fake.mkv", append(peMagic, make([]byte, 500)...))
	_, err := ValidateFile(p, DefaultAllowList())
	if err == nil {
		t.Fatal("expected error")
	}
	var vErr *ValidationError
	if !asValidationError(err, &vErr) || vErr.Reason != ReasonMagicMismatch {
		t.Fatalf("expected ReasonMagicMismatch, got %+v", err)
	}
}

func TestValidate_AcceptsValidMKV(t *testing.T) {
	dir := t.TempDir()
	p := writeBlob(t, dir, "ok.mkv", append(mkvMagic, make([]byte, 500)...))
	res, err := ValidateFile(p, DefaultAllowList())
	if err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if res.Container != "mkv" {
		t.Fatalf("expected container=mkv, got %q", res.Container)
	}
}

func TestValidate_EmptyFileRejected(t *testing.T) {
	dir := t.TempDir()
	p := writeBlob(t, dir, "empty.mkv", nil)
	_, err := ValidateFile(p, DefaultAllowList())
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	var vErr *ValidationError
	if !asValidationError(err, &vErr) || vErr.Reason != ReasonEmptyFile {
		t.Fatalf("expected ReasonEmptyFile, got %+v", err)
	}
}

func TestDefaultAllowList_CoversVideoContainers(t *testing.T) {
	al := DefaultAllowList()
	for _, ext := range []string{".mkv", ".mp4", ".m4v", ".avi", ".mov", ".ts"} {
		if !al.Allows(ext) {
			t.Errorf("ext %s should be allowed", ext)
		}
	}
	if al.Allows(".exe") {
		t.Error("ext .exe must not be allowed")
	}
}
