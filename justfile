set ignore-comments := true

export PATH := "/usr/local/go/bin:" + env("HOME") + "/go/bin:" + env("PATH")

check-format:
    #!/usr/bin/env bash
    set -euo pipefail
    unformatted=$(gofmt -l .)
    if [ -n "$unformatted" ]; then
        echo "The following files need formatting (run 'just format'):"
        echo "$unformatted"
        exit 1
    fi

check-tests *args:
    go test -race {{args}} -coverprofile=coverage.out -covermode=atomic \
        -coverpkg=hegel.dev/go/hegel ./...
    python3 scripts/check-coverage.py

format:
    gofmt -w .

check-lint: check-format
    go vet ./...
    go tool staticcheck ./...

check-docs:
    # verify all exported symbols have doc comments
    go doc -all . > /dev/null 2>&1

docs:
    go doc -http .

build-conformance:
    mkdir -p bin/conformance
    go build -o bin/conformance ./internal/conformance/cmd/...

check-conformance: build-conformance
    uv run --with 'hegel-core==0.5.0' --with pytest --with hypothesis \
        pytest tests/conformance/ -v

# these aliases are provided as ux improvements for local developers. CI should use the longer
# forms.
test *args: (check-tests args)
lint: check-lint
conformance: check-conformance
check: check-lint check-docs check-tests
