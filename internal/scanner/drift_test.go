package scanner

import (
	"context"
	"testing"
	"time"

	"github.com/matijazezelj/aib/internal/graph"
	"github.com/matijazezelj/aib/internal/parser"
	"github.com/matijazezelj/aib/pkg/models"
)

func newDriftTestStore(t *testing.T) *graph.SQLiteStore {
	t.Helper()
	store, err := graph.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("initializing store: %v", err)
	}
	t.Cleanup(func() { store.Close() }) //nolint:errcheck
	return store
}

func TestComputeDrift_FirstScan(t *testing.T) {
	store := newDriftTestStore(t)
	ctx := context.Background()

	result := &parser.ParseResult{
		Nodes: []models.Node{
			{ID: "cfn:vm:Web", Name: "Web", Type: models.AssetVM, Source: "cloudformation"},
			{ID: "cfn:db:DB", Name: "DB", Type: models.AssetDatabase, Source: "cloudformation"},
		},
		Edges: []models.Edge{
			{ID: "cfn:vm:Web->depends_on->cfn:db:DB", FromID: "cfn:vm:Web", ToID: "cfn:db:DB", Type: models.EdgeDependsOn},
		},
	}

	drift, err := computeDrift(ctx, store, result, "cloudformation")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !drift.IsInitial {
		t.Error("expected IsInitial=true for first scan")
	}
	if len(drift.NodesAdded) != 2 {
		t.Errorf("expected 2 nodes added, got %d", len(drift.NodesAdded))
	}
	if len(drift.EdgesAdded) != 1 {
		t.Errorf("expected 1 edge added, got %d", len(drift.EdgesAdded))
	}
}

func TestComputeDrift_NoChanges(t *testing.T) {
	store := newDriftTestStore(t)
	ctx := context.Background()
	now := time.Now()

	// Pre-populate store
	_ = store.UpsertNode(ctx, models.Node{ID: "tf:vm:web", Name: "web", Type: models.AssetVM, Source: "terraform", Metadata: map[string]string{"region": "us-east-1"}, LastSeen: now, FirstSeen: now})
	_ = store.UpsertNode(ctx, models.Node{ID: "tf:db:db1", Name: "db1", Type: models.AssetDatabase, Source: "terraform", Metadata: map[string]string{}, LastSeen: now, FirstSeen: now})
	_ = store.UpsertEdge(ctx, models.Edge{ID: "tf:vm:web->depends_on->tf:db:db1", FromID: "tf:vm:web", ToID: "tf:db:db1", Type: models.EdgeDependsOn, Metadata: map[string]string{}})

	// Same result as store
	result := &parser.ParseResult{
		Nodes: []models.Node{
			{ID: "tf:vm:web", Name: "web", Type: models.AssetVM, Source: "terraform", Metadata: map[string]string{"region": "us-east-1"}},
			{ID: "tf:db:db1", Name: "db1", Type: models.AssetDatabase, Source: "terraform", Metadata: map[string]string{}},
		},
		Edges: []models.Edge{
			{ID: "tf:vm:web->depends_on->tf:db:db1", FromID: "tf:vm:web", ToID: "tf:db:db1", Type: models.EdgeDependsOn},
		},
	}

	drift, err := computeDrift(ctx, store, result, "terraform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if drift.IsInitial {
		t.Error("expected IsInitial=false")
	}
	if drift.HasChanges() {
		t.Errorf("expected no changes, got added=%d removed=%d modified=%d edgesAdded=%d edgesRemoved=%d",
			len(drift.NodesAdded), len(drift.NodesRemoved), len(drift.NodesModified),
			len(drift.EdgesAdded), len(drift.EdgesRemoved))
	}
}

