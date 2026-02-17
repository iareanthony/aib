package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestWithDefaultCommandTimeout_AddsDeadlineWhenMissing(t *testing.T) {
	ctx, cancel := WithDefaultCommandTimeout(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline to be set")
	}

	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > defaultExternalCommandTimeout {
		t.Fatalf("unexpected timeout remaining: %v", remaining)
	}
}

func TestWithDefaultCommandTimeout_RespectsExistingDeadline(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer parentCancel()

	ctx, cancel := WithDefaultCommandTimeout(parent)
	defer cancel()

	parentDeadline, ok := parent.Deadline()
	if !ok {
		t.Fatal("parent context missing deadline")
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("child context missing deadline")
	}

	if !deadline.Equal(parentDeadline) {
		t.Fatalf("deadline changed: got %v, want %v", deadline, parentDeadline)
	}
}
