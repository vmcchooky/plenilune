import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { InboxPanel } from '../src/components/InboxPanel';
import { LocaleProvider } from '../src/i18n';
import enMessages from '../src/i18n/messages.en';
import { runtime } from '../src/lib/api';
import type { Artifact } from '../src/lib/api';
import type { InboxDocument, InboxEntry, WatchRule } from '../src/lib/types';

vi.mock('../src/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../src/lib/api')>();
  return {
    ...actual,
    runtime: {
      health: vi.fn(), analyze: vi.fn(), analyzeReplay: vi.fn(), trace: vi.fn(),
      parse: vi.fn(), normalize: vi.fn(), normalizePolicy: vi.fn(), compare: vi.fn(),
      replayComparison: vi.fn(), agentSession: vi.fn(), repairVerify: vi.fn(), repairReplay: vi.fn(),
      chatAsk: vi.fn(),
      listWatches: vi.fn(), replaceWatches: vi.fn(), listInbox: vi.fn(), markInboxRead: vi.fn(),
    },
  };
});

const text = (key: keyof typeof enMessages): string => enMessages[key];
const artifact = <T,>(data: T): Artifact<T> => ({ data, raw: JSON.stringify(data) });

const changedEntry: InboxEntry = {
  id: 7,
  domain: 'acl',
  name: 'EDGE-IN',
  condition: 'verdict_changed',
  previous_verdict: 'EQUIVALENT',
  verdict: 'DIFFERENT',
  artifact_kind: 'analysis',
  content_sha256: 'a'.repeat(64),
  sha256: 'b'.repeat(64),
  time: '2026-11-02T01:02:03Z',
  read: false,
  stale: false,
};

const violatedEntry: InboxEntry = {
  id: 8,
  domain: 'acl',
  name: 'EDGE-IN',
  condition: 'obligation_failed',
  verdict: 'EQUIVALENT',
  violated_obligations: ['OB-1'],
  artifact_kind: 'comparison',
  content_sha256: 'c'.repeat(64),
  time: '2026-11-02T02:03:04Z',
  read: true,
  stale: true,
};

const watch: WatchRule = { domain: 'acl', name: 'EDGE-IN', conditions: ['verdict_changed', 'any_different'] };

function renderPanel() {
  return render(
    <LocaleProvider>
      <InboxPanel refreshKey={null} />
    </LocaleProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  window.localStorage.setItem('plenilune.locale', 'en');
  vi.mocked(runtime.listInbox).mockResolvedValue(artifact<InboxDocument>({ entries: [], unread: 0 }));
  vi.mocked(runtime.listWatches).mockResolvedValue(artifact({ watches: [] as WatchRule[] }));
});

afterEach(() => cleanup());

describe('P2-8 verdict inbox panel', () => {
  it('shows the empty state when nothing fired and nothing is watched', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText(text('inbox.empty'))).toBeTruthy());
    expect(screen.getByText(text('inbox.watch.none'))).toBeTruthy();
    expect(vi.mocked(runtime.listInbox)).toHaveBeenCalled();
  });

  it('renders fired entries with verdict badges, the change line and the artifact digest', async () => {
    vi.mocked(runtime.listInbox).mockResolvedValue(artifact<InboxDocument>({ entries: [changedEntry, violatedEntry], unread: 1 }));
    vi.mocked(runtime.listWatches).mockResolvedValue(artifact({ watches: [watch] }));
    const { container } = renderPanel();
    const list = await waitFor(() => {
      const el = container.querySelector('.inbox-list');
      expect(el).not.toBeNull();
      return el as HTMLElement;
    });
    // Verdict meaning is a badge: DIFFERENT is the bad tone, the
    // superseded entry carries the stale badge.
    const different = [...list.querySelectorAll('.badge')].find((b) => b.textContent === 'DIFFERENT');
    expect(different?.className).toContain('badge--bad');
    expect(within(list).getByText(text('inbox.condition.verdict_changed'))).toBeTruthy();
    expect(within(list).getByText('EQUIVALENT → DIFFERENT')).toBeTruthy();
    expect(within(list).getByText(text('inbox.condition.obligation_failed'))).toBeTruthy();
    expect(within(list).getByText('Violated: OB-1')).toBeTruthy();
    expect(within(list).getByText(text('inbox.stale'))).toBeTruthy();
    expect(within(list).getByText(new RegExp('a'.repeat(10)))).toBeTruthy();
    expect(screen.getByText('1 unread')).toBeTruthy();
    // The watch list renders the pinned object with its conditions.
    const watchList = container.querySelector('.watch-list') as HTMLElement;
    expect(within(watchList).getByText('EDGE-IN')).toBeTruthy();
    expect(within(watchList).getByText(text('inbox.condition.any_different'))).toBeTruthy();
  });

  it('marks an entry read through the API and refreshes from the response', async () => {
    vi.mocked(runtime.listInbox).mockResolvedValue(artifact<InboxDocument>({ entries: [changedEntry], unread: 1 }));
    vi.mocked(runtime.markInboxRead).mockResolvedValue(artifact<InboxDocument>({
      entries: [{ ...changedEntry, read: true }],
      unread: 0,
    }));
    renderPanel();
    await waitFor(() => expect(screen.getByText(text('inbox.markRead'))).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: text('inbox.markRead') }));
    await waitFor(() => expect(vi.mocked(runtime.markInboxRead)).toHaveBeenCalledWith(7));
    await waitFor(() => expect(screen.queryByText(text('inbox.markRead'))).toBeNull());
  });

  it('adds a watch through the form and removes it from the list', async () => {
    vi.mocked(runtime.replaceWatches).mockImplementation(async (watches) => artifact({ watches }));
    renderPanel();
    await waitFor(() => expect(screen.getByText(text('inbox.watch.none'))).toBeTruthy());
    fireEvent.change(screen.getByLabelText(text('inbox.watch.name')), { target: { value: 'EDGE-IN' } });
    fireEvent.click(screen.getByRole('button', { name: text('inbox.watch.add') }));
    await waitFor(() => expect(vi.mocked(runtime.replaceWatches)).toHaveBeenCalledWith([
      { domain: 'acl', name: 'EDGE-IN', conditions: ['verdict_changed'] },
    ]));
    await waitFor(() => expect(screen.getByText('EDGE-IN')).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: 'Stop watching EDGE-IN' }));
    await waitFor(() => expect(vi.mocked(runtime.replaceWatches)).toHaveBeenLastCalledWith([]));
  });

  it('shows a banner instead of crashing when the inbox cannot load', async () => {
    vi.mocked(runtime.listInbox).mockRejectedValue(new Error('down'));
    renderPanel();
    await waitFor(() => expect(screen.getByText(text('inbox.error'))).toBeTruthy());
  });
});
