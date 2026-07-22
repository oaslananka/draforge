import { useState, useEffect } from 'react';
import type {
  Device,
  DevicePool,
  ResourceClaimInfo,
  DoctorReport,
  ResourceGraph,
  ExplainResult,
  ReasonNode,
  TabId
} from './api/types';
import {
  fetchSummary,
  fetchPools,
  fetchDevices,
  fetchClaims,
  fetchDoctor,
  fetchExplain,
  fetchVersion,
} from './api/api';
import type { Summary, VersionInfo } from './api/types';
import InteractiveGraph from "./components/InteractiveGraph";
import useSSE from "./hooks/useSSE";
import type { ClaimIdentity } from './claims/identity';
import {
  claimIdentityKey,
  findClaimIdentityByKey,
  toClaimIdentity,
} from './claims/identity';
export default function App() {
  const [activeTab, setActiveTab] = useState<TabId>('overview');

  // Data states per section
  const [summary, setSummary] = useState<Summary | null>(null);
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);
  const [pools, setPools] = useState<DevicePool[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [claims, setClaims] = useState<ResourceClaimInfo[]>([]);
  const [doctorReport, setDoctorReport] = useState<DoctorReport | null>(null);
  const [graphData, setGraphData] = useState<ResourceGraph | null>(null);

  // Per-section loading states
  const [loadingSummary, setLoadingSummary] = useState(true);
  const [loadingPools, setLoadingPools] = useState(true);
  const [loadingDevices, setLoadingDevices] = useState(true);
  const [loadingClaims, setLoadingClaims] = useState(true);
  const [loadingDoctor, setLoadingDoctor] = useState(true);

  // Global error and explain states
  const [globalError, setGlobalError] = useState('');
  const [selectedClaim, setSelectedClaim] = useState<ClaimIdentity | null>(null);
  const [explainResult, setExplainResult] = useState<ExplainResult | null>(null);
  const [explainError, setExplainError] = useState('');
  const [loadingExplain, setLoadingExplain] = useState(false);

  // SSE status
  const sseStatus = useSSE(setGraphData);

  // Fetch all initial data
  const fetchAllData = async () => {
    setGlobalError('');

    await Promise.allSettled([
      fetchSummary().then(setSummary).catch(e => setGlobalError(e.message || 'Failed to load summary')).finally(() => setLoadingSummary(false)),
      fetchVersion().then(setVersionInfo).catch(() => {}),
      fetchPools().then(setPools).catch(() => {}).finally(() => setLoadingPools(false)),
      fetchDevices().then(setDevices).catch(() => {}).finally(() => setLoadingDevices(false)),
      fetchClaims().then(data => {
        setClaims(data);
        if (data.length > 0) {
          setSelectedClaim(current => current ?? toClaimIdentity(data[0]));
        }
      }).catch(() => {}).finally(() => setLoadingClaims(false)),
      fetchDoctor().then(setDoctorReport).catch(() => {}).finally(() => setLoadingDoctor(false)),
    ]);
  };

  useEffect(() => {
    fetchAllData();
  }, []);

  // Fetch claim explanation
  const handleExplain = async (claim: ClaimIdentity) => {
    setLoadingExplain(true);
    setExplainError('');
    try {
      const data = await fetchExplain(claim.name, claim.namespace);
      setExplainResult(data);
    } catch (e: unknown) {
      const err = e as { message?: string };
      setExplainError(err?.message || 'Failed to fetch explanation tree.');
      setExplainResult(null);
    } finally {
      setLoadingExplain(false);
    }
  };

  useEffect(() => {
    if (selectedClaim) {
      handleExplain(selectedClaim);
    }
  }, [selectedClaim?.name, selectedClaim?.namespace]);

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

  const initialLoading = loadingSummary && loadingPools && loadingDevices && loadingClaims && loadingDoctor;

  const renderDoctorHeaderBadge = () => {
    if (!doctorReport) return null;
    const fails = doctorReport.summary['FAIL'] ?? 0;
    const warns = doctorReport.summary['WARN'] ?? 0;
    const passes = doctorReport.summary['PASS'] ?? 0;

    let badgeClass = 'badge-success';
    let label = `Doctor: Healthy (${passes} OK)`;

    if (fails > 0) {
      badgeClass = 'badge-danger';
      label = `Doctor: ${fails} Failure${fails > 1 ? 's' : ''}`;
    } else if (warns > 0) {
      badgeClass = 'badge-warning';
      label = `Doctor: ${warns} Warning${warns > 1 ? 's' : ''}`;
    }

    return (
      <span
        className={`badge ${badgeClass}`}
        style={{ alignSelf: 'flex-start', marginTop: '4px', cursor: 'pointer' }}
        onClick={() => setActiveTab('doctor')}
        title="Click to view detailed Doctor diagnostics"
      >
        {label}
      </span>
    );
  };

  if (initialLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: '20px' }}>
        <div className="spinner"></div>
        <h2 className="glow-text">Loading DRAForge Platform...</h2>
      </div>
    );
  }

  const sseBadgeClass = sseStatus === 'connected' ? 'badge-success' : sseStatus === 'reconnecting' ? 'badge-warning' : 'badge-danger';
  const sseLabel = sseStatus === 'connected' ? 'Stream Connected' : sseStatus === 'reconnecting' ? 'Reconnecting...' : 'Disconnected';

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      {/* Header */}
      <header style={{ padding: '20px 40px', background: 'rgba(12, 15, 23, 0.8)', borderBottom: '1px solid var(--border-light)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', backdropFilter: 'blur(8px)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
          <div>
            <h1 className="glow-text" style={{ fontSize: '1.8rem', fontFamily: 'var(--font-title)' }}>DRAForge</h1>
            <p style={{ color: 'var(--text-muted)', fontSize: '0.85rem' }}>Dynamic Resource Allocation Developer Platform</p>
          </div>
          <span className={`badge ${sseBadgeClass}`} style={{ alignSelf: 'flex-start', marginTop: '4px' }}>{sseLabel}</span>
          {renderDoctorHeaderBadge()}
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
        {globalError && (
          <div className="glass-panel" style={{ borderColor: 'var(--color-danger)', marginBottom: '30px', display: 'flex', alignItems: 'center', gap: '15px' }}>
            <span className="badge badge-danger">Connection Error</span>
            <p>{globalError}</p>
          </div>
        )}

        {sseStatus !== 'connected' && activeTab === 'graph' && (
          <div className="glass-panel" style={{ borderColor: 'var(--color-warning)', marginBottom: '20px', display: 'flex', alignItems: 'center', gap: '10px' }}>
            <span className="badge badge-warning">Live Stream</span>
            <p style={{ fontSize: '0.9rem', margin: 0 }}>
              {sseStatus === 'reconnecting' ? 'Dashboard stream disconnected. Reconnecting...' : 'Dashboard stream disconnected.'}
            </p>
          </div>
        )}

        {/* --- Overview --- */}
        {activeTab === 'overview' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '30px' }}>
            {loadingSummary ? (
              <div style={{ textAlign: 'center', padding: '40px', color: 'var(--text-muted)' }}>
                <div className="spinner" style={{ margin: '0 auto 16px' }}></div>
                <p>Loading DRAForge summary...</p>
              </div>
            ) : (
              <>
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

                <div className="glass-panel">
                  <h2 style={{ marginBottom: '15px' }}>Dynamic Resource Allocation Status</h2>
                  <p style={{ color: 'var(--text-secondary)' }}>
                    DRAForge is reporting live data from the connected Kubernetes API: <strong>{summary?.poolsCount ?? 0}</strong> simulated pools, <strong>{summary?.devicesCount ?? 0}</strong> discovered devices, and <strong>{summary?.claimsCount ?? 0}</strong> active claims.
                  </p>
                  <p style={{ color: 'var(--text-muted)', fontSize: '0.85rem', marginTop: '10px' }}>
                    Last refreshed: {summary?.timestamp ? new Date(summary.timestamp).toLocaleString() : 'unknown'}
                  </p>
                </div>
              </>
            )}
          </div>
        )}

        {/* --- Pools --- */}
        {activeTab === 'pools' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Virtual Device Pools</h2>
            {loadingPools ? (
              <div style={{ textAlign: 'center', padding: '40px', color: 'var(--text-muted)' }}>
                <div className="spinner" style={{ margin: '0 auto 16px' }}></div>
                <p>Loading device pools...</p>
              </div>
            ) : pools.length === 0 ? (
              <div style={{ 
                textAlign: 'center', 
                padding: '60px 20px', 
                background: 'var(--bg-secondary)', 
                borderRadius: '12px', 
                border: '1px dashed var(--border-light)' 
              }}>
                <p style={{ color: 'var(--text-secondary)', marginBottom: '12px' }}>No virtual device pools registered in this cluster.</p>
                <p style={{ fontSize: '0.85rem', color: 'var(--text-muted)' }}>
                  Deploy a simulator scenario to register a pool: <code style={{ background: 'var(--bg-tertiary)', padding: '4px 8px', borderRadius: '4px', color: 'var(--accent-secondary)' }}>kubectl apply -f examples/scenarios/basic-gpu.yaml</code>
                </p>
              </div>
            ) : (
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
                </tbody>
              </table>
            )}
          </div>
        )}

        {/* --- Devices --- */}
        {activeTab === 'devices' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Discovered Devices</h2>
            {loadingDevices ? (
              <div style={{ textAlign: 'center', padding: '40px', color: 'var(--text-muted)' }}>
                <div className="spinner" style={{ margin: '0 auto 16px' }}></div>
                <p>Loading devices...</p>
              </div>
            ) : devices.length === 0 ? (
              <div style={{ 
                textAlign: 'center', 
                padding: '60px 20px', 
                background: 'var(--bg-secondary)', 
                borderRadius: '12px', 
                border: '1px dashed var(--border-light)' 
              }}>
                <p style={{ color: 'var(--text-secondary)', marginBottom: '12px' }}>No discovered DRA devices detected.</p>
                <p style={{ fontSize: '0.85rem', color: 'var(--text-muted)' }}>
                  Simulated devices will automatically show up here as soon as the node plugin driver publishes ResourceSlice objects.
                </p>
              </div>
            ) : (
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
                </tbody>
              </table>
            )}
          </div>
        )}

        {/* --- Claims --- */}
        {activeTab === 'claims' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Resource Claims</h2>
            {loadingClaims ? (
              <div style={{ textAlign: 'center', padding: '40px', color: 'var(--text-muted)' }}>
                <div className="spinner" style={{ margin: '0 auto 16px' }}></div>
                <p>Loading claims...</p>
              </div>
            ) : claims.length === 0 ? (
              <div style={{ 
                textAlign: 'center', 
                padding: '60px 20px', 
                background: 'var(--bg-secondary)', 
                borderRadius: '12px', 
                border: '1px dashed var(--border-light)' 
              }}>
                <p style={{ color: 'var(--text-secondary)', marginBottom: '12px' }}>No ResourceClaims found in the current cluster.</p>
                <p style={{ fontSize: '0.85rem', color: 'var(--text-muted)' }}>
                  Deploy a test workload pod requesting a simulated device allocation to trace claims.
                </p>
              </div>
            ) : (
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
                    <tr key={claimIdentityKey(claim)} style={{ borderBottom: '1px solid var(--border-light)' }}>
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
                </tbody>
              </table>
            )}
          </div>
        )}

        {/* --- Graph --- */}
        {activeTab === 'graph' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Resource Relationship Graph</h2>
            <InteractiveGraph
              graphData={graphData}
              onSelectClaim={(claim) => {
                setSelectedClaim(claim);
                setActiveTab('explain');
              }}
            />
          </div>
        )}

        {/* --- Explain --- */}
        {activeTab === 'explain' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Allocation Explanation Engine</h2>
            <div style={{ marginBottom: '20px', display: 'flex', gap: '15px', alignItems: 'center' }}>
              <label htmlFor="claim-select">Select ResourceClaim to diagnose:</label>
              <select
                id="claim-select"
                value={selectedClaim ? claimIdentityKey(selectedClaim) : ''}
                onChange={e => {
                  setSelectedClaim(findClaimIdentityByKey(claims, e.target.value));
                }}
                style={{
                  background: 'var(--bg-secondary)',
                  color: 'white',
                  border: '1px solid var(--border-light)',
                  padding: '8px 16px',
                  borderRadius: '6px',
                }}
              >
                <option value="">-- Choose Claim --</option>
                {claims.map(c => {
                  const identityKey = claimIdentityKey(c);
                  return (
                    <option key={identityKey} value={identityKey}>
                      {c.namespace}/{c.name} ({c.status})
                    </option>
                  );
                })}
              </select>
            </div>

            {loadingExplain && (
              <div style={{ textAlign: 'center', padding: '20px', color: 'var(--text-muted)' }}>
                <div className="spinner" style={{ margin: '0 auto 16px' }}></div>
                <p>Diagnosing allocation...</p>
              </div>
            )}

            {explainError && !loadingExplain && (
              <div className="glass-panel" style={{ borderColor: 'var(--color-danger)' }}>
                <p>{explainError}</p>
              </div>
            )}

            {explainResult && !loadingExplain && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
                <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
                  <span className={`badge ${explainResult.allocated ? 'badge-success' : 'badge-warning'}`}>
                    {explainResult.allocated ? 'Allocated' : 'Not Allocated'}
                  </span>
                  <span style={{ color: 'var(--text-secondary)', fontSize: '0.9rem' }}>
                    {explainResult.targetType}: <strong>{explainResult.targetName}</strong>
                  </span>
                </div>
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

            {!explainResult && !explainError && !loadingExplain && (
              <div style={{ textAlign: 'center', padding: '30px', color: 'var(--text-muted)' }}>
                <p>Select a claim above to diagnose its allocation status.</p>
              </div>
            )}
          </div>
        )}

        {/* --- Doctor --- */}
        {activeTab === 'doctor' && (
          <div className="glass-panel">
            <h2 style={{ marginBottom: '20px' }}>Cluster Diagnostics (Doctor)</h2>
            {loadingDoctor ? (
              <div style={{ textAlign: 'center', padding: '40px', color: 'var(--text-muted)' }}>
                <div className="spinner" style={{ margin: '0 auto 16px' }}></div>
                <p>Running diagnostics...</p>
              </div>
            ) : doctorReport ? (
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
            ) : (
              <div style={{ textAlign: 'center', padding: '40px', color: 'var(--text-muted)' }}>
                <p>Unable to load DRAForge diagnostics.</p>
              </div>
            )}
          </div>
        )}
      </main>

      {/* Footer */}
      <footer style={{ padding: '20px 40px', background: 'rgba(12, 15, 23, 0.8)', borderTop: '1px solid var(--border-light)', display: 'flex', justifyContent: 'space-between', color: 'var(--text-muted)', fontSize: '0.8rem' }}>
        <span>DRAForge Version {versionInfo?.version ?? 'dev'} ({versionInfo?.commit ?? 'dev'})</span>
        <a href="https://github.com/oaslananka/draforge" target="_blank" rel="noreferrer" style={{ color: 'var(--accent-secondary)', textDecoration: 'none' }}>GitHub Repository</a>
      </footer>
    </div>
  );
}
