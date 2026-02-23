# Hegel SDK for go
# This justfile provides the standard build recipes.
# Stage 1 will fill in the stub recipes with real implementations.

# Install dependencies and the hegel binary.
# If HEGEL_BINARY is set, symlinks it into .venv/bin instead of installing from git.
setup:
    #!/usr/bin/env bash
    set -euo pipefail
    uv venv .venv
    if [ -n "${HEGEL_BINARY:-}" ]; then
        mkdir -p .venv/bin
        ln -sf "$HEGEL_BINARY" .venv/bin/hegel
    else
        uv pip install --python .venv/bin/python hegel@git+ssh://git@github.com/antithesishq/hegel-core.git
    fi

# Run tests. Implement this in Stage 1.
test:
    @echo "test recipe not yet implemented" && exit 1

# Auto-format code. Implement this in Stage 1.
format:
    @echo "format recipe not yet implemented" && exit 1

# Check formatting + linting. Implement this in Stage 1.
lint:
    @echo "lint recipe not yet implemented" && exit 1

# Build API documentation from source. Implement this in Stage 1.
docs:
    @echo "docs recipe not yet implemented" && exit 1

# Run lint + docs + test (the full CI check).
check: lint docs test
