# Coverage Script Internals

This file describes how "100% coverage" is actually measured. It should be, but may not have been, kept up to date.

The canonical source of truth for the behaviour is `scripts/check-coverage.py`, and if you notice discrepancies between what you see here and what is happening, check the script and, if necessary, update this file with any changes.

## How the coverage script works

1. Sanitizes `coverage.out` — removes stray lines (test-count output, etc.) that `go test ./...` sometimes appends when running multiple packages.
2. Runs `go tool cover -func=coverage.sanitized.out` to get per-function coverage.
3. Parses the Go coverage profile for uncovered regions, merging duplicate entries by summing execution counts.
4. Checks each uncovered region against automatic false-positive filters.
5. Reports any genuinely uncovered lines.
6. Auto-removes `//nocov` annotations from lines that are now covered.
7. Counts remaining `//nocov`-excluded lines and checks against the ratchet.
8. If the count decreased, tightens the ratchet and updates `.github/coverage-ratchet.json`.

## Automatic exclusions

These patterns are excluded without needing `//nocov`:

- Structural syntax (closing braces `}`, `)`, `});`, empty lines)

## nocov annotations

Two forms are supported:

- **Inline**: `code(); //nocov` — excludes that single line.
- **Block**: `//nocov start` ... `//nocov end` on their own lines — excludes all lines between the markers.

The script auto-removes inline `//nocov` from lines that turn out to be covered. Block markers are never auto-removed.

## Ratchet mechanism

The total count of `//nocov`-excluded lines (inline annotations + lines inside blocks) is tracked in `.github/coverage-ratchet.json`. This count may only decrease. If the script finds fewer excluded lines than the ratchet allows, it tightens the ratchet automatically.

## Coverage profile format

Go coverage profiles use this format per line:

```
module/path/file.go:startLine.startCol,endLine.endCol numStatements executionCount
```

When `go test -coverpkg=PKG ./...` runs multiple test packages, each independently instruments PKG and writes its own entries. The script merges duplicates by summing execution counts so that a region covered by any test binary is not reported as uncovered.

## File path resolution

The script converts module paths (e.g. `hegel.dev/go/hegel/file.go`) to local filesystem paths by progressively stripping leading path components until the file exists on disk.
