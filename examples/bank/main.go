// Bank tests PostgreSQL-compatible stores using concurrent transactions.
//
// Run against a dedicated test database:
//
//	go run hegel.dev/go/hegel/examples/bank@latest 'postgres://user:password@localhost/bank?sslmode=disable' --test-cases=100
//
// Each test case creates and drops its own table. Rules may overlap, but
// invariants run only at join points, when all rule transactions have finished.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
	"hegel.dev/go/hegel"
)

const (
	totalBalance = int64(1_000_000)
	// Small batches exercise multiple FETCH calls even with a modest bank.
	cursorBatch = 16
)

type bank struct {
	db     *sql.DB
	table  string
	nextID atomic.Int64
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
		// The identifier contains only generated ASCII letters and digits.
		m := &bank{db: db, table: "bank_" + strings.ToLower(rand.Text())}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, m.table)); err != nil {
				tc.Note(fmt.Sprintf("drop test table: %v", err))
			}
		}()
		count := hegel.Draw(tc, hegel.Integers(2, 100))
		cuts := []int64{0, totalBalance}
		for range count - 1 {
			cuts = append(cuts, hegel.Draw(tc, hegel.Integers(int64(0), totalBalance)))
		}
		slices.Sort(cuts)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			tc.Errorf("begin initialization: %v", err)
		}
		defer tx.Rollback()
		// Constraints would hide violations that the invariants must detect.
		if _, err := tx.Exec(fmt.Sprintf(`CREATE TABLE %s ( id BIGINT NOT NULL, balance BIGINT NOT NULL )`, m.table)); err != nil {
			tc.Errorf("create accounts: %v", err)
		}
		for i := 1; i < len(cuts); i++ {
			if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO %s (id, balance) VALUES ($1, $2)`, m.table), m.nextID.Add(1), cuts[i]-cuts[i-1]); err != nil {
				tc.Errorf("insert initial account: %v", err)
			}
		}
		if err := tx.Commit(); err != nil {
			tc.Errorf("commit initialization: %v", err)
		}
		hegel.RunStateful(tc, m,
			hegel.WithConcurrency(),
		)
	}, hegel.SuppressHealthCheck(hegel.TooSlow)) // Database I/O dominates generation time.
}

// RuleTransfer preserves the total by debiting and crediting in one transaction.
func (m *bank) RuleTransfer(tc hegel.TestCase) {
	transaction(tc, m.db, nil, func(tx *sql.Tx) error {
		from, to, balances, err := m.lockPair(tc, tx)
		if err != nil || len(balances) != 2 || balances[from] == 0 {
			return err
		}
		amount := hegel.Draw(tc, hegel.Integers(int64(1), balances[from]))
		if _, err := tx.Exec(fmt.Sprintf(`UPDATE %s SET balance = balance - $1 WHERE id = $2`, m.table), amount, from); err != nil {
			return fmt.Errorf("debit: %w", err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`UPDATE %s SET balance = balance + $1 WHERE id = $2`, m.table), amount, to); err != nil {
			return fmt.Errorf("credit: %w", err)
		}
		return nil
	})
}

// RuleAdd creates a zero-balance account without changing the total.
func (m *bank) RuleAdd(tc hegel.TestCase) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := m.db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s (id, balance) VALUES ($1, $2)`, m.table), m.nextID.Add(1), 0); err != nil {
		tc.Note(fmt.Sprintf("add account: %v", err))
	}
}

// RuleMerge moves an account's funds before deleting it in the same transaction.
func (m *bank) RuleMerge(tc hegel.TestCase) {
	transaction(tc, m.db, nil, func(tx *sql.Tx) error {
		from, to, balances, err := m.lockPair(tc, tx)
		if err != nil || len(balances) != 2 {
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf(`UPDATE %s SET balance = balance + $1 WHERE id = $2`, m.table), balances[from], to); err != nil {
			return fmt.Errorf("merge balance: %w", err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, m.table), from); err != nil {
			return fmt.Errorf("delete merged account: %w", err)
		}
		return nil
	})
}

