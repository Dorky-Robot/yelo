package cmd

import "testing"

func TestIsGlacierClass(t *testing.T) {
	tests := []struct {
		name  string
		class string
		want  bool
	}{
		{"glacier", "GLACIER", true},
		{"deep archive", "DEEP_ARCHIVE", true},
		{"glacier ir", "GLACIER_IR", true},
		{"standard", "STANDARD", false},
		{"standard ia", "STANDARD_IA", false},
		{"intelligent tiering", "INTELLIGENT_TIERING", false},
		{"onezone ia", "ONEZONE_IA", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

