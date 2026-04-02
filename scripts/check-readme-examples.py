#!/usr/bin/env python3
"""Check that Go code examples in README.md and hegel.go appear in example_test.go.

Extracts code blocks from both:
  - ```go fenced blocks in README.md
  - tab-indented blocks in the hegel.go package doc comment

For each block containing a test body (t.Run or hegel.Run), verifies it
appears as a substring of example_test.go. Reference-only blocks (API
listings) and single-line syntax illustrations are skipped.

Exits non-zero if any multi-line test example is missing.
"""

import re
import sys


def extract_go_blocks_from_markdown(markdown: str) -> list[str]:
    """Return the bodies of all ```go fenced code blocks."""
    blocks = []
    for m in re.finditer(r"^```go\n(.*?)^```", markdown, re.MULTILINE | re.DOTALL):
        blocks.append(m.group(1).strip())
    return blocks


def extract_go_blocks_from_godoc(source: str) -> list[str]:
    """Return code blocks from the package doc comment in a .go file.

    In Go doc comments, code blocks are contiguous runs of lines where the
    content after the '//' prefix starts with a tab character.
    """
    doc_lines = []
    for line in source.splitlines():
        stripped = line.lstrip()
        if stripped.startswith("//"):
            # Strip the // prefix and at most one space
            after = stripped[2:]
            if after.startswith(" "):
                after = after[1:]
            doc_lines.append(after)
        elif stripped.startswith("package "):
            break
        elif stripped == "":
            continue
        else:
            break

    blocks = []
    current_block: list[str] = []
    for line in doc_lines:
        if line.startswith("\t"):
            current_block.append(line[1:])  # strip leading tab
        else:
            if current_block:
                blocks.append("\n".join(current_block).strip())
                current_block = []
    if current_block:
        blocks.append("\n".join(current_block).strip())
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


def check_blocks(
    blocks: list[str], normalised_examples: str
) -> list[tuple[int, str]]:
    """Check that test bodies from blocks appear in example_test.go.

    Returns a list of (block_number, body) for missing examples.
    """
    missing = []
    checked = 0
    for i, block in enumerate(blocks, 1):
        body = extract_test_body(block)
        if body is None:
            continue
        if "\n" not in body.strip():
            continue
        checked += 1
        normalised_body = normalise(body)
        if normalised_body not in normalised_examples:
            missing.append((i, body))
    return missing


def main() -> int:
    with open("example_test.go") as f:
        examples = f.read()
    normalised_examples = normalise(examples)

    all_missing: list[tuple[str, int, str]] = []

    # Check README.md
    with open("README.md") as f:
        readme = f.read()
    readme_blocks = extract_go_blocks_from_markdown(readme)
    for num, body in check_blocks(readme_blocks, normalised_examples):
        all_missing.append(("README.md", num, body))

    # Check hegel.go package doc
    with open("hegel.go") as f:
        source = f.read()
    godoc_blocks = extract_go_blocks_from_godoc(source)
    for num, body in check_blocks(godoc_blocks, normalised_examples):
        all_missing.append(("hegel.go", num, body))

    total_checked = sum(
        1
        for blocks in [readme_blocks, godoc_blocks]
        for b in blocks
        if (body := extract_test_body(b)) is not None and "\n" in body.strip()
    )

    if all_missing:
        print(
            f"❌ {len(all_missing)} example(s) not found in example_test.go:\n"
        )
        for source_name, num, body in all_missing:
            preview = body.splitlines()[0]
            print(f"  {source_name} block {num}: {preview}...")
        return 1

    print(
        f"✅ All {total_checked} Go test examples from README.md and hegel.go"
        f" found in example_test.go"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
