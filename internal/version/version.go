package version

import "strings"

// Value is set at build time via -ldflags.
var Value = "dev"

// String returns the trimmed version string.
func String() string {
	return strings.TrimSpace(Value)
}
