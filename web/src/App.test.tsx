import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type {
  DoctorReport,
  ExplainResult,
  ResourceClaimInfo,
  Summary,
  VersionInfo,
} from './api/types';
import { claimIdentityKey } from './claims/identity';
import App from './App';
import {
  fetchClaims,
  fetchDevices,
  fetchDoctor,
  fetchExplain,
  fetchPools,
  fetchSummary,
  fetchVersion,
} from './api/api';

vi.mock('./api/api', () => ({
  fetchSummary: vi.fn(),
  fetchVersion: vi.fn(),
  fetchPools: vi.fn(),
  fetchDevices: vi.fn(),
  fetchClaims: vi.fn(),
  fetchDoctor: vi.fn(),
  fetchExplain: vi.fn(),
}));

vi.mock('./hooks/useSSE', () => ({
  default: () => 'connected',
}));

vi.mock('./components/InteractiveGraph', () => ({
  default: ({
    onSelectClaim,
  }: {
    onSelectClaim: (claim: { name: string; namespace: string }) => void;
  }) => (
    <button
      type="button"
      onClick={() => onSelectClaim({ name: 'shared', namespace: 'team-a' })}
    >
      Open team-a claim from graph
    </button>
  ),
}));

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

const summary: Summary = {
  poolsCount: 0,
  devicesCount: 0,
  claimsCount: claims.length,
  doctorStatus: {},
  timestamp: '2026-07-23T00:00:00Z',
};

const version: VersionInfo = {
  version: 'test',
  commit: 'abc123',
};

const doctorReport: DoctorReport = {
  timestamp: '2026-07-23T00:00:00Z',
  summary: { PASS: 1, WARN: 0, FAIL: 0 },
  results: [],
};

const explanation: ExplainResult = {
  targetName: 'shared',
  targetType: 'claim',
  allocated: false,
  reasonTree: {
    message: 'Claim could not be allocated.',
    confidence: 'confirmed',
    evidence: 'Pending',
    sourceType: 'ResourceClaim',
  },
  remedy: [],
};

beforeEach(() => {
  vi.mocked(fetchSummary).mockResolvedValue(summary);
  vi.mocked(fetchVersion).mockResolvedValue(version);
  vi.mocked(fetchPools).mockResolvedValue([]);
  vi.mocked(fetchDevices).mockResolvedValue([]);
  vi.mocked(fetchClaims).mockResolvedValue(claims);
  vi.mocked(fetchDoctor).mockResolvedValue(doctorReport);
  vi.mocked(fetchExplain).mockResolvedValue(explanation);
});

describe('namespace-qualified claim selection', () => {
  it('keeps duplicate names distinct and uses the selected namespace', async () => {
    const user = userEvent.setup();
    render(<App />);

    await waitFor(() => {
      expect(fetchExplain).toHaveBeenCalledWith('shared', 'team-a');
    });

    await user.click(screen.getByRole('button', { name: 'CLAIMS' }));
    expect(screen.getAllByText('shared')).toHaveLength(2);
    expect(screen.getByText('team-a')).toBeTruthy();
    expect(screen.getByText('team-b')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'EXPLAIN' }));
    const selector = screen.getByLabelText('Select ResourceClaim to diagnose:');
    expect(screen.getByRole('option', { name: 'team-a/shared (Pending)' })).toBeTruthy();
    expect(screen.getByRole('option', { name: 'team-b/shared (Pending)' })).toBeTruthy();

    vi.mocked(fetchExplain).mockClear();
    await user.selectOptions(
      selector,
      claimIdentityKey({ name: 'shared', namespace: 'team-b' }),
    );
    await waitFor(() => {
      expect(fetchExplain).toHaveBeenCalledWith('shared', 'team-b');
    });

    vi.mocked(fetchExplain).mockClear();
    await user.click(screen.getByRole('button', { name: 'GRAPH' }));
    await user.click(
      screen.getByRole('button', { name: 'Open team-a claim from graph' }),
    );
    await waitFor(() => {
      expect(fetchExplain).toHaveBeenCalledWith('shared', 'team-a');
    });
    expect(
      (screen.getByLabelText('Select ResourceClaim to diagnose:') as HTMLSelectElement)
        .value,
    ).toBe(claimIdentityKey({ name: 'shared', namespace: 'team-a' }));
  });
});
