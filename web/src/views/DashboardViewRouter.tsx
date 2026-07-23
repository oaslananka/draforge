import type { ReactNode } from 'react';
import type {
  Device,
  DevicePool,
  DoctorCheckResult,
  DoctorReport,
  ExplainResult,
  ReasonNode,
  ResourceClaimInfo,
  ResourceGraph,
  SSEStatus,
  Summary,
  TabId,
} from '../api/types';
import type { ClaimIdentity } from '../claims/identity';
import {
  claimIdentityKey,
  findClaimIdentityByKey,
} from '../claims/identity';
import { claimAllocationLabels, claimClassNames } from '../claims/summary';
import InteractiveGraph from '../components/InteractiveGraph';
import type {
  DashboardErrorState,
  DashboardLoadingState,
} from '../hooks/useDashboardData';

export interface ExplanationViewState {
  readonly result: ExplainResult | null;
  readonly error: string;
  readonly loading: boolean;
}

interface DashboardViewRouterProps {
  readonly activeTab: TabId;
  readonly summary: Summary | null;
  readonly pools: readonly DevicePool[];
  readonly devices: readonly Device[];
  readonly claims: readonly ResourceClaimInfo[];
  readonly doctorReport: DoctorReport | null;
  readonly graphData: ResourceGraph | null;
  readonly sseStatus: SSEStatus;
  readonly loading: DashboardLoadingState;
  readonly errors: DashboardErrorState;
  readonly selectedClaim: ClaimIdentity | null;
  readonly explanation: ExplanationViewState;
  readonly onSelectClaim: (claim: ClaimIdentity | null) => void;
  readonly onGraphClaim: (claim: ClaimIdentity) => void;
}

type LoadingMessageProps = Readonly<{ children: string }>;
type SectionErrorProps = Readonly<{ message?: string }>;
type EmptyStateProps = Readonly<{ title: string; detail: ReactNode }>;
type OverviewViewProps = Readonly<{ summary: Summary | null; loading: boolean }>;
type PoolsViewProps = Readonly<{ pools: readonly DevicePool[]; loading: boolean; error?: string }>;
type DevicesViewProps = Readonly<{ devices: readonly Device[]; loading: boolean; error?: string }>;
type ClaimsViewProps = Readonly<{ claims: readonly ResourceClaimInfo[]; loading: boolean; error?: string }>;
type GraphViewProps = Readonly<{
  graphData: ResourceGraph | null;
  sseStatus: SSEStatus;
  onSelectClaim: (claim: ClaimIdentity) => void;
}>;
type ReasonTreeProps = Readonly<{ node: ReasonNode; depth?: number }>;
type ExplainViewProps = Readonly<{
  claims: readonly ResourceClaimInfo[];
  selectedClaim: ClaimIdentity | null;
  explanation: ExplanationViewState;
  onSelectClaim: (claim: ClaimIdentity | null) => void;
}>;
type DoctorViewProps = Readonly<{ report: DoctorReport | null; loading: boolean; error?: string }>;

function LoadingMessage({ children }: LoadingMessageProps) {
  return (
    <output style={{ display: 'block', textAlign: 'center', padding: '40px', color: 'var(--text-muted)' }}>
      <span className="spinner" style={{ display: 'block', margin: '0 auto 16px' }} />
      <span>{children}</span>
    </output>
  );
}

function SectionError({ message }: SectionErrorProps) {
  if (!message) return null;
  return (
    <div className="glass-panel" style={{ borderColor: 'var(--color-danger)', marginBottom: '20px' }} role="alert">
      <p>{message}</p>
    </div>
  );
}

function EmptyState({ title, detail }: EmptyStateProps) {
  return (
    <div style={{ textAlign: 'center', padding: '60px 20px', background: 'var(--bg-secondary)', borderRadius: '12px', border: '1px dashed var(--border-light)' }}>
      <p style={{ color: 'var(--text-secondary)', marginBottom: '12px' }}>{title}</p>
      <div style={{ fontSize: '0.85rem', color: 'var(--text-muted)' }}>{detail}</div>
    </div>
  );
}

