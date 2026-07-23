import { parseHTML } from 'linkedom';
import type { Environment } from 'vitest/runtime';

const constructorKeys = [
  'Attr',
  'Comment',
  'Document',
  'DocumentFragment',
  'DOMParser',
  'Element',
  'Event',
  'EventTarget',
  'HTMLElement',
  'HTMLAnchorElement',
  'HTMLButtonElement',
  'HTMLFormElement',
  'HTMLInputElement',
  'HTMLLabelElement',
  'HTMLOptionElement',
  'HTMLSelectElement',
  'HTMLTextAreaElement',
  'MutationObserver',
  'Node',
  'NodeList',
  'SVGElement',
  'Text',
] as const;

type PropertySnapshot =
  | { present: false }
  | { present: true; descriptor: PropertyDescriptor };

function styleFor(element: Element) {
  const inlineStyle = (element as HTMLElement).style;
  return {
    display: inlineStyle?.display ?? '',
    visibility: inlineStyle?.visibility ?? '',
    pointerEvents: inlineStyle?.pointerEvents ?? '',
    getPropertyValue(property: string) {
      return inlineStyle?.getPropertyValue(property) ?? '';
    },
  };
}

class TestRange {
  startContainer: Node;
  startOffset = 0;
  endContainer: Node;
  endOffset = 0;

  constructor(private readonly document: Document) {
    this.startContainer = document.body;
    this.endContainer = document.body;
  }

  get collapsed() {
    return this.startContainer === this.endContainer && this.startOffset === this.endOffset;
  }

  get commonAncestorContainer() {
    return this.startContainer === this.endContainer
      ? this.startContainer
      : this.document.body;
  }

  setStart(node: Node, offset: number) {
    this.startContainer = node;
    this.startOffset = Math.max(0, offset);
  }

  setEnd(node: Node, offset: number) {
    this.endContainer = node;
    this.endOffset = Math.max(0, offset);
  }

  setStartBefore(node: Node) {
    const parent = node.parentNode ?? this.document.body;
    this.setStart(parent, Array.from(parent.childNodes as NodeListOf<Node>).indexOf(node));
  }

  setStartAfter(node: Node) {
    const parent = node.parentNode ?? this.document.body;
    this.setStart(parent, Array.from(parent.childNodes as NodeListOf<Node>).indexOf(node) + 1);
  }

  setEndBefore(node: Node) {
    const parent = node.parentNode ?? this.document.body;
    this.setEnd(parent, Array.from(parent.childNodes as NodeListOf<Node>).indexOf(node));
  }

  setEndAfter(node: Node) {
    const parent = node.parentNode ?? this.document.body;
    this.setEnd(parent, Array.from(parent.childNodes as NodeListOf<Node>).indexOf(node) + 1);
  }

  selectNode(node: Node) {
    this.setStartBefore(node);
    this.setEndAfter(node);
  }

  selectNodeContents(node: Node) {
    let length = node.childNodes.length;
    if (node.nodeType === 3) {
      length = node.textContent?.length ?? 0;
    }
    this.setStart(node, 0);
    this.setEnd(node, length);
  }

  cloneRange() {
    const clone = new TestRange(this.document);
    clone.setStart(this.startContainer, this.startOffset);
    clone.setEnd(this.endContainer, this.endOffset);
    return clone;
  }

  comparePoint(node: Node, offset: number) {
    if (node === this.startContainer && offset < this.startOffset) return -1;
    if (node === this.endContainer && offset > this.endOffset) return 1;
    return 0;
  }

  detach() {
    this.setStart(this.document.body, 0);
    this.setEnd(this.document.body, 0);
  }

  deleteContents() {
    if (this.startContainer === this.endContainer && this.startContainer.nodeType === 3) {
      const text = this.startContainer.textContent ?? '';
      this.startContainer.textContent = text.slice(0, this.startOffset) + text.slice(this.endOffset);
    }
    this.setEnd(this.startContainer, this.startOffset);
  }

  extractContents() {
    return this.document.createDocumentFragment();
  }

  cloneContents() {
    return this.document.createDocumentFragment();
  }

  insertNode(node: Node) {
    this.startContainer.appendChild(node);
  }

  surroundContents(node: Node) {
    node.appendChild(this.cloneContents());
    this.insertNode(node);
  }

  createContextualFragment(markup: string) {
    const template = this.document.createElement('template');
    template.innerHTML = markup;
    return template.content;
  }

  isPointInRange(node: Node, offset: number) {
    return this.comparePoint(node, offset) === 0;
  }

  intersectsNode(node: Node) {
    return node === this.startContainer || node === this.endContainer;
  }

  compareBoundaryPoints() {
    return 0;
  }

  toString() {
    if (this.startContainer !== this.endContainer) return '';
    const text = this.startContainer.textContent ?? '';
    return text.slice(this.startOffset, this.endOffset);
  }
}

class TestSelection {
  anchorNode: Node | null = null;
  anchorOffset = 0;
  focusNode: Node | null = null;
  focusOffset = 0;
  private ranges: Range[] = [];

  get isCollapsed() {
    return this.anchorNode === this.focusNode && this.anchorOffset === this.focusOffset;
  }

  get rangeCount() {
    return this.ranges.length;
  }

  get type() {
    if (this.rangeCount === 0) return 'None';
    return this.isCollapsed ? 'Caret' : 'Range';
  }

