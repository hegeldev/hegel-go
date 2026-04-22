#!/usr/bin/env bash
set -euo pipefail

# Generate Python wrapper scripts for compiled Go conformance binaries.
# hegel-core 0.4.x invokes conformance binaries via `python3 <binary>`,
# so we need thin Python scripts that exec the actual Go executables.

for binary in bin/conformance/go/*; do
    name=$(basename "$binary")
    cat > "bin/conformance/$name" <<'WRAPPER'
import os, subprocess, sys
go_bin = os.path.join(os.path.dirname(os.path.abspath(__file__)), "go", os.path.basename(__file__))
sys.exit(subprocess.run([go_bin] + sys.argv[1:], env=os.environ).returncode)
WRAPPER
done
