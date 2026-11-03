import { describe, expect, it } from 'vitest';
import { extractObjectSource, originalLineInRanges, originalSourceLine } from '../src/lib/sourceMapping';
import type { Extent } from '../src/lib/types';

async function digest(text: string) {
  const bytes = new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text)));
  return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
}

describe('exact object source extraction', () => {
  it('concatenates discontiguous CRLF rows without a synthetic header or normalized EOL', async () => {
    const first = 'access-list 42 deny any\r\n';
    const last = 'access-list 42 permit host 192.0.2.7';
    const source = `! café\r\n${first}ip prefix-list OTHER permit 203.0.113.0/24\r\n${last}`;
    const extent: Extent = { lines: [[2, 2], [4, 4]], sha256: await digest(first + last), units: 2 };
    await expect(extractObjectSource(source, extent)).resolves.toEqual({ source: first + last, sourceLines: [2, 4] });
  });

  it('uses byte offsets after a BOM and multibyte UTF8 prefix', async () => {
    const prefix = '\uFEFF! Tiếng Việt 🌙\r\n';
    const object = 'ip access-list standard X\r\n permit any\r\n';
    const start = new TextEncoder().encode(prefix).length;
    await expect(extractObjectSource(prefix + object + '!\r\n', {
      lines: [[2, 3]], byte_start: start, byte_end: start + new TextEncoder().encode(object).length, sha256: await digest(object),
    })).resolves.toEqual({ source: object, sourceLines: [2, 3] });
  });

  it('preserves a selected leading BOM and the existing standalone byte slice', async () => {
    const source = '\uFEFFaccess-list 42 permit any\n';
    await expect(extractObjectSource(source, { lines: [[1, 1]], byte_start: 0, byte_end: new TextEncoder().encode(source).length, sha256: await digest(source) })).resolves.toEqual({ source, sourceLines: [1] });
  });

  for (const lines of [[], [[0, 1]], [[2, 1]], [[1, 2], [2, 2]], [[2, 2], [1, 1]], [[1, 3]], [[1.5, 2]], [[1, Number.MAX_SAFE_INTEGER + 1]]]) {
    it(`rejects invalid/overlapping/reversed/OOB range ${JSON.stringify(lines)}`, async () => {
      await expect(extractObjectSource('one\r\ntwo\r\n', { lines: lines as [number, number][], sha256: await digest('one\r\ntwo\r\n') })).rejects.toMatchObject({ code: 'SOURCE_SLICE_MISMATCH' });
    });
  }

  it('rejects mismatched digest, partial offsets, invalid offsets and offset/range disagreement', async () => {
    const source = 'one\ntwo\n';
    const extent = { lines: [[1, 2]] as [number, number][], sha256: await digest(source) };
    for (const change of [{ sha256: '0'.repeat(64) }, { byte_start: 0 }, { byte_end: 8 }, { byte_start: -1, byte_end: 8 }, { byte_start: 0.5, byte_end: 8 }, { byte_start: 0, byte_end: 99 }, { byte_start: 4, byte_end: 8 }]) {
      await expect(extractObjectSource(source, { ...extent, ...change })).rejects.toMatchObject({ code: 'SOURCE_SLICE_MISMATCH' });
    }
  });
});

it('maps local physical lines independently from rule ordinal and retains affine fallback', () => {
  expect(originalSourceLine(2, [2, 7], 2)).toBe(7);
  expect(originalSourceLine(3, [2, 7], 2)).toBeUndefined();
  expect(originalSourceLine(2, undefined, 9)).toBe(10);
  expect(originalSourceLine(undefined, [2, 7])).toBeUndefined();
  expect(originalLineInRanges(3, [[2, 3], [9, 10]])).toBe(9);
  expect(originalLineInRanges(5, [[2, 3], [9, 10]])).toBeUndefined();
  expect(originalLineInRanges(1, [[3, 2]])).toBeUndefined();
});
