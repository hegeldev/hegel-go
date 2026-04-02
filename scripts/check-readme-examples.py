#!/usr/bin/env python3
"""Check that every Go code example in README.md appears in example_test.go.

Extracts ```go fenced code blocks from the README, finds the test body
(the t.Run(...) or hegel.Run(...) call), and verifies it appears as a
substring of example_test.go. Reference-only blocks (API listings) and
single-line syntax illustrations are skipped.

Exits non-zero if any multi-line test example is missing.
"""

import re
import sys


def extract_go_blocks(markdown: str) -> list[str]:
    """Return the bodies of all ```go fenced code blocks."""
    blocks = []
    for m in re.finditer(r"^```go\n(.*?)^```", markdown, re.MULTILINE | re.DOTALL):
        blocks.append(m.group(1).strip())
    return blocks


def extract_test_body(block: str) -> str | None:
    """Extract the t.Run(...) or hegel.Run(...) portion from a code block.

    Returns None if the block doesn't contain a test call (e.g. API reference
    listings).
    """
    lines = block.splitlines()
    for i, line in enumerate(lines):
        stripped = line.strip()
        if stripped.startswith("t.Run(") or stripped.startswith("err := hegel.Run("):
            return "\n".join(lines[i:]).strip()
    return None


def normalise(code: str) -> str:
    """Strip leading whitespace from each line."""
    lines = [line.strip() for line in code.splitlines()]
    return "\n".join(lines)


def main() -> int:
    with open("README.md") as f:
        readme = f.read()
    with open("example_test.go") as f:
        examples = f.read()

    normalised_examples = normalise(examples)

    blocks = extract_go_blocks(readme)
    if not blocks:
        print("WARNING: no Go code blocks found in README.md", file=sys.stderr)
        return 1

    checked = 0
    missing = []
    for i, block in enumerate(blocks, 1):
        body = extract_test_body(block)
        if body is None:
            continue

        # Skip single-line syntax illustrations
        if "\n" not in body.strip():
            continue

        checked += 1
        normalised_body = normalise(body)
        if normalised_body not in normalised_examples:
            missing.append((i, body))

    if missing:
        print(f"❌ {len(missing)} README example(s) not found in example_test.go:\n")
        for num, body in missing:
            preview = body.splitlines()[0]
            print(f"  Block {num}: {preview}...")
        return 1

    print(f"✅ All {checked} Go test examples from README.md found in example_test.go")
    return 0


if __name__ == "__main__":
    sys.exit(main())
