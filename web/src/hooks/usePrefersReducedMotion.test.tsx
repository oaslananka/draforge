import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import usePrefersReducedMotion from './usePrefersReducedMotion';

interface MockMediaQueryList extends MediaQueryList {
  setMatches(value: boolean): void;
}

function createMediaQueryList(initialMatches: boolean): MockMediaQueryList {
  let matches = initialMatches;
  const listeners = new Set<(event: MediaQueryListEvent) => void>();

  return {
    media: '(prefers-reduced-motion: reduce)',
    onchange: null,
    get matches() {
      return matches;
    },
    addEventListener: (_type: string, listener: EventListenerOrEventListenerObject) => {
      listeners.add(listener as (event: MediaQueryListEvent) => void);
    },
    removeEventListener: (_type: string, listener: EventListenerOrEventListenerObject) => {
      listeners.delete(listener as (event: MediaQueryListEvent) => void);
    },
    addListener: () => undefined,
    removeListener: () => undefined,
    dispatchEvent: () => true,
    setMatches(value: boolean) {
      matches = value;
      const event = { matches, media: this.media } as MediaQueryListEvent;
      listeners.forEach((listener) => listener(event));
    },
  };
}

describe('usePrefersReducedMotion', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('tracks the browser reduced-motion preference', () => {
    const mediaQuery = createMediaQueryList(true);
    vi.stubGlobal('matchMedia', vi.fn(() => mediaQuery));

    const { result } = renderHook(() => usePrefersReducedMotion());
    expect(result.current).toBe(true);

    act(() => mediaQuery.setMatches(false));
    expect(result.current).toBe(false);
  });
});
