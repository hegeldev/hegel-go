package main

import "fmt"

type node struct {
	id      int
	counter int64
}

type edge struct {
	from, to int
}

func checkRing(nodes []node, edges []edge, acknowledged []int64) error {
	count := len(acknowledged)
	if len(nodes) != count {
		return fmt.Errorf("node count = %d, want %d", len(nodes), count)
	}
	seen := make(map[int]bool, count)
	for _, n := range nodes {
		if n.id < 0 || n.id >= count || seen[n.id] {
			return fmt.Errorf("unexpected or duplicate node ID: %d", n.id)
		}
		seen[n.id] = true
		// A failed commit can still take effect, so database counters may be higher.
		if n.counter < acknowledged[n.id] {
			return fmt.Errorf("node %d counter = %d, below %d acknowledged swaps", n.id, n.counter, acknowledged[n.id])
		}
	}
	if len(edges) != count {
		return fmt.Errorf("edge count = %d, want %d", len(edges), count)
	}
	next := make(map[int]int, count)
	incoming := make(map[int]bool, count)
	for _, e := range edges {
		if !seen[e.from] || !seen[e.to] {
			return fmt.Errorf("edge references a missing node: %v", e)
		}
		if _, exists := next[e.from]; exists || incoming[e.to] {
			return fmt.Errorf("node has multiple outgoing or incoming edges: %v", e)
		}
		next[e.from] = e.to
		incoming[e.to] = true
	}
	visited := make(map[int]bool, count)
	current := 0
	for range count {
		if visited[current] {
			return fmt.Errorf("ring closes before visiting all %d nodes", count)
		}
		visited[current] = true
		current = next[current]
	}
	return nil
}

// swapEdges exchanges the selected nodes' positions, including adjacent nodes.
func swapEdges(edges []edge, first, second int) (removed, added []edge) {
	swap := func(id int) int {
		switch id {
		case first:
			return second
		case second:
			return first
		default:
			return id
		}
	}
	for _, e := range edges {
		if e.from == first || e.from == second || e.to == first || e.to == second {
			removed = append(removed, e)
			added = append(added, edge{swap(e.from), swap(e.to)})
		}
	}
	return removed, added
}
