#!/usr/bin/env python3
"""Verify that release and development version metadata cannot drift."""

from __future__ import annotations

import argparse
import json
import re
import sys
import tempfile
from pathlib import Path

SEMVER_RE = re.compile(
    r"^(0|[1-9][0-9]*)\."
    r"(0|[1-9][0-9]*)\."
    r"(0|[1-9][0-9]*)"
    r"(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$"
)

GO_MAIN_FILES = (
    Path("cmd/draforge/main.go"),
    Path("cmd/draforge-controller/main.go"),
    Path("cmd/draforge-sim-driver/main.go"),
)

SOURCE_DOCKERFILES = (
    Path("build/package/Dockerfile.server"),
    Path("build/package/Dockerfile.controller"),
    Path("build/package/Dockerfile.sim-driver"),
)


def read_text(root: Path, relative: Path, errors: list[str]) -> str:
    path = root / relative
    try:
        return path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as exc:
        errors.append(f"cannot read {relative}: {exc}")
        return ""


def chart_value(chart: str, key: str) -> str | None:
    match = re.search(
        rf'^\s*{re.escape(key)}:\s*["\']?([^"\'\s#]+)["\']?\s*(?:#.*)?$',
        chart,
        re.MULTILINE,
    )
    return match.group(1) if match else None


def validate(root: Path) -> list[str]:
    errors: list[str] = []

    chart = read_text(root, Path("deploy/helm/draforge/Chart.yaml"), errors)
    chart_version = chart_value(chart, "version")
    app_version = chart_value(chart, "appVersion")

    if chart_version is None:
        errors.append("Chart.yaml does not define version")
    elif not SEMVER_RE.fullmatch(chart_version):
        errors.append(f"Chart.yaml version is not semantic: {chart_version}")

    if app_version is None:
        errors.append("Chart.yaml does not define appVersion")
    elif not SEMVER_RE.fullmatch(app_version):
        errors.append(f"Chart.yaml appVersion is not semantic: {app_version}")

    if chart_version and app_version and chart_version != app_version:
        errors.append(
            f"Chart.yaml version {chart_version} does not match appVersion {app_version}"
        )

    package_text = read_text(root, Path("web/package.json"), errors)
    web_version: str | None = None
    if package_text:
        try:
            package = json.loads(package_text)
        except json.JSONDecodeError as exc:
            errors.append(f"web/package.json is invalid JSON: {exc}")
        else:
            value = package.get("version")
            if not isinstance(value, str) or not value:
                errors.append("web/package.json does not define a string version")
            else:
                web_version = value
                if not SEMVER_RE.fullmatch(web_version):
                    errors.append(
                        f"web/package.json version is not semantic: {web_version}"
                    )

    if app_version and web_version and app_version != web_version:
        errors.append(
            f"web/package.json version {web_version} does not match chart appVersion {app_version}"
        )

    changelog = read_text(root, Path("CHANGELOG.md"), errors)
    if app_version and changelog:
        release_heading = re.compile(
            rf"^## \[{re.escape(app_version)}\](?:\s+-\s+\d{{4}}-\d{{2}}-\d{{2}})?\s*$",
            re.MULTILINE,
        )
        if not release_heading.search(changelog):
            errors.append(
                f"CHANGELOG.md does not contain a release heading for {app_version}"
            )

    for relative in GO_MAIN_FILES:
        source = read_text(root, relative, errors)
        if source and not re.search(r'^\s*versionVal\s*=\s*"dev"\s*$', source, re.MULTILINE):
            errors.append(f"{relative} must default versionVal to dev")
        if source and not re.search(
            r'^\s*commitSHA\s*=\s*"unknown"\s*$', source, re.MULTILINE
        ):
            errors.append(f"{relative} must default commitSHA to unknown")

    for relative in SOURCE_DOCKERFILES:
        dockerfile = read_text(root, relative, errors)
        if dockerfile and not re.search(
            r"^ARG VERSION=dev$", dockerfile, re.MULTILINE
        ):
            errors.append(f"{relative} must default VERSION to dev")
        if dockerfile and not re.search(
            r"^ARG COMMIT=unknown$", dockerfile, re.MULTILINE
        ):
            errors.append(f"{relative} must default COMMIT to unknown")
        if dockerfile and (
            "-X main.versionVal=${VERSION}" not in dockerfile
            or "-X main.commitSHA=${COMMIT}" not in dockerfile
        ):
            errors.append(f"{relative} does not inject VERSION and COMMIT ldflags")

    goreleaser = read_text(root, Path(".goreleaser.yaml"), errors)
    if goreleaser:
        version_injections = goreleaser.count("-X main.versionVal={{.Version}}")
        commit_injections = goreleaser.count("-X main.commitSHA={{.ShortCommit}}")
        if version_injections != len(GO_MAIN_FILES):
            errors.append(
                ".goreleaser.yaml must inject the release version into all three binaries"
            )
        if commit_injections != len(GO_MAIN_FILES):
            errors.append(
                ".goreleaser.yaml must inject the release commit into all three binaries"
            )

    return errors


