import type { DoctorReport, SSEStatus, TabId } from '../api/types';

const sections: ReadonlyArray<{ id: TabId; label: string; purpose: string }> = [
  { id: 'overview', label: 'Overview', purpose: 'Check resource counts and cluster health at a glance.' },
  { id: 'pools', label: 'Pools', purpose: 'Inspect simulated pools and the nodes advertising them.' },
  { id: 'devices', label: 'Devices', purpose: 'See discovered devices, their pools and availability.' },
  { id: 'claims', label: 'Claims', purpose: 'Find pending and allocated resource claims.' },
  { id: 'graph', label: 'Relationships', purpose: 'Trace a claim through classes, pools and devices.' },
  { id: 'explain', label: 'Explain a claim', purpose: 'Understand why a claim was allocated or is still pending.' },
  { id: 'doctor', label: 'Diagnostics', purpose: 'Review actionable cluster and DRA health checks.' },
];

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
    return { badgeClass: 'badge-danger', label: 'Doctor: ' + failures + (failures > 1 ? ' Failures' : ' Failure') };
  }
  if (warnings > 0) {
    return { badgeClass: 'badge-warning', label: 'Doctor: ' + warnings + (warnings > 1 ? ' Warnings' : ' Warning') };
  }
  return { badgeClass: 'badge-success', label: 'Doctor: Healthy (' + passes + ' OK)' };
}

export default function DashboardHeader({
  activeTab,
  doctorReport,
  sseStatus,
  onTabChange,
}: Readonly<DashboardHeaderProps>) {
  const stream = streamSummary(sseStatus);
  const doctor = doctorReport ? doctorSummary(doctorReport) : null;
  const active = sections.find((section) => section.id === activeTab) ?? sections[0];

  return (
    <header className="dashboard-header">
      <div className="dashboard-header-top">
        <div className="dashboard-brand">
          <span className="dashboard-eyebrow">Kubernetes DRA / Read-only observability</span>
          <h1 className="dashboard-title">DRAForge</h1>
          <p className="dashboard-subtitle">See what is allocated, what is pending, and why.</p>
        </div>
        <div className="dashboard-system-state">
          <span className={'badge ' + stream.badgeClass} role="status"
            aria-label="Live stream status" aria-live="polite">
            {stream.label}
          </span>
          {doctor && (
            <button type="button" className={'badge dashboard-health ' + doctor.badgeClass}
              onClick={() => { onTabChange('doctor'); }} title="View detailed Doctor diagnostics">
              {doctor.label}
            </button>
          )}
        </div>
      </div>
      <nav className="dashboard-nav" aria-label="Dashboard sections">
        {sections.map(({ id, label }) => (
          <button
            type="button"
            className="dashboard-nav-item"
            key={id}
            onClick={() => { onTabChange(id); }}
            aria-pressed={activeTab === id}
          >
            {label}
          </button>
        ))}
      </nav>
      <div className="dashboard-context" aria-live="off">
        <span className="dashboard-context-label">{active.label}</span>
        <span>{active.purpose}</span>
      </div>
    </header>
  );
}
