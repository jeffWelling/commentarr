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

func TestRender_EmptyPlaceholderCollapsesOrphanSeparator(t *testing.T) {
	// Live homelab test produced "Brazil (1985) - .mkv" because the
	// default template includes " - {edition}" and edition was empty.
	// Render now collapses the orphan separator so the filename stays
	// clean.
	cases := []struct {
		name string
		tmpl string
		data map[string]string
		want string
	}{
		{
			name: "edition empty drops the dash",
			tmpl: "{title} ({year}) - {edition}.{ext}",
			data: map[string]string{"title": "Brazil", "year": "1985", "ext": "mkv"},
			want: "Brazil (1985).mkv",
		},
		{
			name: "edition present keeps the dash",
			tmpl: "{title} ({year}) - {edition}.{ext}",
			data: map[string]string{"title": "Brazil", "year": "1985", "edition": "Criterion", "ext": "mkv"},
			want: "Brazil (1985) - Criterion.mkv",
		},
		{
			name: "year + edition both empty",
			tmpl: "{title} ({year}) - {edition}.{ext}",
			data: map[string]string{"title": "Movie", "ext": "mkv"},
			want: "Movie ().mkv",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.tmpl, tc.data)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestRender_UnclosedBraceErrors(t *testing.T) {
	_, err := Render("{title", map[string]string{"title": "x"})
	if err == nil {
		t.Fatal("expected error for unclosed brace")
	}
}
