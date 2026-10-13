# Concurrent ring workload

[main.go](main.go) is a single-file, standalone example implementing the [Antithesis ring test](https://antithesis.com/docs/resources/ring_test/)
using the same `hegel.Workload`, `database/sql`, and PostgreSQL driver setup as
[bank](../bank/README.md). It needs no Antithesis services. The example shares
Hegel's module and release version; importing Hegel does not import the SQL driver.

Once published, run against a dedicated test database:

```sh
go run hegel.dev/go/hegel/examples/ring@latest 'postgres://user:password@localhost/ring?sslmode=disable' --test-cases=100
```

From this checkout, run the single file from the repository root:

```sh
go run examples/ring/main.go 'postgres://user:password@localhost/ring?sslmode=disable' --test-cases=100
```

Omit `--test-cases` to run indefinitely. Hegel's `--database` flag selects the
saved-example directory; the first argument selects the SQL store. The database
user needs permission to create and drop tables. Each test case, including
shrinking replays, creates a fresh ring of 3–10 nodes and drops its tables afterward.

The swap rule chooses two distinct nodes and exchanges their positions by
deleting and recreating their incident edges within one serializable transaction.
Relabeling both endpoints handles adjacent nodes, including three-node rings,
and separated nodes. Node counters increase in the same transaction. There are
up to 500 rounds per case, with a Hegel-selected worker count up to `GOMAXPROCS`.
Reading the edges and writing the replacements use the same transaction snapshot.
Transactions can conflict and abort; there is no global client lock or retry loop.
Health checks remain enabled, so a slow run can stop with `TooSlow`.

The invariant reads nodes, counters, and edges in one serializable snapshot.
It checks that the original node IDs each occur exactly once, the edge count is
unchanged, every node has one incoming and one outgoing edge, and a traversal
visits every node before returning to its start. Disconnected cycles and self
loops fail even if their total node and edge counts are correct. Validation
runs before and after transformations and is sampled at intermediate join points,
when all workers have finished.

For durability, atomic local counters track swaps whose commits returned
success. Each database counter must be at least its local counter. Equality
would wrongly reject transactions that committed but returned a connection error.
The local counters last for one test case and survive database restarts while
the workload process stays alive. They do not survive workload process restarts;
this example does not persist counters to a local file or inject faults itself.

SQL errors in rules and validation are noted and skipped, as in bank, since
fault injection can cause spurious failures. Initialization errors fail the
workload, and cleanup errors are noted. Skipped checks do not establish a
property; run long enough for successful reads and commits. The schema has no
uniqueness or foreign-key constraints, allowing the invariant to detect
corruption. The checks expose violations of these properties but do not prove
full ACID compliance.

`main_test.go` tests the topology and counter checks and every ordered node pair
for rings of size 3–10. Set `HEGEL_RING_DATABASE_URL` to run the PostgreSQL
integration tests for commit outcomes, isolation options, and SQL error reporting.
These tests check that local acknowledgements match committed writes.
The standalone example, like bank, is excluded from the
unit-test coverage threshold and is also checked against a live database. To adapt another DBMS, change
its SQL syntax and isolation options in `main.go`. PostgreSQL wire compatibility
alone does not guarantee serializable transaction support. Operations have
30-second context deadlines; configure database lock timeouts externally if needed.
