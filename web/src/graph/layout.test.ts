import { describe, expect, it } from 'vitest';
import { deterministicInitialPosition } from './layout';

describe('deterministicInitialPosition', () => {
  it('returns the same position for the same node identity', () => {
    const first = deterministicInitialPosition('device/driver-a/pool-a/dev-0', 800, 500);
    const second = deterministicInitialPosition('device/driver-a/pool-a/dev-0', 800, 500);

    expect(second).toEqual(first);
  });

  it('keeps positions inside the requested spread and separates identities', () => {
    const first = deterministicInitialPosition('device/driver-a/pool-a/dev-0', 800, 500);
    const second = deterministicInitialPosition('device/driver-b/pool-a/dev-0', 800, 500);

    expect(first).not.toEqual(second);
    expect(first.x).toBeGreaterThanOrEqual(350);
    expect(first.x).toBeLessThanOrEqual(450);
    expect(first.y).toBeGreaterThanOrEqual(200);
    expect(first.y).toBeLessThanOrEqual(300);
  });
});
