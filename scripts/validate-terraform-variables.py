#!/usr/bin/env python3
"""Reject Terraform input variables that do not affect showcase configuration."""

from __future__ import annotations

import argparse
import re
import sys
import tempfile
from pathlib import Path

VARIABLE_DECLARATION_RE = re.compile(
    r'^\s*variable\s+"([A-Za-z_][A-Za-z0-9_]*)"\s*\{',
    re.MULTILINE,
)
VARIABLE_REFERENCE_RE = re.compile(r"\bvar\.([A-Za-z_][A-Za-z0-9_]*)\b")


def find_unused_variables(tf_dir: Path) -> list[str]:
    variables_file = tf_dir / "variables.tf"
    if not variables_file.is_file():
        raise FileNotFoundError(f"missing Terraform variables file: {variables_file}")

    declared = set(
        VARIABLE_DECLARATION_RE.findall(variables_file.read_text(encoding="utf-8"))
    )
    referenced: set[str] = set()
    for tf_file in sorted(tf_dir.glob("*.tf")):
        if tf_file == variables_file:
            continue
        referenced.update(
            VARIABLE_REFERENCE_RE.findall(tf_file.read_text(encoding="utf-8"))
        )

    return sorted(declared - referenced)


def validate(tf_dir: Path) -> int:
    unused = find_unused_variables(tf_dir)
    if unused:
        print("Unused Terraform input variables:", file=sys.stderr)
        for name in unused:
            print(f"  - {name}", file=sys.stderr)
        return 1

    print(
        "==> Terraform variable contract passed: "
        "every declared input affects configuration."
    )
    return 0


def self_test() -> int:
    with tempfile.TemporaryDirectory(
        prefix="draforge-tf-variable-contract-"
    ) as temp_dir:
        root = Path(temp_dir)
        safe = root / "safe"
        unsafe = root / "unsafe"
        safe.mkdir()
        unsafe.mkdir()

        (safe / "variables.tf").write_text(
            'variable "region" { type = string }\n',
            encoding="utf-8",
        )
        (safe / "main.tf").write_text(
            'locals { selected_region = var.region }\n',
            encoding="utf-8",
        )

        (unsafe / "variables.tf").write_text(
            'variable "region" { type = string }\n'
            'variable "unused_toggle" { type = bool }\n',
            encoding="utf-8",
        )
        (unsafe / "main.tf").write_text(
            'locals { selected_region = var.region }\n',
            encoding="utf-8",
        )

        if find_unused_variables(safe):
            print(
                "safe variable-contract fixture unexpectedly failed",
                file=sys.stderr,
            )
            return 1
        if find_unused_variables(unsafe) != ["unused_toggle"]:
            print(
                "unsafe variable-contract fixture unexpectedly passed",
                file=sys.stderr,
            )
            return 1

    print("==> Terraform variable contract fixtures passed.")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("tf_dir", nargs="?", type=Path)
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()

    if args.self_test:
        result = self_test()
        if result != 0:
            return result

    if args.tf_dir is None:
        if args.self_test:
            return 0
        parser.error("tf_dir is required unless --self-test is used")

    try:
        return validate(args.tf_dir)
    except (OSError, UnicodeError) as exc:
        print(
            f"Terraform variable contract validation failed: {exc}",
            file=sys.stderr,
        )
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
