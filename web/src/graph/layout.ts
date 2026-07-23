export interface GraphPoint {
  readonly x: number;
  readonly y: number;
}

function hashString(value: string): number {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
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
