package output

import (
	"bytes"
	"testing"

	"github.com/dorkyrobot/yelo/internal/aws"
)

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
		{"just under KB", 1023, "1023 B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"full RFC3339", "2024-01-15T10:30:00Z", "2024-01-15"},
		{"date only", "2024-01-15", "2024-01-15"},
		{"short string", "abc", "abc"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDate(tt.s)
			if got != tt.want {
				t.Errorf("formatDate(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

func TestListObjects(t *testing.T) {
	objects := []aws.ObjectInfo{
		{Key: "photos/", IsPrefix: true},
		{Key: "readme.txt", Size: 1024, StorageClass: "STANDARD", LastModified: "2024-01-15T10:30:00Z"},
		{Key: "data.csv", Size: 2048, StorageClass: "GLACIER", LastModified: "2024-02-20T08:00:00Z"},
	}

	t.Run("pipe bare keys", func(t *testing.T) {
		var buf bytes.Buffer
		ListObjects(&buf, objects, false, false)
		want := "photos/\nreadme.txt\ndata.csv\n"
		if buf.String() != want {
			t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
		}
	})

	t.Run("pipe long format", func(t *testing.T) {
		var buf bytes.Buffer
		ListObjects(&buf, objects, true, false)
		want := "photos/\t-\tPREFIX\t-\n" +
			"readme.txt\t1024\tSTANDARD\t2024-01-15T10:30:00Z\n" +
			"data.csv\t2048\tGLACIER\t2024-02-20T08:00:00Z\n"
		if buf.String() != want {
			t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
		}
	})

	t.Run("tty short", func(t *testing.T) {
		var buf bytes.Buffer
		ListObjects(&buf, objects, false, true)
		want := "photos/\nreadme.txt\ndata.csv\n"
		if buf.String() != want {
			t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
		}
	})

	t.Run("tty long format", func(t *testing.T) {
		var buf bytes.Buffer
		ListObjects(&buf, objects, true, true)
		got := buf.String()
		// tabwriter output — verify key content is present
		if got == "" {
			t.Error("expected non-empty output")
		}
		// Check that each key appears
		for _, obj := range objects {
			if !bytes.Contains([]byte(got), []byte(obj.Key)) {
				t.Errorf("output missing key %q", obj.Key)
			}
		}
	})

	t.Run("empty list", func(t *testing.T) {
		var buf bytes.Buffer
		ListObjects(&buf, nil, false, false)
		if buf.String() != "" {
			t.Errorf("expected empty output, got %q", buf.String())
		}
	})
}

func TestFormatStat(t *testing.T) {
	obj := &aws.ObjectInfo{
		Key:          "photos/cat.jpg",
		Size:         2048,
		StorageClass: "STANDARD",
		LastModified: "2024-01-15T10:30:00Z",
		ContentType:  "image/jpeg",
		ETag:         `"abc123"`,
	}

	t.Run("tty mode", func(t *testing.T) {
		var buf bytes.Buffer
		FormatStat(&buf, obj, true)
		got := buf.String()
		expects := []string{"Key: photos/cat.jpg", "Size:", "Class: STANDARD", "Modified:", "Type: image/jpeg", "ETag:"}
		for _, e := range expects {
			if !bytes.Contains([]byte(got), []byte(e)) {
				t.Errorf("tty output missing %q", e)
			}
		}
	})

	t.Run("pipe mode", func(t *testing.T) {
		var buf bytes.Buffer
		FormatStat(&buf, obj, false)
		got := buf.String()
		expects := []string{"key=photos/cat.jpg", "size=2048", "class=STANDARD", "type=image/jpeg", "etag="}
		for _, e := range expects {
			if !bytes.Contains([]byte(got), []byte(e)) {
				t.Errorf("pipe output missing %q", e)
			}
		}
	})

	t.Run("with restore status tty", func(t *testing.T) {
		objRestore := *obj
		objRestore.RestoreStatus = "in-progress"
		var buf bytes.Buffer
		FormatStat(&buf, &objRestore, true)
		if !bytes.Contains(buf.Bytes(), []byte("Restore: in-progress")) {
			t.Error("tty output missing restore status")
		}
	})

	t.Run("with restore status pipe", func(t *testing.T) {
		objRestore := *obj
		objRestore.RestoreStatus = "available"
		var buf bytes.Buffer
		FormatStat(&buf, &objRestore, false)
		if !bytes.Contains(buf.Bytes(), []byte("restore=available")) {
			t.Error("pipe output missing restore status")
		}
	})

	t.Run("without restore status omitted", func(t *testing.T) {
		var buf bytes.Buffer
		FormatStat(&buf, obj, true)
		if bytes.Contains(buf.Bytes(), []byte("Restore:")) {
			t.Error("restore line should be omitted when status is empty")
		}
	})
}
