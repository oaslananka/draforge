import type {
  Summary,
  DevicePool,
  Device,
  ResourceClaimInfo,
  ResourceGraph,
  ExplainResult,
  DoctorReport,
} from './types';

interface ApiError {
  message: string;
  code?: string;
}

function parseApiError(body: unknown): ApiError {
  if (body && typeof body === 'object') {
    const obj = body as Record<string, unknown>;
    const err = obj.error;
    if (err && typeof err === 'object') {
      const e = err as Record<string, unknown>;
      return {
        message: typeof e.message === 'string' ? e.message : 'Unknown API error',
        code: typeof e.code === 'string' ? e.code : undefined,
      };
    }
    if (typeof err === 'string') {
      return { message: err };
    }
  }
  return { message: 'Unknown API error' };
}

async function typedFetch<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    let apiErr: ApiError = { message: `HTTP ${res.status}: ${res.statusText}` };
    try {
      const body = await res.json();
      apiErr = parseApiError(body);
    } catch {
      // ignore parse failure, use default message
    }
    throw apiErr;
  }
  try {
    return (await res.json()) as T;
  } catch {
    throw { message: 'Failed to parse API response as JSON' } satisfies ApiError;
  }
}

export function fetchSummary(): Promise<Summary> {
  return typedFetch<Summary>('/api/summary');
}

export function fetchPools(): Promise<DevicePool[]> {
  return typedFetch<DevicePool[]>('/api/pools');
}

export function fetchDevices(): Promise<Device[]> {
  return typedFetch<Device[]>('/api/devices');
}

export function fetchClaims(): Promise<ResourceClaimInfo[]> {
  return typedFetch<ResourceClaimInfo[]>('/api/claims');
}

export function fetchGraph(): Promise<ResourceGraph> {
  return typedFetch<ResourceGraph>('/api/graph');
}

export function fetchDoctor(): Promise<DoctorReport> {
  return typedFetch<DoctorReport>('/api/doctor');
}

export function fetchExplain(claim: string, namespace: string = 'default'): Promise<ExplainResult> {
  return typedFetch<ExplainResult>(`/api/explain?claim=${encodeURIComponent(claim)}&namespace=${encodeURIComponent(namespace)}`);
}
