package scanner

import (
	"context"
	"sort"

	"github.com/matijazezelj/aib/internal/graph"
	"github.com/matijazezelj/aib/internal/parser"
	"github.com/matijazezelj/aib/pkg/models"
)

// computeDrift compares a parse result against the existing store state for the
// same source and returns a summary of what changed. This must be called BEFORE
// upserting the new nodes/edges so the comparison reflects the previous state.
func computeDrift(ctx context.Context, store *graph.SQLiteStore, result *parser.ParseResult, source string) (*graph.DriftSummary, error) {
	// Fetch existing nodes for this source
	existingNodes, err := store.ListNodes(ctx, graph.NodeFilter{Source: source})
	if err != nil {
		return nil, err
	}

	summary := &graph.DriftSummary{}

	// First scan — everything is new
	if len(existingNodes) == 0 {
		summary.IsInitial = true
		for _, n := range result.Nodes {
			summary.NodesAdded = append(summary.NodesAdded, graph.NodeChange{
				ID: n.ID, Name: n.Name, Type: string(n.Type),
			})
		}
		for _, e := range result.Edges {
			summary.EdgesAdded = append(summary.EdgesAdded, graph.EdgeChange{
				ID: e.ID, FromID: e.FromID, ToID: e.ToID, Type: string(e.Type),
			})
		}
		return summary, nil
	}

	// Build maps for comparison
	existingNodeMap := make(map[string]models.Node, len(existingNodes))
	for _, n := range existingNodes {
		existingNodeMap[n.ID] = n
	}

	newNodeMap := make(map[string]models.Node, len(result.Nodes))
	for _, n := range result.Nodes {
		newNodeMap[n.ID] = n
	}

	// Detect added and modified nodes
	for _, n := range result.Nodes {
		old, exists := existingNodeMap[n.ID]
		if !exists {
			summary.NodesAdded = append(summary.NodesAdded, graph.NodeChange{
				ID: n.ID, Name: n.Name, Type: string(n.Type),
			})
			continue
		}
		// Check for modifications
		changes := compareNode(old, n)
		if len(changes) > 0 {
			summary.NodesModified = append(summary.NodesModified, graph.NodeModification{
				ID: n.ID, Name: n.Name, Changes: changes,
			})
		}
	}

	// Detect removed nodes
	for _, n := range existingNodes {
		if _, exists := newNodeMap[n.ID]; !exists {
			summary.NodesRemoved = append(summary.NodesRemoved, graph.NodeChange{
				ID: n.ID, Name: n.Name, Type: string(n.Type),
			})
		}
	}

	// Edge comparison — scope to edges between nodes of this source
	existingEdges, err := store.ListEdges(ctx, graph.EdgeFilter{})
	if err != nil {
		return nil, err
	}

	// Filter existing edges to those where both endpoints belong to this source
	existingEdgeMap := make(map[string]models.Edge)
	for _, e := range existingEdges {
		if _, fromOK := existingNodeMap[e.FromID]; fromOK {
			if _, toOK := existingNodeMap[e.ToID]; toOK {
				existingEdgeMap[e.ID] = e
			}
		}
	}

	newEdgeMap := make(map[string]models.Edge, len(result.Edges))
	for _, e := range result.Edges {
		newEdgeMap[e.ID] = e
	}

	// Detect added edges
	for _, e := range result.Edges {
		if _, exists := existingEdgeMap[e.ID]; !exists {
			summary.EdgesAdded = append(summary.EdgesAdded, graph.EdgeChange{
				ID: e.ID, FromID: e.FromID, ToID: e.ToID, Type: string(e.Type),
			})
		}
	}

	// Detect removed edges
	for _, e := range existingEdges {
		// Only report edges that belonged to this source's nodes
		if _, ok := existingEdgeMap[e.ID]; !ok {
			continue
		}
		if _, exists := newEdgeMap[e.ID]; !exists {
			summary.EdgesRemoved = append(summary.EdgesRemoved, graph.EdgeChange{
				ID: e.ID, FromID: e.FromID, ToID: e.ToID, Type: string(e.Type),
			})
		}
	}

	return summary, nil
}

// compareNode detects differences between an old and new version of a node.
func compareNode(old, new models.Node) []string {
	var changes []string

	if old.Name != new.Name {
		changes = append(changes, "name")
	}
	if old.Type != new.Type {
		changes = append(changes, "type")
	}

	// Compare metadata
	metaChanges := compareMetadata(old.Metadata, new.Metadata)
	changes = append(changes, metaChanges...)

	return changes
}

// compareMetadata detects changed, added, and removed metadata keys.
func compareMetadata(old, new map[string]string) []string {
	var changes []string

	// Check for changed or removed keys
	for k, v := range old {
		if newV, ok := new[k]; !ok {
			changes = append(changes, "metadata."+k)
		} else if v != newV {
			changes = append(changes, "metadata."+k)
		}
	}

	// Check for added keys
	for k := range new {
		if _, ok := old[k]; !ok {
			changes = append(changes, "metadata."+k)
		}
	}

	sort.Strings(changes)
	return changes
}
