package coverage

import (
	"sort"
)

// ImpactSet computes the deterministic anomaly impact set for a seed board. The
// affected members are the union of the seed board, boards reachable through
// the locked adjacency graph, boards sharing the same base measurement zone and
// boards belonging to the same mortar batch. Members are de-duplicated and
// sorted by board id so the same anomaly always yields the same set.
func ImpactSet(seed string, boards []BoardPlacement, adjacency []AdjEdge) []string {
	members := map[string]bool{seed: true}

	byID := map[string]BoardPlacement{}
	for _, b := range boards {
		byID[b.ID] = b
	}
	seedBoard, ok := byID[seed]
	if !ok {
		// Unknown seed: return just the seed for determinism.
		out := []string{seed}
		sort.Strings(out)
		return out
	}

	// Build adjacency map (undirected).
	adj := map[string]map[string]bool{}
	addEdge := func(a, b string) {
		if adj[a] == nil {
			adj[a] = map[string]bool{}
		}
		adj[a][b] = true
		if adj[b] == nil {
			adj[b] = map[string]bool{}
		}
		adj[b][a] = true
	}
	for _, e := range adjacency {
		addEdge(e.A, e.B)
	}

	// BFS through adjacency.
	queue := []string{seed}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for n := range adj[cur] {
			if !members[n] {
				members[n] = true
				queue = append(queue, n)
			}
		}
	}

	// Shared base zone and same mortar batch.
	for _, b := range boards {
		if b.BaseZone != "" && b.BaseZone == seedBoard.BaseZone {
			members[b.ID] = true
		}
		if b.Material != "" && b.Material == seedBoard.Material {
			members[b.ID] = true
		}
	}

	out := make([]string, 0, len(members))
	for id := range members {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
