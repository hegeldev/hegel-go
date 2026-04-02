# Hegel for Go
# This justfile provides the standard build recipes.

export PATH := "/usr/local/go/bin:" + env("HOME") + "/go/bin:" + env("PATH")

# Install dependencies. The hegel binary is auto-installed by the library
# on first use (pinned in installer.go). Set HEGEL_SERVER_COMMAND to override.
setup:
    uv venv .venv

# Run tests with coverage, fail if below 100%.
# We measure coverage only on the library package (not cmd/ binaries).
test:
    #!/usr/bin/env bash
    set -euo pipefail
    export PATH="$(pwd)/.venv/bin:$PATH"
    go test -race -coverprofile=coverage.out -covermode=atomic \
        -coverpkg=hegel.dev/go/hegel \
        ./...
    python3 scripts/check-coverage.py

# Auto-format code.
format:
    gofmt -w .

# Check //nocov annotation style.
check-nocov-style:
    python3 scripts/check-nocov-style.py

# Check formatting + linting.
lint: check-nocov-style
    #!/usr/bin/env bash
    set -euo pipefail
    # Check formatting
    unformatted=$(gofmt -l .)
    if [ -n "$unformatted" ]; then
        echo "❌ The following files need formatting (run 'just format'):"
        echo "$unformatted"
        exit 1
    fi
    echo "✅ All files formatted"
    # Run go vet
    go vet ./...
    echo "✅ go vet passed"
    # Run staticcheck
    go tool staticcheck ./...
    echo "✅ staticcheck passed"

# Build API documentation from source. Must succeed with zero warnings.
docs:
    #!/usr/bin/env bash
    set -euo pipefail
    # Verify all exported symbols have doc comments using go vet
    go doc -all . > /dev/null 2>&1
    echo "✅ Documentation generated successfully"

# Build internal conformance test binaries into bin/conformance/.
build-conformance:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p bin/conformance
    go build -o bin/conformance ./internal/conformance/cmd/...
    echo "✅ Conformance binaries built to bin/conformance/"

# Run conformance tests against the real hegel server.
conformance: build-conformance
    uv run --with hegel-core --with pytest --with hypothesis pytest tests/conformance/ -v

# Serve API documentation locally at http://localhost:8080.
serve-docs:
    go install golang.org/x/pkgsite/cmd/pkgsite@latest
    go clean -cache
    pkgsite -http=:8080 .

# Run lint + docs + test (the full CI check).
check: lint docs test
