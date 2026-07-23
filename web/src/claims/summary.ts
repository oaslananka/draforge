import type { ClaimAllocation, ResourceClaimInfo } from '../api/types';

export function claimClassNames(claim: ResourceClaimInfo): string[] {
  const classes = new Set<string>();
  for (const request of claim.requests ?? []) {
    for (const alternative of request.alternatives ?? []) {
      if (alternative.deviceClassName) classes.add(alternative.deviceClassName);
    }
  }
  if (classes.size === 0 && claim.deviceClassName) classes.add(claim.deviceClassName);
  return [...classes].sort();
}

export function effectiveClaimAllocations(claim: ResourceClaimInfo): ClaimAllocation[] {
  if (claim.allocations && claim.allocations.length > 0) return claim.allocations;
  if (!claim.allocatedDevice) return [];
  return [{
    request: '',
    driverName: claim.allocatedDriver ?? '',
    poolName: '',
    deviceName: claim.allocatedDevice,
    nodeName: claim.allocatedNode,
  }];
}

export function claimAllocationLabels(claim: ResourceClaimInfo): string[] {
  return effectiveClaimAllocations(claim).map(allocation => {
    const request = allocation.request || 'request';
    const node = allocation.nodeName || 'unknown-node';
    return `${request}=${allocation.driverName}/${allocation.poolName}/${allocation.deviceName}@${node}`;
  });
}
