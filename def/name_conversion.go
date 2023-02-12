package def

import (
	"regexp"
	"strings"
)

var trimRx = regexp.MustCompile(`(\**)(Vk|VK_|vk)?(.+)`)

func trimVk(s string) string {
	if s == "" {
		return s
	}
	r := trimRx.FindStringSubmatch(s)
	// r[0] = s
	// r[1] = pointers
	// r[2] = Vk, vk, or VK_
	// r[3] = Trimmed typename
	return r[1] + r[3]
}

// RenameIdentifier trims the leading Vk (or variants) and then renames any
// keywords or typenames reserved in Go. Every type and value definer needs to
// call this on every registry name.
func RenameIdentifier(s string) string {
	s = trimVk(s)
	s = strings.TrimRight(s, "*")

	switch s {
	case "type":
		return "typ"
	case "range":
		return "rang"
	case "float32":
		fallthrough
	case "int32":
		fallthrough
	case "uint32":
		return "type" + strings.Title(s)
	case "bool":
		return "b"
	default:
		return s
	}
}
