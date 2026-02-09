package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matijazezelj/aib/internal/graph"
	"github.com/matijazezelj/aib/internal/scanner"
	"github.com/matijazezelj/aib/pkg/models"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

// newTestApp returns a cliApp wired to a temp DB and captured output buffer.
func newTestApp(t *testing.T) (*cliApp, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	app := &cliApp{
		dbPath:    filepath.Join(t.TempDir(), "test.db"),
		logFormat: "text",
		logLevel:  "info",
		version:   "test",
		out:       &buf,
		errOut:    io.Discard,
		in:        strings.NewReader(""),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return app, &buf
}

// seedTestData pre-populates 2 nodes and 1 edge via the app's store, then closes it.
// The caller's command will reopen the store via a.openStore().
func seedTestData(t *testing.T, app *cliApp) {
	t.Helper()
	store, _, err := app.openStore()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	if err := store.UpsertNode(ctx, models.Node{
		ID: "vm:web1", Name: "web1", Type: models.AssetVM,
		Source: "terraform", Provider: "aws", Metadata: map[string]string{},
		LastSeen: now, FirstSeen: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertNode(ctx, models.Node{
		ID: "db:pg1", Name: "pg1", Type: models.AssetDatabase,
		Source: "terraform", Provider: "aws", Metadata: map[string]string{},
		LastSeen: now, FirstSeen: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEdge(ctx, models.Edge{
		FromID: "vm:web1", ToID: "db:pg1", Type: models.EdgeDependsOn,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

// runCmd executes a cobra command attached to a root, returning any error.
func runCmd(app *cliApp, cmd *cobra.Command, args ...string) error {
	root := &cobra.Command{Use: "aib"}
	root.AddCommand(cmd)
	root.SetArgs(args)
	root.SetOut(app.out)
	root.SetErr(app.errOut)
	return root.Execute()
}

// --- Pure utility tests (no cliApp needed) ---

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    slog.Level
		wantErr bool
	}{
		{"debug", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"DEBUG", slog.LevelDebug, false},
		{"INFO", slog.LevelInfo, false},
		{"WARN", slog.LevelWarn, false},
		{"Error", slog.LevelError, false},
		{"invalid", slog.LevelInfo, true},
		{"", slog.LevelInfo, true},
		{"trace", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		got, err := parseLogLevel(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseLogLevel(%q) expected error", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("parseLogLevel(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCountTreeNodes(t *testing.T) {
	tree := &graph.ImpactNode{
		NodeID: "root",
		Children: []graph.ImpactNode{
			{NodeID: "child1"},
			{NodeID: "child2", Children: []graph.ImpactNode{
				{NodeID: "grandchild"},
			}},
		},
	}

	count := countTreeNodes(tree)
	if count != 4 {
		t.Errorf("countTreeNodes = %d, want 4", count)
	}
}

func TestCountTreeNodes_Leaf(t *testing.T) {
	tree := &graph.ImpactNode{NodeID: "leaf"}
	if count := countTreeNodes(tree); count != 1 {
		t.Errorf("countTreeNodes(leaf) = %d, want 1", count)
	}
}

func TestCollectWarnings_NoExpiry(t *testing.T) {
	tree := &graph.ImpactNode{
		NodeID: "root",
		Node:   &models.Node{ID: "root"},
	}
	warnings := collectWarnings(tree)
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(warnings))
	}
}

func TestCollectWarnings_ExpiringCert(t *testing.T) {
	soon := time.Now().Add(5 * 24 * time.Hour)
	tree := &graph.ImpactNode{
		NodeID: "cert1",
		Node: &models.Node{
			ID:        "cert1",
			ExpiresAt: &soon,
		},
	}
	warnings := collectWarnings(tree)
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
}

func TestCollectWarnings_Recursive(t *testing.T) {
	soon := time.Now().Add(5 * 24 * time.Hour)
	far := time.Now().Add(90 * 24 * time.Hour)
	tree := &graph.ImpactNode{
		NodeID: "root",
		Node:   &models.Node{ID: "root", ExpiresAt: &soon},
		Children: []graph.ImpactNode{
			{
				NodeID: "child",
				Node:   &models.Node{ID: "child", ExpiresAt: &far},
			},
			{
				NodeID: "child2",
				Node:   &models.Node{ID: "child2", ExpiresAt: &soon},
			},
		},
	}
	warnings := collectWarnings(tree)
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(warnings))
	}
}

// --- version ---

func TestVersionCmd(t *testing.T) {
	app, buf := newTestApp(t)
	cmd := app.versionCmd()
	cmd.Run(cmd, nil)

	output := buf.String()
	if !strings.Contains(output, "aib") {
		t.Errorf("version output should contain 'aib', got %q", output)
	}
	if !strings.Contains(output, "test") {
		t.Errorf("version output should contain 'test', got %q", output)
	}
}

// --- completion ---

func TestCompletionCmd_Bash(t *testing.T) {
	app, buf := newTestApp(t)
	err := runCmd(app, app.completionCmd(), "completion", "bash")
	if err != nil {
		t.Fatalf("completion bash error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("completion bash produced no output")
	}
}

func TestCompletionCmd_Zsh(t *testing.T) {
	app, buf := newTestApp(t)
	err := runCmd(app, app.completionCmd(), "completion", "zsh")
	if err != nil {
		t.Fatalf("completion zsh error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("completion zsh produced no output")
	}
}

func TestCompletionCmd_Fish(t *testing.T) {
	app, buf := newTestApp(t)
	err := runCmd(app, app.completionCmd(), "completion", "fish")
	if err != nil {
		t.Fatalf("completion fish error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("completion fish produced no output")
	}
}

func TestCompletionCmd_PowerShell(t *testing.T) {
	app, buf := newTestApp(t)
	err := runCmd(app, app.completionCmd(), "completion", "powershell")
	if err != nil {
		t.Fatalf("completion powershell error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("completion powershell produced no output")
	}
}

func TestCompletionCmd_InvalidShell(t *testing.T) {
	app, _ := newTestApp(t)
	err := runCmd(app, app.completionCmd(), "completion", "invalid")
	if err == nil {
		t.Error("expected error for invalid shell")
	}
}

// --- printScanResult ---

func TestPrintScanResult_Success(t *testing.T) {
	app, buf := newTestApp(t)
	app.printScanResult(scanner.ScanResult{
		ScanID:     1,
		NodesFound: 10,
		EdgesFound: 5,
		Warnings:   []string{"missing provider"},
	})

	output := buf.String()
	if !strings.Contains(output, "10 nodes") {
		t.Errorf("output should mention nodes, got: %s", output)
	}
	if !strings.Contains(output, "5 edges") {
		t.Errorf("output should mention edges, got: %s", output)
	}
	if !strings.Contains(output, "missing provider") {
		t.Errorf("output should mention warning, got: %s", output)
	}
}

func TestPrintScanResult_Error(t *testing.T) {
	app, buf := newTestApp(t)
	app.printScanResult(scanner.ScanResult{
		Error: fmt.Errorf("scan failed"),
	})

	output := buf.String()
	if !strings.Contains(output, "failed") {
		t.Errorf("output should mention failure, got: %s", output)
	}
}

// --- printTree ---

func TestPrintTree(t *testing.T) {
	app, buf := newTestApp(t)

	tree := &graph.ImpactNode{
		NodeID: "root",
		Node:   &models.Node{ID: "root", Type: models.AssetVM},
		Children: []graph.ImpactNode{
			{
				NodeID:   "child1",
				Node:     &models.Node{ID: "child1", Type: models.AssetNetwork},
				EdgeType: models.EdgeDependsOn,
			},
			{
				NodeID:   "child2",
				Node:     &models.Node{ID: "child2", Type: models.AssetDatabase},
				EdgeType: models.EdgeConnectsTo,
			},
		},
	}

	app.printTree(context.Background(), tree, "  ", true)

	output := buf.String()
	if !strings.Contains(output, "root") {
		t.Errorf("output should contain root, got: %s", output)
	}
	if !strings.Contains(output, "child1") {
		t.Errorf("output should contain child1, got: %s", output)
	}
}

// --- openStore error handling ---

func TestOpenStore_InvalidConfig(t *testing.T) {
	app, _ := newTestApp(t)
	app.cfgFile = "/nonexistent/config.yaml"
	_, _, err := app.openStore()
	if err == nil {
		t.Error("expected error for nonexistent config")
	}
}

// --- graph show ---

func TestGraphShowCmd(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphShowCmd(), "show")
	if err != nil {
		t.Fatalf("graph show error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Total nodes: 2") {
		t.Errorf("expected 'Total nodes: 2' in output, got: %s", output)
	}
	if !strings.Contains(output, "Total edges: 1") {
		t.Errorf("expected 'Total edges: 1' in output, got: %s", output)
	}
}

// --- graph nodes ---

func TestGraphNodesCmd(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphNodesCmd(), "nodes")
	if err != nil {
		t.Fatalf("graph nodes error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "vm:web1") {
		t.Errorf("expected 'vm:web1' in output, got: %s", output)
	}
	if !strings.Contains(output, "db:pg1") {
		t.Errorf("expected 'db:pg1' in output, got: %s", output)
	}
}

func TestGraphNodesCmd_Filter(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphNodesCmd(), "nodes", "--type", string(models.AssetVM))
	if err != nil {
		t.Fatalf("graph nodes --type error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "vm:web1") {
		t.Errorf("expected 'vm:web1' in output, got: %s", output)
	}
	if strings.Contains(output, "db:pg1") {
		t.Errorf("db:pg1 should be filtered out, got: %s", output)
	}
}

// --- graph edges ---

func TestGraphEdgesCmd(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphEdgesCmd(), "edges")
	if err != nil {
		t.Fatalf("graph edges error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "vm:web1") {
		t.Errorf("expected 'vm:web1' in edges output, got: %s", output)
	}
	if !strings.Contains(output, "db:pg1") {
		t.Errorf("expected 'db:pg1' in edges output, got: %s", output)
	}
}

// --- graph neighbors ---

func TestGraphNeighborsCmd(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphNeighborsCmd(), "neighbors", "vm:web1")
	if err != nil {
		t.Fatalf("graph neighbors error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "db:pg1") {
		t.Errorf("expected neighbor 'db:pg1' in output, got: %s", output)
	}
}

func TestGraphNeighborsCmd_NotFound(t *testing.T) {
	app, _ := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphNeighborsCmd(), "neighbors", "nonexistent:node")
	if err == nil {
		t.Error("expected error for nonexistent node")
	}
}

// --- graph export ---

func TestGraphExportCmd_JSON(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphExportCmd(), "export", "--format", "json")
	if err != nil {
		t.Fatalf("graph export json error: %v", err)
	}

	output := buf.String()
	// Validate it's valid JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Errorf("export JSON is not valid JSON: %v\nOutput: %s", err, output)
	}
}

func TestGraphExportCmd_DOT(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphExportCmd(), "export", "--format", "dot")
	if err != nil {
		t.Fatalf("graph export dot error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "digraph") {
		t.Errorf("export DOT should contain 'digraph', got: %s", output)
	}
}

// --- graph path ---

func TestGraphPathCmd(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphPathCmd(), "path", "vm:web1", "db:pg1")
	if err != nil {
		t.Fatalf("graph path error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Shortest path") {
		t.Errorf("expected 'Shortest path' in output, got: %s", output)
	}
}

// --- graph deps ---

func TestGraphDepsCmd(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphDepsCmd(), "deps", "vm:web1")
	if err != nil {
		t.Fatalf("graph deps error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Dependencies of") {
		t.Errorf("expected 'Dependencies of' in output, got: %s", output)
	}
}

// --- graph cycles ---

func TestGraphCyclesCmd(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphCyclesCmd(), "cycles")
	if err != nil {
		t.Fatalf("graph cycles error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No circular dependencies found.") {
		t.Errorf("expected no cycles message, got: %s", output)
	}
}

// --- graph spof ---

func TestGraphSPOFCmd(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphSPOFCmd(), "spof")
	if err != nil {
		t.Fatalf("graph spof error: %v", err)
	}

	output := buf.String()
	// With 2 nodes and 1 edge, there may or may not be SPOFs depending on direction
	if output == "" {
		t.Error("expected some output from spof command")
	}
}

// --- graph orphans ---

func TestGraphOrphansCmd(t *testing.T) {
	app, buf := newTestApp(t)
	// Seed data, then add an orphan node
	seedTestData(t, app)

	store, _, err := app.openStore()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	_ = store.UpsertNode(context.Background(), models.Node{
		ID: "orphan:lonely", Name: "lonely", Type: models.AssetVM,
		Source: "terraform", Metadata: map[string]string{},
		LastSeen: now, FirstSeen: now,
	})
	_ = store.Close()

	err = runCmd(app, app.graphOrphansCmd(), "orphans")
	if err != nil {
		t.Fatalf("graph orphans error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "orphan:lonely") {
		t.Errorf("expected 'orphan:lonely' in output, got: %s", output)
	}
}

// --- graph prune ---

func TestGraphPruneCmd_Force(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphPruneCmd(), "prune", "--source", "terraform", "--force")
	if err != nil {
		t.Fatalf("graph prune error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Deleted") {
		t.Errorf("expected 'Deleted' in output, got: %s", output)
	}
}

func TestGraphPruneCmd_NoFilter(t *testing.T) {
	app, _ := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.graphPruneCmd(), "prune")
	if err == nil {
		t.Error("expected error when no filter is specified")
	}
}

// --- impact node ---

func TestImpactNodeCmd(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.impactCmd(), "impact", "node", "db:pg1")
	if err != nil {
		t.Fatalf("impact node error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Impact Analysis") {
		t.Errorf("expected 'Impact Analysis' in output, got: %s", output)
	}
	if !strings.Contains(output, "Blast Radius") {
		t.Errorf("expected 'Blast Radius' in output, got: %s", output)
	}
}

func TestImpactNodeCmd_NotFound(t *testing.T) {
	app, _ := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.impactCmd(), "impact", "node", "nonexistent:node")
	if err == nil {
		t.Error("expected error for nonexistent node")
	}
}

// --- scan commands (real fixtures) ---

func TestScanTerraformCmd(t *testing.T) {
	app, buf := newTestApp(t)

	fixture, err := filepath.Abs("../../testdata/terraform/sample.tfstate")
	if err != nil {
		t.Fatal(err)
	}

	err = runCmd(app, app.scanCmd(), "scan", "terraform", fixture)
	if err != nil {
		t.Fatalf("scan terraform error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Discovered") {
		t.Errorf("expected 'Discovered' in output, got: %s", output)
	}
}

func TestScanCloudFormationCmd(t *testing.T) {
	app, buf := newTestApp(t)

	fixture, err := filepath.Abs("../../testdata/cloudformation/template.yaml")
	if err != nil {
		t.Fatal(err)
	}

	err = runCmd(app, app.scanCmd(), "scan", "cloudformation", fixture)
	if err != nil {
		t.Fatalf("scan cloudformation error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Discovered") {
		t.Errorf("expected 'Discovered' in output, got: %s", output)
	}
}

func TestScanPulumiCmd(t *testing.T) {
	app, buf := newTestApp(t)

	fixture, err := filepath.Abs("../../internal/parser/pulumi/testdata/simple.json")
	if err != nil {
		t.Fatal(err)
	}

	err = runCmd(app, app.scanCmd(), "scan", "pulumi", fixture)
	if err != nil {
		t.Fatalf("scan pulumi error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Discovered") {
		t.Errorf("expected 'Discovered' in output, got: %s", output)
	}
}

// --- db stats ---

func TestDBStatsCmd(t *testing.T) {
	app, buf := newTestApp(t)
	seedTestData(t, app)

	err := runCmd(app, app.dbCmd(), "db", "stats")
	if err != nil {
		t.Fatalf("db stats error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Nodes: 2") {
		t.Errorf("expected 'Nodes: 2' in output, got: %s", output)
	}
}