func TestComputeDrift_AddedNodes(t *testing.T) {
	store := newDriftTestStore(t)
	ctx := context.Background()
	now := time.Now()

	// Pre-populate store with one node
	_ = store.UpsertNode(ctx, models.Node{ID: "tf:vm:web", Name: "web", Type: models.AssetVM, Source: "terraform", Metadata: map[string]string{}, LastSeen: now, FirstSeen: now})

	// New result has two nodes
	result := &parser.ParseResult{
		Nodes: []models.Node{
			{ID: "tf:vm:web", Name: "web", Type: models.AssetVM, Source: "terraform", Metadata: map[string]string{}},
			{ID: "tf:db:db1", Name: "db1", Type: models.AssetDatabase, Source: "terraform", Metadata: map[string]string{}},
		},
	}

	drift, err := computeDrift(ctx, store, result, "terraform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drift.NodesAdded) != 1 {
		t.Fatalf("expected 1 added node, got %d", len(drift.NodesAdded))
	}
	if drift.NodesAdded[0].ID != "tf:db:db1" {
		t.Errorf("added node ID = %q, want %q", drift.NodesAdded[0].ID, "tf:db:db1")
	}
}

func TestComputeDrift_RemovedNodes(t *testing.T) {
	store := newDriftTestStore(t)
	ctx := context.Background()
	now := time.Now()

	// Pre-populate store with two nodes
	_ = store.UpsertNode(ctx, models.Node{ID: "tf:vm:web", Name: "web", Type: models.AssetVM, Source: "terraform", Metadata: map[string]string{}, LastSeen: now, FirstSeen: now})
	_ = store.UpsertNode(ctx, models.Node{ID: "tf:db:db1", Name: "db1", Type: models.AssetDatabase, Source: "terraform", Metadata: map[string]string{}, LastSeen: now, FirstSeen: now})

	// New result has only one node
	result := &parser.ParseResult{
		Nodes: []models.Node{
			{ID: "tf:vm:web", Name: "web", Type: models.AssetVM, Source: "terraform", Metadata: map[string]string{}},
		},
	}

	drift, err := computeDrift(ctx, store, result, "terraform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drift.NodesRemoved) != 1 {
		t.Fatalf("expected 1 removed node, got %d", len(drift.NodesRemoved))
	}
	if drift.NodesRemoved[0].ID != "tf:db:db1" {
		t.Errorf("removed node ID = %q, want %q", drift.NodesRemoved[0].ID, "tf:db:db1")
	}
}

func TestComputeDrift_ModifiedNodes(t *testing.T) {
	store := newDriftTestStore(t)
	ctx := context.Background()
	now := time.Now()

	// Pre-populate with a node
	_ = store.UpsertNode(ctx, models.Node{
		ID: "tf:vm:web", Name: "web", Type: models.AssetVM, Source: "terraform",
		Metadata: map[string]string{"region": "us-east-1", "instance_type": "t3.micro"},
		LastSeen: now, FirstSeen: now,
	})

	// Changed: name, metadata.instance_type changed, metadata.region same
	result := &parser.ParseResult{
		Nodes: []models.Node{
			{ID: "tf:vm:web", Name: "web-server", Type: models.AssetVM, Source: "terraform",
				Metadata: map[string]string{"region": "us-east-1", "instance_type": "t3.large"}},
		},
	}

	drift, err := computeDrift(ctx, store, result, "terraform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drift.NodesModified) != 1 {
		t.Fatalf("expected 1 modified node, got %d", len(drift.NodesModified))
	}

	mod := drift.NodesModified[0]
	if mod.ID != "tf:vm:web" {
		t.Errorf("modified node ID = %q, want %q", mod.ID, "tf:vm:web")
	}

	// Should have "name" and "metadata.instance_type" changes
	changeMap := make(map[string]bool)
	for _, c := range mod.Changes {
		changeMap[c] = true
	}
	if !changeMap["name"] {
		t.Error("expected 'name' in changes")
	}
	if !changeMap["metadata.instance_type"] {
		t.Error("expected 'metadata.instance_type' in changes")
	}
	if changeMap["metadata.region"] {
		t.Error("region did not change, should not be in changes")
	}
}

