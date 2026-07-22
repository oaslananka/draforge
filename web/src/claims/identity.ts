import type { GraphNode, ResourceClaimInfo } from '../api/types';

export interface ClaimIdentity {
  name: string;
  namespace: string;
}

export function toClaimIdentity(
  claim: Pick<ResourceClaimInfo, 'name' | 'namespace'>,
): ClaimIdentity {
  return {
    name: claim.name,
    namespace: claim.namespace,
  };
}

export function claimIdentityKey(claim: ClaimIdentity): string {
  return `${encodeURIComponent(claim.namespace)}/${encodeURIComponent(claim.name)}`;
}

export function findClaimIdentityByKey(
  claims: ReadonlyArray<Pick<ResourceClaimInfo, 'name' | 'namespace'>>,
  key: string,
): ClaimIdentity | null {
  const claim = claims.find(candidate => claimIdentityKey(candidate) === key);
  return claim ? toClaimIdentity(claim) : null;
}

export function claimIdentityFromGraphNode(
  node: GraphNode,
): ClaimIdentity | null {
  if (node.type !== 'ResourceClaim') {
    return null;
  }

  const metadataNamespace = node.metadata?.namespace;
  if (typeof metadataNamespace === 'string' && metadataNamespace !== '') {
    return {
      name: node.label,
      namespace: metadataNamespace,
    };
  }

  const prefix = 'claim/';
  if (!node.id.startsWith(prefix)) {
    return null;
  }
  const qualifiedName = node.id.slice(prefix.length);
  const separator = qualifiedName.indexOf('/');
  if (separator <= 0 || separator === qualifiedName.length - 1) {
    return null;
  }

  return {
    namespace: qualifiedName.slice(0, separator),
    name: qualifiedName.slice(separator + 1),
  };
}
