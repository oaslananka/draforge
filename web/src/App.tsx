import { useState, useEffect, useRef } from 'react';
import type {
  Device,
  DevicePool,
  ResourceClaimInfo,
  DoctorReport,
  ResourceGraph,
  ExplainResult,
  ReasonNode,
  TabId,
  SSEStatus,
} from './api/types';
import {
  fetchSummary,
  fetchPools,
  fetchDevices,
  fetchClaims,
  fetchDoctor,
  fetchExplain,
} from './api/api';
import type { Summary } from './api/types';

const versionVal = 'v0.1.0-rc.1';

interface NodePosition {
  id: string;
  x: number;
  y: number;
  vx: number;
  vy: number;
  type: string;
  label: string;
}

function InteractiveGraph({ graphData, onSelectClaim }: { graphData: ResourceGraph | null, onSelectClaim: (name: string) => void }) {
  const width = 800;
  const height = 500;

  const [nodes, setNodes] = useState<NodePosition[]>([]);
  const [selectedNode, setSelectedNode] = useState<NodePosition | null>(null);
  const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
  const [hoveredEdge, setHoveredEdge] = useState<ResourceGraph['edges'][0] | null>(null);
  const [graphEmpty, setGraphEmpty] = useState(false);

  const nodesRef = useRef<NodePosition[]>([]);
  const draggingIdRef = useRef<string | null>(null);
  const svgRef = useRef<SVGSVGElement | null>(null);

  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const isPanningRef = useRef(false);
  const panStartRef = useRef({ x: 0, y: 0 });

  // Sync / Diff nodes on graphData changes
  useEffect(() => {
    if (!graphData) {
      setGraphEmpty(true);
      return;
    }
    const safeNodes = graphData.nodes ?? [];
    const safeEdges = graphData.edges ?? [];
    if (safeNodes.length === 0 && safeEdges.length === 0) {
      setGraphEmpty(true);
      return;
    }
    setGraphEmpty(false);

    const currentNodes = [...nodesRef.current];
    const newNodes: NodePosition[] = safeNodes.map(n => {
      const existing = currentNodes.find(cn => cn.id === n.id);
      if (existing) {
        return {
          ...existing,
          type: n.type,
          label: n.label,
        };
      }
      return {
        id: n.id,
        x: width / 2 + (Math.random() - 0.5) * 100,
        y: height / 2 + (Math.random() - 0.5) * 100,
        vx: 0,
        vy: 0,
        type: n.type,
        label: n.label,
      };
    });

    nodesRef.current = newNodes;
    setNodes(newNodes);
  }, [graphData]);

  // Physics Simulation loop
  useEffect(() => {
    let animationFrameId: number;

    const tick = () => {
      const current = nodesRef.current;
      if (current.length === 0) {
        animationFrameId = requestAnimationFrame(tick);
        return;
      }

      const charge = 1200;
      const gravity = 0.05;
      const linkStrength = 0.06;
      const friction = 0.8;

      for (let i = 0; i < current.length; i++) {
        const n1 = current[i];
        if (n1.id === draggingIdRef.current) continue;

        for (let j = 0; j < current.length; j++) {
          if (i === j) continue;
          const n2 = current[j];
          const dx = n1.x - n2.x;
          const dy = n1.y - n2.y;
          const distSq = dx * dx + dy * dy + 0.1;
          const dist = Math.sqrt(distSq);

          if (dist < 180) {
            const force = charge / distSq;
            n1.vx += (dx / dist) * force;
            n1.vy += (dy / dist) * force;
          }
        }
      }

      const graphEdges = graphData?.edges ?? [];
      graphEdges.forEach(edge => {
        const source = current.find(n => n.id === edge.from);
        const target = current.find(n => n.id === edge.to);
        if (source && target) {
          const dx = target.x - source.x;
          const dy = target.y - source.y;
          const dist = Math.sqrt(dx * dx + dy * dy) || 0.1;
          const force = dist * linkStrength;
          const fx = (dx / dist) * force;
          const fy = (dy / dist) * force;
          if (source.id !== draggingIdRef.current) { source.vx += fx; source.vy += fy; }
          if (target.id !== draggingIdRef.current) { target.vx -= fx; target.vy -= fy; }
        }
      });

      current.forEach(n => {
        if (n.id === draggingIdRef.current) return;
        const dx = width / 2 - n.x;
        const dy = height / 2 - n.y;
        n.vx += dx * gravity;
        n.vy += dy * gravity;
        n.x += n.vx;
        n.y += n.vy;
        n.vx *= friction;
        n.vy *= friction;
      });

      setNodes([...current]);
      animationFrameId = requestAnimationFrame(tick);
    };

    animationFrameId = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(animationFrameId);
  }, [graphData]);

  const handleNodeMouseDown = (e: React.MouseEvent, node: NodePosition) => {
    e.stopPropagation();
    draggingIdRef.current = node.id;
  };

  const handleMouseMove = (e: React.MouseEvent) => {
    if (draggingIdRef.current && svgRef.current) {
      const rect = svgRef.current.getBoundingClientRect();
      const mouseX = (e.clientX - rect.left - pan.x) / zoom;
      const mouseY = (e.clientY - rect.top - pan.y) / zoom;
      const current = nodesRef.current;
      const node = current.find(n => n.id === draggingIdRef.current);
      if (node) {
        node.x = mouseX;
        node.y = mouseY;
        node.vx = 0;
        node.vy = 0;
        setNodes([...current]);
      }
    } else if (isPanningRef.current) {
      setPan({ x: e.clientX - panStartRef.current.x, y: e.clientY - panStartRef.current.y });
    }
  };

  const handleMouseUp = () => { draggingIdRef.current = null; isPanningRef.current = false; };

  const handleSvgMouseDown = (e: React.MouseEvent) => {
    isPanningRef.current = true;
    panStartRef.current = { x: e.clientX - pan.x, y: e.clientY - pan.y };
  };

  const handleWheel = (e: React.WheelEvent) => {
    e.preventDefault();
    const zoomFactor = 1.1;
    if (e.deltaY < 0) { setZoom(prev => Math.min(prev * zoomFactor, 3)); }
    else { setZoom(prev => Math.max(prev / zoomFactor, 0.5)); }
  };

  const handleResetZoom = () => { setZoom(1); setPan({ x: 0, y: 0 }); };

  const getNodeColor = (type: string, id: string) => {
    if (type === 'Device' && id.includes('missing')) return '#ef4444';
    const safeNodes = graphData?.nodes ?? [];
    if (type === 'ResourceClaim') {
      const claim = safeNodes.find(n => n.id === id);
      if (claim?.metadata && typeof claim.metadata === 'object' && (claim.metadata as Record<string, unknown>).status === 'Pending') return '#f59e0b';
    }
    switch (type) {
      case 'Pod': case 'ResourceClaim': return '#10b981';
      case 'Device': return '#3b82f6';
      case 'Pool': return '#8b5cf6';
      case 'Driver': return '#6366f1';
      case 'Node': return '#06b6d4';
      case 'DeviceClass': return '#ec4899';
      default: return '#6b7280';
    }
  };

  const getNodeRadius = (type: string) => {
    switch (type) {
      case 'Pod': return 22;
      case 'Node': return 26;
      case 'Pool': return 20;
      case 'Device': return 16;
      default: return 18;
    }
  };

  if (graphEmpty) {
    return (
      <div style={{ 
        textAlign: 'center', 
        padding: '60px 20px', 
        color: 'var(--text-secondary)',
        background: 'var(--bg-secondary)',
        borderRadius: '16px',
        border: '1px dashed var(--border-light)',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        gap: '15px'
      }}>
        <span className="badge badge-warning" style={{ fontSize: '0.85rem', padding: '4px 12px' }}>No Active Scenario</span>
        <p style={{ maxWidth: '480px', margin: '0 auto', fontSize: '0.95rem' }}>
          No dynamic resource relationships discovered in this cluster. Deploy a SimulatedDevicePool scenario or run a workload pod to populate the interactive graph.
        </p>
        <div style={{ display: 'flex', gap: '10px', marginTop: '10px', flexWrap: 'wrap', justifyContent: 'center' }}>
          <code style={{ background: 'var(--bg-tertiary)', padding: '6px 12px', borderRadius: '6px', fontSize: '0.85rem', color: 'var(--accent-secondary)' }}>
            task demo:up
          </code>
          <code style={{ background: 'var(--bg-tertiary)', padding: '6px 12px', borderRadius: '6px', fontSize: '0.85rem', color: 'var(--accent-secondary)' }}>
            kubectl apply -f examples/scenarios/basic-gpu.yaml
          </code>
        </div>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', gap: '20px', flexDirection: 'column' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div style={{ display: 'flex', gap: '10px' }}>
          <button className="badge badge-success" style={{ border: 'none', cursor: 'pointer' }} onClick={handleResetZoom}>Reset View</button>
        </div>
        <span style={{ fontSize: '0.8rem', color: 'var(--text-muted)' }}>Use mouse wheel to zoom, drag background to pan.</span>
      </div>

      <div style={{ display: 'flex', gap: '20px', flexWrap: 'wrap' }}>
        <div style={{ flex: 2, minWidth: '300px', background: 'var(--bg-secondary)', borderRadius: '12px', border: '1px solid var(--border-light)', overflow: 'hidden', position: 'relative' }}>
          <svg
            ref={svgRef}
            width="100%"
            height={height}
            viewBox={`0 0 ${width} ${height}`}
            onMouseMove={handleMouseMove}
            onMouseUp={handleMouseUp}
            onMouseLeave={handleMouseUp}
            onMouseDown={handleSvgMouseDown}
            onWheel={handleWheel}
            style={{ cursor: isPanningRef.current ? 'grabbing' : 'grab', display: 'block' }}
          >
            <defs>
              <marker id="arrow" viewBox="0 0 10 10" refX="24" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
                <path d="M 0 0 L 10 5 L 0 10 z" fill="var(--text-muted)" />
              </marker>
              <marker id="arrow-danger" viewBox="0 0 10 10" refX="24" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
                <path d="M 0 0 L 10 5 L 0 10 z" fill="#ef4444" />
              </marker>
            </defs>

            <g transform={`translate(${pan.x}, ${pan.y}) scale(${zoom})`}>
              {(graphData?.edges ?? []).map((edge, idx) => {
                const source = nodes.find(n => n.id === edge.from);
                const target = nodes.find(n => n.id === edge.to);
                if (!source || !target) return null;

                const isDanger = edge.type === 'allocates-missing';
                const strokeColor = isDanger ? '#ef4444' : 'var(--border-light)';
                const isHovered = hoveredNodeId === edge.from || hoveredNodeId === edge.to || (hoveredEdge?.from === edge.from && hoveredEdge?.to === edge.to);

                return (
                  <g
                    key={`${edge.from}-${edge.to}-${idx}`}
                    onMouseEnter={() => setHoveredEdge(edge)}
                    onMouseLeave={() => setHoveredEdge(null)}
                    style={{ cursor: 'pointer' }}
                  >
                    <line
                      x1={source.x}
                      y1={source.y}
                      x2={target.x}
                      y2={target.y}
                      stroke={strokeColor}
                      strokeWidth={isHovered ? 3 : 1.5}
                      strokeDasharray={edge.type === 'managed-by' ? '4 4' : undefined}
                      markerEnd={isDanger ? "url(#arrow-danger)" : "url(#arrow)"}
                      opacity={hoveredNodeId && !isHovered ? 0.2 : 0.8}
                    />
                    {isHovered && (
                      <text
                        x={(source.x + target.x) / 2}
                        y={(source.y + target.y) / 2 - 4}
                        fill={isDanger ? '#ef4444' : 'var(--text-secondary)'}
                        fontSize="9"
                        textAnchor="middle"
                        fontWeight="600"
                        style={{ pointerEvents: 'none' }}
                      >
                        {edge.type}
                      </text>
                    )}
                  </g>
                );
              })}

              {nodes.map(node => {
                const radius = getNodeRadius(node.type);
                const color = getNodeColor(node.type, node.id);
                const isHovered = hoveredNodeId === node.id || (hoveredEdge?.from === node.id || hoveredEdge?.to === node.id);
                const opacity = hoveredNodeId && !isHovered ? 0.3 : 1;

                return (
                  <g
                    key={node.id}
                    transform={`translate(${node.x}, ${node.y})`}
                    onMouseDown={(e) => handleNodeMouseDown(e, node)}
                    onClick={() => setSelectedNode(node)}
                    onMouseEnter={() => setHoveredNodeId(node.id)}
                    onMouseLeave={() => setHoveredNodeId(null)}
                    style={{ cursor: 'pointer', opacity }}
                  >
                    <circle
                      r={radius}
                      fill={color}
                      stroke="white"
                      strokeWidth={isHovered ? 2.5 : 1.5}
                    />
                    <text
                      dy=".3em"
                      fill="white"
                      fontSize="9"
                      fontWeight="bold"
                      textAnchor="middle"
                      style={{ pointerEvents: 'none', userSelect: 'none' }}
                    >
                      {node.type.substring(0, 3).toUpperCase()}
                    </text>
                    <text
                      y={radius + 12}
                      fill="var(--text-secondary)"
                      fontSize="9"
                      textAnchor="middle"
                      style={{ pointerEvents: 'none', userSelect: 'none' }}
                    >
                      {node.label}
                    </text>
                  </g>
                );
              })}
            </g>
          </svg>
        </div>

        <div style={{ flex: 1, minWidth: '240px', display: 'flex', flexDirection: 'column', gap: '20px' }}>
          <div className="glass-panel" style={{ background: 'var(--bg-secondary)', height: '100%' }}>
            <h3>Selected Component</h3>
            {selectedNode ? (
              <div style={{ marginTop: '15px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
                <div>
                  <span className="badge badge-success" style={{ background: getNodeColor(selectedNode.type, selectedNode.id), color: 'white', borderColor: 'transparent' }}>{selectedNode.type}</span>
                  <h4 style={{ fontSize: '1.2rem', marginTop: '6px' }}>{selectedNode.label}</h4>
                </div>
                <div style={{ fontSize: '0.85rem', color: 'var(--text-secondary)' }}>
                  <p><strong>ID:</strong> {selectedNode.id}</p>
                </div>
                {selectedNode.type === 'ResourceClaim' && (
                  <button
                    className="badge badge-warning"
                    onClick={() => onSelectClaim(selectedNode.label)}
                    style={{ border: 'none', cursor: 'pointer', padding: '8px 14px', alignSelf: 'flex-start', marginTop: '10px' }}
                  >
                    Diagnose Allocation
                  </button>
                )}
                {(() => {
                  const getRelatedNodes = (nodeId: string) => {
                    if (!graphData) return [];
                    const related = [];
                    const edges = graphData.edges ?? [];
                    const graphNodes = graphData.nodes ?? [];

                    for (const edge of edges) {
                      if (edge.from === nodeId) {
                        const targetNode = graphNodes.find(n => n.id === edge.to);
                        if (targetNode) {
                          related.push({ node: targetNode, type: 'outgoing', relType: edge.type });
                        }
                      } else if (edge.to === nodeId) {
                        const sourceNode = graphNodes.find(n => n.id === edge.from);
                        if (sourceNode) {
                          related.push({ node: sourceNode, type: 'incoming', relType: edge.type });
                        }
                      }
                    }
                    return related;
                  };

                  const related = getRelatedNodes(selectedNode.id);
                  if (related.length === 0) return null;
                  return (
                    <div style={{ marginTop: '15px', borderTop: '1px solid var(--border-light)', paddingTop: '15px' }}>
                      <h5 style={{ fontSize: '0.9rem', color: 'var(--text-muted)', textTransform: 'uppercase', marginBottom: '8px' }}>Relationships</h5>
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                        {related.map((rel, idx) => (
                          <div 
                            key={idx} 
                            style={{ 
                              fontSize: '0.8rem', 
                              display: 'flex', 
                              justifyContent: 'space-between', 
                              alignItems: 'center', 
                              background: 'var(--bg-tertiary)', 
                              padding: '6px 10px', 
                              borderRadius: '6px' 
                            }}
                          >
                            <span style={{ color: 'var(--text-muted)' }}>
                              {rel.type === 'incoming' ? '← ' : '→ '}
                              {rel.relType}
                            </span>
                            <span 
                              style={{ 
                                color: getNodeColor(rel.node.type, rel.node.id), 
                                fontWeight: 'bold', 
                                cursor: 'pointer',
                                textDecoration: 'underline'
                              }}
                              onClick={() => {
                                const found = nodes.find(n => n.id === rel.node.id);
                                if (found) setSelectedNode(found);
                              }}
                            >
                              {rel.node.label}
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>
                  );
                })()}
              </div>
            ) : (
              <p style={{ color: 'var(--text-muted)', marginTop: '15px' }}>Click a node in the graph to inspect details.</p>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function useSSE(onGraph: (data: ResourceGraph) => void) {
  const [sseStatus, setSseStatus] = useState<SSEStatus>('disconnected');
  const onGraphRef = useRef(onGraph);
  onGraphRef.current = onGraph;

  useEffect(() => {
    let es: EventSource | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let delay = 1000;
    const maxDelay = 5000;
    let closed = false;

    function connect() {
      if (closed) return;
      es = new EventSource('/api/stream');

      es.onopen = () => {
        setSseStatus('connected');
        delay = 1000; // reset on success
      };

      es.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as unknown;
          if (data && typeof data === 'object' && 'nodes' in data) {
            onGraphRef.current(data as ResourceGraph);
          }
        } catch {
          console.error('Failed to parse SSE graph stream');
        }
      };

      es.onerror = () => {
        es?.close();
        es = null;
        if (closed) return;
        setSseStatus('reconnecting');
        retryTimer = setTimeout(() => {
          delay = Math.min(delay * 2, maxDelay);
          connect();
        }, delay);
      };
    }

    connect();

    return () => {
      closed = true;
      if (retryTimer) clearTimeout(retryTimer);
      es?.close();
      es = null;
    };
  }, []);

  return sseStatus;
}

export default function App() {
  const [activeTab, setActiveTab] = useState<TabId>('overview');

  // Data states per section
  const [summary, setSummary] = useState<Summary | null>(null);
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
  const [selectedClaim, setSelectedClaim] = useState<string>('');
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
      fetchPools().then(setPools).catch(() => {}).finally(() => setLoadingPools(false)),
      fetchDevices().then(setDevices).catch(() => {}).finally(() => setLoadingDevices(false)),
      fetchClaims().then(data => {
        setClaims(data);
        if (data.length > 0 && !selectedClaim) {
          setSelectedClaim(data[0].name);
        }
      }).catch(() => {}).finally(() => setLoadingClaims(false)),
      fetchDoctor().then(setDoctorReport).catch(() => {}).finally(() => setLoadingDoctor(false)),
    ]);
  };

  useEffect(() => {
    fetchAllData();
  }, []);

  // Fetch claim explanation
  const handleExplain = async (claimName: string) => {
    if (!claimName) return;
    setLoadingExplain(true);
    setExplainError('');
    try {
      const data = await fetchExplain(claimName);
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
  }, [selectedClaim]);

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
                    DRAForge is running on DOKS in region <strong>fra1</strong>. Both worker nodes are healthy. All devices below are simulated synthetic devices, clearly labeled as mock hardware.
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
              onSelectClaim={(name) => {
                setSelectedClaim(name);
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
        <span>DRAForge Version {versionVal}</span>
        <a href="https://github.com/oaslananka/draforge" target="_blank" rel="noreferrer" style={{ color: 'var(--accent-secondary)', textDecoration: 'none' }}>GitHub Repository</a>
      </footer>
    </div>
  );
}
