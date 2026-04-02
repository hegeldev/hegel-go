# Coverage Script Internals

This file describes how "100% coverage" is actually measured. It should be, but may not have been, kept up to date.

The canonical source of truth for the behaviour is `scripts/check-coverage.py` and `.testcoverage.yml`, and if you notice discrepancies between what you see here and what is happening, check those files and, if necessary, update this file with any changes.

## How the coverage script works

1. Sanitizes `coverage.out` — removes stray lines (test-count output, etc.) that `go test ./...` sometimes appends when running multiple packages.
2. Runs `go-test-coverage` (configured in `.testcoverage.yml`) which enforces 100% per-file coverage, excluding lines annotated with `// coverage-ignore`.
3. Counts `// coverage-ignore` annotations in non-test `.go` files and checks against the ratchet.
4. If the count decreased, tightens the ratchet and updates `.github/coverage-ratchet.json`.

## coverage-ignore annotations

Uses the standard `go-test-coverage` annotation: `// coverage-ignore`.

- **Block-level**: Place on the `if`/`switch`/`for`/`select` opening `{` line to exclude the entire block body.
- **Function-level**: Place on the `func ... {` line to exclude the entire function.
- **Line-level**: Place on any other line to exclude just that coverage profile block.

## Ratchet mechanism

The total count of `// coverage-ignore` annotations is tracked in `.github/coverage-ratchet.json`. This count may only decrease. If the script finds fewer annotations than the ratchet allows, it tightens the ratchet automatically.
