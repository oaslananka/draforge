#!/usr/bin/env python3
"""Fail closed when a Go profile cannot map to checked-in project sources."""
# SPDX-License-Identifier: Apache-2.0

import argparse
import tempfile
from pathlib import Path

MODULE = "github.com/oaslananka/draforge/"


def check(root: Path, report: Path) -> list[str]:
    errors: list[str] = []
    config = root / "codecov.yml"
    if not config.is_file():
        errors.append("missing repository codecov.yml")
    elif f'"{MODULE}::"' not in config.read_text(encoding="utf-8"):
        errors.append("missing Go module path mapping in codecov.yml")
    if not report.is_file():
        return errors + ["missing coverage.out"]
    lines = report.read_text(encoding="utf-8").splitlines()
    if not lines or lines[0] not in ("mode: atomic", "mode: count", "mode: set"):
        return errors + ["missing Go coverage mode"]
    if len(lines) < 2:
        return errors + ["empty Go coverage profile"]
    sources: set[Path] = set()
    covered = 0
    for i, line in enumerate(lines[1:], start=2):
        path, separator, segment = line.partition(":")
        if not separator or not path.startswith(MODULE):
            errors.append(f"line {i}: invalid module-qualified Go source path")
            continue
        source = Path(path[len(MODULE):])
        if source.is_absolute() or ".." in source.parts or source.suffix != ".go":
            errors.append(f"line {i}: unsafe or non-Go coverage path")
            continue
        source_path = (root / source).resolve()
        if not source_path.is_relative_to(root.resolve()) or not source_path.is_file():
            errors.append(f"line {i}: source missing from repository: {source}")
            continue
        sources.add(source)
        blocks = segment.split()
        if len(blocks) != 3:
            errors.append(f"line {i}: malformed Go coverage record")
            continue
        try:
            statements, executions = int(blocks[1]), int(blocks[2])
        except ValueError:
            errors.append(f"line {i}: non-numeric Go coverage values")
            continue
        if statements < 0 or executions < 0:
            errors.append(f"line {i}: invalid statement/hit count")
        if executions > 0:
            covered += statements
    if len(sources) < 1 or covered < 1:
        errors.append("no real covered Go source statements")
    return errors


def self_test() -> None:
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary)
        (root / "codecov.yml").write_text('fixes:\n  - "github.com/oaslananka/draforge/::"\n')
        (root / "cmd").mkdir()
        (root / "cmd" / "main.go").write_text("package main\n")
        report = root / "coverage.out"
        report.write_text("mode: atomic\n" + MODULE + "cmd/main.go:1.1,1.3 1 1\n")
        assert check(root, report) == []
        report.write_text("mode: atomic\n" + MODULE + "cmd/missing.go:1.1,1.3 1 1\n")
        assert any("source missing" in issue for issue in check(root, report))
        report.write_text("mode: atomic\n" + MODULE + "../secret.go:1.1,1.3 1 1\n")
        assert any("unsafe" in issue for issue in check(root, report))
        report.write_text("mode: atomic\n" + MODULE + "cmd/main.go:1.1,1.3 1 1\n"
                          + MODULE + "cmd/main.go:2.1,2.3 0 0\n")
        assert check(root, report) == []  # Valid Go profiles may contain 0-statement blocks.
        report.write_text("mode: atomic\n" + MODULE + "cmd/main.go:1.1,1.3 1 0\n")
        assert any("no real covered" in issue for issue in check(root, report))
        report.write_text("mode: atomic\n")
        assert any("empty" in issue for issue in check(root, report))
    print("Codecov source-path fixtures passed.")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--root", type=Path, default=Path("."))
    parser.add_argument("--report", type=Path, default=Path("coverage.out"))
    args = parser.parse_args()
    if args.self_test:
        self_test()
    if args.report.is_file():
        issues = check(args.root, args.report)
        for issue in issues:
            print(f"Codecov source-path error: {issue}")
        if issues:
            return 1
        print("Codecov Go coverage file paths verified against repository sources.")
        return 0
    if args.self_test:
        return 0
    print("Codecov source-path error: coverage report missing")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
