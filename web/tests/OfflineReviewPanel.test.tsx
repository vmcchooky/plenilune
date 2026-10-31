import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { LocaleProvider } from '../src/i18n';
import { OfflineReviewPanel } from '../src/components/OfflineReviewPanel';
import { comparisonSnapshot } from './fixtures';

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

function inputFile(name: string, text: string): File {
  const bytes = new TextEncoder().encode(text);
  return { name, size: bytes.byteLength, arrayBuffer: async () => bytes.buffer } as File;
}

async function upload(label: RegExp, name: string, text: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { files: [inputFile(name, text)] } });
  await waitFor(() => expect(screen.getByText(new RegExp(name))).toBeTruthy());
}

async function prepare() {
  window.localStorage.setItem('plenilune.locale', 'en');
  render(<LocaleProvider><OfflineReviewPanel /></LocaleProvider>);
  await upload(/Before configuration/, 'before.acl', 'before\r\n');
  await upload(/After configuration/, 'after.acl', 'after');
  await upload(/Standalone receipt/, 'comparison.json', JSON.stringify(comparisonSnapshot.comparison));
}

describe('fresh-session review', () => {
  it('requires explicit replay and cannot call AI merely by importing files', async () => {
    const fetch = vi.fn();
    vi.stubGlobal('fetch', fetch);
    await prepare();
    expect(screen.getByText('Not replayed')).toBeTruthy();
    expect(fetch).not.toHaveBeenCalled();
  });

  it('ignores late responses after original source files change, even when transport ignores abort', async () => {
    let resolve!: (response: Response) => void;
    const fetch = vi.fn().mockImplementation(() => new Promise<Response>((done) => { resolve = done; }));
    vi.stubGlobal('fetch', fetch);
    await prepare();
    fireEvent.click(screen.getByRole('button', { name: 'Replay imported files' }));
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    const requestSignal = fetch.mock.calls[0][1].signal as AbortSignal;
    await upload(/After configuration/, 'changed.acl', 'changed');
    expect(requestSignal.aborted).toBe(true);
    await act(async () => resolve(new Response(JSON.stringify({ status: 'COMPARISON_REPLAYED', sha256: comparisonSnapshot.comparison.sha256, rechecked_witnesses: 1 }))));
    expect(screen.getByText('Not replayed')).toBeTruthy();
    expect(screen.queryByText(/Original outcome reproduced/)).toBeNull();
  });

  it('keeps mismatch separate from a provider/network failure and from a successful replay', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: 'EVIDENCE_MISMATCH' } }), { status: 400 })));
    await prepare();
    fireEvent.click(screen.getByRole('button', { name: 'Replay imported files' }));
    await waitFor(() => expect(screen.getByText(/Receipt does not match source bytes/)).toBeTruthy());
    expect(screen.getByText(/Original outcome \(replay does not upgrade it\)/)).toBeTruthy();
    expect(screen.queryByText(/Original outcome reproduced/)).toBeNull();
  });
});
