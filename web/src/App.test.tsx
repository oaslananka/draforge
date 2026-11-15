import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App';
import type {
  DoctorReport,
  ResourceGraph,
  ExplainResult,
  ResourceClaimInfo,
  SSEStatus,
  Summary,
  VersionInfo,
} from './api/types';
import {
  fetchClaims,
  fetchDevices,
  fetchDoctor,
  fetchExplain,
  fetchPools,
  fetchSummary,
  fetchVersion,
} from './api/api';
import useSSE from './hooks/useSSE';

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
  default: vi.fn(),
}));

vi.mock('./components/InteractiveGraph', () => ({
  default: ({ onSelectClaim }: { onSelectClaim: (claim: { name: string; namespace: string }) => void }) => (
    <button type="button" onClick={() => onSelectClaim({ name: 'shared', namespace: 'team-b' })}>
      Open team-b/shared from graph
    </button>
  ),
}));

const mockFetchSummary = vi.mocked(fetchSummary);
const mockFetchVersion = vi.mocked(fetchVersion);
const mockFetchPools = vi.mocked(fetchPools);
const mockFetchDevices = vi.mocked(fetchDevices);
const mockFetchClaims = vi.mocked(fetchClaims);
const mockFetchDoctor = vi.mocked(fetchDoctor);
const mockFetchExplain = vi.mocked(fetchExplain);
const mockUseSSE = vi.mocked(useSSE);

const summary: Summary = {
  poolsCount: 0,
  devicesCount: 0,
  claimsCount: 2,
  doctorStatus: { PASS: 1, WARN: 0, FAIL: 0 },
  timestamp: '2026-07-23T12:00:00Z',
};

const version: VersionInfo = { version: 'v0.4.0', commit: 'test' };

const duplicateClaims: ResourceClaimInfo[] = [
  {
    name: 'shared',
    namespace: 'team-a',
    status: 'Pending',
    requests: [],
    allocations: [],
  },
  {
    name: 'shared',
    namespace: 'team-b',
    status: 'Allocated',
    requests: [],
    allocations: [],
  },
];

const doctorReport: DoctorReport = {
  timestamp: '2026-07-23T12:00:00Z',
  summary: { PASS: 1, WARN: 0, FAIL: 1 },
  results: [
    {
      id: 'DRA-001',
      name: 'Resource API availability',
      category: 'cluster',
      status: 'FAIL',
      severity: 'high',
      evidence: 'ResourceClaims API is unavailable.',
      remediation: 'Enable the resource.k8s.io/v1 API.',
      docReference: 'docs/operations/troubleshooting.md',
    },
  ],
};

const explainResult: ExplainResult = {
  targetName: 'shared',
  targetType: 'claim',
  allocated: false,
  reasonTree: {
    message: 'Claim could not be allocated.',
    confidence: 'confirmed',
    evidence: 'No matching device.',
    sourceType: 'ResourceClaim',
  },
  remedy: ['Create a matching DeviceClass.'],
};

function configureSuccessfulAPIs(status: SSEStatus = 'connected') {
  mockUseSSE.mockReturnValue(status);
  mockFetchSummary.mockResolvedValue(summary);
  mockFetchVersion.mockResolvedValue(version);
  mockFetchPools.mockResolvedValue([]);
  mockFetchDevices.mockResolvedValue([]);
  mockFetchClaims.mockResolvedValue(duplicateClaims);
  mockFetchDoctor.mockResolvedValue(doctorReport);
  mockFetchExplain.mockResolvedValue(explainResult);
}

