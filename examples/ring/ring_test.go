package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func initialRing(count int) ([]node, []edge) {
	nodes := make([]node, count)
	edges := make([]edge, count)
	for id := range count {
		nodes[id] = node{id: id}
		edges[id] = edge{id, (id + 1) % count}
	}
	return nodes, edges
}

func TestCheckRing(t *testing.T) {
	tests := []struct {
		name string
		edit func(*[]node, *[]edge, []int64)
		want string
	}{
		{"initial", func(*[]node, *[]edge, []int64) {}, ""},
		{"reordered", func(n *[]node, e *[]edge, _ []int64) { slices.Reverse(*n); slices.Reverse(*e) }, ""},
		{"acknowledged", func(n *[]node, _ *[]edge, a []int64) { (*n)[0].counter = 2; a[0] = 2 }, ""},
		{"unknown commit", func(n *[]node, _ *[]edge, a []int64) { (*n)[0].counter = 3; a[0] = 2 }, ""},
		{"lost commit", func(n *[]node, _ *[]edge, a []int64) { (*n)[0].counter = 1; a[0] = 2 }, "below"},
		{"negative counter", func(n *[]node, _ *[]edge, _ []int64) { (*n)[0].counter = -1 }, "below"},
		{"missing node", func(n *[]node, _ *[]edge, _ []int64) { *n = (*n)[:3] }, "node count"},
		{"extra node", func(n *[]node, _ *[]edge, _ []int64) { *n = append(*n, node{id: 4}) }, "node count"},
		{"negative ID", func(n *[]node, _ *[]edge, _ []int64) { (*n)[0].id = -1 }, "node ID"},
		{"replaced node", func(n *[]node, _ *[]edge, _ []int64) { (*n)[0].id = 4 }, "node ID"},
		{"duplicate ID", func(n *[]node, _ *[]edge, _ []int64) { (*n)[0].id = 1 }, "node ID"},
		{"missing edge", func(_ *[]node, e *[]edge, _ []int64) { *e = (*e)[:3] }, "edge count"},
		{"extra edge", func(_ *[]node, e *[]edge, _ []int64) { *e = append(*e, edge{0, 1}) }, "edge count"},
		{"missing source", func(_ *[]node, e *[]edge, _ []int64) { (*e)[0].from = 4 }, "missing node"},
		{"missing destination", func(_ *[]node, e *[]edge, _ []int64) { (*e)[0].to = -1 }, "missing node"},
		{"duplicate outgoing", func(_ *[]node, e *[]edge, _ []int64) { (*e)[1].from = 0 }, "multiple"},
		{"duplicate incoming", func(_ *[]node, e *[]edge, _ []int64) { (*e)[1].to = 1 }, "multiple"},
		{"split cycles", func(_ *[]node, e *[]edge, _ []int64) { *e = []edge{{0, 1}, {1, 0}, {2, 3}, {3, 2}} }, "closes before"},
		{"self loops", func(_ *[]node, e *[]edge, _ []int64) { *e = []edge{{0, 0}, {1, 1}, {2, 2}, {3, 3}} }, "closes before"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, edges := initialRing(4)
			acknowledged := make([]int64, 4)
			tt.edit(&nodes, &edges, acknowledged)
			err := checkRing(nodes, edges, acknowledged)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("checkRing = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestSwapEdges(t *testing.T) {
	for count := 3; count <= 10; count++ {
		for first := range count {
			for second := range count {
				if first == second {
					continue
				}
				t.Run(fmt.Sprintf("%d/%d/%d", count, first, second), func(t *testing.T) {
					nodes, original := initialRing(count)
					edges := slices.Clone(original)
					removed, added := swapEdges(edges, first, second)
					if !slices.Equal(edges, original) {
						t.Fatal("swap changed its input")
					}
					for _, e := range removed {
						index := slices.Index(edges, e)
						if index < 0 {
							t.Fatalf("deleted nonexistent edge: %v", e)
						}
						edges = slices.Delete(edges, index, index+1)
					}
					edges = append(edges, added...)
					if err := checkRing(nodes, edges, make([]int64, count)); err != nil {
						t.Fatal(err)
					}
					// A valid ring alone is insufficient: the selected positions must exchange.
					order := make([]int, count)
					for id := range count {
						order[id] = id
					}
					order[first], order[second] = order[second], order[first]
					for index, id := range order {
						want := edge{id, order[(index+1)%count]}
						if !slices.Contains(edges, want) {
							t.Fatalf("swapped ring %v missing %v", edges, want)
						}
					}
				})
			}
		}
	}
}
