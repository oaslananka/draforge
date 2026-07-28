#!/usr/bin/env python3
"""Verify repository-owned GoReleaser Docker v2 artifact metadata."""

from __future__ import annotations

import json
import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import cast

ROOT = Path(__file__).resolve().parents[1]
DIST_DIR = ROOT / "dist"
ARTIFACTS_NAME = "artifacts.json"
METADATA_NAME = "metadata.json"
PLATFORMS = ("linux/amd64", "linux/arm64")
ARCH_BY_PLATFORM = {platform: platform.rsplit("/", 1)[1] for platform in PLATFORMS}
LEGACY_TYPES = {"Published Docker Image", "Docker Manifest"}


@dataclass(frozen=True)
class Component:
    item_id: str
    image: str


COMPONENTS = (
    Component("draforge-server-image", "ghcr.io/oaslananka/draforge-server"),
    Component("draforge-controller-image", "ghcr.io/oaslananka/draforge-controller"),
    Component("draforge-sim-driver-image", "ghcr.io/oaslananka/draforge-sim-driver"),
)
COMPONENT_IDS = {component.item_id for component in COMPONENTS}

Artifact = dict[str, object]


def read_json(path: Path) -> object:
    return json.loads(path.read_text(encoding="utf-8"))


def load_artifacts(dist_dir: Path) -> tuple[list[Artifact], list[str]]:
    path = dist_dir / ARTIFACTS_NAME
    try:
        value = read_json(path)
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        return [], [f"cannot read {path}: {exc}"]
    if not isinstance(value, list):
        return [], [f"{ARTIFACTS_NAME} must contain a list"]
    invalid_count = sum(not isinstance(item, dict) for item in value)
    artifacts = [cast(Artifact, item) for item in value if isinstance(item, dict)]
    errors = (
        [f"{ARTIFACTS_NAME} contains {invalid_count} non-object entries"]
        if invalid_count
        else []
    )
    return artifacts, errors


def load_version(dist_dir: Path) -> tuple[str | None, list[str]]:
    path = dist_dir / METADATA_NAME
    try:
        value = read_json(path)
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        return None, [f"cannot read {path}: {exc}"]
    if not isinstance(value, dict):
        return None, [f"{METADATA_NAME} must contain an object"]
    version = value.get("version")
    if not isinstance(version, str) or not version:
        return None, [f"{METADATA_NAME} does not contain a non-empty version"]
    return version, []


def version_tags(version: str) -> tuple[str, str, str]:
    version_core = version.split("-", 1)[0].split("+", 1)[0]
    pieces = version_core.split(".")
    if len(pieces) < 2 or not all(piece.isdigit() for piece in pieces[:2]):
        raise ValueError(f"metadata version cannot produce a major/minor tag: {version}")
    return version, f"v{pieces[0]}.{pieces[1]}", "latest"


def expected_names(component: Component, version: str, snapshot: bool) -> set[str]:
    tags = version_tags(version)
    if snapshot:
        return {
            f"{component.image}:{tag}-{ARCH_BY_PLATFORM[platform]}"
            for platform in PLATFORMS
            for tag in tags
        }
    return {f"{component.image}:{tag}" for tag in tags}


def artifact_extra(artifact: Artifact) -> dict[str, object] | None:
    extra = artifact.get("extra")
    return cast(dict[str, object], extra) if isinstance(extra, dict) else None


def collect_v2_artifacts(artifacts: list[Artifact]) -> list[Artifact]:
    result: list[Artifact] = []
    for artifact in artifacts:
        if artifact.get("type") != "Docker Image":
            continue
        extra = artifact_extra(artifact)
        if extra is not None and extra.get("ID") in COMPONENT_IDS:
            result.append(artifact)
    return result


def validate_legacy_types(artifacts: list[Artifact]) -> list[str]:
    count = sum(artifact.get("type") in LEGACY_TYPES for artifact in artifacts)
    return [f"found {count} legacy Docker image/manifest artifacts"] if count else []


def validate_common_metadata(artifacts: list[Artifact]) -> list[str]:
    errors: list[str] = []
    for artifact in artifacts:
        name = artifact.get("name")
        extra = artifact_extra(artifact)
        if extra is None:
            errors.append(f"Docker artifact has no extra metadata: {name}")
            continue
        digest = extra.get("Digest")
        if not isinstance(digest, str) or not digest:
            errors.append(f"Docker artifact has no digest: {name}")
    return errors


def artifacts_for_component(artifacts: list[Artifact], item_id: str) -> list[Artifact]:
    result: list[Artifact] = []
    for artifact in artifacts:
        extra = artifact_extra(artifact)
        if extra is not None and extra.get("ID") == item_id:
            result.append(artifact)
    return result


def validate_names(
    component: Component,
    artifacts: list[Artifact],
    version: str,
    snapshot: bool,
) -> list[str]:
    actual = {
        name
        for artifact in artifacts
        if isinstance((name := artifact.get("name")), str)
    }
    expected = expected_names(component, version, snapshot)
    if actual == expected:
        return []
    mode = "snapshot" if snapshot else "release"
    return [
        f"{component.item_id} image tags differ in {mode} mode: "
        f"expected={sorted(expected)}, actual={sorted(actual)}"
    ]


def expected_platforms(name: str, snapshot: bool) -> list[str] | None:
    if not snapshot:
        return list(PLATFORMS)
    for platform, arch in ARCH_BY_PLATFORM.items():
        if name.endswith(f"-{arch}"):
            return [platform]
    return None