function OverviewView({ summary, loading }: OverviewViewProps) {
  if (loading) return <LoadingMessage>Loading DRAForge summary...</LoadingMessage>;

  const refreshedAt = summary?.timestamp
    ? new Date(summary.timestamp).toLocaleString()
    : 'unknown';
  const cards: ReadonlyArray<readonly [string, number]> = [
    ['Simulated Pools', summary?.poolsCount ?? 0],
    ['Discovered Devices', summary?.devicesCount ?? 0],
    ['Active Claims', summary?.claimsCount ?? 0],
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '30px' }}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: '24px' }}>
        {cards.map(([label, value]) => (
          <div className="glass-panel" key={label}>
            <h3 style={{ color: 'var(--text-muted)', fontSize: '0.9rem', textTransform: 'uppercase' }}>{label}</h3>
            <h2 style={{ fontSize: '2.5rem', marginTop: '10px' }} className="accent-glow-text">{value}</h2>
          </div>
        ))}
      </div>
      <div className="glass-panel">
        <h2 style={{ marginBottom: '15px' }}>Dynamic Resource Allocation Status</h2>
        <p style={{ color: 'var(--text-secondary)' }}>
          DRAForge is reporting live data from the connected Kubernetes API: <strong>{summary?.poolsCount ?? 0}</strong> simulated pools, <strong>{summary?.devicesCount ?? 0}</strong> discovered devices, and <strong>{summary?.claimsCount ?? 0}</strong> active claims.
        </p>
        <p style={{ color: 'var(--text-muted)', fontSize: '0.85rem', marginTop: '10px' }}>
          Last refreshed: {refreshedAt}
        </p>
      </div>
    </div>
  );
}

function poolTable(pools: readonly DevicePool[]) {
  return (
    <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
      <thead><tr style={{ borderBottom: '1px solid var(--border-light)', color: 'var(--text-muted)' }}>
        {['POOL NAME', 'DRIVER', 'NODE', 'DEVICE TYPE', 'STATUS'].map((heading) => <th style={{ padding: '12px' }} key={heading}>{heading}</th>)}
      </tr></thead>
      <tbody>{pools.map((pool) => (
        <tr key={`${pool.driverName}/${pool.name}/${pool.nodeName}`} style={{ borderBottom: '1px solid var(--border-light)' }}>
          <td style={{ padding: '12px' }}><strong>{pool.name}</strong></td>
          <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{pool.driverName}</td>
          <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{pool.nodeName || 'Cluster-scoped'}</td>
          <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{pool.deviceType}</td>
          <td style={{ padding: '12px' }}><span className="badge badge-success">{pool.health}</span></td>
        </tr>
      ))}</tbody>
    </table>
  );
}

function PoolsView({ pools, loading, error }: PoolsViewProps) {
  let content: ReactNode;
  if (loading) {
    content = <LoadingMessage>Loading device pools...</LoadingMessage>;
  } else if (pools.length === 0) {
    content = (
      <EmptyState
        title="No virtual device pools registered in this cluster."
        detail={<>Deploy a simulator scenario to register a pool: <code style={{ background: 'var(--bg-tertiary)', padding: '4px 8px', borderRadius: '4px', color: 'var(--accent-secondary)' }}>kubectl apply -f examples/scenarios/basic-gpu.yaml</code></>}
      />
    );
  } else {
    content = poolTable(pools);
  }

  return (
    <div className="glass-panel">
      <h2 style={{ marginBottom: '20px' }}>Virtual Device Pools</h2>
      <SectionError message={error} />
      {content}
    </div>
  );
}

function deviceTable(devices: readonly Device[]) {
  return (
    <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
      <thead><tr style={{ borderBottom: '1px solid var(--border-light)', color: 'var(--text-muted)' }}>
        {['DEVICE NAME', 'TYPE', 'NODE', 'POOL', 'SYNTHETIC', 'STATUS'].map((heading) => <th style={{ padding: '12px' }} key={heading}>{heading}</th>)}
      </tr></thead>
      <tbody>{devices.map((device) => (
        <tr key={device.id} style={{ borderBottom: '1px solid var(--border-light)' }}>
          <td style={{ padding: '12px' }}><strong>{device.name}</strong></td>
          <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{device.type}</td>
          <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{device.nodeName || 'Cluster-scoped'}</td>
          <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{device.poolName}</td>
          <td style={{ padding: '12px' }}><span className="badge badge-success">{device.isSynthetic ? 'Yes' : 'No'}</span></td>
          <td style={{ padding: '12px' }}><span className="badge badge-success">{device.status}</span></td>
        </tr>
      ))}</tbody>
    </table>
  );
}

function DevicesView({ devices, loading, error }: DevicesViewProps) {
  let content: ReactNode;
  if (loading) {
    content = <LoadingMessage>Loading devices...</LoadingMessage>;
  } else if (devices.length === 0) {
    content = <EmptyState title="No discovered DRA devices detected." detail="Simulated devices will automatically show up here as soon as the node plugin driver publishes ResourceSlice objects." />;
  } else {
    content = deviceTable(devices);
  }

  return (
    <div className="glass-panel">
      <h2 style={{ marginBottom: '20px' }}>Discovered Devices</h2>
      <SectionError message={error} />
      {content}
    </div>
  );
}

