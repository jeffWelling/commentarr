package placer

import "testing"

func TestRender_BasicPlaceholders(t *testing.T) {
	got, err := Render("{title} ({year}).{ext}", map[string]string{
		"title": "The Thing", "year": "1982", "ext": "mkv",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "The Thing (1982).mkv" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestRender_MissingPlaceholderIsEmpty(t *testing.T) {
	got, err := Render("{title} - {edition}.{ext}", map[string]string{
		"title": "Blade Runner", "ext": "mkv",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Blade Runner - .mkv" {
		t.Fatalf("expected missing placeholder to render as empty, got %q", got)
	}
}

func TestRender_UnclosedBraceErrors(t *testing.T) {
	_, err := Render("{title", map[string]string{"title": "x"})
	if err == nil {
		t.Fatal("expected error for unclosed brace")
	}
}
