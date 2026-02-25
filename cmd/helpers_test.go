package cmd

import "testing"

func TestIsGlacierClass(t *testing.T) {
	tests := []struct {
		class string
		want  bool
	}{
		{"GLACIER", true},
		{"DEEP_ARCHIVE", true},
		{"GLACIER_IR", true},
		{"STANDARD", false},
		{"STANDARD_IA", false},
		{"INTELLIGENT_TIERING", false},
		{"ONEZONE_IA", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.class, func(t *testing.T) {
			got := isGlacierClass(tt.class)
			if got != tt.want {
				t.Errorf("isGlacierClass(%q) = %v, want %v", tt.class, got, tt.want)
			}
		})
	}
}

func TestParseBucketPath(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantBucket string
		wantPath   string
	}{
		{"bucket and path", "mybucket:foo/bar", "mybucket", "foo/bar"},
		{"just path", "just/a/path", "", "just/a/path"},
		{"bucket no path", "mybucket:", "mybucket", ""},
		{"colon at start", ":path", "", "path"},
		{"empty", "", "", ""},
		{"multiple colons", "bucket:path:with:colons", "bucket", "path:with:colons"},
		{"single key", "file.txt", "", "file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBucket, gotPath := parseBucketPath(tt.input)
			if gotBucket != tt.wantBucket || gotPath != tt.wantPath {
				t.Errorf("parseBucketPath(%q) = (%q, %q), want (%q, %q)",
					tt.input, gotBucket, gotPath, tt.wantBucket, tt.wantPath)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero", 0, "0 B"},
		{"bytes", 512, "512 B"},
		{"one KB", 1024, "1.0 KB"},
		{"1.5 KB", 1536, "1.5 KB"},
		{"one MB", 1024 * 1024, "1.0 MB"},
		{"one GB", 1024 * 1024 * 1024, "1.0 GB"},
		{"one TB", 1024 * 1024 * 1024 * 1024, "1.0 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
