package state

import (
	"path"
	"strings"
)

// ResolvePath resolves an FTP-style path against the current prefix.
// Absolute paths (starting with /) are used as-is.
// Relative paths are joined to the current prefix.
// ".." navigates up one level.
// The result is always clean and never starts with "/".
func ResolvePath(current, target string) string {
	var resolved string
	if strings.HasPrefix(target, "/") {
		resolved = target
	} else {
		resolved = path.Join("/", current, target)
	}

	resolved = path.Clean(resolved)

	// Strip leading slash — S3 prefixes don't start with /
	resolved = strings.TrimPrefix(resolved, "/")

	// path.Clean("/.") returns "/", trimmed to "". That's correct for root.
	if resolved == "." {
		resolved = ""
	}

	return resolved
}

// ResolvePrefix is like ResolvePath but ensures the result ends with "/"
// when non-empty (suitable for S3 prefix listing).
func ResolvePrefix(current, target string) string {
	p := ResolvePath(current, target)
	if p != "" && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}
