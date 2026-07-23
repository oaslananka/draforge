'use strict';

const escapeEntities = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  "'": '&#39;',
  '"': '&quot;',
};

const unescapeEntities = {
  '&amp;': '&',
  '&#38;': '&',
  '&lt;': '<',
  '&#60;': '<',
  '&gt;': '>',
  '&#62;': '>',
  '&apos;': "'",
  '&#39;': "'",
  '&quot;': '"',
  '&#34;': '"',
};

function escape(value) {
  return String(value).replace(/[&<>'"]/g, (character) => escapeEntities[character]);
}

function unescape(value) {
  return String(value).replace(
    /&(?:amp|#38|lt|#60|gt|#62|apos|#39|quot|#34);/g,
    (entity) => unescapeEntities[entity],
  );
}

module.exports = { escape, unescape };