  addRange(range: Range) {
    this.ranges = [range];
    this.anchorNode = range.startContainer;
    this.anchorOffset = range.startOffset;
    this.focusNode = range.endContainer;
    this.focusOffset = range.endOffset;
  }

  removeAllRanges() {
    this.ranges = [];
    this.anchorNode = null;
    this.anchorOffset = 0;
    this.focusNode = null;
    this.focusOffset = 0;
  }

  getRangeAt(index: number) {
    const range = this.ranges[index];
    if (!range) throw new DOMException('Invalid range index', 'IndexSizeError');
    return range;
  }

  setBaseAndExtent(
    anchorNode: Node,
    anchorOffset: number,
    focusNode: Node,
    focusOffset: number,
  ) {
    this.anchorNode = anchorNode;
    this.anchorOffset = anchorOffset;
    this.focusNode = focusNode;
    this.focusOffset = focusOffset;
  }

  collapse(node: Node | null, offset = 0) {
    if (!node) {
      this.removeAllRanges();
      return;
    }
    this.setBaseAndExtent(node, offset, node, offset);
  }

  collapseToStart() {
    if (this.anchorNode) this.collapse(this.anchorNode, this.anchorOffset);
  }

  collapseToEnd() {
    if (this.focusNode) this.collapse(this.focusNode, this.focusOffset);
  }

  extend(node: Node, offset = 0) {
    if (!this.anchorNode) {
      this.setBaseAndExtent(node, offset, node, offset);
      return;
    }
    this.focusNode = node;
    this.focusOffset = offset;
  }

  selectAllChildren(node: Node) {
    this.setBaseAndExtent(node, 0, node, node.childNodes.length);
  }

  toString() {
    if (!this.anchorNode || !this.focusNode || this.isCollapsed) return '';
    return this.anchorNode === this.focusNode ? this.anchorNode.textContent ?? '' : '';
  }
}

const environment: Environment = {
  name: 'linkedom',
  viteEnvironment: 'client',
  setup(global) {
    const { window, document } = parseHTML(
      '<!doctype html><html><head></head><body></body></html>',
    );
    const snapshots = new Map<PropertyKey, PropertySnapshot>();

    const install = (key: PropertyKey, value: unknown) => {
      const descriptor = Object.getOwnPropertyDescriptor(global, key);
      snapshots.set(key, descriptor
        ? { present: true, descriptor }
        : { present: false });
      Object.defineProperty(global, key, {
        configurable: true,
        enumerable: false,
        writable: true,
        value,
      });
    };

    class TestMouseEvent extends window.Event {
      button: number;
      buttons: number;
      clientX: number;
      clientY: number;
      ctrlKey: boolean;
      metaKey: boolean;
      shiftKey: boolean;

      constructor(type: string, init: MouseEventInit = {}) {
        super(type, init);
        this.button = init.button ?? 0;
        this.buttons = init.buttons ?? 0;
        this.clientX = init.clientX ?? 0;
        this.clientY = init.clientY ?? 0;
        this.ctrlKey = init.ctrlKey ?? false;
        this.metaKey = init.metaKey ?? false;
        this.shiftKey = init.shiftKey ?? false;
      }
    }

    class TestKeyboardEvent extends window.Event {
      altKey: boolean;
      code: string;
      ctrlKey: boolean;
      key: string;
      metaKey: boolean;
      shiftKey: boolean;

      constructor(type: string, init: KeyboardEventInit = {}) {
        super(type, init);
        this.altKey = init.altKey ?? false;
        this.code = init.code ?? '';
        this.ctrlKey = init.ctrlKey ?? false;
        this.key = init.key ?? '';
        this.metaKey = init.metaKey ?? false;
        this.shiftKey = init.shiftKey ?? false;
      }
    }

    const selection = new TestSelection();
    const getSelection = () => selection;
    const createRange = () => new TestRange(document) as unknown as Range;
    Object.defineProperties(document, {
      createRange: {
        configurable: true,
        value: createRange,
      },
      getSelection: {
        configurable: true,
        value: getSelection,
      },
    });

    const getComputedStyle = (element: Element) => styleFor(element);
    const requestAnimationFrame = (callback: FrameRequestCallback) =>
      global.setTimeout(() => callback(global.performance.now()), 0);
    const cancelAnimationFrame = (handle: number) => global.clearTimeout(handle);

    Object.assign(window, {
      KeyboardEvent: TestKeyboardEvent,
      MouseEvent: TestMouseEvent,
      PointerEvent: TestMouseEvent,
      cancelAnimationFrame,
      createRange,
      getComputedStyle,
      getSelection,
      requestAnimationFrame,
    });

    install('window', window);
    install('self', window);
    install('document', document);
    install('navigator', window.navigator);
    install('KeyboardEvent', TestKeyboardEvent);
    install('MouseEvent', TestMouseEvent);
    install('PointerEvent', TestMouseEvent);
    install('Range', TestRange);
    install('Selection', TestSelection);
    install('getComputedStyle', getComputedStyle);
    install('getSelection', getSelection);
    install('requestAnimationFrame', requestAnimationFrame);
    install('cancelAnimationFrame', cancelAnimationFrame);

    for (const key of constructorKeys) {
      const value = window[key];
      if (value !== undefined) install(key, value);
    }

    return {
      teardown(target) {
        document.body.replaceChildren();
        for (const [key, snapshot] of snapshots) {
          if (snapshot.present) {
            Object.defineProperty(target, key, snapshot.descriptor);
          } else {
            delete target[key];
          }
        }
      },
    };
  },
};

export default environment;
