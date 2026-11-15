#!/usr/bin/env python3
"""Validate Go coverage source mapping without accepting filesystem paths."""
# SPDX-License-Identifier: AGPL-3.0-only

import argparse
import tempfile
from pathlib import Path

MODULE = "github.com/oaslananka/draforge/"
MODE_ATOMIC = "mode: atomic\n"


def coverage_record(line: str, root: Path) -> tuple[Path | None, int, str]:
    path, separator, values = line.partition(":")
    if not separator or not path.startswith(MODULE):
        return None, 0, "invalid module-qualified Go source path"
    name = Path(path[len(MODULE):])
    if name.is_absolute() or ".." in name.parts or name.suffix != ".go":
        return None, 0, "unsafe or non-Go coverage path"
    source = (root / name).resolve()
    if not source.is_relative_to(root.resolve()) or not source.is_file():
        return None, 0, f"source missing from repository: {name}"
    fields = values.split()
    if len(fields) != 3:
        return None, 0, "malformed Go coverage record"
    try:
        statements, executions = int(fields[1]), int(fields[2])
    except ValueError:
        return None, 0, "non-numeric Go coverage values"
    if statements < 0 or executions < 0:
        return None, 0, "invalid statement/hit count"
    return name, statements if executions > 0 else 0, ""


def check(root: Path, profile: str) -> list[str]:
    errors: list[str] = []
    # This path is derived from the script's checkout, not caller input.
    config = root / "codecov.yml"
    if not config.is_file():
        errors.append("missing repository codecov.yml")
    elif f'"{MODULE}::"' not in config.read_text(encoding="utf-8"):
        errors.append("missing Go module path mapping in codecov.yml")
    lines = profile.splitlines()
    if not lines or lines[0] not in ("mode: atomic", "mode: count", "mode: set"):
        return errors + ["missing Go coverage mode"]
    if len(lines) < 2:
        return errors + ["empty Go coverage profile"]

    sources: set[Path] = set()
    covered = 0
    for number, line in enumerate(lines[1:], start=2):
        name, statements, error = coverage_record(line, root)
        if error:
            errors.append(f"line {number}: {error}")
        elif name is not None:
            sources.add(name)
            covered += statements
    if not sources or covered < 1:
        errors.append("no real covered Go source statements")
    return errors


def self_test() -> None:
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary)
        (root / "codecov.yml").write_text('fixes:\n  - "' + MODULE + '::"\n')
        (root / "cmd").mkdir()
        (root / "cmd" / "main.go").write_text("package main\n")
        stem = MODULE + "cmd/main.go:"
        fixtures = [
            (MODE_ATOMIC + stem + "1.1,1.3 1 1\n", False),
            (MODE_ATOMIC + MODULE + "cmd/missing.go:1.1,1.3 1 1\n", True),
            (MODE_ATOMIC + MODULE + "../secret.go:1.1,1.3 1 1\n", True),
            (MODE_ATOMIC + stem + "1.1,1.3 1 1\n" + stem + "2.1,2.3 0 0\n", False),
            (MODE_ATOMIC + stem + "1.1,1.3 1 0\n", True),
            (MODE_ATOMIC, True),
        ]
        for number, (profile, should_fail) in enumerate(fixtures, start=1):
            errors = check(root, profile)
            if bool(errors) != should_fail:
                raise ValueError(f"Codecov fixture {number} incorrect: {errors}")
    print("Codecov source-path fixtures passed.")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
    root = Path(__file__).resolve().parent.parent
    report = root / "coverage.out"
    if not report.is_file():
        if args.self_test:
            return 0
        print("Codecov source-path error: checkout coverage.out is missing")
        return 1
    issues = check(root, report.read_text(encoding="utf-8"))
    for issue in issues:
        print("Codecov source-path error: " + issue)
    if issues:
        return 1
    print("Codecov Go coverage paths verified against repository sources.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
