import { describe, expect, it } from 'vitest';
import type { GraphNode, ResourceClaimInfo } from '../api/types';
import {
  claimIdentityFromGraphNode,
  claimIdentityKey,
  findClaimIdentityByKey,
  toClaimIdentity,
} from './identity';

describe('claim identity', () => {
  it('keeps duplicate claim names distinct across namespaces', () => {
    const first: ResourceClaimInfo = {
      name: 'shared',
      namespace: 'team-a',
      deviceClassName: 'gpu',
      status: 'Pending',
    };
    const second: ResourceClaimInfo = {
      ...first,
      namespace: 'team-b',
    };

    expect(claimIdentityKey(toClaimIdentity(first))).not.toBe(
      claimIdentityKey(toClaimIdentity(second)),
    );
  });

  it('resolves the selected duplicate by its namespace-qualified key', () => {
    const claims: ResourceClaimInfo[] = [
      {
        name: 'shared',
        namespace: 'team-a',
        deviceClassName: 'gpu',
        status: 'Pending',
      },
      {
        name: 'shared',
        namespace: 'team-b',
        deviceClassName: 'gpu',
        status: 'Pending',
      },
    ];

    expect(
      findClaimIdentityByKey(
        claims,
        claimIdentityKey({ name: 'shared', namespace: 'team-b' }),
      ),
    ).toEqual({ name: 'shared', namespace: 'team-b' });
  });

  it('builds a namespace-qualified identity from a graph claim node', () => {
    const node: GraphNode = {
      id: 'claim/team-b/shared',
      type: 'ResourceClaim',
      label: 'shared',
      metadata: { namespace: 'team-b' },
    };

    expect(claimIdentityFromGraphNode(node)).toEqual({
      name: 'shared',
      namespace: 'team-b',
    });
  });

  it('falls back to the deterministic graph node id when metadata is absent', () => {
    const node: GraphNode = {
      id: 'claim/team-c/claim-a',
      type: 'ResourceClaim',
      label: 'claim-a',
    };

    expect(claimIdentityFromGraphNode(node)).toEqual({
      name: 'claim-a',
      namespace: 'team-c',
    });
  });
});
