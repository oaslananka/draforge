import { useEffect, useState } from 'react';
import { fetchExplain } from '../api/api';
import type { ExplainResult } from '../api/types';
import type { ClaimIdentity } from '../claims/identity';

export interface ClaimExplanationState {
  result: ExplainResult | null;
  error: string;
  loading: boolean;
}

export default function useClaimExplanation(
  selectedClaim: ClaimIdentity | null,
): ClaimExplanationState {
  const [result, setResult] = useState<ExplainResult | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!selectedClaim) {
      setResult(null);
      setError('');
      setLoading(false);
      return undefined;
    }

    let active = true;
    setLoading(true);
    setError('');

    void fetchExplain(selectedClaim.name, selectedClaim.namespace)
      .then((value) => {
        if (active) setResult(value);
      })
      .catch((requestError: unknown) => {
        if (!active) return;
        const message = requestError instanceof Error && requestError.message
          ? requestError.message
          : 'Failed to fetch explanation tree.';
        setError(message);
        setResult(null);
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
    };
  }, [selectedClaim?.name, selectedClaim?.namespace]);

  return { result, error, loading };
}
