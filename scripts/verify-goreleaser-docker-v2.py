#!/usr/bin/env python3
"""Verify the repository-owned GoReleaser Docker v2 contract."""

from __future__ import annotations

import re
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CONFIG_NAME = ".goreleaser.yaml"
REQUIRED_TAGS = (
    '"{{ .Version }}"',
    '"v{{ .Major }}.{{ .Minor }}"',
    '"latest"',
)
REQUIRED_PLATFORMS = ("linux/amd64", "linux/arm64")
REQUIRED_LABELS = (
    "org.opencontainers.image.created",
    "org.opencontainers.image.title",
    "org.opencontainers.image.revision",
    "org.opencontainers.image.version",
)


@dataclass(frozen=True)
class Component:
    item_id: str
    build_id: str
    image: str
    dockerfile: str
    binary: str
    extra_file: str | None = None


COMPONENTS = (
    Component(
        "draforge-server-image",
        "draforge",
        "ghcr.io/oaslananka/draforge-server",
        "build/package/Dockerfile.server.goreleaser",
        "draforge",
        "web/dist",
    ),
    Component(
        "draforge-controller-image",
        "draforge-controller",
        "ghcr.io/oaslananka/draforge-controller",
        "build/package/Dockerfile.controller.goreleaser",
        "draforge-controller",
    ),
    Component(
        "draforge-sim-driver-image",
        "draforge-sim-driver",
        "ghcr.io/oaslananka/draforge-sim-driver",
        "build/package/Dockerfile.sim-driver.goreleaser",
        "draforge-sim-driver",
    ),
)


def top_level_section(text: str, name: str) -> str | None:
    match = re.search(rf"(?m)^{re.escape(name)}:\s*$", text)
    if match is None:
        return None
    start = match.end()
    next_section = re.search(r"(?m)^\w+:\s*$", text[start:])
    end = start + next_section.start() if next_section else len(text)
    return text[start:end]


def docker_items(section: str) -> dict[str, str]:
    items: dict[str, str] = {}
    for part in re.split(r"(?m)^ {2}- ", section)[1:]:
        item = "  - " + part
        match = re.search(r"(?m)^ {2}- id:\s*([^\s#]+)\s*$", item)
        if match is not None:
            items[match.group(1)] = item
    return items


def validate_signing(config: str) -> list[str]:
    section = top_level_section(config, "docker_signs")
    if section is None:
        return ["top-level docker_signs configuration is missing"]
    required = (
        "  - artifacts: all",
        "    cmd: cosign",
        '      - "sign"',
        '      - "--yes"',
        '      - "${artifact}"',
    )
    return [
        f"docker_signs is missing contract fragment: {fragment}"
        for fragment in required
        if fragment not in section
    ]


def validate_dockerfile(root: Path, component: Component) -> list[str]:
    path = root / component.dockerfile
    try:
        text = path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as exc:
        return [f"cannot read {component.dockerfile}: {exc}"]

    errors: list[str] = []
    if not re.search(r"(?m)^ARG TARGETPLATFORM\s*$", text):
        errors.append(f"{component.dockerfile} must declare ARG TARGETPLATFORM")
    copy_line = f"COPY $TARGETPLATFORM/{component.binary} ./"
    if copy_line not in text:
        errors.append(
            f"{component.dockerfile} must copy the platform binary with: {copy_line}"
        )
    if component.extra_file and "COPY web/dist ./web/dist" not in text:
        errors.append(f"{component.dockerfile} must preserve the embedded web/dist context")
    return errors


def required_fragments(component: Component) -> list[str]:
    fragments = [
        f"      - {component.build_id}",
        f'      - "{component.image}"',
        f"    dockerfile: {component.dockerfile}",
        "    platforms:",
        "    labels:",
        "    sbom: true",
    ]
    fragments.extend(f"      - {platform}" for platform in REQUIRED_PLATFORMS)
    fragments.extend(f"      - {tag}" for tag in REQUIRED_TAGS)
    fragments.extend(f'      "{label}":' for label in REQUIRED_LABELS)
    if component.extra_file:
        fragments.append(f"      - {component.extra_file}")
    return fragments


def validate_component(root: Path, item: str, component: Component) -> list[str]:
    errors = [
        f"{component.item_id} is missing contract fragment: {fragment}"
        for fragment in required_fragments(component)
        if fragment not in item
    ]
    errors.extend(validate_dockerfile(root, component))
    return errors


def validate_config(root: Path = ROOT) -> list[str]:
    path = root / CONFIG_NAME
    try:
        config = path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as exc:
        return [f"cannot read {CONFIG_NAME}: {exc}"]

    errors: list[str] = []
    if re.search(r"(?m)^dockers:\s*$", config):
        errors.append("legacy top-level dockers configuration is still present")
    if re.search(r"(?m)^docker_manifests:\s*$", config):
        errors.append("legacy top-level docker_manifests configuration is still present")

    section = top_level_section(config, "dockers_v2")
    if section is None:
        errors.append("top-level dockers_v2 configuration is missing")
        return errors

    errors.extend(validate_signing(config))
    items = docker_items(section)
    expected_ids = {component.item_id for component in COMPONENTS}
    actual_ids = set(items)
    if actual_ids != expected_ids:
        errors.append(
            "dockers_v2 IDs differ: "
            f"expected={sorted(expected_ids)}, actual={sorted(actual_ids)}"
        )
    for component in COMPONENTS:
        item = items.get(component.item_id)
        if item is not None:
            errors.extend(validate_component(root, item, component))
    return errors


