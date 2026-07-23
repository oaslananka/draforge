import type { DoctorReport, SSEStatus, TabId } from '../api/types';

const tabs: TabId[] = ['overview', 'pools', 'devices', 'claims', 'graph', 'explain', 'doctor'];

function streamSummary(status: SSEStatus): { badgeClass: string; label: string } {
  switch (status) {
    case 'connected':
      return { badgeClass: 'badge-success', label: 'Stream Connected' };
    case 'reconnecting':
      return { badgeClass: 'badge-warning', label: 'Reconnecting...' };
    case 'disconnected':
      return { badgeClass: 'badge-danger', label: 'Disconnected' };
  }
}

interface DashboardHeaderProps {
  activeTab: TabId;
  doctorReport: DoctorReport | null;
  sseStatus: SSEStatus;
  onTabChange: (tab: TabId) => void;
}

function doctorSummary(report: DoctorReport): { badgeClass: string; label: string } {
  const failures = report.summary.FAIL ?? 0;
  const warnings = report.summary.WARN ?? 0;
  const passes = report.summary.PASS ?? 0;

  if (failures > 0) {
    return {
      badgeClass: 'badge-danger',
      label: `Doctor: ${failures} Failure${failures > 1 ? 's' : ''}`,
    };
  }
  if (warnings > 0) {
    return {
      badgeClass: 'badge-warning',
      label: `Doctor: ${warnings} Warning${warnings > 1 ? 's' : ''}`,
    };
  }
  return {
    badgeClass: 'badge-success',
    label: `Doctor: Healthy (${passes} OK)`,
  };
}

export default function DashboardHeader({
  activeTab,
  doctorReport,
  sseStatus,
  onTabChange,
}: DashboardHeaderProps) {
  const stream = streamSummary(sseStatus);
  const doctor = doctorReport ? doctorSummary(doctorReport) : null;

  return (
    <header style={{ padding: '20px 40px', background: 'rgba(12, 15, 23, 0.8)', borderBottom: '1px solid var(--border-light)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', backdropFilter: 'blur(8px)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
        <div>
          <h1 className="glow-text" style={{ fontSize: '1.8rem', fontFamily: 'var(--font-title)' }}>DRAForge</h1>
          <p style={{ color: 'var(--text-muted)', fontSize: '0.85rem' }}>Dynamic Resource Allocation Developer Platform</p>
        </div>
        <span className={`badge ${stream.badgeClass}`} style={{ alignSelf: 'flex-start', marginTop: '4px' }} aria-live="polite">
          {stream.label}
        </span>
        {doctor && (
          <button
            type="button"
            className={`badge ${doctor.badgeClass}`}
            onClick={() => onTabChange('doctor')}
            title="View detailed Doctor diagnostics"
            style={{ alignSelf: 'flex-start', marginTop: '4px', cursor: 'pointer', border: 0 }}
          >
            {doctor.label}
          </button>
        )}
      </div>
      <nav style={{ display: 'flex', gap: '15px' }} aria-label="Dashboard sections">
        {tabs.map((tab) => (
          <button
            type="button"
            key={tab}
            onClick={() => onTabChange(tab)}
            aria-pressed={activeTab === tab}
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
  );
}
