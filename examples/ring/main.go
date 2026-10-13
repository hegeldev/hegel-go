// Ring tests PostgreSQL-compatible stores using concurrent graph transactions.
//
// Run against a dedicated test database:
//
//	go run hegel.dev/go/hegel/examples/ring@latest 'postgres://user:password@localhost/ring?sslmode=disable' --test-cases=100
//
// Each test case creates and drops its own tables. Invariants run at join points,
// after all rule transactions and local counter updates have finished.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
	"hegel.dev/go/hegel"
)

type ring struct {
	db           *sql.DB
	nodes, edges string
	acknowledged []atomic.Int64
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s DATABASE_URL [hegel flags]", os.Args[0])
	}
	databaseURL := os.Args[1]
	os.Args = append(os.Args[:1], os.Args[2:]...)
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	hegel.Workload(func(tc hegel.TestCase) {
		name := "ring_" + strings.ToLower(rand.Text())
		count := hegel.Draw(tc, hegel.Integers(3, 10))
		m := &ring{db: db, nodes: name + "_nodes", edges: name + "_edges", acknowledged: make([]atomic.Int64, count)}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s, %s`, m.edges, m.nodes)); err != nil {
				tc.Note(fmt.Sprintf("drop test tables: %v", err))
			}
		}()
		if err := transaction(db, false, func(tx *sql.Tx) error {
			// The invariant must detect duplicate IDs and dangling edges.
			if _, err := tx.Exec(fmt.Sprintf(`CREATE TABLE %s (id BIGINT NOT NULL, counter BIGINT NOT NULL)`, m.nodes)); err != nil {
				return fmt.Errorf("create nodes: %w", err)
			}
			if _, err := tx.Exec(fmt.Sprintf(`CREATE TABLE %s (src BIGINT NOT NULL, dst BIGINT NOT NULL)`, m.edges)); err != nil {
				return fmt.Errorf("create edges: %w", err)
			}
			for id := range count {
				if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO %s (id, counter) VALUES ($1, 0)`, m.nodes), id); err != nil {
					return fmt.Errorf("insert node: %w", err)
				}
				if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO %s (src, dst) VALUES ($1, $2)`, m.edges), id, (id+1)%count); err != nil {
					return fmt.Errorf("insert edge: %w", err)
				}
			}
			return nil
		}); err != nil {
			tc.Errorf("initialize ring: %v", err)
		}
		hegel.RunStateful(tc, m, hegel.WithConcurrency(), hegel.WithStatefulStepCount(500))
	}, hegel.SuppressHealthCheck(hegel.TooSlow))
}

// RuleSwap exchanges two distinct nodes' positions in one serializable transaction.
//
// It selects the nodes through tc.
//
// It notes SQL errors and counts only successfully acknowledged commits.
func (m *ring) RuleSwap(tc hegel.TestCase) {
	first := hegel.Draw(tc, hegel.Integers(0, len(m.acknowledged)-1))
	second := hegel.Draw(tc, hegel.Integers(0, len(m.acknowledged)-2))
	if second >= first {
		second++
	}
	err := transaction(m.db, false, func(tx *sql.Tx) error {
		edges, err := m.readEdges(tx)
		if err != nil {
			return err
		}
		removed, added := swapEdges(edges, first, second)
		for _, e := range removed {
			if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s WHERE src = $1 AND dst = $2`, m.edges), e.from, e.to); err != nil {
				return fmt.Errorf("delete edge: %w", err)
			}
		}
		for _, e := range added {
			if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO %s (src, dst) VALUES ($1, $2)`, m.edges), e.from, e.to); err != nil {
				return fmt.Errorf("create edge: %w", err)
			}
		}
		if _, err := tx.Exec(fmt.Sprintf(`UPDATE %s SET counter = counter + 1 WHERE id IN ($1, $2)`, m.nodes), first, second); err != nil {
			return fmt.Errorf("increment counters: %w", err)
		}
		return nil
	})
	if err != nil {
		tc.Note(fmt.Sprintf("swap transaction: %v", err))
		return
	}
	// Count only acknowledged commits; a connection error leaves the outcome unknown.
	m.acknowledged[first].Add(1)
	m.acknowledged[second].Add(1)
}

// InvariantRing requires the original nodes to form one ring and retain acknowledged counters.
//
// It reads a consistent snapshot after all rule workers have finished.
//
// It reports property violations through tc and notes SQL errors.
func (m *ring) InvariantRing(tc hegel.TestCase) {
	err := transaction(m.db, true, func(tx *sql.Tx) error {
		rows, err := tx.Query(fmt.Sprintf(`SELECT id, counter FROM %s`, m.nodes))
		if err != nil {
			return fmt.Errorf("query nodes: %w", err)
		}
		var nodes []node
		for rows.Next() {
			var n node
			if err := rows.Scan(&n.id, &n.counter); err != nil {
				rows.Close()
				return fmt.Errorf("scan node: %w", err)
			}
			nodes = append(nodes, n)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return fmt.Errorf("read nodes: %w", err)
		}
		edges, err := m.readEdges(tx)
		if err != nil {
			return err
		}
		acknowledged := make([]int64, len(m.acknowledged))
		for id := range acknowledged {
			acknowledged[id] = m.acknowledged[id].Load()
		}
		if err := checkRing(nodes, edges, acknowledged); err != nil {
			tc.Errorf("ring property violated: %v", err)
		}
		return nil
	})
	if err != nil {
		tc.Note(fmt.Sprintf("ring validation: %v", err))
	}
}

func (m *ring) readEdges(tx *sql.Tx) ([]edge, error) {
	rows, err := tx.Query(fmt.Sprintf(`SELECT src, dst FROM %s ORDER BY src, dst`, m.edges))
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()
	var edges []edge
	for rows.Next() {
		var e edge
		if err := rows.Scan(&e.from, &e.to); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

func transaction(db *sql.DB, readOnly bool, fn func(*sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: readOnly})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	// Rollback also runs if Hegel aborts the callback after a property violation.
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

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
