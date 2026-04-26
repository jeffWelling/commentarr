// Package placer renames + moves imported files into their library
// destinations, in one of three modes: replace-with-safeties, sidecar,
// or separate-library.
package placer

import (
	"fmt"
	"regexp"
	"strings"
)

// orphanSeparator matches " - " adjacent to an empty position (start
// of string, end of string, or right before the extension dot).
// Doesn't touch " - " between two non-empty tokens.
var orphanSeparator = regexp.MustCompile(`(?:^| )- (?:$|\.)`)

// Render replaces {placeholder} tokens in tmpl with values from data.
// Unknown placeholders + empty values render as empty strings, then
// any orphaned separators (e.g., " - " left dangling when {edition} is
// empty) are collapsed so a default template like
// "{title} ({year}) - {edition}.{ext}" produces "Brazil (1985).mkv"
// instead of "Brazil (1985) - .mkv" when no edition is set.
func Render(tmpl string, data map[string]string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(tmpl); i++ {
		c := tmpl[i]
		if c != '{' {
			b.WriteByte(c)
			continue
		}
		end := strings.IndexByte(tmpl[i+1:], '}')
		if end < 0 {
			return "", fmt.Errorf("unclosed brace at position %d", i)
		}
		name := tmpl[i+1 : i+1+end]
		b.WriteString(data[name])
		i += 1 + end
	}
	out := b.String()
	// Collapse the orphan-separator case: " - " right before the
	// extension dot ("Brazil (1985) - .mkv") or at end-of-string.
	// In-the-middle " - " separating two real tokens is preserved.
	out = orphanSeparator.ReplaceAllStringFunc(out, func(m string) string {
		// Preserve a leading or trailing space if the match included one.
		if strings.HasSuffix(m, ".") {
			return "."
		}
		return ""
	})
	// Collapse double-spaces left behind by other empty placeholders.
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return out, nil
}
