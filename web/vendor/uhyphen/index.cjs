'use strict';

const isUpper = (code) => code >= 65 && code <= 90;
const isLower = (code) => code >= 97 && code <= 122;
const isDigit = (code) => code >= 48 && code <= 57;
const isUpperOrDigit = (code) => isUpper(code) || isDigit(code);
const isLowerOrDigit = (code) => isLower(code) || isDigit(code);

function uhyphen(value) {
  const input = String(value);
  const output = [];
  let index = 0;

  while (index < input.length) {
    const first = input.charCodeAt(index);
    if (
      isUpperOrDigit(first)
      && isUpperOrDigit(input.charCodeAt(index + 1))
      && isLower(input.charCodeAt(index + 2))
    ) {
      output.push(input[index], '-', input[index + 1], input[index + 2]);
      index += 3;
      continue;
    }

    if (isLowerOrDigit(first)) {
      let runEnd = index + 1;
      while (runEnd < input.length && isLowerOrDigit(input.charCodeAt(runEnd))) {
        runEnd += 1;
      }

      if (isUpper(input.charCodeAt(runEnd))) {
        output.push(input.slice(index, runEnd), '-', input[runEnd]);
        index = runEnd + 1;
        continue;
      }

      let nextAcronymBoundary = -1;
      for (let cursor = index + 1; cursor + 2 < runEnd; cursor += 1) {
        if (
          isDigit(input.charCodeAt(cursor))
          && isDigit(input.charCodeAt(cursor + 1))
          && isLower(input.charCodeAt(cursor + 2))
        ) {
          nextAcronymBoundary = cursor;
          break;
        }
      }

      if (nextAcronymBoundary >= 0) {
        output.push(input.slice(index, nextAcronymBoundary));
        index = nextAcronymBoundary;
        continue;
      }

      output.push(input.slice(index, runEnd));
      index = runEnd;
      continue;
    }

    output.push(input[index]);
    index += 1;
  }

  return output.join('').toLowerCase();
}

module.exports = uhyphen;
module.exports.default = uhyphen;