// InvariantQuery checks all four bank guarantees using one aggregate query.
func (m *bank) InvariantQuery(tc hegel.TestCase) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var total, minimum sql.NullInt64
	var count, distinct int64
	err := m.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT SUM(balance), MIN(balance), COUNT(*), COUNT(DISTINCT id) FROM %s`, m.table)).
		Scan(&total, &minimum, &count, &distinct)
	if err != nil {
		tc.Note(fmt.Sprintf("query validation: %v", err))
		return
	}
	checkBank(tc, total, minimum, count, distinct)
}

// InvariantCursor checks the same guarantees across server-side cursor fetches.
func (m *bank) InvariantCursor(tc hegel.TestCase) {
	transaction(tc, m.db, &sql.TxOptions{ReadOnly: true}, func(tx *sql.Tx) error {
		var total, minimum sql.NullInt64
		var count, distinct int64
		if _, err := tx.Exec(fmt.Sprintf(`DECLARE bank_validation NO SCROLL CURSOR FOR SELECT id, balance FROM %s ORDER BY id`, m.table)); err != nil {
			return fmt.Errorf("declare validation cursor: %w", err)
		}
		var previousID int64
		for {
			// sql.Rows alone does not guarantee a server-side cursor or batched FETCH.
			rows, err := tx.Query(fmt.Sprintf(`FETCH FORWARD %d FROM bank_validation`, cursorBatch))
			if err != nil {
				return fmt.Errorf("fetch validation cursor: %w", err)
			}
			fetched := 0
			for rows.Next() {
				var id, balance int64
				if err := rows.Scan(&id, &balance); err != nil {
					rows.Close()
					return fmt.Errorf("scan validation cursor: %w", err)
				}
				if count == 0 || id != previousID {
					distinct++
				}
				previousID = id
				total.Int64 += balance
				total.Valid = true
				if !minimum.Valid || balance < minimum.Int64 {
					minimum = sql.NullInt64{Int64: balance, Valid: true}
				}
				count++
				fetched++
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return fmt.Errorf("read validation cursor: %w", err)
			}
			if fetched == 0 {
				break
			}
		}
		checkBank(tc, total, minimum, count, distinct)
		return nil
	})
}

func checkBank(tc hegel.TestCase, total, minimum sql.NullInt64, count, distinct int64) {
	if count < 1 {
		tc.Errorf("no accounts remain")
	}
	if distinct != count {
		tc.Errorf("account IDs are not unique: %d distinct IDs in %d rows", distinct, count)
	}
	if !total.Valid || total.Int64 != totalBalance {
		tc.Errorf("total balance = %v, want %d", total, totalBalance)
	}
	if !minimum.Valid || minimum.Int64 < 0 {
		tc.Errorf("negative or missing minimum balance: %v", minimum)
	}
}

func (m *bank) lockPair(tc hegel.TestCase, tx *sql.Tx) (int64, int64, map[int64]int64, error) {
	rows, err := tx.Query(fmt.Sprintf(`SELECT id FROM %s ORDER BY id`, m.table))
	if err != nil {
		return 0, 0, nil, fmt.Errorf("list accounts: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, 0, nil, fmt.Errorf("scan account ID: %w", err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(ids) < 2 {
		return 0, 0, nil, err
	}
	first := hegel.Draw(tc, hegel.Integers(0, len(ids)-1))
	second := hegel.Draw(tc, hegel.Integers(0, len(ids)-2))
	if second >= first {
		second++
	}
	from, to := ids[first], ids[second]
	// Every transaction locks in ID order, even when the transfer goes backwards.
	rows, err = tx.Query(fmt.Sprintf(`SELECT id, balance FROM %s WHERE id IN ($1, $2) ORDER BY id FOR UPDATE`, m.table), from, to)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("lock accounts: %w", err)
	}
	defer rows.Close()
	balances := make(map[int64]int64)
	for rows.Next() {
		var id, balance int64
		if err := rows.Scan(&id, &balance); err != nil {
			return 0, 0, nil, fmt.Errorf("scan locked account: %w", err)
		}
		balances[id] = balance
	}
	return from, to, balances, rows.Err()
}

func transaction(tc hegel.TestCase, db *sql.DB, opts *sql.TxOptions, fn func(*sql.Tx) error) {
	err := func() error {
		if opts == nil {
			opts = &sql.TxOptions{Isolation: sql.LevelSerializable}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tx, err := db.BeginTx(ctx, opts)
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}
		// Rollback also runs if Hegel aborts the transaction callback.
		defer tx.Rollback()
		if err := fn(tx); err != nil {
			return err
		}
		return tx.Commit()
	}()
	if err != nil {
		tc.Note(fmt.Sprintf("SQL operation failed: %v", err))
	}
}
