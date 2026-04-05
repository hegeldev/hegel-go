export PATH := "/usr/local/go/bin:" + env("HOME") + "/go/bin:" + env("PATH")

# Run tests with coverage.
# We measure coverage only on the library package (not cmd/ binaries).
test:
    #!/usr/bin/env bash
    set -euo pipefail
    go test -race -coverprofile=coverage.out -covermode=atomic \
        -coverpkg=hegel.dev/go/hegel \
        ./...
    python3 scripts/check-coverage.py

format:
    gofmt -w .

lint:
    #!/usr/bin/env bash
    set -euo pipefail
    unformatted=$(gofmt -l .)
    if [ -n "$unformatted" ]; then
        echo "The following files need formatting (run 'just format'):"
        echo "$unformatted"
        exit 1
    fi
    go vet ./...
    go tool staticcheck ./...

docs:
    #!/usr/bin/env bash
    set -euo pipefail
    # Verify all exported symbols have doc comments using go vet
    go doc -all . > /dev/null 2>&1

build-conformance:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p bin/conformance
    go build -o bin/conformance ./internal/conformance/cmd/...

conformance: build-conformance
    uv run --with 'hegel-core==0.3.0' --with pytest --with hypothesis \
        pytest tests/conformance/ -v

serve-docs:
    go install golang.org/x/pkgsite/cmd/pkgsite@latest
    go clean -cache
    pkgsite -http=:8080 .

# Run lint + docs + test (the full CI check).
check: lint docs test
