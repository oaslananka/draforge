import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ResourceGraph } from '../api/types';
import InteractiveGraph from './InteractiveGraph';

const graph: ResourceGraph = {
  nodes: [
    {
      id: 'claim/team-b/shared',
      type: 'ResourceClaim',
      label: 'shared',
      metadata: { namespace: 'team-b', status: 'Allocated' },
    },
  ],
  edges: [],
};

describe('InteractiveGraph accessibility', () => {
  beforeEach(() => {
    vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1));
    vi.stubGlobal('cancelAnimationFrame', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('lets keyboard users inspect a claim and open its namespace-qualified explanation', async () => {
    const user = userEvent.setup();
    const onSelectClaim = vi.fn();
    render(<InteractiveGraph graphData={graph} onSelectClaim={onSelectClaim} />);

    const claimNode = await screen.findByRole('button', { name: 'ResourceClaim shared' });
    claimNode.focus();
    fireEvent.keyDown(claimNode, { key: 'Enter' });

    const diagnose = screen.getByRole('button', { name: 'Diagnose Allocation' });
    await user.click(diagnose);

    expect(onSelectClaim).toHaveBeenCalledWith({ name: 'shared', namespace: 'team-b' });
  });
});
