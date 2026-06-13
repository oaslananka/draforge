// web/src/App.tsx
// SPDX-License-Identifier: Apache-2.0
import { useState, useEffect } from 'react';

const versionVal = 'v0.1.0-rc.1';

// Types matching Go model structs
interface Device {
  id: string;
  name: string;
  type: string;
  status: string;
  nodeName: string;
  poolName: string;
  attributes: Record<string, string>;
  capacities: Record<string, number>;
  isSynthetic: boolean;
}

interface DevicePool {
  name: string;
  driverName: string;
  nodeName: string;
  deviceCount: number;
  deviceType: string;
  health: string;
  isSynthetic: boolean;
}

interface ResourceClaimInfo {
  name: string;
  namespace: string;
  deviceClassName: string;
  status: string;
  ownerPodName: string;
  allocatedDevice?: string;
  allocatedNode?: string;
}

interface DoctorCheckResult {
  id: string;
  name: string;
  category: string;
  status: string;
  severity: string;
  evidence: string;
  remediation: string;
  docReference: string;
}

interface DoctorReport {
  timestamp: string;
  summary: Record<string, number>;
  results: DoctorCheckResult[];
}

interface GraphNode {
  id: string;
  type: string;
  label: string;
  metadata?: Record<string, any>;
}

interface GraphEdge {
  from: string;
  to: string;
  type: string;
}

interface ResourceGraph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

interface ReasonNode {
  message: string;
  confidence: string;
  evidence: string;
  sourceType: string;
  children?: ReasonNode[];
}

interface ExplainResult {
  targetName: string;
  targetType: string;
  allocated: boolean;
  reasonTree: ReasonNode;
  remedy: string[];
}

