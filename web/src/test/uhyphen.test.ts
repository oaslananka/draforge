import { describe, expect, it } from 'vitest';

function hyphenatedProperty(value: string) {
  const element = document.createElement('div');
  const style = (element as HTMLElement).style as unknown as Record<string, string>;
  style[value] = 'x';
  return element.getAttribute('style')?.split(':', 1)[0] ?? '';
}

describe('repository-owned uhyphen adapter', () => {
  it.each([
    ['camelCase', 'camel-case'],
    ['XMLHttpRequest', 'xml-http-request'],
    ['WebGL2RenderingContext', 'web-gl2-rendering-context'],
    ['aBCd', 'a-bcd'],
    ['already-hyphen', 'already-hyphen'],
  ])('preserves legacy property conversion for %s', (value, expected) => {
    expect(hyphenatedProperty(value)).toBe(expected);
  });

  it('handles long non-matching input without polynomial backtracking', () => {
    const input = `${'0'.repeat(20_000)}!`;
    const startedAt = performance.now();

    expect(hyphenatedProperty(input)).toBe(input);
    expect(performance.now() - startedAt).toBeLessThan(250);
  });
});
