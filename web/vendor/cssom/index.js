class LocalStyleSheet {
  constructor(source) {
    this.cssRules = [];
    this.rules = this.cssRules;
    this.disabled = false;
    this.href = null;
    this.media = { length: 0, mediaText: '' };
    this.ownerRule = null;
    this.source = source;
  }

  insertRule(rule, index = this.cssRules.length) {
    const entry = { cssText: String(rule) };
    this.cssRules.splice(index, 0, entry);
    return index;
  }

  deleteRule(index) {
    this.cssRules.splice(index, 1);
  }
}

export function parse(source = '') {
  return new LocalStyleSheet(String(source));
}