def write_fixture(root: Path) -> None:
    files = {
        Path("deploy/helm/draforge/Chart.yaml"): (
            'apiVersion: v2\nname: draforge\nversion: 0.2.0\nappVersion: "0.2.0"\n'
        ),
        Path("web/package.json"): '{"name":"draforge-web","version":"0.2.0"}\n',
        Path("CHANGELOG.md"): "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-06-21\n",
        Path(".goreleaser.yaml"): "\n".join(
            [
                "- -X main.versionVal={{.Version}} -X main.commitSHA={{.ShortCommit}}"
                for _ in GO_MAIN_FILES
            ]
        )
        + "\n",
    }
    for relative in GO_MAIN_FILES:
        files[relative] = 'var (\n versionVal = "dev"\n commitSHA = "unknown"\n)\n'
    for relative in SOURCE_DOCKERFILES:
        files[relative] = (
            "ARG VERSION=dev\n"
            "ARG COMMIT=unknown\n"
            'RUN go build -ldflags="-X main.versionVal=${VERSION} '
            '-X main.commitSHA=${COMMIT}"\n'
        )

    for relative, content in files.items():
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")


def self_test() -> int:
    with tempfile.TemporaryDirectory(prefix="draforge-release-metadata-") as temp_dir:
        root = Path(temp_dir)
        write_fixture(root)

        errors = validate(root)
        if errors:
            print("valid release metadata fixture failed:", file=sys.stderr)
            for error in errors:
                print(f"  - {error}", file=sys.stderr)
            return 1

        package_path = root / "web/package.json"
        package_path.write_text(
            '{"name":"draforge-web","version":"0.1.0"}\n',
            encoding="utf-8",
        )
        errors = validate(root)
        if not any("does not match chart appVersion" in error for error in errors):
            print("web version mismatch fixture unexpectedly passed", file=sys.stderr)
            return 1

        write_fixture(root)
        go_path = root / GO_MAIN_FILES[0]
        go_path.write_text(
            'var (\n versionVal = "v0.2.0"\n commitSHA = "unknown"\n)\n',
            encoding="utf-8",
        )
        errors = validate(root)
        if not any("must default versionVal to dev" in error for error in errors):
            print("released Go fallback fixture unexpectedly passed", file=sys.stderr)
            return 1

        write_fixture(root)
        docker_path = root / SOURCE_DOCKERFILES[0]
        docker_path.write_text(
            "ARG VERSION=v0.2.0\nARG COMMIT=unknown\n",
            encoding="utf-8",
        )
        errors = validate(root)
        if not any("must default VERSION to dev" in error for error in errors):
            print("released Docker fallback fixture unexpectedly passed", file=sys.stderr)
            return 1

    print("==> Release metadata verifier fixtures passed.")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, default=Path.cwd())
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()

    if args.self_test:
        result = self_test()
        if result != 0:
            return result

    errors = validate(args.root.resolve())
    if errors:
        print("Release metadata contract failed:", file=sys.stderr)
        for error in errors:
            print(f"  - {error}", file=sys.stderr)
        return 1

    chart = read_text(
        args.root.resolve(), Path("deploy/helm/draforge/Chart.yaml"), []
    )
    release_version = chart_value(chart, "appVersion") or "unknown"
    print(
        "==> Release metadata contract passed: "
        f"release={release_version}, source-build=dev/unknown."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
