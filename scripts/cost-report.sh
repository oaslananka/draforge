#!/usr/bin/env bash
# scripts/cost-report.sh
# Generates an estimated cost report based on active resources tagged with draforge.

set -euo pipefail

echo "==> DRAForge Active Resource Cost Report <=="

# Query resources using doctl
droplets_json=$(doctl compute droplet list --output json)
lb_json=$(doctl compute load-balancer list --output json)
volume_json=$(doctl compute volume list --output json)

# Count tagged resources
worker_count=$(echo "$droplets_json" | jq -r '.[] | select(.tags[]? | contains("draforge")) | .id' | wc -l | tr -d ' ')
lb_count=$(echo "$lb_json" | jq -r '.[] | select(.name | contains("draforge")) | .id' | wc -l | tr -d ' ')
vol_count=$(echo "$volume_json" | jq -r '.[] | select(.tags[]? | contains("draforge")) | .id' | wc -l | tr -d ' ')

# Standard pricing estimates
node_unit_price=48
lb_unit_price=12
vol_unit_price=1
registry_price=5

node_total=$((worker_count * node_unit_price))
lb_total=$((lb_count * lb_unit_price))
vol_total=$((vol_count * vol_unit_price))
grand_total=$((node_total + lb_total + vol_total + registry_price))

echo "--------------------------------------------------"
echo "Worker Nodes:      $worker_count (Est. \$${node_total}/mo)"
echo "Load Balancers:    $lb_count (Est. \$${lb_total}/mo)"
echo "Storage Volumes:   $vol_count (Est. \$${vol_total}/mo)"
echo "Container Registry: 1 (Est. \$${registry_price}/mo)"
echo "--------------------------------------------------"
echo "Grand Total Est:   \$${grand_total}/month"
echo "--------------------------------------------------"
exit 0
