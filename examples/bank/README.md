# Concurrent bank workload

[main.go](main.go) is a single-file, standalone
example of concurrent stateful testing against a PostgreSQL-compatible store.
It uses `database/sql` with the lib/pq driver and needs no Antithesis services.
The example shares Hegel's module and release version. Importing Hegel does not
import the example or its SQL driver.

Once published, run it against a dedicated test database from any directory:

```sh
go run hegel.dev/go/hegel/examples/bank@latest 'postgres://user:password@localhost/bank?sslmode=disable' --test-cases=100
```

To run this checkout, use the repository root and omit the version suffix:

```sh
go run hegel.dev/go/hegel/examples/bank 'postgres://user:password@localhost/bank?sslmode=disable' --test-cases=100
```

Omit `--test-cases` to run indefinitely. Hegel's `--database` flag controls
the saved-example directory; the first argument selects the SQL store. The database
user must be able to create and drop tables. Every test case, including replays
during shrinking, initializes a fresh table and drops it afterward.

The machine starts with 2–100 accounts sharing a total balance of 1,000,000.
Its three rules transfer funds, add a zero-balance account, and merge/delete an
account. `hegel.WithConcurrency()` lets Hegel choose a worker count up to
`GOMAXPROCS` for each test case.
Transfers and merges use serializable transactions and lock accounts in ID order.
SQL errors in rules and invariants stop the operation or check without retrying
or failing a property, since fault injection may cause spurious failures.
Initialization errors fail the workload, and cleanup errors are logged. After
initialization, only observed violations of the four bank properties fail the
workload.

The two invariants run on the initial and final states and are sampled at join
points after all workers finish. One uses an aggregate SQL query; the other declares a
server-side cursor and computes the same checks across repeated `FETCH` calls.
Both require at least one account, unique IDs, a total balance of 1,000,000, and
non-negative balances. Small cursor batches exercise multiple fetches. The
schema has no uniqueness or non-negative-balance constraints, so the checks can
detect violations. These assertions check the bank's safety properties; they
do not establish full serializability or durability across database restarts.

To adapt another DBMS, change the SQL statements in `main.go` for its parameter,
locking, and cursor syntax. Account IDs come from a Go atomic counter, so the schema needs
no database-specific identity columns. Operations use Go context deadlines;
configure database-specific lock timeouts externally if needed. PostgreSQL wire
compatibility alone does not guarantee support for the required SQL or isolation
levels.
