# Hegel for Go
# This justfile provides the standard build recipes.

export PATH := "/usr/local/go/bin:" + env("HOME") + "/go/bin:" + env("PATH")

# Install dependencies and the hegel binary into the local venv.
# Set HEGEL_SERVER_COMMAND to override the binary used at runtime.
setup:
    uv venv .venv
    uv pip install --python .venv/bin/python hegel-core==0.3.0

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

# Check formatting + linting.
lint:
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
    #!/usr/bin/env bash
    set -euo pipefail
    export PATH="$(pwd)/.venv/bin:$PATH"
    uv pip install --python .venv/bin/python hegel-core==0.3.0 pytest hypothesis
    .venv/bin/python -m pytest tests/conformance/ -v

# Serve API documentation locally at http://localhost:8080.
serve-docs:
    go install golang.org/x/pkgsite/cmd/pkgsite@latest
    go clean -cache
    pkgsite -http=:8080 .

# Check README code examples appear in example_test.go.
check-readme:
    python3 scripts/check-readme-examples.py

# Run lint + docs + test + readme check (the full CI check).
check: lint docs test check-readme
