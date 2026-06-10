package terraform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// validStateJSON is a minimal but realistic terraform state used by the fake CLI.
const validStateJSON = `{"version":4,"resources":[{"mode":"managed","type":"aws_instance","name":"web","provider":"provider[\"registry.terraform.io/hashicorp/aws\"]","instances":[{"attributes":{"name":"web-1"},"dependencies":[]}]}]}`

// installFakeTerraform writes an executable shell script named "terraform"
// into a temp dir and prepends that dir to PATH. The script body receives the
// usual terraform args ($1 = -chdir=..., $2/$3 = subcommand).
func installFakeTerraform(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "terraform")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestPullStateBytes_Success(t *testing.T) {
	installFakeTerraform(t, `
if [ "$2" = "state" ] && [ "$3" = "pull" ]; then
  printf '%s' '`+validStateJSON+`'
  exit 0
fi
exit 1
`)

	data, err := pullStateBytes(context.Background(), t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "aws_instance") {
		t.Errorf("pulled state missing expected resource: %s", data)
	}
}

func TestPullStateBytes_CommandFails(t *testing.T) {
	installFakeTerraform(t, `
echo "Error: Backend initialization required" >&2
exit 1
`)

	_, err := pullStateBytes(context.Background(), t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for failing terraform CLI")
	}
	if !strings.Contains(err.Error(), "Backend initialization required") {
		t.Errorf("error %q should contain CLI stderr", err)
	}
}

func TestPullStateBytes_EmptyOutput(t *testing.T) {
	installFakeTerraform(t, `exit 0`)

	_, err := pullStateBytes(context.Background(), t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for empty state pull output")
	}
	if !strings.Contains(err.Error(), "empty output") {
		t.Errorf("error %q should mention empty output", err)
	}
}

func TestPullStateBytes_InvalidJSON(t *testing.T) {
	installFakeTerraform(t, `echo 'this is not json'`)

	_, err := pullStateBytes(context.Background(), t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for non-JSON state pull output")
	}
	if !strings.Contains(err.Error(), "parsing state pull output") {
		t.Errorf("error %q should mention JSON parsing", err)
	}
}

func TestPullStateBytes_WorkspaceSelected(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("TF_FAKE_LOG", logFile)
	installFakeTerraform(t, `
if [ "$2" = "workspace" ] && [ "$3" = "select" ]; then
  echo "select $4" >> "$TF_FAKE_LOG"
  exit 0
fi
if [ "$2" = "state" ] && [ "$3" = "pull" ]; then
  printf '%s' '`+validStateJSON+`'
  exit 0
fi
exit 1
`)

	if _, err := pullStateBytes(context.Background(), t.TempDir(), "staging"); err != nil {
		t.Fatal(err)
	}

	log, err := os.ReadFile(logFile) //nolint:gosec // test-controlled path
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "select staging") {
		t.Errorf("expected workspace select to be invoked, log: %q", log)
	}
}

func TestPullStateBytes_WorkspaceSelectFails(t *testing.T) {
	installFakeTerraform(t, `
if [ "$2" = "workspace" ] && [ "$3" = "select" ]; then
  echo "workspace \"missing\" doesn't exist" >&2
  exit 1
fi
exit 0
`)

	_, err := pullStateBytes(context.Background(), t.TempDir(), "missing")
	if err == nil {
		t.Fatal("expected error for failing workspace select")
	}
	if !strings.Contains(err.Error(), `selecting workspace "missing"`) {
		t.Errorf("error %q should mention workspace selection", err)
	}
}

func TestPullStateBytes_NotInPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir: no terraform binary

	_, err := pullStateBytes(context.Background(), t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when terraform is not in PATH")
	}
	if !strings.Contains(err.Error(), "terraform CLI not found in PATH") {
		t.Errorf("error %q should mention missing CLI", err)
	}
}

func TestPullStateBytes_ContextCancellation(t *testing.T) {
	// exec replaces the shell so the context kill signal reaches sleep directly.
	// (Orphaned grandchildren holding the pipes are covered by commandWaitDelay.)
	installFakeTerraform(t, `exec sleep 30`)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := pullStateBytes(ctx, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when context deadline expires")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("pullStateBytes took %s, should be killed by context deadline", elapsed)
	}
}

