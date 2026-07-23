import { describe, expect, it } from 'vitest';
import type { ResourceClaimInfo } from '../api/types';
import { claimAllocationLabels, claimClassNames, effectiveClaimAllocations } from './summary';

const completeClaim: ResourceClaimInfo = {
  name: 'multi',
  namespace: 'team-a',
  status: 'Allocated',
  requests: [
    { name: 'gpu', mode: 'Exactly', alternatives: [{ deviceClassName: 'gpu-class', allocationMode: 'ExactCount', count: 2 }] },
    { name: 'accelerator', mode: 'FirstAvailable', alternatives: [
      { name: 'nic', deviceClassName: 'nic-class', allocationMode: 'ExactCount', count: 1 },
      { name: 'fpga', deviceClassName: 'fpga-class', allocationMode: 'ExactCount', count: 1 },
    ] },
  ],
  allocations: [
    { request: 'gpu', driverName: 'driver-a.example', poolName: 'shared', deviceName: 'dev-0', nodeName: 'node-a' },
    { request: 'accelerator/nic', driverName: 'driver-b.example', poolName: 'shared', deviceName: 'dev-0' },
  ],
};

describe('claim collection summaries', () => {
  it('returns every requested class in stable order', () => {
    expect(claimClassNames(completeClaim)).toEqual(['fpga-class', 'gpu-class', 'nic-class']);
  });

  it('returns every allocation identity without collapsing drivers', () => {
    expect(claimAllocationLabels(completeClaim)).toEqual([
      'gpu=driver-a.example/shared/dev-0@node-a',
      'accelerator/nic=driver-b.example/shared/dev-0@unknown-node',
    ]);
  });

  it('keeps deprecated single-allocation responses readable', () => {
    const legacy: ResourceClaimInfo = {
      name: 'legacy', namespace: 'default', status: 'Allocated',
      deviceClassName: 'legacy-class', allocatedDevice: 'dev-0', allocatedDriver: 'driver.example', allocatedNode: 'node-a',
    };
    expect(claimClassNames(legacy)).toEqual(['legacy-class']);
    expect(effectiveClaimAllocations(legacy)).toHaveLength(1);
  });
});
