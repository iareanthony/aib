package graph

// DriftSummary describes what changed between a scan's parse result and the
// existing store state for the same source.
type DriftSummary struct {
	NodesAdded    []NodeChange       `json:"nodes_added"`
	NodesRemoved  []NodeChange       `json:"nodes_removed"`
	NodesModified []NodeModification `json:"nodes_modified"`
	EdgesAdded    []EdgeChange       `json:"edges_added"`
	EdgesRemoved  []EdgeChange       `json:"edges_removed"`
	IsInitial     bool               `json:"is_initial"`
}

// NodeChange records a node that was added or removed.
type NodeChange struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// NodeModification records a node whose attributes changed.
type NodeModification struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Changes []string `json:"changes"` // e.g. ["name", "metadata.region"]
}

// EdgeChange records an edge that was added or removed.
type EdgeChange struct {
	ID     string `json:"id"`
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
	Type   string `json:"type"`
}

// HasChanges returns true if the summary contains any drift.
func (d *DriftSummary) HasChanges() bool {
	return len(d.NodesAdded) > 0 || len(d.NodesRemoved) > 0 ||
		len(d.NodesModified) > 0 || len(d.EdgesAdded) > 0 || len(d.EdgesRemoved) > 0
}
