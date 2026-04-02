# Coverage Script Internals

This file describes how "100% coverage" is actually measured. It should be, but may not have been, kept up to date.

The canonical source of truth for the behaviour is `scripts/check-coverage.py`, and if you notice discrepancies between what you see here and what is happening, check the script and, if necessary, update this file with any changes.

## How the coverage script works

1. Sanitizes `coverage.out` — removes stray lines (e.g. test-count output) that `go test ./...` sometimes appends when running multiple packages.
2. Runs `go tool cover -func=coverage.sanitized.out` to get per-function coverage.
3. If total coverage is 100%, exits successfully.
4. Otherwise, parses the Go coverage profile for uncovered regions.
5. Merges duplicate entries — when `go test -coverpkg=PKG ./...` runs multiple test packages, each independently instruments PKG. Execution counts are summed so that a region covered by *any* test binary is not reported as uncovered.
6. Checks each uncovered region against automatic false-positive filters.
7. Reports any genuinely uncovered lines.

## Automatic exclusions

These patterns are excluded without needing `//nocov`:

- Structural syntax (closing braces `}`, `)`, `});`, empty lines)
- Lines containing `//nocov`

## Coverage profile format

Go coverage profiles use this format per line:

```
module/path/file.go:startLine.startCol,endLine.endCol numStatements executionCount
```

The script uses **line-level** analysis — a line with execution count > 0 is covered. This means inline closures on covered lines don't need separate tests.

## File path resolution

The script converts module paths (e.g. `hegel.dev/go/hegel/file.go`) to local filesystem paths by progressively stripping leading path components until the file exists on disk.