describe('App critical dashboard flows', () => {
  beforeEach(() => {
    configureSuccessfulAPIs();
  });

  it('keeps duplicate claim names namespace-qualified for explanation requests', async () => {
    const user = userEvent.setup();
    render(<App />);

    await screen.findByRole('button', { name: 'Explain a claim' });
    await user.click(screen.getByRole('button', { name: 'Explain a claim' }));

    const selector = screen.getByRole('combobox', { name: /Select ResourceClaim/i });
    expect(screen.getByRole('option', { name: 'team-a/shared (Pending)' }).isConnected).toBe(true);
    expect(screen.getByRole('option', { name: 'team-b/shared (Allocated)' }).isConnected).toBe(true);

    await user.selectOptions(selector, 'team-b/shared');

    await waitFor(() => {
      expect(mockFetchExplain).toHaveBeenLastCalledWith('shared', 'team-b');
    });
  });

  it('navigates from a graph claim to the namespace-qualified explanation', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole('button', { name: 'Relationships' }));
    await user.click(screen.getByRole('button', { name: 'Open team-b/shared from graph' }));

    expect((await screen.findByRole('heading', { name: 'Allocation Explanation Engine' })).isConnected).toBe(true);
    await waitFor(() => {
      expect(mockFetchExplain).toHaveBeenLastCalledWith('shared', 'team-b');
    });
  });

  it('shows reconnecting stream state without blocking graph navigation', async () => {
    configureSuccessfulAPIs('reconnecting');
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole('button', { name: 'Relationships' }));

    expect(screen.getByText('Reconnecting...').isConnected).toBe(true);
    expect(screen.getByText('Dashboard stream disconnected. Reconnecting...').isConnected).toBe(true);
    expect((screen.getByRole('button', { name: 'Open team-b/shared from graph' }) as HTMLButtonElement).disabled).toBe(false);
  });

  it('keeps unaffected empty views usable when one API request fails', async () => {
    mockFetchSummary.mockRejectedValue(new Error('summary unavailable'));
    mockFetchClaims.mockResolvedValue([]);
    const user = userEvent.setup();
    render(<App />);

    expect((await screen.findByText('summary unavailable')).isConnected).toBe(true);
    await user.click(screen.getByRole('button', { name: 'Claims' }));
    expect(screen.getByText('No ResourceClaims found in the current cluster.').isConnected).toBe(true);
  });

  it('renders diagnostics through an accessible navigation control', async () => {
    const user = userEvent.setup();
    render(<App />);

    const doctorButton = await screen.findByRole('button', { name: /Doctor: 1 Failure/i });
    await user.click(doctorButton);

    expect(screen.getByRole('heading', { name: 'Cluster Diagnostics (Doctor)' }).isConnected).toBe(true);
    expect(screen.getByText('ResourceClaims API is unavailable.').isConnected).toBe(true);
    expect(screen.getByText(/Enable the resource.k8s.io\/v1 API/).isConnected).toBe(true);
  });

  it('refreshes claim status and selected explanation after a changed live graph, without polling unchanged frames', async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByRole('button', { name: 'Claims' });
    await waitFor(() => {
      expect(mockFetchClaims).toHaveBeenCalledTimes(1);
      expect(mockFetchExplain).toHaveBeenCalledWith('shared', 'team-a');
    });

    const pendingGraph: ResourceGraph = {
      nodes: [{ id: 'claim/team-a/shared', type: 'ResourceClaim', label: 'shared', metadata: { namespace: 'team-a', status: 'Pending' } }],
      edges: [],
    };
    const onGraph = () => mockUseSSE.mock.lastCall?.[0];
    const sendGraph = (graph: ResourceGraph) => {
      act(() => {
        const callback = onGraph();
        if (!callback) throw new Error('SSE handler not installed');
        callback(graph);
      });
    };
    sendGraph(pendingGraph);
    sendGraph({ ...pendingGraph, nodes: [...pendingGraph.nodes] });
    expect(mockFetchClaims).toHaveBeenCalledTimes(1);

    const recoveredClaims: ResourceClaimInfo[] = [
      { ...duplicateClaims[0], status: 'Allocated', allocations: [{
        request: 'gpu',
        driverName: 'driver.example',
        poolName: 'recovery-pool',
        deviceName: 'recovered-gpu',
        nodeName: 'worker-a',
      }] },
      duplicateClaims[1],
    ];
    mockFetchClaims.mockResolvedValue(recoveredClaims);
    mockFetchExplain.mockResolvedValue({
      ...explainResult,
      allocated: true,
      reasonTree: { ...explainResult.reasonTree, message: 'Claim is allocated.', evidence: 'Recovered from pending state.' },
      remedy: [],
    });

    const recoveredGraph: ResourceGraph = {
      nodes: [{ ...pendingGraph.nodes[0], metadata: { namespace: 'team-a', status: 'Allocated', allocationCount: 1 } }],
      edges: [],
    };
    sendGraph(recoveredGraph);
    await waitFor(() => {
      expect(mockFetchClaims).toHaveBeenCalledTimes(2);
      expect(mockFetchExplain).toHaveBeenCalledTimes(2);
    });
    await user.click(screen.getByRole('button', { name: 'Claims' }));
    const recoveredRow = screen.getByRole('row', { name: /shared team-a/ });
    expect(within(recoveredRow).getByText('Allocated')).toBeTruthy();
    expect(within(recoveredRow).getByText(/recovery-pool/)).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Explain a claim' }));
    expect(screen.getByText('Claim is allocated.')).toBeTruthy();
    const claimReads = mockFetchClaims.mock.calls.length;
    const explainReads = mockFetchExplain.mock.calls.length;
    sendGraph({ ...recoveredGraph, nodes: [...recoveredGraph.nodes] });
    expect(mockFetchClaims).toHaveBeenCalledTimes(claimReads);
    expect(mockFetchExplain).toHaveBeenCalledTimes(explainReads);
  });

  it('clears a transient section error when the live resource graph changes and the API recovers', async () => {
    mockFetchSummary.mockRejectedValueOnce(new Error('summary temporarily unavailable'));
    render(<App />);
    expect(await screen.findByText('summary temporarily unavailable')).toBeTruthy();

    const sendGraph = (status: string) => {
      act(() => {
        const callback = mockUseSSE.mock.lastCall?.[0];
        if (!callback) throw new Error('SSE handler not installed');
        callback({ nodes: [{ id: 'claim/team-a/shared', type: 'ResourceClaim', label: 'shared', metadata: { status } }], edges: [] });
      });
    };
    sendGraph('Pending');
    sendGraph('Allocated');
    await waitFor(() => {
      expect(mockFetchSummary).toHaveBeenCalledTimes(2);
      expect(screen.queryByText('summary temporarily unavailable')).toBeNull();
    });
  });

  it('announces the live stream state through a status role', async () => {
    render(<App />);

    const status = await screen.findByRole('status', { name: 'Live stream status' });
    expect(status.textContent).toContain('Stream Connected');
  });

  it('marks the active tab for assistive technology', async () => {
    const user = userEvent.setup();
    render(<App />);

    const poolsTab = await screen.findByRole('button', { name: 'Pools' });
    await user.click(poolsTab);

    expect(poolsTab.getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByText('Inspect simulated pools and the nodes advertising them.').isConnected).toBe(true);
    expect(screen.getByRole('button', { name: 'Overview' }).getAttribute('aria-pressed')).toBe('false');
  });
});
