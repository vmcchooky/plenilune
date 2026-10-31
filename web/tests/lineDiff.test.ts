import { describe, expect, it } from 'vitest';
import { changedLines } from '../src/lib/lineDiff';

describe('changedLines', () => {
  it('marks an inserted line only on the after side', () => {
    const diff = changedLines(['a', 'c'], ['a', 'b', 'c']);
    expect([...diff.before]).toEqual([]);
    expect([...diff.after]).toEqual([2]);
  });

  it('marks a removed line only on the before side', () => {
    const diff = changedLines(['a', 'b', 'c'], ['a', 'c']);
    expect([...diff.before]).toEqual([2]);
    expect([...diff.after]).toEqual([]);
  });

  it('reports no changes for identical inputs', () => {
    const diff = changedLines(['a', 'b', 'c'], ['a', 'b', 'c']);
    expect(diff.before.size).toBe(0);
    expect(diff.after.size).toBe(0);
  });

  it('does not mark the shadowed permit as changed when a deny is inserted before it', () => {
    const before = ['header', 'remark', 'permit-22', 'permit-443', 'deny-any', '!'];
    const after = ['header', 'remark', 'deny-22', 'permit-22', 'permit-443', 'deny-any-renumbered', '!'];
    const diff = changedLines(before, after);
    expect(diff.after.has(3)).toBe(true);
    expect(diff.before.has(3)).toBe(false);
  });

  it('falls back to prefix/suffix for inputs above the LCS limit', () => {
    const before = Array.from({ length: 1301 }, (_, index) => `line-${index}`);
    const after = [...before];
    after[1300] = 'line-changed';
    const diff = changedLines(before, after);
    expect([...diff.before]).toEqual([1301]);
    expect([...diff.after]).toEqual([1301]);
  });

  it('keeps a large identical input change-free through the fallback', () => {
    const lines = Array.from({ length: 1301 }, (_, index) => `line-${index}`);
    const diff = changedLines(lines, [...lines]);
    expect(diff.before.size).toBe(0);
    expect(diff.after.size).toBe(0);
  });
});
