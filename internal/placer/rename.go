// Package placer renames + moves imported files into their library
// destinations, in one of three modes: replace-with-safeties, sidecar,
// or separate-library.
package placer

import (
	"fmt"
	"strings"
)

// Render replaces {placeholder} tokens in tmpl with values from data.
// Unknown placeholders render as empty strings.
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
	return b.String(), nil
}
