import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  decodeUtf8, inspectReceipt, MAX_SOURCE_BYTES, readReviewFile,
  replayImported, replayInstructions, standaloneRepairRaw,
} from '../src/lib/reviewArtifacts';
import type { ImportedFile } from '../src/lib/reviewArtifacts';
import { comparisonSnapshot, repairSnapshot } from './fixtures';

const file = (text: string, name = 'source.acl'): ImportedFile => ({ name, text, bytes: new TextEncoder().encode(text) });
afterEach(() => vi.unstubAllGlobals());

describe('independent review artifacts', () => {
  it('roundtrips BOM, CRLF, blank lines and no-final-newline without editing source bytes', () => {
    for (const input of ['\uFEFFip access-list extended TEST\r\n permit ip any any', 'one\r\n\r\ntwo\r\n', 'no-final-newline']) {
      const bytes = new TextEncoder().encode(input);
      expect(decodeUtf8(bytes)).toBe(input);
      expect(new TextEncoder().encode(decodeUtf8(bytes))).toEqual(bytes);
    }
    expect(() => decodeUtf8(new Uint8Array([0xc0, 0xaf]))).toThrow();
  });

  it('refuses oversized files before reading or allocating their content', async () => {
    const arrayBuffer = vi.fn();
    await expect(readReviewFile({ size: MAX_SOURCE_BYTES + 1, arrayBuffer } as unknown as File, MAX_SOURCE_BYTES)).rejects.toThrow('FILE_TOO_LARGE');
    expect(arrayBuffer).not.toHaveBeenCalled();
  });

  it('extracts the already sealed AI repair as a standalone receipt, without changing its hash', () => {
    const receipt = repairSnapshot.receipt;
    const wrapper = JSON.stringify({ schema: 'plenilune.session/v1', repairs: [receipt] });
    const exported = standaloneRepairRaw(receipt, wrapper);
    expect(JSON.parse(exported)).toEqual(receipt);
    expect(JSON.parse(exported).sha256).toBe(receipt.sha256);
    expect(() => inspectReceipt(wrapper)).toThrow('RECEIPT_INVALID');
    const raw = `\n${JSON.stringify(receipt)}\n`;
    expect(standaloneRepairRaw(receipt, raw)).toBe(raw);
  });

  it('sends exact original source text and verbatim receipt JSON to offline replay only', async () => {
    const evidence = comparisonSnapshot.comparison;
    // A duplicate key must reach the Go decoder instead of being silently lost.
    const raw = JSON.stringify(evidence).replace('"schema":', '"schema":"duplicate", "schema":');
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ status: 'COMPARISON_REPLAYED', sha256: evidence.sha256, rechecked_witnesses: 1 })));
    vi.stubGlobal('fetch', fetch);
    const before = file('\uFEFFbefore\r\n');
    const after = file('after\r\nno-final-newline');
    await replayImported(before, after, file(raw, 'comparison.json'));
    expect(fetch).toHaveBeenCalledTimes(1);
    const [url, init] = fetch.mock.calls[0];
    expect(url).toBe('/api/compare/replay');
    expect(init.body).toContain(`"evidence":${raw}`);
    expect(JSON.parse(init.body).before).toBe(before.text);
    expect(JSON.parse(init.body).after).toBe(after.text);
  });

  it('does not confuse replaying a rejected result with approving it and distinguishes timing sensitivity', async () => {
    const receipt = { ...repairSnapshot.receipt, status: 'REJECTED' as const };
    const raw = JSON.stringify(receipt);
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ status: 'TIMING_SENSITIVE', sha256: receipt.sha256, rechecked_witnesses: 0 })));
    vi.stubGlobal('fetch', fetch);
    const result = await replayImported(file('before'), file('after'), file(raw));
    expect(result.status).toBe('TIMING_SENSITIVE');
    expect(inspectReceipt(raw).originalOutcome).toBe('REJECTED');
    expect(fetch.mock.calls[0][0]).toBe('/api/repair/replay');
  });

  it('rejects an unrelated replay response and preserves an explicit mismatch error', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ status: 'REPAIR_REPLAYED', sha256: '0'.repeat(64), rechecked_witnesses: 0 })))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: { code: 'EVIDENCE_MISMATCH' } }), { status: 400 }));
    vi.stubGlobal('fetch', fetch);
    const replay = () => replayImported(file('before'), file('after'), file(JSON.stringify(repairSnapshot.receipt)));
    await expect(replay()).rejects.toMatchObject({ code: 'RESPONSE_INVALID' });
    await expect(replay()).rejects.toMatchObject({ code: 'EVIDENCE_MISMATCH' });
  });

  it('writes runnable commands for the actual downloaded filenames in both languages', () => {
    for (const locale of ['vi', 'en'] as const) {
      const instructions = replayInstructions(locale, true, '0.2.0-dev');
      expect(instructions).toContain('--before before.acl --after after.acl --evidence comparison.json');
      expect(instructions).toContain('--evidence repair.json');
      expect(instructions).toContain('TIMING_SENSITIVE');
    }
  });
});