func TestComputeDrift_Edges(t *testing.T) {
	store := newDriftTestStore(t)
	ctx := context.Background()
	now := time.Now()

	// Pre-populate with nodes and one edge
	_ = store.UpsertNode(ctx, models.Node{ID: "tf:vm:web", Name: "web", Type: models.AssetVM, Source: "terraform", Metadata: map[string]string{}, LastSeen: now, FirstSeen: now})
	_ = store.UpsertNode(ctx, models.Node{ID: "tf:db:db1", Name: "db1", Type: models.AssetDatabase, Source: "terraform", Metadata: map[string]string{}, LastSeen: now, FirstSeen: now})
	_ = store.UpsertNode(ctx, models.Node{ID: "tf:net:vpc", Name: "vpc", Type: models.AssetNetwork, Source: "terraform", Metadata: map[string]string{}, LastSeen: now, FirstSeen: now})
	_ = store.UpsertEdge(ctx, models.Edge{ID: "tf:vm:web->depends_on->tf:db:db1", FromID: "tf:vm:web", ToID: "tf:db:db1", Type: models.EdgeDependsOn, Metadata: map[string]string{}})

	// New result: remove old edge, add new edge
	result := &parser.ParseResult{
		Nodes: []models.Node{
			{ID: "tf:vm:web", Name: "web", Type: models.AssetVM, Source: "terraform", Metadata: map[string]string{}},
			{ID: "tf:db:db1", Name: "db1", Type: models.AssetDatabase, Source: "terraform", Metadata: map[string]string{}},
			{ID: "tf:net:vpc", Name: "vpc", Type: models.AssetNetwork, Source: "terraform", Metadata: map[string]string{}},
		},
		Edges: []models.Edge{
			{ID: "tf:vm:web->connects_to->tf:net:vpc", FromID: "tf:vm:web", ToID: "tf:net:vpc", Type: models.EdgeConnectsTo},
		},
	}

	drift, err := computeDrift(ctx, store, result, "terraform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drift.EdgesAdded) != 1 {
		t.Errorf("expected 1 edge added, got %d", len(drift.EdgesAdded))
	}
	if len(drift.EdgesRemoved) != 1 {
		t.Errorf("expected 1 edge removed, got %d", len(drift.EdgesRemoved))
	}
	if len(drift.EdgesAdded) > 0 && drift.EdgesAdded[0].ID != "tf:vm:web->connects_to->tf:net:vpc" {
		t.Errorf("added edge ID = %q, want %q", drift.EdgesAdded[0].ID, "tf:vm:web->connects_to->tf:net:vpc")
	}
	if len(drift.EdgesRemoved) > 0 && drift.EdgesRemoved[0].ID != "tf:vm:web->depends_on->tf:db:db1" {
		t.Errorf("removed edge ID = %q, want %q", drift.EdgesRemoved[0].ID, "tf:vm:web->depends_on->tf:db:db1")
	}
}

func TestComputeDrift_SourceScoping(t *testing.T) {
	store := newDriftTestStore(t)
	ctx := context.Background()
	now := time.Now()

	// Pre-populate with nodes from two different sources
	_ = store.UpsertNode(ctx, models.Node{ID: "tf:vm:web", Name: "web", Type: models.AssetVM, Source: "terraform", Metadata: map[string]string{}, LastSeen: now, FirstSeen: now})
	_ = store.UpsertNode(ctx, models.Node{ID: "k8s:pod:api", Name: "api", Type: models.AssetPod, Source: "kubernetes", Metadata: map[string]string{}, LastSeen: now, FirstSeen: now})

	// Terraform scan — should NOT flag k8s node as removed
	result := &parser.ParseResult{
		Nodes: []models.Node{
			{ID: "tf:vm:web", Name: "web", Type: models.AssetVM, Source: "terraform", Metadata: map[string]string{}},
		},
	}

	drift, err := computeDrift(ctx, store, result, "terraform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drift.NodesRemoved) != 0 {
		t.Errorf("expected 0 removed nodes (source scoping), got %d: %v", len(drift.NodesRemoved), drift.NodesRemoved)
	}
	if drift.HasChanges() {
		t.Error("expected no changes for same-source comparison")
	}
}