export default function App() {
  const [activeTab, setActiveTab] = useState<'overview' | 'pools' | 'devices' | 'claims' | 'graph' | 'explain' | 'doctor'>('overview');
  const [summary, setSummary] = useState<any>(null);
  const [pools, setPools] = useState<DevicePool[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [claims, setClaims] = useState<ResourceClaimInfo[]>([]);
  const [doctorReport, setDoctorReport] = useState<DoctorReport | null>(null);
  const [graphData, setGraphData] = useState<ResourceGraph | null>(null);
  
  // Explain States
  const [selectedClaim, setSelectedClaim] = useState<string>('');
  const [explainResult, setExplainResult] = useState<ExplainResult | null>(null);
  const [explainError, setExplainError] = useState<string>('');

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  // Fetch basic data
  const fetchData = async () => {
    try {
      const summaryRes = await fetch('/api/summary');
      const summaryData = await summaryRes.json();
      setSummary(summaryData);

      const poolsRes = await fetch('/api/pools');
      setPools(await poolsRes.json());

      const devicesRes = await fetch('/api/devices');
      setDevices(await devicesRes.json());

      const claimsRes = await fetch('/api/claims');
      const claimsData = await claimsRes.json();
      setClaims(claimsData);
      if (claimsData.length > 0 && !selectedClaim) {
        setSelectedClaim(claimsData[0].name);
      }

      const doctorRes = await fetch('/api/doctor');
      setDoctorReport(await doctorRes.json());

      setError('');
    } catch (err: any) {
      setError(err.message || 'Failed to fetch cluster data');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    
    // Server-Sent Events for Live Graph Updates
    const eventSource = new EventSource('/api/stream');
    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        setGraphData(data);
      } catch (e) {
        console.error('Failed to parse SSE graph stream:', e);
      }
    };

    eventSource.onerror = (err) => {
      console.warn('SSE stream lost connection, attempting reconnect...', err);
    };

    return () => {
      eventSource.close();
    };
  }, []);

  // Fetch claim explanation
  const handleExplain = async (claimName: string) => {
    if (!claimName) return;
    try {
      setExplainError('');
      const res = await fetch(`/api/explain?claim=${claimName}&namespace=default`);
      const data = await res.json();
      if (data.error) {
        setExplainError(data.error);
        setExplainResult(null);
      } else {
        setExplainResult(data);
      }
    } catch (err: any) {
      setExplainError('Failed to fetch explanation tree.');
      setExplainResult(null);
    }
  };

  useEffect(() => {
    if (selectedClaim) {
      handleExplain(selectedClaim);
    }
  }, [selectedClaim]);

  // Recursively render explain reason tree
  const renderReasonNode = (node: ReasonNode, depth = 0) => {
    const isSuccess = node.confidence === 'confirmed' && !node.message.includes('could not');
    const badgeClass = isSuccess ? 'badge-success' : 'badge-warning';
    
    return (
      <div key={node.message} style={{ marginLeft: `${depth * 20}px`, marginTop: '10px', borderLeft: '2px solid var(--border-light)', paddingLeft: '12px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <span className={`badge ${badgeClass}`}>{node.confidence}</span>
          <strong style={{ fontSize: '0.95rem' }}>{node.message}</strong>
        </div>
        <p style={{ color: 'var(--text-secondary)', fontSize: '0.85rem', marginTop: '4px' }}>
          {node.evidence} <span style={{ color: 'var(--text-muted)' }}>({node.sourceType})</span>
        </p>
        {node.children && node.children.map(child => renderReasonNode(child, depth + 1))}
      </div>
    );
  };

  if (loading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: '20px' }}>
        <div className="spinner"></div>
        <h2 className="glow-text">Loading DRAForge Platform...</h2>
      </div>
    );
  }

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      {/* Header */}
      <header style={{ padding: '20px 40px', background: 'rgba(12, 15, 23, 0.8)', borderBottom: '1px solid var(--border-light)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', backdropFilter: 'blur(8px)' }}>
        <div>
          <h1 className="glow-text" style={{ fontSize: '1.8rem', fontFamily: 'var(--font-title)' }}>DRAForge</h1>
          <p style={{ color: 'var(--text-muted)', fontSize: '0.85rem' }}>Dynamic Resource Allocation Developer Platform</p>
        </div>
        <nav style={{ display: 'flex', gap: '15px' }}>
          {(['overview', 'pools', 'devices', 'claims', 'graph', 'explain', 'doctor'] as const).map(tab => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              style={{
                background: activeTab === tab ? 'var(--accent-primary)' : 'transparent',
                color: 'white',
                padding: '8px 16px',
                borderRadius: '6px',
                fontWeight: 600,
                border: activeTab === tab ? 'none' : '1px solid var(--border-light)',
              }}
            >
              {tab.toUpperCase()}
            </button>
          ))}
        </nav>
      </header>

      {/* Main Body */}
      <main style={{ flex: 1, padding: '40px' }}>
        {error && (
          <div className="glass-panel" style={{ borderColor: 'var(--color-danger)', marginBottom: '30px', display: 'flex', alignItems: 'center', gap: '15px' }}>
            <span className="badge badge-danger">Connection Error</span>
            <p>{error}</p>
          </div>
        )}

        {/* Tab content switch */}
        {activeTab === 'overview' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '30px' }}>
            {/* Counts Grid */}
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: '24px' }}>
              <div className="glass-panel">
                <h3 style={{ color: 'var(--text-muted)', fontSize: '0.9rem', textTransform: 'uppercase' }}>Simulated Pools</h3>
                <h2 style={{ fontSize: '2.5rem', marginTop: '10px' }} className="accent-glow-text">{summary?.poolsCount ?? 0}</h2>
              </div>
              <div className="glass-panel">
                <h3 style={{ color: 'var(--text-muted)', fontSize: '0.9rem', textTransform: 'uppercase' }}>Discovered Devices</h3>
                <h2 style={{ fontSize: '2.5rem', marginTop: '10px' }} className="accent-glow-text">{summary?.devicesCount ?? 0}</h2>
              </div>
              <div className="glass-panel">
                <h3 style={{ color: 'var(--text-muted)', fontSize: '0.9rem', textTransform: 'uppercase' }}>Active Claims</h3>
                <h2 style={{ fontSize: '2.5rem', marginTop: '10px' }} className="accent-glow-text">{summary?.claimsCount ?? 0}</h2>
              </div>
            </div>

            {/* Quick overview detail */}
            <div className="glass-panel">
              <h2 style={{ marginBottom: '15px' }}>Dynamic Resource Allocation Status</h2>
              <p style={{ color: 'var(--text-secondary)' }}>
                DRAForge is running on DOKS in region <strong>fra1</strong>. Both worker nodes are healthy. All devices below are simulated synthetic devices, clearly labeled as mock hardware.
              </p>
            </div>
          </div>
        )}

        {activeTab === 'pools' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Virtual Device Pools</h2>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border-light)', color: 'var(--text-muted)' }}>
                  <th style={{ padding: '12px' }}>POOL NAME</th>
                  <th style={{ padding: '12px' }}>DRIVER</th>
                  <th style={{ padding: '12px' }}>NODE</th>
                  <th style={{ padding: '12px' }}>DEVICE TYPE</th>
                  <th style={{ padding: '12px' }}>STATUS</th>
                </tr>
              </thead>
              <tbody>
                {pools.map(pool => (
                  <tr key={pool.name + pool.nodeName} style={{ borderBottom: '1px solid var(--border-light)' }}>
                    <td style={{ padding: '12px' }}><strong>{pool.name}</strong></td>
                    <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{pool.driverName}</td>
                    <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{pool.nodeName}</td>
                    <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{pool.deviceType}</td>
                    <td style={{ padding: '12px' }}>
                      <span className="badge badge-success">{pool.health}</span>
                    </td>
                  </tr>
                ))}
                {pools.length === 0 && (
                  <tr>
                    <td colSpan={5} style={{ padding: '24px', textAlign: 'center', color: 'var(--text-muted)' }}>No active device pools found.</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        )}

        {activeTab === 'devices' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Discovered Devices</h2>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border-light)', color: 'var(--text-muted)' }}>
                  <th style={{ padding: '12px' }}>DEVICE NAME</th>
                  <th style={{ padding: '12px' }}>TYPE</th>
                  <th style={{ padding: '12px' }}>NODE</th>
                  <th style={{ padding: '12px' }}>POOL</th>
                  <th style={{ padding: '12px' }}>SYNTHETIC</th>
                  <th style={{ padding: '12px' }}>STATUS</th>
                </tr>
              </thead>
              <tbody>
                {devices.map(dev => (
                  <tr key={dev.id} style={{ borderBottom: '1px solid var(--border-light)' }}>
                    <td style={{ padding: '12px' }}><strong>{dev.name}</strong></td>
                    <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{dev.type}</td>
                    <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{dev.nodeName}</td>
                    <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{dev.poolName}</td>
                    <td style={{ padding: '12px' }}>
                      <span className="badge badge-success">{dev.isSynthetic ? 'Yes' : 'No'}</span>
                    </td>
                    <td style={{ padding: '12px' }}>
                      <span className="badge badge-success">{dev.status}</span>
                    </td>
                  </tr>
                ))}
                {devices.length === 0 && (
                  <tr>
                    <td colSpan={6} style={{ padding: '24px', textAlign: 'center', color: 'var(--text-muted)' }}>No discovered devices. Publish a SimulatedDevicePool scenario.</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        )}

        {activeTab === 'claims' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Resource Claims</h2>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border-light)', color: 'var(--text-muted)' }}>
                  <th style={{ padding: '12px' }}>CLAIM NAME</th>
                  <th style={{ padding: '12px' }}>NAMESPACE</th>
                  <th style={{ padding: '12px' }}>CLASS</th>
                  <th style={{ padding: '12px' }}>CONSUMER POD</th>
                  <th style={{ padding: '12px' }}>ALLOCATED DEVICE</th>
                  <th style={{ padding: '12px' }}>STATUS</th>
                </tr>
              </thead>
              <tbody>
                {claims.map(claim => (
                  <tr key={claim.name} style={{ borderBottom: '1px solid var(--border-light)' }}>
                    <td style={{ padding: '12px' }}><strong>{claim.name}</strong></td>
                    <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{claim.namespace}</td>
                    <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{claim.deviceClassName}</td>
                    <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{claim.ownerPodName || 'None'}</td>
                    <td style={{ padding: '12px', color: 'var(--text-secondary)' }}>{claim.allocatedDevice || 'Pending'}</td>
                    <td style={{ padding: '12px' }}>
                      <span className={`badge ${claim.status === 'Allocated' ? 'badge-success' : 'badge-warning'}`}>{claim.status}</span>
                    </td>
                  </tr>
                ))}
                {claims.length === 0 && (
                  <tr>
                    <td colSpan={6} style={{ padding: '24px', textAlign: 'center', color: 'var(--text-muted)' }}>No active ResourceClaims in namespace.</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        )}

        {activeTab === 'graph' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Resource Relationship Graph</h2>
            <p style={{ color: 'var(--text-muted)', fontSize: '0.85rem', marginBottom: '20px' }}>
              Live connections updated via Server-Sent Events. Double-click or hover elements to trace bounds.
            </p>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '20px', minHeight: '300px' }}>
              {graphData?.nodes.map(node => {
                const borderGlow = node.type === 'Pod' ? 'var(--color-success)' : node.type === 'ResourceClaim' ? 'var(--color-warning)' : 'var(--accent-primary)';
                return (
                  <div key={node.id} className="glass-panel" style={{ borderColor: borderGlow, padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px' }}>
                    <span className="badge badge-success" style={{ alignSelf: 'flex-start' }}>{node.type}</span>
                    <strong style={{ fontSize: '1rem' }}>{node.label}</strong>
                    <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>ID: {node.id}</span>
                  </div>
                );
              })}
              {(!graphData || graphData.nodes.length === 0) && (
                <div style={{ gridColumn: '1 / -1', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-muted)' }}>
                  No active graph nodes to display.
                </div>
              )}
            </div>
          </div>
        )}

        {activeTab === 'explain' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Allocation Explanation Engine</h2>
            <div style={{ marginBottom: '20px', display: 'flex', gap: '15px', alignItems: 'center' }}>
              <label htmlFor="claim-select">Select ResourceClaim to diagnose:</label>
              <select
                id="claim-select"
                value={selectedClaim}
                onChange={e => setSelectedClaim(e.target.value)}
                style={{
                  background: 'var(--bg-secondary)',
                  color: 'white',
                  border: '1px solid var(--border-light)',
                  padding: '8px 16px',
                  borderRadius: '6px',
                }}
              >
                <option value="">-- Choose Claim --</option>
                {claims.map(c => (
                  <option key={c.name} value={c.name}>{c.namespace}/{c.name} ({c.status})</option>
                ))}
              </select>
            </div>

            {explainError && (
              <div className="glass-panel" style={{ borderColor: 'var(--color-danger)' }}>
                <p>{explainError}</p>
              </div>
            )}

            {explainResult && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
                <div className="glass-panel" style={{ background: 'var(--bg-secondary)' }}>
                  <h3>Reason Tree Diagnostic</h3>
                  {renderReasonNode(explainResult.reasonTree)}
                </div>

                {explainResult.remedy.length > 0 && (
                  <div className="glass-panel" style={{ borderColor: 'var(--color-warning)' }}>
                    <h3 style={{ color: 'var(--color-warning)', marginBottom: '10px' }}>Suggested Remediation Steps</h3>
                    <ul>
                      {explainResult.remedy.map((rem, idx) => (
                        <li key={idx} style={{ marginLeft: '20px', listStyleType: 'decimal', marginTop: '6px' }}>{rem}</li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            )}
          </div>
        )}

        {activeTab === 'doctor' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Cluster Diagnostics (Doctor)</h2>
            {doctorReport && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
                <div style={{ display: 'flex', gap: '15px' }}>
                  <span className="badge badge-success">PASS: {doctorReport.summary['PASS'] ?? 0}</span>
                  <span className="badge badge-warning">WARN: {doctorReport.summary['WARN'] ?? 0}</span>
                  <span className="badge badge-danger">FAIL: {doctorReport.summary['FAIL'] ?? 0}</span>
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '15px' }}>
                  {doctorReport.results.map(res => {
                    const statusClass = res.status === 'PASS' ? 'badge-success' : res.status === 'WARN' ? 'badge-warning' : 'badge-danger';
                    return (
                      <div key={res.id} className="glass-panel" style={{ background: 'var(--bg-secondary)', padding: '16px' }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                          <h4 style={{ fontSize: '1.1rem' }}>{res.name}</h4>
                          <span className={`badge ${statusClass}`}>{res.status}</span>
                        </div>
                        <p style={{ marginTop: '8px', color: 'var(--text-secondary)' }}>{res.evidence}</p>
                        {res.status !== 'PASS' && (
                          <div style={{ marginTop: '10px', padding: '10px', background: 'var(--bg-tertiary)', borderRadius: '6px', fontSize: '0.9rem' }}>
                            <strong style={{ color: 'var(--color-warning)' }}>Remediation:</strong> {res.remediation}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        )}
      </main>

      {/* Footer */}
      <footer style={{ padding: '20px 40px', background: 'rgba(12, 15, 23, 0.8)', borderTop: '1px solid var(--border-light)', display: 'flex', justifyContent: 'space-between', color: 'var(--text-muted)', fontSize: '0.8rem' }}>
        <span>DRAForge Version {versionVal}</span>
        <a href="https://github.com/oaslananka/draforge" target="_blank" rel="noreferrer" style={{ color: 'var(--accent-secondary)', textDecoration: 'none' }}>GitHub Repository</a>
      </footer>
    </div>
  );
}
