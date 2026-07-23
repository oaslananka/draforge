import { useEffect, useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';
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
  readonly summary: boolean;
  readonly pools: boolean;
  readonly devices: boolean;
  readonly claims: boolean;
  readonly doctor: boolean;
}

export type DashboardErrorState = Partial<Record<DashboardSection, string>>;

export interface DashboardDataState {
  readonly summary: Summary | null;
  readonly versionInfo: VersionInfo | null;
  readonly pools: readonly DevicePool[];
  readonly devices: readonly Device[];
  readonly claims: readonly ResourceClaimInfo[];
  readonly doctorReport: DoctorReport | null;
  readonly loading: DashboardLoadingState;
  readonly errors: DashboardErrorState;
  readonly initialLoading: boolean;
  readonly globalError: string;
}

const initialLoadingState: DashboardLoadingState = {
  summary: true,
  pools: true,
  devices: true,
  claims: true,
  doctor: true,
};

type StateSetter<T> = Dispatch<SetStateAction<T>>;
type ActiveCheck = () => boolean;

interface SectionLoadOptions<T> {
  readonly section: DashboardSection;
  readonly request: () => Promise<T>;
  readonly apply: (value: T) => void;
  readonly fallback: string;
  readonly isActive: ActiveCheck;
  readonly setErrors: StateSetter<DashboardErrorState>;
  readonly setLoading: StateSetter<DashboardLoadingState>;
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

function markSectionFinished(
  section: DashboardSection,
  setLoading: StateSetter<DashboardLoadingState>,
) {
  setLoading((current) => ({ ...current, [section]: false }));
}

function reportSectionError(
  section: DashboardSection,
  message: string,
  setErrors: StateSetter<DashboardErrorState>,
) {
  setErrors((current) => ({ ...current, [section]: message }));
}

async function loadSection<T>(options: SectionLoadOptions<T>) {
  try {
    const value = await options.request();
    if (options.isActive()) options.apply(value);
  } catch (error: unknown) {
    if (options.isActive()) {
      reportSectionError(
        options.section,
        errorMessage(error, options.fallback),
        options.setErrors,
      );
    }
  } finally {
    if (options.isActive()) {
      markSectionFinished(options.section, options.setLoading);
    }
  }
}

async function loadVersion(
  isActive: ActiveCheck,
  apply: (value: VersionInfo) => void,
) {
  try {
    const value = await fetchVersion();
    if (isActive()) apply(value);
  } catch {
    // Build metadata is optional and has a stable dev fallback in the footer.
  }
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
    const isActive = () => active;
    const common = { isActive, setErrors, setLoading };

    void Promise.allSettled([
      loadSection({ section: 'summary', request: fetchSummary, apply: setSummary, fallback: 'Failed to load summary', ...common }),
      loadVersion(isActive, setVersionInfo),
      loadSection({ section: 'pools', request: fetchPools, apply: setPools, fallback: 'Failed to load device pools', ...common }),
      loadSection({ section: 'devices', request: fetchDevices, apply: setDevices, fallback: 'Failed to load devices', ...common }),
      loadSection({ section: 'claims', request: fetchClaims, apply: setClaims, fallback: 'Failed to load ResourceClaims', ...common }),
      loadSection({ section: 'doctor', request: fetchDoctor, apply: setDoctorReport, fallback: 'Failed to load diagnostics', ...common }),
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
