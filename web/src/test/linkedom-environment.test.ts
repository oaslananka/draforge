import { describe, expect, it } from 'vitest';

describe('repository-owned linkedom environment', () => {
  it('provides stateful document selection for user interactions', () => {
    const button = document.createElement('button');
    document.body.appendChild(button);

    const selection = document.getSelection();
    expect(selection).not.toBeNull();

    selection?.setBaseAndExtent(button, 0, button, 0);
    expect(selection?.focusNode).toBe(button);
    expect(selection?.isCollapsed).toBe(true);
  });

  it('uses the local CSSOM adapter for style sheet access', () => {
    const style = document.createElement('style');
    style.textContent = '.card { display: grid; }';
    document.head.appendChild(style);

    const sheet = style.sheet;
    expect(sheet).not.toBeNull();
    expect(sheet?.cssRules).toHaveLength(0);
    expect(sheet?.insertRule('.badge { display: inline-flex; }')).toBe(0);
    expect(sheet?.cssRules).toHaveLength(1);

    sheet?.deleteRule(0);
    expect(sheet?.cssRules).toHaveLength(0);
  });
});
