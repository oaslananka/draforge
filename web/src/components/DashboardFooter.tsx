import type { VersionInfo } from '../api/types';

interface DashboardFooterProps {
  versionInfo: VersionInfo | null;
}

export default function DashboardFooter({ versionInfo }: DashboardFooterProps) {
  return (
    <footer style={{ padding: '20px 40px', background: 'rgba(12, 15, 23, 0.8)', borderTop: '1px solid var(--border-light)', display: 'flex', justifyContent: 'space-between', color: 'var(--text-muted)', fontSize: '0.8rem' }}>
      <span>DRAForge Version {versionInfo?.version ?? 'dev'} ({versionInfo?.commit ?? 'dev'})</span>
      <a href="https://github.com/oaslananka/draforge" target="_blank" rel="noreferrer" style={{ color: 'var(--accent-secondary)', textDecoration: 'none' }}>
        GitHub Repository
      </a>
    </footer>
  );
}
