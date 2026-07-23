'use strict';

function uhyphen(value) {
  return String(value)
    .replace(/(([A-Z0-9])([A-Z0-9][a-z]))|(([a-z0-9]+)([A-Z]))/g, '$2$5-$3$6')
    .toLowerCase();
}

module.exports = uhyphen;
module.exports.default = uhyphen;