function claimTable(claims: readonly ResourceClaimInfo[]) {
  return (
    <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
      <thead><tr style={{ borderBottom: '1px solid var(--border-light)', color: 'var(--text-muted)' }}>
        {['CLAIM NAME', 'NAMESPACE', 'CLASS', 'CONSUMER POD', 'ALLOCATED DEVICE', 'STATUS'].map((heading) => <th style={{ padding: '12px' }} key={heading}>{heading}</th>)}
      </tr></thead>
      <tbody>{claims.map((claim) => (
        <tr key={claimIdentityKey(claim)} style={{ borderBottom: '1px solid var(--border-light)' }}>
          <td style={{ padding: '12px' }}><strong>{claim.name}</strong></td>
          <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{claim.namespace}</td>
          <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{claimClassNames(claim).join(', ') || '-'}</td>
          <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{claim.ownerPodName || 'None'}</td>
          <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{claimAllocationLabels(claim).join('; ') || 'Pending'}</td>
          <td style={{ padding: '12px' }}><span className={`badge ${claim.status === 'Allocated' ? 'badge-success' : 'badge-warning'}`}>{claim.status}</span></td>
        </tr>
      ))}</tbody>
    </table>
  );
}

function ClaimsView({ claims, loading, error }: ClaimsViewProps) {
  let content: ReactNode;
  if (loading) {
    content = <LoadingMessage>Loading claims...</LoadingMessage>;
  } else if (claims.length === 0) {
    content = <EmptyState title="No ResourceClaims found in the current cluster." detail="Deploy a test workload pod requesting a simulated device allocation to trace claims." />;
  } else {
    content = claimTable(claims);
  }

  return (
    <div className="glass-panel">
      <h2 style={{ marginBottom: '20px' }}>Resource Claims</h2>
      <SectionError message={error} />
      {content}
    </div>
  );
}

function streamWarning(status: SSEStatus) {
  if (status === 'connected') return null;
  const message = status === 'reconnecting'
    ? 'Dashboard stream disconnected. Reconnecting...'
    : 'Dashboard stream disconnected.';
  return (
    <output className="glass-panel" style={{ display: 'flex', borderColor: 'var(--color-warning)', marginBottom: '20px', alignItems: 'center', gap: '10px' }}>
      <span className="badge badge-warning">Live Stream</span>
      <span style={{ fontSize: '0.9rem' }}>{message}</span>
    </output>
  );
}

function GraphView({ graphData, sseStatus, onSelectClaim }: GraphViewProps) {
  return (
    <>
      {streamWarning(sseStatus)}
      <div className="glass-panel">
        <h2 style={{ marginBottom: '20px' }}>Resource Relationship Graph</h2>
        <InteractiveGraph graphData={graphData} onSelectClaim={onSelectClaim} />
      </div>
    </>
  );
}

