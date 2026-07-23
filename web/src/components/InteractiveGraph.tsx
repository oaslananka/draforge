import { useState, useEffect, useRef } from 'react';
import type { ResourceGraph } from '../api/types';
import type { ClaimIdentity } from '../claims/identity';
import { claimIdentityFromGraphNode } from '../claims/identity';
import { deterministicInitialPosition } from '../graph/layout';

interface NodePosition {
  id: string;
  x: number;
  y: number;
  vx: number;
  vy: number;
  type: string;
  label: string;
  metadata?: Record<string, unknown>;
}

interface InteractiveGraphProps {
  readonly graphData: ResourceGraph | null;
  readonly onSelectClaim: (claim: ClaimIdentity) => void;
}

type GraphNode = ResourceGraph['nodes'][number];

type RelatedNode = Readonly<{
  node: GraphNode;
  direction: 'incoming' | 'outgoing';
  relationship: string;
}>;

function nodeColor(graphData: ResourceGraph | null, type: string, id: string) {
  if (type === 'Device' && id.includes('missing')) return '#ef4444';
  if (type === 'ResourceClaim') {
    const claim = graphData?.nodes.find((node) => node.id === id);
    if (claim?.metadata?.status === 'Pending') return '#f59e0b';
  }
  switch (type) {
    case 'Pod':
    case 'ResourceClaim':
      return '#10b981';
    case 'Device':
      return '#3b82f6';
    case 'Allocation':
      return '#f97316';
    case 'ResourcePool':
      return '#8b5cf6';
    case 'Driver':
      return '#6366f1';
    case 'Node':
      return '#06b6d4';
    case 'DeviceClass':
      return '#ec4899';
    default:
      return '#6b7280';
  }
}

function relatedNodes(graphData: ResourceGraph | null, nodeId: string): RelatedNode[] {
  if (!graphData) return [];
  const nodesByID = new Map(graphData.nodes.map((node) => [node.id, node]));
  const related: RelatedNode[] = [];

  for (const edge of graphData.edges) {
    if (edge.from === nodeId) {
      const node = nodesByID.get(edge.to);
      if (node) related.push({ node, direction: 'outgoing', relationship: edge.type });
      continue;
    }
    if (edge.to === nodeId) {
      const node = nodesByID.get(edge.from);
      if (node) related.push({ node, direction: 'incoming', relationship: edge.type });
    }
  }
  return related;
}

type RelationshipsProps = Readonly<{
  graphData: ResourceGraph | null;
  nodeId: string;
  nodes: readonly NodePosition[];
  onSelectNode: (node: NodePosition) => void;
}>;

function Relationships({ graphData, nodeId, nodes, onSelectNode }: RelationshipsProps) {
  const relationships = relatedNodes(graphData, nodeId);
  if (relationships.length === 0) return null;

  const selectRelatedNode = (id: string) => {
    const found = nodes.find((candidate) => candidate.id === id);
    if (found) onSelectNode(found);
  };

  return (
    <div style={{ marginTop: '15px', borderTop: '1px solid var(--border-light)', paddingTop: '15px' }}>
      <h5 style={{ fontSize: '0.9rem', color: 'var(--text-muted)', textTransform: 'uppercase', marginBottom: '8px' }}>Relationships</h5>
      <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
        {relationships.map((relation) => (
          <div
            key={`${relation.direction}/${relation.relationship}/${relation.node.id}`}
            style={{
              fontSize: '0.8rem',
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              background: 'var(--bg-tertiary)',
              padding: '6px 10px',
              borderRadius: '6px',
            }}
          >
            <span style={{ color: 'var(--text-muted)' }}>
              {relation.direction === 'incoming' ? '← ' : '→ '}
              {relation.relationship}
            </span>
            <button
              type="button"
              style={{
                color: nodeColor(graphData, relation.node.type, relation.node.id),
                fontWeight: 'bold',
                cursor: 'pointer',
                textDecoration: 'underline',
                background: 'transparent',
                border: 0,
                padding: 0,
              }}
              onClick={() => selectRelatedNode(relation.node.id)}
            >
              {relation.node.label}
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}

export default function InteractiveGraph({ graphData, onSelectClaim }: InteractiveGraphProps) {
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
          metadata: n.metadata,
        };
      }
      const position = deterministicInitialPosition(n.id, width, height);
      return {
        id: n.id,
        x: position.x,
        y: position.y,
        vx: 0,
        vy: 0,
        type: n.type,
        label: n.label,
        metadata: n.metadata,
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

  const getNodeRadius = (type: string) => {
    switch (type) {
      case 'Pod': return 22;
      case 'Node': return 26;
      case 'ResourcePool': return 20;
      case 'Allocation': return 14;
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
          <button type="button" className="badge badge-success" style={{ border: 'none', cursor: 'pointer' }} onClick={handleResetZoom}>Reset View</button>
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
                const color = nodeColor(graphData, node.type, node.id);
                const isHovered = hoveredNodeId === node.id || (hoveredEdge?.from === node.id || hoveredEdge?.to === node.id);
                const opacity = hoveredNodeId && !isHovered ? 0.3 : 1;

                return (
                  <g
                    key={node.id}
                    transform={`translate(${node.x}, ${node.y})`}
                    role="button"
                    tabIndex={0}
                    aria-label={`${node.type} ${node.label}`}
                    onMouseDown={(event) => handleNodeMouseDown(event, node)}
                    onClick={() => setSelectedNode(node)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault();
                        setSelectedNode(node);
                      }
                    }}
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
                  <span className="badge badge-success" style={{ background: nodeColor(graphData, selectedNode.type, selectedNode.id), color: 'white', borderColor: 'transparent' }}>{selectedNode.type}</span>
                  <h4 style={{ fontSize: '1.2rem', marginTop: '6px' }}>{selectedNode.label}</h4>
                </div>
                <div style={{ fontSize: '0.85rem', color: 'var(--text-secondary)' }}>
                  <p><strong>ID:</strong> {selectedNode.id}</p>
                </div>
                {selectedNode.type === 'ResourceClaim' && (
                  <button
                    type="button"
                    className="badge badge-warning"
                    onClick={() => {
                      const claim = claimIdentityFromGraphNode(selectedNode);
                      if (claim) {
                        onSelectClaim(claim);
                      }
                    }}
                    style={{ border: 'none', cursor: 'pointer', padding: '8px 14px', alignSelf: 'flex-start', marginTop: '10px' }}
                  >
                    Diagnose Allocation
                  </button>
                )}
                <Relationships
                  graphData={graphData}
                  nodeId={selectedNode.id}
                  nodes={nodes}
                  onSelectNode={setSelectedNode}
                />
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
