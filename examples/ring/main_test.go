package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"hegel.dev/go/hegel"
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
		{"isolated node", func(_ *[]node, e *[]edge, _ []int64) { *e = []edge{{0, 1}, {1, 2}, {2, 0}, {3, 3}} }, "closes before"},
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

type noteTestCase struct {
	hegel.TestCase
	notes []string
}

func (tc *noteTestCase) Note(message string) {
	tc.notes = append(tc.notes, message)
}

func TestTransactionCommitCallbacks(t *testing.T) {
	databaseURL := os.Getenv("HEGEL_RING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set HEGEL_RING_DATABASE_URL to run PostgreSQL integration tests")
	}
	tests := []struct {
		name                                   string
		callbackError, commitError, beginError bool
		opts                                   *sql.TxOptions
		want                                   int
	}{
		{name: "committed", want: 1},
		{name: "callback failed", callbackError: true},
		{name: "commit failed", commitError: true},
		{name: "begin failed", beginError: true},
		{name: "read only", opts: &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := sql.Open("postgres", databaseURL)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			table := "ring_transaction_" + rand.Text()
			if _, err := db.Exec(fmt.Sprintf(`CREATE TABLE %s (id INT UNIQUE DEFERRABLE INITIALLY DEFERRED)`, table)); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := db.Exec(fmt.Sprintf(`DROP TABLE %s`, table)); err != nil {
					t.Error(err)
				}
			}()
			txDB := db
			if tt.beginError {
				txDB, err = sql.Open("postgres", databaseURL)
				if err != nil {
					t.Fatal(err)
				}
				if err := txDB.Close(); err != nil {
					t.Fatal(err)
				}
			}
			tc := &noteTestCase{}
			acknowledged := 0
			callbackInvoked := false
			transaction(tc, txDB, tt.opts, func(tx *sql.Tx) error {
				callbackInvoked = true
				var isolation string
				if err := tx.QueryRow(`SHOW transaction_isolation`).Scan(&isolation); err != nil {
					return err
				}
				if isolation != "serializable" {
					t.Errorf("transaction isolation = %q, want serializable", isolation)
				}
				if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO %s VALUES (1)`, table)); err != nil {
					return err
				}
				if tt.callbackError {
					return fmt.Errorf("operation failed after writing")
				}
				if tt.commitError {
					_, err := tx.Exec(fmt.Sprintf(`INSERT INTO %s VALUES (1)`, table))
					return err
				}
				return nil
			}, func() { acknowledged++ })
			if callbackInvoked == tt.beginError {
				t.Errorf("transaction callback invoked = %v, begin error = %v", callbackInvoked, tt.beginError)
			}
			if tt.want == 1 {
				if len(tc.notes) != 0 {
					t.Errorf("successful transaction notes: %v", tc.notes)
				}
			} else if len(tc.notes) != 1 || !strings.Contains(tc.notes[0], "SQL operation failed:") {
				t.Errorf("failed transaction notes: %v", tc.notes)
			}
			var count int
			if err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if acknowledged != tt.want || count != tt.want {
				t.Fatalf("acknowledged = %d, stored rows = %d, want %d", acknowledged, count, tt.want)
			}
		})
	}
}
