import { useEffect, useState } from 'react';
import {
  fetchClaims,
  fetchDevices,
  fetchDoctor,
  fetchPools,
  fetchSummary,
  fetchVersion,
} from '../api/api';
import type {
  Device,
  DevicePool,
  DoctorReport,
  ResourceClaimInfo,
  Summary,
  VersionInfo,
} from '../api/types';

export type DashboardSection = 'summary' | 'pools' | 'devices' | 'claims' | 'doctor';

export interface DashboardLoadingState {
  summary: boolean;
  pools: boolean;
  devices: boolean;
  claims: boolean;
  doctor: boolean;
}

export type DashboardErrorState = Partial<Record<DashboardSection, string>>;

export interface DashboardDataState {
  summary: Summary | null;
  versionInfo: VersionInfo | null;
  pools: DevicePool[];
  devices: Device[];
  claims: ResourceClaimInfo[];
  doctorReport: DoctorReport | null;
  loading: DashboardLoadingState;
  errors: DashboardErrorState;
  initialLoading: boolean;
  globalError: string;
}

const initialLoadingState: DashboardLoadingState = {
  summary: true,
  pools: true,
  devices: true,
  claims: true,
  doctor: true,
};

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

export default function useDashboardData(): DashboardDataState {
  const [summary, setSummary] = useState<Summary | null>(null);
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);
  const [pools, setPools] = useState<DevicePool[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [claims, setClaims] = useState<ResourceClaimInfo[]>([]);
  const [doctorReport, setDoctorReport] = useState<DoctorReport | null>(null);
  const [loading, setLoading] = useState<DashboardLoadingState>(initialLoadingState);
  const [errors, setErrors] = useState<DashboardErrorState>({});

  useEffect(() => {
    let active = true;

    const settle = <T,>(
      section: DashboardSection,
      request: Promise<T>,
      apply: (value: T) => void,
      fallback: string,
    ) => request
      .then((value) => {
        if (active) apply(value);
      })
      .catch((error: unknown) => {
        if (!active) return;
        setErrors((current) => ({
          ...current,
          [section]: errorMessage(error, fallback),
        }));
      })
      .finally(() => {
        if (!active) return;
        setLoading((current) => ({ ...current, [section]: false }));
      });

    void Promise.allSettled([
      settle('summary', fetchSummary(), setSummary, 'Failed to load summary'),
      fetchVersion().then((value) => {
        if (active) setVersionInfo(value);
      }).catch(() => undefined),
      settle('pools', fetchPools(), setPools, 'Failed to load device pools'),
      settle('devices', fetchDevices(), setDevices, 'Failed to load devices'),
      settle('claims', fetchClaims(), setClaims, 'Failed to load ResourceClaims'),
      settle('doctor', fetchDoctor(), setDoctorReport, 'Failed to load diagnostics'),
    ]);

    return () => {
      active = false;
    };
  }, []);

  return {
    summary,
    versionInfo,
    pools,
    devices,
    claims,
    doctorReport,
    loading,
    errors,
    initialLoading: Object.values(loading).every(Boolean),
    globalError: errors.summary ?? '',
  };
}