def validate_platforms(artifacts: list[Artifact], snapshot: bool) -> list[str]:
    errors: list[str] = []
    for artifact in artifacts:
        name = artifact.get("name")
        extra = artifact_extra(artifact)
        if not isinstance(name, str) or extra is None:
            continue
        wanted = expected_platforms(name, snapshot)
        if wanted is None:
            errors.append(f"snapshot Docker artifact lacks a platform suffix: {name}")
            continue
        actual = extra.get("Platforms")
        if actual != wanted:
            errors.append(f"{name} platforms differ: expected={wanted}, actual={actual}")
    return errors


def validate_dist(dist_dir: Path = DIST_DIR) -> list[str]:
    artifacts, artifact_errors = load_artifacts(dist_dir)
    version, version_errors = load_version(dist_dir)
    errors = artifact_errors + version_errors
    if errors or version is None:
        return errors

    try:
        version_tags(version)
    except ValueError as exc:
        return [str(exc)]

    snapshot = "SNAPSHOT" in version.upper()
    v2_artifacts = collect_v2_artifacts(artifacts)
    expected_count = len(COMPONENTS) * (len(PLATFORMS) * 3 if snapshot else 3)
    if len(v2_artifacts) != expected_count:
        mode = "snapshot" if snapshot else "release"
        errors.append(
            f"expected {expected_count} Docker Image V2 artifacts in {mode} mode, "
            f"got {len(v2_artifacts)}"
        )

    errors.extend(validate_legacy_types(artifacts))
    errors.extend(validate_common_metadata(v2_artifacts))
    for component in COMPONENTS:
        component_artifacts = artifacts_for_component(v2_artifacts, component.item_id)
        errors.extend(validate_names(component, component_artifacts, version, snapshot))
        errors.extend(validate_platforms(component_artifacts, snapshot))
    return errors


def fixture_payload(snapshot: bool) -> tuple[list[Artifact], dict[str, str]]:
    version = "0.2.1-SNAPSHOT-test" if snapshot else "0.2.1"
    artifacts: list[Artifact] = []
    for component in COMPONENTS:
        for name in sorted(expected_names(component, version, snapshot)):
            platforms = expected_platforms(name, snapshot)
            assert platforms is not None
            artifacts.append(
                {
                    "name": name,
                    "path": name,
                    "type": "Docker Image",
                    "extra": {
                        "ID": component.item_id,
                        "Digest": f"sha256:{component.item_id}-{len(artifacts)}",
                        "Platforms": platforms,
                    },
                }
            )
    return artifacts, {"version": version}


def write_fixture(root: Path, artifacts: list[Artifact], metadata: dict[str, str]) -> None:
    (root / ARTIFACTS_NAME).write_text(json.dumps(artifacts), encoding="utf-8")
    (root / METADATA_NAME).write_text(json.dumps(metadata), encoding="utf-8")


def valid_fixture_errors(root: Path, snapshot: bool) -> list[str]:
    artifacts, metadata = fixture_payload(snapshot)
    write_fixture(root, artifacts, metadata)
    mode = "snapshot" if snapshot else "release"
    return [f"valid {mode} fixture failed: {error}" for error in validate_dist(root)]


def self_test() -> list[str]:
    with tempfile.TemporaryDirectory(prefix="draforge-docker-artifacts-") as temp_dir:
        root = Path(temp_dir)
        if errors := valid_fixture_errors(root, snapshot=True):
            return errors
        if errors := valid_fixture_errors(root, snapshot=False):
            return errors

        artifacts, metadata = fixture_payload(snapshot=True)
        artifacts.pop()
        write_fixture(root, artifacts, metadata)
        if not any("expected 18" in error for error in validate_dist(root)):
            return ["missing snapshot artifact fixture unexpectedly passed"]

        artifacts, metadata = fixture_payload(snapshot=False)
        artifacts.pop()
        write_fixture(root, artifacts, metadata)
        if not any("expected 9" in error for error in validate_dist(root)):
            return ["missing release artifact fixture unexpectedly passed"]

        artifacts, metadata = fixture_payload(snapshot=True)
        artifacts[0]["type"] = "Docker Manifest"
        write_fixture(root, artifacts, metadata)
        if not any("legacy Docker" in error for error in validate_dist(root)):
            return ["legacy artifact fixture unexpectedly passed"]

        artifacts, metadata = fixture_payload(snapshot=False)
        first_extra = artifact_extra(artifacts[0])
        assert first_extra is not None
        first_extra["Platforms"] = ["linux/arm64"]
        write_fixture(root, artifacts, metadata)
        if not any("platforms differ" in error for error in validate_dist(root)):
            return ["wrong release platform fixture unexpectedly passed"]
    return []


def main() -> int:
    allowed = {"--self-test", "--validate"}
    arguments = set(sys.argv[1:])
    unknown = arguments - allowed
    if unknown:
        print(f"unsupported arguments: {sorted(unknown)}", file=sys.stderr)
        return 2

    run_validation = not arguments or "--validate" in arguments
    errors: list[str] = []
    if "--self-test" in arguments:
        fixture_errors = self_test()
        if fixture_errors:
            errors.extend(fixture_errors)
        else:
            print("==> GoReleaser Docker artifact verifier fixtures passed.")
    if run_validation:
        errors.extend(validate_dist())

    if errors:
        print("GoReleaser Docker artifact contract failed:", file=sys.stderr)
        for error in errors:
            print(f"  - {error}", file=sys.stderr)
        return 1
    if run_validation:
        print("==> GoReleaser Docker artifact contract passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
