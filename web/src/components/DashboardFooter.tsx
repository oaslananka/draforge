import type { VersionInfo } from '../api/types';

interface DashboardFooterProps {
  versionInfo: VersionInfo | null;
}

export default function DashboardFooter({ versionInfo }: Readonly<DashboardFooterProps>) {
  return (
    <footer className="dashboard-footer">
      <span>DRAForge Version {versionInfo?.version ?? 'dev'} ({versionInfo?.commit ?? 'dev'})</span>
      <a href="https://github.com/oaslananka/draforge" target="_blank" rel="noreferrer" style={{ color: 'var(--accent-secondary)', textDecoration: 'none' }}>
        GitHub Repository
      </a>
    </footer>
  );
}
