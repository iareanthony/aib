package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeResolvePath_ResolvesExistingPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sample.txt")

	if err := os.WriteFile(file, []byte("ok"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	resolved, err := SafeResolvePath(file)
	if err != nil {
		t.Fatalf("SafeResolvePath returned unexpected error: %v", err)
	}

	expected, err := filepath.EvalSymlinks(file)
	if err != nil {
		t.Fatalf("EvalSymlinks returned unexpected error: %v", err)
	}

	if resolved != expected {
		t.Fatalf("resolved path = %q, want %q", resolved, expected)
	}
}

func TestSafeResolvePath_MissingPathReturnsError(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := SafeResolvePath(missingPath)
	if err == nil {
		t.Fatal("expected error for missing path")
	}

	if !strings.Contains(err.Error(), "evaluating symlinks") {
		t.Fatalf("unexpected error: %v", err)
	}
}
