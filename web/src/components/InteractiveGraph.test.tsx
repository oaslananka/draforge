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
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: false,
      media: '(prefers-reduced-motion: reduce)',
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(() => true),
    })));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('lets keyboard users inspect a claim and open its namespace-qualified explanation', async () => {
    const user = userEvent.setup();
    const onSelectClaim = vi.fn();
    render(<InteractiveGraph graphData={graph} onSelectClaim={onSelectClaim} />);

    const claimNode = await screen.findByRole('button', {
      name: 'ResourceClaim shared in namespace team-b, status Allocated',
    });
    claimNode.focus();
    fireEvent.keyDown(claimNode, { key: 'Enter' });

    const diagnose = screen.getByRole('button', { name: 'Diagnose Allocation' });
    await user.click(diagnose);

    expect(onSelectClaim).toHaveBeenCalledWith({ name: 'shared', namespace: 'team-b' });
  });

  it('stops graph animation and exposes selected state when reduced motion is requested', () => {
    const requestAnimationFrame = vi.fn(() => 1);
    vi.stubGlobal('requestAnimationFrame', requestAnimationFrame);
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: true,
      media: '(prefers-reduced-motion: reduce)',
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(() => true),
    })));

    render(<InteractiveGraph graphData={graph} onSelectClaim={vi.fn()} />);

    const claimNode = screen.getByRole('button', {
      name: 'ResourceClaim shared in namespace team-b, status Allocated',
    });
    expect(claimNode.getAttribute('aria-pressed')).toBe('false');
    fireEvent.click(claimNode);
    expect(claimNode.getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByText('Reduced motion is active; graph nodes use fixed positions.').isConnected).toBe(true);
    expect(requestAnimationFrame).not.toHaveBeenCalled();
  });

});
