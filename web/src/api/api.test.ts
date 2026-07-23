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


describe('API error handling', () => {
  it('throws an Error instance for structured API failures', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 403,
      statusText: 'Forbidden',
      json: async () => ({ error: { message: 'access denied', code: 'forbidden' } }),
    } as Response));

    await expect(fetchExplain('claim-a', 'default')).rejects.toMatchObject({
      name: 'ApiRequestError',
      message: 'access denied',
      code: 'forbidden',
    });
  });

  it('throws an Error instance when a success response is not JSON', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => { throw new SyntaxError('invalid JSON'); },
    } as unknown as Response));

    await expect(fetchExplain('claim-a', 'default')).rejects.toMatchObject({
      name: 'ApiRequestError',
      message: 'Failed to parse API response as JSON',
    });
  });
});
