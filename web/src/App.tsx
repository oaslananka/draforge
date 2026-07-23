import { useEffect, useState } from 'react';
import type { ResourceGraph, TabId } from './api/types';
import type { ClaimIdentity } from './claims/identity';
import {
  findClaimIdentityByKey,
  toClaimIdentity,
} from './claims/identity';
import DashboardFooter from './components/DashboardFooter';
import DashboardHeader from './components/DashboardHeader';
import useClaimExplanation from './hooks/useClaimExplanation';
import useDashboardData from './hooks/useDashboardData';
import useSSE from './hooks/useSSE';
import DashboardViewRouter from './views/DashboardViewRouter';

export default function App() {
  const [activeTab, setActiveTab] = useState<TabId>('overview');
  const [graphData, setGraphData] = useState<ResourceGraph | null>(null);
  const [selectedClaim, setSelectedClaim] = useState<ClaimIdentity | null>(null);
  const data = useDashboardData();
  const sseStatus = useSSE(setGraphData);
  const explanation = useClaimExplanation(selectedClaim);

  useEffect(() => {
    setSelectedClaim((current) => {
      if (current) {
        const identity = `${current.namespace}/${current.name}`;
        if (findClaimIdentityByKey(data.claims, identity)) return current;
      }
      return data.claims.length > 0 ? toClaimIdentity(data.claims[0]) : null;
    });
  }, [data.claims]);

  if (data.initialLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: '20px' }} role="status">
        <div className="spinner" />
        <h2 className="glow-text">Loading DRAForge Platform...</h2>
      </div>
    );
  }

  const selectGraphClaim = (claim: ClaimIdentity) => {
    setSelectedClaim(claim);
    setActiveTab('explain');
  };

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      <DashboardHeader
        activeTab={activeTab}
        doctorReport={data.doctorReport}
        sseStatus={sseStatus}
        onTabChange={setActiveTab}
      />
      <main style={{ flex: 1, padding: '40px' }}>
        {data.globalError && (
          <div className="glass-panel" style={{ borderColor: 'var(--color-danger)', marginBottom: '30px', display: 'flex', alignItems: 'center', gap: '15px' }} role="alert">
            <span className="badge badge-danger">Connection Error</span>
            <p>{data.globalError}</p>
          </div>
        )}
        <DashboardViewRouter
          activeTab={activeTab}
          summary={data.summary}
          pools={data.pools}
          devices={data.devices}
          claims={data.claims}
          doctorReport={data.doctorReport}
          graphData={graphData}
          sseStatus={sseStatus}
          loading={data.loading}
          errors={data.errors}
          selectedClaim={selectedClaim}
          explanation={explanation}
          onSelectClaim={setSelectedClaim}
          onGraphClaim={selectGraphClaim}
        />
      </main>
      <DashboardFooter versionInfo={data.versionInfo} />
    </div>
  );
}
