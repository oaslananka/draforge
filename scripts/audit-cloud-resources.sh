#!/usr/bin/env bash
# scripts/audit-cloud-resources.sh
# Audits resources used by the optional DigitalOcean showcase (max 2 worker nodes, max 1 cluster, no GPUs).

set -euo pipefail

echo "==> Auditing DigitalOcean resources for DRAForge..."

# Verify doctl authentication
if ! doctl account get &>/dev/null; then
    echo "ERROR: doctl is not authenticated or unable to reach API" >&2
    exit 1
fi

# 1. Audit Kubernetes Clusters
clusters_json=$(doctl kubernetes cluster list --output json)
cluster_count=$(echo "$clusters_json" | jq -r '.[] | select(.tags[]? | contains("draforge")) | .id' | wc -l | tr -d ' ')

echo "Found $cluster_count Kubernetes clusters matching 'draforge'"
if [ "$cluster_count" -gt 1 ]; then
    echo "FAIL: More than 1 DOKS cluster exists for DRAForge" >&2
    exit 2
fi

# 2. Audit Compute Nodes (Worker Droplets)
droplets_json=$(doctl compute droplet list --output json)
worker_count=$(echo "$droplets_json" | jq -r '.[] | select(.tags[]? | contains("draforge")) | .id' | wc -l | tr -d ' ')

echo "Found $worker_count worker node droplets matching 'draforge'"
if [ "$worker_count" -gt 2 ]; then
    echo "FAIL: More than 2 worker nodes exist for DRAForge" >&2
    exit 3
fi

# 3. Check for GPU sizes in any active droplets
gpu_sizes="g-.*|gpu-.*"
gpu_detected=$(echo "$droplets_json" | jq -r --arg gpu "$gpu_sizes" '.[] | select(.tags[]? | contains("draforge")) | select(.size_slug | test($gpu)) | .name')
if [ -n "$gpu_detected" ]; then
    echo "FAIL: GPU droplet size detected: $gpu_detected" >&2
    exit 4
fi

# 4. Check for autoscaling on the cluster
if [ "$cluster_count" -eq 1 ]; then
    cluster_id=$(echo "$clusters_json" | jq -r '.[] | select(.tags[]? | contains("draforge")) | .id')
    autoscaling_enabled=$(doctl kubernetes cluster get "$cluster_id" --output json | jq -r '.[] | .node_pools[].auto_scale' | grep -q "true" && echo "true" || echo "false")
    if [ "$autoscaling_enabled" = "true" ]; then
        echo "FAIL: Autoscaling is enabled on the DOKS cluster" >&2
        exit 5
    fi
fi

echo "PASS: Cloud resource audit successful. All constraints satisfied."
exit 0