function ReasonTree({ node, depth = 0 }: ReasonTreeProps) {
  const isSuccess = node.confidence === 'confirmed' && !node.message.includes('could not');
  return (
    <div style={{ marginLeft: `${depth * 20}px`, marginTop: '10px', borderLeft: '2px solid var(--border-light)', paddingLeft: '12px' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <span className={`badge ${isSuccess ? 'badge-success' : 'badge-warning'}`}>{node.confidence}</span>
        <strong style={{ fontSize: '0.95rem' }}>{node.message}</strong>
      </div>
      <p style={{ color: 'var(--text-secondary)', fontSize: '0.85rem', marginTop: '4px' }}>
        {node.evidence} <span style={{ color: 'var(--text-muted)' }}>({node.sourceType})</span>
      </p>
      {node.children?.map((child, index) => <ReasonTree key={`${child.message}-${index}`} node={child} depth={depth + 1} />)}
    </div>
  );
}

function explanationContent(explanation: ExplanationViewState) {
  if (explanation.loading) return <LoadingMessage>Diagnosing allocation...</LoadingMessage>;
  if (explanation.error) return <SectionError message={explanation.error} />;
  if (!explanation.result) {
    return <div style={{ textAlign: 'center', padding: '30px', color: 'var(--text-muted)' }}><p>Select a claim above to diagnose its allocation status.</p></div>;
  }

  const result = explanation.result;
  const allocationClass = result.allocated ? 'badge-success' : 'badge-warning';
  const allocationLabel = result.allocated ? 'Allocated' : 'Not Allocated';
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
      <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
        <span className={`badge ${allocationClass}`}>{allocationLabel}</span>
        <span style={{ color: 'var(--text-secondary)', fontSize: '0.9rem' }}>{result.targetType}: <strong>{result.targetName}</strong></span>
      </div>
      <div className="glass-panel" style={{ background: 'var(--bg-secondary)' }}>
        <h3>Reason Tree Diagnostic</h3>
        <ReasonTree node={result.reasonTree} />
      </div>
      {result.remedy.length > 0 && (
        <div className="glass-panel" style={{ borderColor: 'var(--color-warning)' }}>
          <h3 style={{ color: 'var(--color-warning)', marginBottom: '10px' }}>Suggested Remediation Steps</h3>
          <ol>{result.remedy.map((remedy) => <li key={remedy} style={{ marginLeft: '20px', marginTop: '6px' }}>{remedy}</li>)}</ol>
        </div>
      )}
    </div>
  );
}

function ExplainView({ claims, selectedClaim, explanation, onSelectClaim }: ExplainViewProps) {
  return (
    <div className="glass-panel">
      <h2 style={{ marginBottom: '20px' }}>Allocation Explanation Engine</h2>
      <div style={{ marginBottom: '20px', display: 'flex', gap: '15px', alignItems: 'center' }}>
        <label htmlFor="claim-select">Select ResourceClaim to diagnose:</label>
        <select
          id="claim-select"
          value={selectedClaim ? claimIdentityKey(selectedClaim) : ''}
          onChange={(event) => onSelectClaim(findClaimIdentityByKey(claims, event.target.value))}
          style={{ background: 'var(--bg-secondary)', color: 'white', border: '1px solid var(--border-light)', padding: '8px 16px', borderRadius: '6px' }}
        >
          <option value="">-- Choose Claim --</option>
          {claims.map((claim) => {
            const identity = claimIdentityKey(claim);
            return <option key={identity} value={identity}>{claim.namespace}/{claim.name} ({claim.status})</option>;
          })}
        </select>
      </div>
      {explanationContent(explanation)}
    </div>
  );
}

function doctorStatusClass(status: DoctorCheckResult['status']) {
  if (status === 'PASS') return 'badge-success';
  if (status === 'WARN') return 'badge-warning';
  return 'badge-danger';
}

function doctorReport(report: DoctorReport) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
      <div style={{ display: 'flex', gap: '15px' }}>
        <span className="badge badge-success">PASS: {report.summary.PASS ?? 0}</span>
        <span className="badge badge-warning">WARN: {report.summary.WARN ?? 0}</span>
        <span className="badge badge-danger">FAIL: {report.summary.FAIL ?? 0}</span>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: '15px' }}>
        {report.results.map((result) => (
          <article key={result.id} className="glass-panel" style={{ background: 'var(--bg-secondary)', padding: '16px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3 style={{ fontSize: '1.1rem' }}>{result.name}</h3>
              <span className={`badge ${doctorStatusClass(result.status)}`}>{result.status}</span>
            </div>
            <p style={{ marginTop: '8px', color: 'var(--text-secondary)' }}>{result.evidence}</p>
            {result.status !== 'PASS' && (
              <div style={{ marginTop: '10px', padding: '10px', background: 'var(--bg-tertiary)', borderRadius: '6px', fontSize: '0.9rem' }}>
                <strong style={{ color: 'var(--color-warning)' }}>Remediation:</strong> {result.remediation}
              </div>
            )}
          </article>
        ))}
      </div>
    </div>
  );
}

function DoctorView({ report, loading, error }: DoctorViewProps) {
  let content: ReactNode;
  if (loading) {
    content = <LoadingMessage>Running diagnostics...</LoadingMessage>;
  } else if (report) {
    content = doctorReport(report);
  } else {
    content = <EmptyState title="Unable to load DRAForge diagnostics." detail="Retry after the Kubernetes API becomes available." />;
  }

  return (
    <div className="glass-panel">
      <h2 style={{ marginBottom: '20px' }}>Cluster Diagnostics (Doctor)</h2>
      <SectionError message={error} />
      {content}
    </div>
  );
}

export default function DashboardViewRouter(props: Readonly<DashboardViewRouterProps>) {
  switch (props.activeTab) {
    case 'overview':
      return <OverviewView summary={props.summary} loading={props.loading.summary} />;
    case 'pools':
      return <PoolsView pools={props.pools} loading={props.loading.pools} error={props.errors.pools} />;
    case 'devices':
      return <DevicesView devices={props.devices} loading={props.loading.devices} error={props.errors.devices} />;
    case 'claims':
      return <ClaimsView claims={props.claims} loading={props.loading.claims} error={props.errors.claims} />;
    case 'graph':
      return <GraphView graphData={props.graphData} sseStatus={props.sseStatus} onSelectClaim={props.onGraphClaim} />;
    case 'explain':
      return <ExplainView claims={props.claims} selectedClaim={props.selectedClaim} explanation={props.explanation} onSelectClaim={props.onSelectClaim} />;
    case 'doctor':
      return <DoctorView report={props.doctorReport} loading={props.loading.doctor} error={props.errors.doctor} />;
  }
}