def run_goreleaser_check() -> list[str]:
    try:
        result = subprocess.run(
            ["goreleaser", "check"],
            cwd=ROOT,
            check=False,
            capture_output=True,
            text=True,
        )
    except OSError as exc:
        return [f"cannot execute goreleaser: {exc}"]

    output = result.stdout + result.stderr
    if output:
        print(output, end="" if output.endswith("\n") else "\n")
    errors: list[str] = []
    if result.returncode != 0:
        errors.append(f"goreleaser check exited with {result.returncode}")
    if re.search(r"(?i)phased out|deprecated", output):
        errors.append("goreleaser check emitted a deprecation warning")
    return errors


def fixture_config() -> str:
    lines = ["version: 2", "dockers_v2:"]
    for component in COMPONENTS:
        lines.extend(
            [
                f"  - id: {component.item_id}",
                "    ids:",
                f"      - {component.build_id}",
                "    images:",
                f'      - "{component.image}"',
                "    tags:",
                '      - "{{ .Version }}"',
                '      - "v{{ .Major }}.{{ .Minor }}"',
                '      - "latest"',
                f"    dockerfile: {component.dockerfile}",
            ]
        )
        if component.extra_file:
            lines.extend(["    extra_files:", f"      - {component.extra_file}"])
        lines.extend(
            [
                "    platforms:",
                "      - linux/amd64",
                "      - linux/arm64",
                "    labels:",
                '      "org.opencontainers.image.created": "{{.Date}}"',
                '      "org.opencontainers.image.title": "{{.ProjectName}}"',
                '      "org.opencontainers.image.revision": "{{.FullCommit}}"',
                '      "org.opencontainers.image.version": "{{.Version}}"',
                "    sbom: true",
            ]
        )
    lines.extend(
        [
            "docker_signs:",
            "  - artifacts: all",
            "    cmd: cosign",
            "    args:",
            '      - "sign"',
            '      - "--yes"',
            '      - "${artifact}"',
        ]
    )
    return "\n".join(lines) + "\n"


def write_fixture(root: Path, config: str, *, bad_copy: bool = False) -> None:
    (root / CONFIG_NAME).write_text(config, encoding="utf-8")
    for component in COMPONENTS:
        path = root / component.dockerfile
        path.parent.mkdir(parents=True, exist_ok=True)
        copy_line = (
            f"COPY {component.binary} ./"
            if bad_copy and component.item_id == "draforge-controller-image"
            else f"COPY $TARGETPLATFORM/{component.binary} ./"
        )
        extra = "COPY web/dist ./web/dist\n" if component.extra_file else ""
        path.write_text(
            "FROM alpine:3.20.0\n"
            "ARG TARGETPLATFORM\n"
            f"{copy_line}\n"
            f"{extra}",
            encoding="utf-8",
        )


def self_test() -> list[str]:
    with tempfile.TemporaryDirectory(prefix="draforge-goreleaser-v2-") as temp_dir:
        root = Path(temp_dir)
        valid = fixture_config()
        write_fixture(root, valid)
        if errors := validate_config(root):
            return [f"valid fixture failed: {error}" for error in errors]

        write_fixture(root, valid + "dockers:\n  - image_templates: []\n")
        if not any("legacy top-level dockers" in error for error in validate_config(root)):
            return ["legacy Docker fixture unexpectedly passed"]

        missing_arm64 = valid.replace("      - linux/arm64\n", "", 1)
        write_fixture(root, missing_arm64)
        if not any("linux/arm64" in error for error in validate_config(root)):
            return ["missing arm64 fixture unexpectedly passed"]

        write_fixture(root, valid, bad_copy=True)
        if not any("platform binary" in error for error in validate_config(root)):
            return ["platform-agnostic COPY fixture unexpectedly passed"]
    return []


def main() -> int:
    allowed = {"--self-test", "--check"}
    arguments = set(sys.argv[1:])
    unknown = arguments - allowed
    if unknown:
        print(f"unsupported arguments: {sorted(unknown)}", file=sys.stderr)
        return 2

    errors: list[str] = []
    if "--self-test" in arguments:
        fixture_errors = self_test()
        if fixture_errors:
            errors.extend(fixture_errors)
        else:
            print("==> GoReleaser Docker v2 verifier fixtures passed.")
    errors.extend(validate_config())
    if "--check" in arguments:
        errors.extend(run_goreleaser_check())

    if errors:
        print("GoReleaser Docker v2 contract failed:", file=sys.stderr)
        for error in errors:
            print(f"  - {error}", file=sys.stderr)
        return 1
    print("==> GoReleaser Docker v2 contract passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
