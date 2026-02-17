package ui

import (
	"io/fs"
	"testing"
)

func TestStaticFiles_ContainsExpectedAssets(t *testing.T) {
	files := StaticFiles()

	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	want := map[string]bool{
		"index.html": false,
		"app.js":     false,
		"style.css":  false,
	}

	for _, entry := range entries {
		if _, ok := want[entry.Name()]; ok {
			want[entry.Name()] = true
		}
	}

	for name, found := range want {
		if !found {
			t.Fatalf("missing embedded file: %s", name)
		}
	}
}

func TestStaticFiles_IndexReadable(t *testing.T) {
	files := StaticFiles()

	data, err := fs.ReadFile(files, "index.html")
	if err != nil {
		t.Fatalf("ReadFile index.html failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("expected index.html content to be non-empty")
	}
}
