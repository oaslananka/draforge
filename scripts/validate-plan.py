# scripts/validate-plan.py
import json
import sys
import re

def validate_plan(plan_path):
    with open(plan_path, 'r', encoding='utf-8') as f:
        plan = json.load(f)

    resource_changes = plan.get('resource_changes', [])
    
    cluster_count = 0
    errors = []

    for change in resource_changes:
        r_type = change.get('type')
        r_name = change.get('name')
        actions = change.get('change', {}).get('actions', [])
        
        # Check resources being created, updated, or replaced
        if not any(act in actions for act in ['create', 'update', 'no-op']):
            # If we're deleting, we don't block
            if 'delete' in actions and len(actions) == 1:
                continue

        after = change.get('change', {}).get('after', {}) or {}

        if r_type == 'digitalocean_droplet':
            # Block creation of additional standalone droplets
            if 'create' in actions:
                errors.append(f"Forbidden resource: Droplet creation ({r_name}) is prohibited.")

        elif r_type == 'digitalocean_kubernetes_cluster':
            if 'create' in actions:
                cluster_count += 1
                if cluster_count > 1:
                    errors.append("Forbidden configuration: More than one DOKS cluster is proposed.")
            
            # Check HA
            if after.get('ha') is True:
                errors.append("Forbidden configuration: Paid HA control plane is enabled.")

            # Check node pool constraints in cluster definition
            node_pools = after.get('node_pool', [])
            if not isinstance(node_pools, list):
                node_pools = [node_pools]
            for np in node_pools:
                if not np:
                    continue
                node_count = np.get('node_count')
                if node_count is not None and (node_count < 1 or node_count > 2):
                    errors.append(f"Forbidden configuration: Node pool node_count ({node_count}) must be <= 2.")
                
                size = np.get('size', '')
                if re.match(r'.*gpu.*|.*g-.*', size, re.IGNORECASE):
                    errors.append(f"Forbidden configuration: GPU node size ({size}) is prohibited.")
                
                if np.get('auto_scale') is True:
                    errors.append("Forbidden configuration: Autoscaling is enabled on the cluster node pool.")

        elif r_type == 'digitalocean_kubernetes_node_pool':
            node_count = after.get('node_count')
            if node_count is not None and (node_count < 1 or node_count > 2):
                errors.append(f"Forbidden configuration: Node pool node_count ({node_count}) must be <= 2.")
            
            size = after.get('size', '')
            if re.match(r'.*gpu.*|.*g-.*', size, re.IGNORECASE):
                errors.append(f"Forbidden configuration: GPU node size ({size}) is prohibited.")
            
            if after.get('auto_scale') is True:
                errors.append("Forbidden configuration: Autoscaling is enabled on the node pool.")

    if errors:
        print("==> Terraform Plan Audit FAILED with the following violations:")
        for err in errors:
            print(f"  - {err}")
        sys.exit(1)
    else:
        print("==> Terraform Plan Audit PASSED. All constraints satisfied.")
        sys.exit(0)

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: python validate-plan.py <path_to_tfplan_json>")
        sys.exit(1)
    validate_plan(sys.argv[1])
