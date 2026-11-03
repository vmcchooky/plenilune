import { ApiError } from './api';
import type { Extent } from './types';

export interface SourceLineMap { before: number[]; after: number[] }
export interface FirstSourceLine { before: number; after: number }

function mismatch(): never {
  throw new ApiError('Object location or bytes do not match the analysis.', 'SOURCE_SLICE_MISMATCH');
}

function validRanges(ranges: readonly (readonly number[])[], maxLine: number): boolean {
  if (!Array.isArray(ranges) || ranges.length === 0) return false;
  let previous = 0;
  for (const range of ranges) {
    if (!Array.isArray(range) || range.length !== 2) return false;
    const [start, end] = range;
    if (!Number.isSafeInteger(start) || !Number.isSafeInteger(end) || start <= previous || end < start || end > maxLine) return false;
    previous = end;
  }
  return true;
}

// A local parser line is a physical row of the extracted document, independent
// of effective rule ordinal. No affine guess is made when an explicit map exists.
export function originalSourceLine(local: number | undefined, lines?: readonly number[], firstLine = 1): number | undefined {
  if (local === undefined || !Number.isSafeInteger(local) || local < 1) return undefined;
  const value = lines ? lines[local - 1] : local + firstLine - 1;
  return Number.isSafeInteger(value) && value! > 0 ? value : undefined;
}

// Analysis witnesses also use local extracted-document lines before that object
// has been opened. Walk ranges without allocating potentially untrusted sizes.
export function originalLineInRanges(local: number | undefined, ranges: readonly (readonly number[])[] | undefined): number | undefined {
  if (local === undefined || !Number.isSafeInteger(local) || local < 1 || !ranges || !validRanges(ranges, Number.MAX_SAFE_INTEGER)) return undefined;
  let remaining = local;
  for (const [start, end] of ranges) {
    const count = end - start + 1;
    if (remaining <= count) return start + remaining - 1;
    remaining -= count;
  }
  return undefined;
}

// Exact local extraction: retain original EOL bytes and concatenate only selected
// physical rows for discontiguous numbered ACLs. A contiguous offset path must
// agree with the line ranges too. No synthetic header or newline is introduced.
export async function extractObjectSource(source: string, extent: Pick<Extent, 'lines' | 'byte_start' | 'byte_end' | 'sha256'>): Promise<{ source: string; sourceLines: number[] }> {
  const bytes = new TextEncoder().encode(source);
  const spans: [number, number][] = [];
  for (let start = 0; start < bytes.length;) {
    const newline = bytes.indexOf(10, start);
    const end = newline < 0 ? bytes.length : newline + 1;
    spans.push([start, end]);
    start = end;
  }
  if (!validRanges(extent.lines, spans.length)) mismatch();
  const sourceLines: number[] = [];
  let length = 0;
  for (const [first, last] of extent.lines) {
    for (let line = first; line <= last; line++) {
      sourceLines.push(line);
      length += spans[line - 1][1] - spans[line - 1][0];
    }
  }
  const selected = new Uint8Array(length);
  let position = 0;
  for (const line of sourceLines) {
    const [start, end] = spans[line - 1];
    selected.set(bytes.subarray(start, end), position);
    position += end - start;
  }
  const start = extent.byte_start;
  const end = extent.byte_end;
  if (start !== undefined || end !== undefined) {
    if (!Number.isSafeInteger(start) || !Number.isSafeInteger(end) || start! < 0 || end! > bytes.length || start! >= end!) mismatch();
    const slice = bytes.subarray(start, end);
    if (slice.length !== selected.length || slice.some((value, index) => value !== selected[index])) mismatch();
  }
  if (!/^[a-f0-9]{64}$/.test(extent.sha256)) mismatch();
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', selected));
  const hex = Array.from(digest, b => b.toString(16).padStart(2, '0')).join('');
  if (hex !== extent.sha256) mismatch();
  try {
    return { source: new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(selected), sourceLines };
  } catch { return mismatch(); }
}
