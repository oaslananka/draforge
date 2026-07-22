import { afterEach, describe, expect, expectTypeOf, it, vi } from 'vitest';
import type { ExplainResult } from './types';
import { fetchExplain } from './api';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchExplain', () => {
  it('requires an explicit namespace in its public contract', () => {
    expectTypeOf(fetchExplain).toEqualTypeOf<
      (claim: string, namespace: string) => Promise<ExplainResult>
    >();
  });

  it('URL-encodes both claim and namespace values', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        targetName: 'gpu claim/1',
        targetType: 'claim',
        allocated: false,
        reasonTree: {
          message: 'pending',
          confidence: 'confirmed',
          evidence: 'test',
          sourceType: 'ResourceClaim',
        },
        remedy: [],
      }),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await fetchExplain('gpu claim/1', 'team blue/edge');

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/explain?claim=gpu%20claim%2F1&namespace=team%20blue%2Fedge',
    );
  });
});
