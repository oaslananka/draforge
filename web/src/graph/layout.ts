export interface GraphPoint {
  readonly x: number;
  readonly y: number;
}

function hashString(value: string): number {
  let hash = 0x811c9dc5;
  for (const symbol of value) {
    hash ^= symbol.codePointAt(0) ?? 0;
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

function unitInterval(value: string): number {
  return hashString(value) / 0xffffffff;
}

export function deterministicInitialPosition(
  id: string,
  width: number,
  height: number,
  spread = 100,
): GraphPoint {
  return {
    x: width / 2 + (unitInterval(`x:${id}`) - 0.5) * spread,
    y: height / 2 + (unitInterval(`y:${id}`) - 0.5) * spread,
  };
}

// The physics solver spreads nodes apart during normal motion. A reduced-motion
// preference disables that solver, so the original compact hash positions are
// not a usable static layout for anything beyond a tiny graph.
//
// Give every node a stable cell, independent of API arrival order or new SSE
// frames. Use a vertical canvas when more rows are needed; do not compress
// hit targets to fit a fixed 500px box.
export function reducedMotionLayout(
  ids: readonly string[],
  width: number,
  minimumHeight: number,
): { positions: ReadonlyMap<string, GraphPoint>; height: number } {
  const horizontalMargin = 80;
  const verticalMargin = 65;
  const columnSpacing = 160;
  const rowSpacing = 100;
  const columns = Math.max(1, Math.floor((width - 2 * horizontalMargin) / columnSpacing) + 1);
  const sortedIDs = [...new Set(ids)].sort();
  const positions = new Map<string, GraphPoint>();

  sortedIDs.forEach((id, index) => {
    positions.set(id, {
      x: horizontalMargin + (index % columns) * columnSpacing,
      y: verticalMargin + Math.floor(index / columns) * rowSpacing,
    });
  });

  const rows = Math.ceil(sortedIDs.length / columns);
  const height = Math.max(minimumHeight, 2 * verticalMargin + Math.max(0, rows - 1) * rowSpacing);
  return { positions, height };
}
