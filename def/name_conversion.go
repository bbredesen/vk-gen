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

func renameIdentifier(s string) string {
	s = trimVk(s)
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
		return "as" + strings.Title(s)
	default:
		return s
	}
}
