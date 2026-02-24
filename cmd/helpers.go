package cmd

import (
	"fmt"
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

// formatSize returns a human-readable size string.
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
