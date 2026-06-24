export interface Device {
  id: string;
  name: string;
  type: string;
  status: string;
  driverName: string;
  nodeName: string;
  poolName: string;
  attributes: Record<string, string>;
  capacities: Record<string, number>;
  isSynthetic: boolean;
  lastUpdated: string;
}

export interface DevicePool {
  name: string;
  driverName: string;
  nodeName: string;
  deviceCount: number;
  deviceType: string;
  health: string;
  isSynthetic: boolean;
  labels?: Record<string, string>;
}

export interface ResourceClaimInfo {
  name: string;
  namespace: string;
  deviceClassName: string;
  status: string;
  ownerPodName?: string;
  allocatedDevice?: string;
  allocatedNode?: string;
  allocatedDriver?: string;
  createdAt?: string;
}

export interface GraphNode {
  id: string;
  type: string;
  label: string;
  metadata?: Record<string, unknown>;
}

export interface GraphEdge {
  from: string;
  to: string;
  type: string;
}

export interface ResourceGraph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface ReasonNode {
  message: string;
  confidence: string;
  evidence: string;
  sourceType: string;
  fieldPath?: string;
  children?: ReasonNode[];
}

export interface ExplainResult {
  targetName: string;
  targetType: string;
  allocated: boolean;
  reasonTree: ReasonNode;
  remedy: string[];
}

export interface DoctorCheckResult {
  id: string;
  name: string;
  category: string;
  status: string;
  severity: string;
  evidence: string;
  remediation: string;
  docReference: string;
}

export interface DoctorReport {
  timestamp: string;
  summary: Record<string, number>;
  results: DoctorCheckResult[];
}

export interface Summary {
  poolsCount: number;
  devicesCount: number;
  claimsCount: number;
  doctorStatus: Record<string, number>;
  timestamp: string;
}

export interface VersionInfo {
  version: string;
  commit: string;
}

export type TabId = 'overview' | 'pools' | 'devices' | 'claims' | 'graph' | 'explain' | 'doctor';

export type SSEStatus = 'connected' | 'disconnected' | 'reconnecting';
