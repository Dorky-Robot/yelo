package state

import "testing"

func TestResolvePath(t *testing.T) {
	tests := []struct {
		name    string
		current string
		target  string
		want    string
	}{
		{"absolute path", "foo/bar", "/baz/qux", "baz/qux"},
		{"absolute root", "foo/bar", "/", ""},
		{"relative simple", "foo/bar", "baz", "foo/bar/baz"},
		{"relative nested", "foo", "bar/baz", "foo/bar/baz"},
		{"parent traversal", "foo/bar", "..", "foo"},
		{"parent to root", "foo", "..", ""},
		{"multiple parent past root", "foo", "../../..", ""},
		{"current dir dot", "foo", ".", "foo"},
		{"dot from root", "", ".", ""},
		{"empty current relative target", "", "foo/bar", "foo/bar"},
		{"empty current and target", "", "", ""},
		{"trailing slash stripped", "foo", "bar/", "foo/bar"},
		{"absolute with trailing slash", "", "/foo/bar/", "foo/bar"},
		{"parent from deep", "a/b/c/d", "../..", "a/b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolvePath(tt.current, tt.target)
			if got != tt.want {
				t.Errorf("ResolvePath(%q, %q) = %q, want %q", tt.current, tt.target, got, tt.want)
			}
		})
	}
}

func TestResolvePrefix(t *testing.T) {
	tests := []struct {
		name    string
		current string
		target  string
		want    string
	}{
		{"non-empty gets trailing slash", "", "foo", "foo/"},
		{"empty stays empty", "", "/", ""},
		{"already has slash semantics", "foo", "bar", "foo/bar/"},
		{"root is empty", "foo", "..", ""},
		{"nested prefix", "", "a/b/c", "a/b/c/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolvePrefix(tt.current, tt.target)
			if got != tt.want {
				t.Errorf("ResolvePrefix(%q, %q) = %q, want %q", tt.current, tt.target, got, tt.want)
			}
		})
	}
}
