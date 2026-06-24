#!/usr/bin/env python3
"""Validate a Terraform JSON plan for the DRAForge showcase environment."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from typing import Any

GPU_SIZE_RE = re.compile(r".*gpu.*|.*g-.*", re.IGNORECASE)
MAX_CLUSTER_CREATES = 1
MAX_NODE_COUNT = 2
ALLOWED_CREATE_TYPES = {
    "digitalocean_container_registry",
    "digitalocean_kubernetes_cluster",
    "digitalocean_project",
    "digitalocean_vpc",
}


def _actions(change: dict[str, Any]) -> list[str]:
    actions = change.get("change", {}).get("actions", [])
    return actions if isinstance(actions, list) else []


def _after(change: dict[str, Any]) -> dict[str, Any]:
    after = change.get("change", {}).get("after", {})
    return after if isinstance(after, dict) else {}


def _is_mutating(actions: list[str]) -> bool:
    return any(action in actions for action in ("create", "update", "replace", "delete"))


def _check_node_pool(node_pool: dict[str, Any], errors: list[str]) -> None:
    node_count = node_pool.get("node_count")
    if node_count is not None and (node_count < 1 or node_count > MAX_NODE_COUNT):
        errors.append(f"Forbidden configuration: node_count ({node_count}) must be between 1 and {MAX_NODE_COUNT}.")

    size = str(node_pool.get("size", ""))
    if GPU_SIZE_RE.match(size):
        errors.append(f"Forbidden configuration: GPU node size ({size}) is prohibited.")

    if node_pool.get("auto_scale") is True:
        errors.append("Forbidden configuration: autoscaling is enabled on a node pool.")


def validate_plan(plan_path: str | Path) -> list[str]:
    with Path(plan_path).open("r", encoding="utf-8") as f:
        plan = json.load(f)

    resource_changes = plan.get("resource_changes", [])
    if not isinstance(resource_changes, list):
        return ["Invalid plan: resource_changes must be a list."]

    cluster_creates = 0
    errors: list[str] = []

    for change in resource_changes:
        if not isinstance(change, dict):
            errors.append("Invalid plan: every resource change must be an object.")
            continue

        resource_type = change.get("type")
        resource_name = change.get("name", "unknown")
        actions = _actions(change)
        after = _after(change)

        if "create" in actions and resource_type not in ALLOWED_CREATE_TYPES:
            errors.append(f"Forbidden resource: creating {resource_type}.{resource_name} is not allowed in the showcase plan.")

        if resource_type == "digitalocean_droplet" and "create" in actions:
            errors.append(f"Forbidden resource: standalone droplet creation ({resource_name}) is prohibited.")

        if resource_type == "digitalocean_kubernetes_cluster":
            if "create" in actions:
                cluster_creates += 1
                if cluster_creates > MAX_CLUSTER_CREATES:
                    errors.append("Forbidden configuration: more than one DOKS cluster is proposed.")

            if after.get("ha") is True:
                errors.append("Forbidden configuration: paid HA control plane is enabled.")

            node_pools = after.get("node_pool", [])
            if isinstance(node_pools, dict):
                node_pools = [node_pools]
            if isinstance(node_pools, list):
                for node_pool in node_pools:
                    if isinstance(node_pool, dict):
                        _check_node_pool(node_pool, errors)

        if resource_type == "digitalocean_kubernetes_node_pool" and _is_mutating(actions):
            _check_node_pool(after, errors)

    return errors


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print("Usage: validate-plan.py <terraform-plan-json>", file=sys.stderr)
        return 2

    errors = validate_plan(argv[1])
    if errors:
        print("==> Terraform Plan Audit FAILED with the following violations:")
        for error in errors:
            print(f"  - {error}")
        return 1

    print("==> Terraform Plan Audit PASSED. All constraints satisfied.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
