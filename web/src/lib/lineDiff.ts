export function changedLines(before: string[], after: string[]) {
  if (before.length > 1200 || after.length > 1200) return fallbackDiff(before, after);
  const rows = before.length + 1;
  const cols = after.length + 1;
  const table = Array.from({ length: rows }, () => new Uint16Array(cols));
  for (let i = before.length - 1; i >= 0; i--) {
    for (let j = after.length - 1; j >= 0; j--) {
      table[i][j] = before[i] === after[j]
        ? table[i + 1][j + 1] + 1
        : Math.max(table[i + 1][j], table[i][j + 1]);
    }
  }
  const beforeChanged = new Set<number>();
  const afterChanged = new Set<number>();
  let i = 0;
  let j = 0;
  while (i < before.length && j < after.length) {
    if (before[i] === after[j]) {
      i++;
      j++;
    } else if (table[i + 1][j] >= table[i][j + 1]) {
      beforeChanged.add(++i);
    } else {
      afterChanged.add(++j);
    }
  }
  while (i < before.length) beforeChanged.add(++i);
  while (j < after.length) afterChanged.add(++j);
  return { before: beforeChanged, after: afterChanged };
}

function fallbackDiff(before: string[], after: string[]) {
  let prefix = 0;
  while (prefix < before.length && prefix < after.length && before[prefix] === after[prefix]) prefix++;
  let suffix = 0;
  while (suffix < before.length - prefix && suffix < after.length - prefix && before[before.length - 1 - suffix] === after[after.length - 1 - suffix]) suffix++;
  return {
    before: new Set(before.slice(prefix, before.length - suffix).map((_, index) => prefix + index + 1)),
    after: new Set(after.slice(prefix, after.length - suffix).map((_, index) => prefix + index + 1)),
  };
}
