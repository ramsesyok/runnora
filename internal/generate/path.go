package generate

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// safeFileSegment converts OpenAPI identifiers into a single portable path component.
// A hash keeps distinct identifiers distinct when their unsafe characters normalize
// to the same spelling.
func safeFileSegment(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}

	segment := strings.Trim(b.String(), "._")
	if segment == "" {
		segment = "item"
	}
	if len(segment) > 80 {
		segment = segment[:80]
	}
	if isReservedFileName(segment) {
		segment = "_" + segment
	}
	if segment != value {
		sum := sha256.Sum256([]byte(value))
		segment += fmt.Sprintf("_%x", sum[:4])
	}
	return segment
}

func isReservedFileName(segment string) bool {
	name, _, _ := strings.Cut(strings.ToUpper(segment), ".")
	switch name {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}
