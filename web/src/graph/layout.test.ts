import { describe, expect, it } from 'vitest';
import { deterministicInitialPosition, reducedMotionLayout } from './layout';

describe('deterministicInitialPosition', () => {
  it('returns the same position for the same node identity', () => {
    const first = deterministicInitialPosition('device/driver-a/pool-a/dev-0', 800, 500);
    const second = deterministicInitialPosition('device/driver-a/pool-a/dev-0', 800, 500);

    expect(second).toEqual(first);
  });

  it('hashes Unicode identities by code point deterministically', () => {
    const identity = 'device/driver/pool/gpu-😀';
    expect(deterministicInitialPosition(identity, 800, 500)).toEqual(
      deterministicInitialPosition(identity, 800, 500),
    );
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

describe('reducedMotionLayout', () => {
  const ids = Array.from({ length: 19 }, (_, i) => `claim/draforge-e2e/claim-${i}`);

  it('places dense graphs in stable non-overlapping rows and columns', () => {
    const first = reducedMotionLayout(ids, 800, 500);
    const reversed = reducedMotionLayout([...ids].reverse(), 800, 500);
    expect(first).toEqual(reversed);
    expect(first.positions.size).toBe(19);
    for (const [id, position] of first.positions) {
      expect(position.x).toBeGreaterThanOrEqual(60);
      expect(position.x).toBeLessThanOrEqual(740);
      expect(position.y).toBeGreaterThanOrEqual(60);
      for (const [other, neighbor] of first.positions) {
        if (id === other) continue;
        expect(Math.hypot(position.x - neighbor.x, position.y - neighbor.y)).toBeGreaterThanOrEqual(100);
      }
    }
  });

  it('grows vertically for large graphs without compressing click targets', () => {
    const large = reducedMotionLayout(Array.from({ length: 105 }, (_, i) => `node/${i}`), 800, 500);
    expect(large.height).toBeGreaterThan(1600);
    expect(large.positions.size).toBe(105);
    expect(large.positions.get('node/3')).toEqual(reducedMotionLayout(
      Array.from({ length: 105 }, (_, i) => `node/${i}`), 800, 500,
    ).positions.get('node/3'));
  });
});
