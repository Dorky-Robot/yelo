package cmd

import (
	"strings"
)

// isGlacierClass returns true if the storage class requires restore before download.
func isGlacierClass(class string) bool {
	switch class {
	case "GLACIER", "DEEP_ARCHIVE", "GLACIER_IR":
		return true
	}
	return false
}

// parseBucketPath splits "bucket:path" or just "path".
// If the input contains ":", the part before is the bucket.
func parseBucketPath(s string) (bucket, path string) {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}
