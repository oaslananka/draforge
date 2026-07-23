import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ResourceGraph } from '../api/types';
import useSSE from './useSSE';

class MockEventSource {
  static instances: MockEventSource[] = [];

  readonly url: string;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  close = vi.fn();

  constructor(url: string | URL) {
    this.url = String(url);
    MockEventSource.instances.push(this);
  }
}

describe('useSSE', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    MockEventSource.instances = [];
    vi.stubGlobal('EventSource', MockEventSource);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('reports connection state, delivers graph updates, and reconnects after errors', () => {
    const onGraph = vi.fn<(graph: ResourceGraph) => void>();
    const { result, unmount } = renderHook(() => useSSE(onGraph));

    expect(result.current).toBe('disconnected');
    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.instances[0].url).toBe('/api/stream');

    act(() => {
      MockEventSource.instances[0].onopen?.(new Event('open'));
    });
    expect(result.current).toBe('connected');

    const graph: ResourceGraph = {
      nodes: [{ id: 'claim/team-a/shared', type: 'ResourceClaim', label: 'shared' }],
      edges: [],
    };
    act(() => {
      MockEventSource.instances[0].onmessage?.(
        new MessageEvent('message', { data: JSON.stringify(graph) }),
      );
    });
    expect(onGraph).toHaveBeenCalledWith(graph);

    act(() => {
      MockEventSource.instances[0].onerror?.(new Event('error'));
    });
    expect(result.current).toBe('reconnecting');
    expect(MockEventSource.instances[0].close).toHaveBeenCalledOnce();

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(MockEventSource.instances).toHaveLength(2);

    act(() => {
      MockEventSource.instances[1].onopen?.(new Event('open'));
    });
    expect(result.current).toBe('connected');

    unmount();
    expect(MockEventSource.instances[1].close).toHaveBeenCalledOnce();
  });

  it('ignores malformed stream payloads without dropping the connection', () => {
    const onGraph = vi.fn<(graph: ResourceGraph) => void>();
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const { result } = renderHook(() => useSSE(onGraph));

    act(() => {
      MockEventSource.instances[0].onopen?.(new Event('open'));
      MockEventSource.instances[0].onmessage?.(
        new MessageEvent('message', { data: '{not-json' }),
      );
    });

    expect(result.current).toBe('connected');
    expect(onGraph).not.toHaveBeenCalled();
    expect(consoleError).toHaveBeenCalledWith('Failed to parse SSE graph stream');
    consoleError.mockRestore();
  });
});
