package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/dorkyrobot/yelo/internal/aws"
)

// IsTTY returns true if the given file is a terminal.
func IsTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// FormatSize returns a human-readable size string.
func FormatSize(bytes int64) string {
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

// ListObjects writes object listing to w.
// When tty is true, produces a human-readable columnar format.
// When tty is false, produces bare keys (one per line) or tab-separated long format.
func ListObjects(w io.Writer, objects []aws.ObjectInfo, long bool, tty bool) {
	if !tty && !long {
		// Bare keys, one per line — ideal for piping.
		for _, obj := range objects {
			fmt.Fprintln(w, obj.Key)
		}
		return
	}

	if !tty && long {
		// Tab-separated: key, size, class, date
		for _, obj := range objects {
			if obj.IsPrefix {
				fmt.Fprintf(w, "%s\t-\tPREFIX\t-\n", obj.Key)
			} else {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", obj.Key, obj.Size, obj.StorageClass, obj.LastModified)
			}
		}
		return
	}

	if !long {
		// TTY short: just the names, but strip common prefix for readability.
		for _, obj := range objects {
			name := obj.Key
			if obj.IsPrefix && strings.HasSuffix(name, "/") {
				fmt.Fprintf(w, "%s\n", name)
			} else {
				fmt.Fprintln(w, name)
			}
		}
		return
	}

	// TTY long format: aligned columns.
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	for _, obj := range objects {
		if obj.IsPrefix {
			fmt.Fprintf(tw, "PRE\t-\t%s\n", obj.Key)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				obj.StorageClass,
				FormatSize(obj.Size),
				formatDate(obj.LastModified),
				obj.Key,
			)
		}
	}
	tw.Flush()
}

// FormatStat writes object metadata to w.
func FormatStat(w io.Writer, obj *aws.ObjectInfo, tty bool) {
	if tty {
		fmt.Fprintf(w, "       Key: %s\n", obj.Key)
		fmt.Fprintf(w, "      Size: %s (%d bytes)\n", FormatSize(obj.Size), obj.Size)
		fmt.Fprintf(w, "     Class: %s\n", obj.StorageClass)
		fmt.Fprintf(w, "  Modified: %s\n", obj.LastModified)
		fmt.Fprintf(w, "      Type: %s\n", obj.ContentType)
		fmt.Fprintf(w, "      ETag: %s\n", obj.ETag)
		if obj.RestoreStatus != "" {
			fmt.Fprintf(w, "   Restore: %s\n", obj.RestoreStatus)
		}
	} else {
		fmt.Fprintf(w, "key=%s\n", obj.Key)
		fmt.Fprintf(w, "size=%d\n", obj.Size)
		fmt.Fprintf(w, "class=%s\n", obj.StorageClass)
		fmt.Fprintf(w, "modified=%s\n", obj.LastModified)
		fmt.Fprintf(w, "type=%s\n", obj.ContentType)
		fmt.Fprintf(w, "etag=%s\n", obj.ETag)
		if obj.RestoreStatus != "" {
			fmt.Fprintf(w, "restore=%s\n", obj.RestoreStatus)
		}
	}
}

func formatDate(s string) string {
	// Input is RFC3339-ish from AWS: "2006-01-02T15:04:05Z"
	// For display, trim the time if you want shorter output.
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