func TestListWorkspaces_Parsing(t *testing.T) {
	installFakeTerraform(t, `
if [ "$2" = "workspace" ] && [ "$3" = "list" ]; then
  echo "  default"
  echo "* staging"
  echo "  prod"
  echo ""
  exit 0
fi
exit 1
`)

	workspaces, err := ListWorkspaces(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"default", "staging", "prod"}
	if len(workspaces) != len(want) {
		t.Fatalf("workspaces = %v, want %v", workspaces, want)
	}
	for i := range want {
		if workspaces[i] != want[i] {
			t.Errorf("workspaces[%d] = %q, want %q", i, workspaces[i], want[i])
		}
	}
}

func TestListWorkspaces_CommandFails(t *testing.T) {
	installFakeTerraform(t, `
echo "Error: workspaces not supported" >&2
exit 1
`)

	_, err := ListWorkspaces(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error for failing workspace list")
	}
	if !strings.Contains(err.Error(), "workspaces not supported") {
		t.Errorf("error %q should contain CLI stderr", err)
	}
}

func TestListWorkspaces_NotInPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := ListWorkspaces(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when terraform is not in PATH")
	}
}

func TestPullRemoteMulti_SingleDir(t *testing.T) {
	installFakeTerraform(t, `
if [ "$2" = "state" ] && [ "$3" = "pull" ]; then
  printf '%s' '`+validStateJSON+`'
  exit 0
fi
exit 1
`)

	result, err := PullRemoteMulti(context.Background(), []string{t.TempDir()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1 (warnings: %v)", len(result.Nodes), result.Warnings)
	}
	if result.Nodes[0].ID != "tf:vm:web-1" {
		t.Errorf("node ID = %q, want tf:vm:web-1", result.Nodes[0].ID)
	}
}

func TestPullRemoteMulti_AllWorkspaces(t *testing.T) {
	installFakeTerraform(t, `
if [ "$2" = "workspace" ] && [ "$3" = "list" ]; then
  echo "* default"
  echo "  staging"
  exit 0
fi
if [ "$2" = "workspace" ] && [ "$3" = "select" ]; then
  exit 0
fi
if [ "$2" = "state" ] && [ "$3" = "pull" ]; then
  printf '%s' '`+validStateJSON+`'
  exit 0
fi
exit 1
`)

	dir := t.TempDir()
	result, err := PullRemoteMulti(context.Background(), []string{dir}, "*")
	if err != nil {
		t.Fatal(err)
	}
	// One node per workspace (default + staging).
	if len(result.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2 (warnings: %v)", len(result.Nodes), result.Warnings)
	}
	// Source labels include the workspace suffix.
	for _, n := range result.Nodes {
		if !strings.HasPrefix(n.SourceFile, dir+"/") {
			t.Errorf("node source file %q should be labeled <dir>/<workspace>", n.SourceFile)
		}
	}
}

func TestPullRemoteMulti_FailuresBecomeWarnings(t *testing.T) {
	installFakeTerraform(t, `
echo "Error: no backend" >&2
exit 1
`)

	result, err := PullRemoteMulti(context.Background(), []string{t.TempDir()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 0 {
		t.Errorf("nodes = %d, want 0", len(result.Nodes))
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected a warning for the failed pull")
	}
	if !strings.Contains(result.Warnings[0], "no backend") {
		t.Errorf("warning %q should contain CLI stderr", result.Warnings[0])
	}
}

func TestPullRemoteMulti_ListWorkspacesFailureBecomesWarning(t *testing.T) {
	installFakeTerraform(t, `
echo "Error: cannot list" >&2
exit 1
`)

	result, err := PullRemoteMulti(context.Background(), []string{t.TempDir()}, "*")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected a warning for the failed workspace list")
	}
	if !strings.Contains(result.Warnings[0], "listing workspaces") {
		t.Errorf("warning %q should mention workspace listing", result.Warnings[0])
	}
}
