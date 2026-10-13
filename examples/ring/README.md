# Ring workload

Run against a dedicated PostgreSQL-compatible test database:

```sh
go run hegel.dev/go/hegel/examples/ring@latest 'postgres://user:password@localhost/ring?sslmode=disable' --test-cases=100
```

Use a database user that can create and drop tables.
Omit `--test-cases` to run indefinitely.
