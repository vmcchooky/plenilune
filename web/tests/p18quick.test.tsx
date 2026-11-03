import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from '../src/App';
import { LocaleProvider } from '../src/i18n';
import viMessages from '../src/i18n/messages.vi';
import { runtime } from '../src/lib/api';
import type { Artifact } from '../src/lib/api';
import type { Comparison, InboxDocument } from '../src/lib/types';
import {
  candidateComparison,
  comparison,
  normalizedDigest,
  parsedResult,
  scenario,
  standaloneAnalysis,
} from './fixtures';

vi.mock('../src/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../src/lib/api')>();
  return {
    ...actual,
    runtime: {
      health: vi.fn(), analyze: vi.fn(), analyzeReplay: vi.fn(), trace: vi.fn(),
      parse: vi.fn(), normalize: vi.fn(), normalizePolicy: vi.fn(), compare: vi.fn(),
      replayComparison: vi.fn(), agentSession: vi.fn(), repairVerify: vi.fn(), repairReplay: vi.fn(),
      listWatches: vi.fn(), replaceWatches: vi.fn(), listInbox: vi.fn(), markInboxRead: vi.fn(),
    },
  };
});

const artifact = <T,>(data: T): Artifact<T> => ({ data, raw: JSON.stringify(data) });
const text = (key: keyof typeof viMessages): string => viMessages[key];

// What the accepted-policy comparison returns; tests override per scenario.
let acceptedComparison: Comparison = comparison;

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  acceptedComparison = comparison;
  vi.mocked(runtime.health).mockResolvedValue(artifact({ service: 'plenilune-demo', runtime: {} }));
  vi.mocked(runtime.listWatches).mockResolvedValue(artifact({ watches: [] }));
  vi.mocked(runtime.listInbox).mockResolvedValue(artifact({ entries: [], unread: 0 }));
  vi.mocked(runtime.analyze).mockImplementation(async (before, after, _select, policies) => {
    const data = await standaloneAnalysis(before, after);
    if (policies) {
      // The scoped re-analysis after acceptance must bind to the accepted
      // snapshot: parsed digests and the accepted policy hash.
      const object = data.objects[0];
      if (object.acl) object.acl = { ...object.acl, policy_sha256: 'd'.repeat(64) };
      if (object.before) object.before = { ...object.before, sha256: parsedResult.before.sha256 };
      if (object.after) object.after = { ...object.after, sha256: parsedResult.after.sha256 };
    }
    return artifact(data);
  });
  vi.mocked(runtime.parse).mockResolvedValue(artifact(parsedResult));
  vi.mocked(runtime.normalizePolicy).mockImplementation(async (policy) => artifact({
    policy,
    policy_sha256: (policy.operator_accepted ? 'd' : 'c').repeat(64),
    obligations_digest: normalizedDigest,
  }));
  vi.mocked(runtime.compare).mockImplementation(async (_before, _after, conditions) =>
    artifact(conditions ? acceptedComparison : comparison));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The per-obligation status listing inside the obligations panel. (The same
// string also appears in the verdict banner facts, so scope to the panel.)
function listingText(): string | null {
  return document.querySelector('.quick-obligations__listing')?.textContent ?? null;
}

function renderApp() {
  return render(
    <LocaleProvider>
      <App />
    </LocaleProvider>,
  );
}

async function awaitVerdict() {
  return screen.findByText('Khác biệt', undefined, { timeout: 5000 });
}

async function evaluateObligations() {
  // Editor controls stay disabled while a run is settling (busy state), and
  // a click on a disabled button is a no-op: wait for the button to enable.
  const previewButton = screen.getByRole('button', { name: text('accept.previewAction') });
  await waitFor(() => {
    expect((previewButton as HTMLButtonElement).disabled).toBe(false);
  }, { timeout: 5000 });
  fireEvent.click(previewButton);
  await screen.findByText(text('accept.previewTitle'), undefined, { timeout: 5000 });
  fireEvent.click(screen.getByRole('button', { name: text('accept.action') }));
}

describe('P1-8 quick verdict path', () => {
  it('preserves the open watch draft and focus when unread notifications arrive', async () => {
    let finishInbox!: (doc: Artifact<InboxDocument>) => void;
    vi.mocked(runtime.listInbox).mockImplementation(() => new Promise(resolve => { finishInbox = resolve; }));
    renderApp();
    fireEvent.change(document.querySelector('#sample-select')!, { target: { value: 'blank' } });
    const drawer = document.querySelector<HTMLDetailsElement>('.watch-drawer')!;
    drawer.open = true;
    const name = screen.getByLabelText(text('inbox.watch.name')) as HTMLInputElement;
    fireEvent.change(name, { target: { value: 'DRAFT-ACL' } });
    name.focus();
    await act(async () => { finishInbox(artifact({ entries: [], unread: 1 })); });
    expect(document.querySelector('.watch-drawer')).toBe(drawer);
    expect(drawer.open).toBe(true);
    expect(screen.getByLabelText(text('inbox.watch.name'))).toBe(name);
    expect(name.value).toBe('DRAFT-ACL');
    expect(document.activeElement).toBe(name);
    expect(vi.mocked(runtime.listInbox)).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('button', { name: '1 chưa đọc' }));
    expect(drawer.open).toBe(true);
    expect(document.activeElement).toBe(drawer.querySelector('summary'));
    expect(name.value).toBe('DRAFT-ACL');
  });

  it('keeps the initial quick result short and reveals editors only on request', async () => {
    renderApp();
    await awaitVerdict();
    const sources = document.querySelector<HTMLDetailsElement>('.quick-sources')!;
    const policy = document.querySelector<HTMLDetailsElement>('.quick-policy')!;
    expect(sources.open).toBe(false);
    expect(policy.open).toBe(false);
    expect(document.querySelector<HTMLDetailsElement>('.quick-questions')!.open).toBe(false);
    expect(document.querySelector('.quick-outcome__example')?.textContent).toContain('192.0.2.10');
    fireEvent.click(screen.getByRole('button', { name: text('quick.sources') }));
    expect(sources.open).toBe(true);
    fireEvent.click(screen.getByRole('button', { name: text('quick.checkIntent') }));
    expect(policy.open).toBe(true);
  });

  it('keeps incomplete-result warnings outside collapsed details', async () => {
    vi.mocked(runtime.compare).mockResolvedValue(artifact({ ...comparison, report: {
      ...comparison.report, verdict: 'INCONCLUSIVE', search_complete: false, reason: 'operation_limit',
    } }));
    renderApp();
    await screen.findByText('Chưa kết luận', { selector: 'code.badge' }, { timeout: 5000 });
    expect(document.querySelector('.quick-outcome > [role="status"]')).toBeTruthy();
    expect(document.querySelector<HTMLDetailsElement>('.quick-outcome__details')!.open).toBe(false);
    expect(screen.queryByText('Tương đương')).toBeNull();
  });

  it('(a) auto-runs on the default sample and shows the verdict with zero clicks', async () => {
    renderApp();
    await awaitVerdict();
    expect(vi.mocked(runtime.analyze).mock.calls.length).toBeGreaterThan(0);
    // The verdict is a behavior-only verdict until obligations are accepted.
    expect(screen.getByText(text('result.noPolicy'))).toBeTruthy();
    // The object list with its status chip is on the same screen.
    expect(document.querySelector('.quick-outcome__identity')?.textContent).toContain('MGMT');
    // The limitations line stays on the verdict screen.
    expect(screen.getByText(text('quick.scopeNote'))).toBeTruthy();
    // Quick mode is the entry: no step rail.
    expect(screen.queryByRole('navigation', { name: text('nav.aria') })).toBeNull();
  });

  it('(b) reaches the verdict with no obligations at all and is never blocked', async () => {
    const { container } = renderApp();
    const select = container.querySelector('#sample-select');
    expect(select).toBeTruthy();
    fireEvent.change(select!, { target: { value: 'blank' } });
    fireEvent.change(screen.getByLabelText(text('source.before')), { target: { value: scenario.before } });
    fireEvent.change(screen.getByLabelText(text('source.after')), { target: { value: scenario.after } });
    await awaitVerdict();
    // The obligations panel is present but empty; nothing had to be visited
    // or filled in for the verdict to appear.
    expect(screen.getByText(text('cond.empty'))).toBeTruthy();
    expect(screen.getByText(text('quick.obligations.help'))).toBeTruthy();
  });

  it('(c) edits obligations inline and updates the status listing in place', { timeout: 30000 }, async () => {
    // The accepted comparison reports exactly the obligations it was given.
    vi.mocked(runtime.compare).mockImplementation(async (_before, _after, conditions) => {
      if (!conditions) return artifact(comparison);
      const obligations = comparison.report.obligations.filter(
        (outcome) => conditions.obligations.some((obligation) => obligation.id === outcome.id),
      );
      return artifact({ ...comparison, report: { ...comparison.report, obligations } });
    });
    renderApp();
    await awaitVerdict();
    await evaluateObligations();
    await waitFor(() => {
      expect(listingText()).toContain('OB-1');
      expect(listingText()).toContain('OB-2');
      expect(listingText()).toContain('OB-3');
    }, { timeout: 5000 });

    // Removing an obligation inline clears the stale listing immediately,
    // on the same screen, with the verdict still in place.
    const removeOb3 = screen.getByRole('button', { name: text('cond.remove').replace('{id}', 'OB-3') });
    await waitFor(() => {
      expect((removeOb3 as HTMLButtonElement).disabled).toBe(false);
    }, { timeout: 5000 });
    fireEvent.click(removeOb3);
    await waitFor(() => {
      expect(listingText()).toBeNull();
    }, { timeout: 5000 });
    expect(screen.getByText('Khác biệt')).toBeTruthy();

    // Re-evaluating in place re-renders the listing without OB-3.
    await evaluateObligations();
    await waitFor(() => {
      expect(listingText()).toContain('OB-1');
      expect(listingText()).toContain('OB-2');
      expect(listingText()).not.toContain('OB-3');
    }, { timeout: 5000 });
    expect(screen.queryByRole('navigation', { name: text('nav.aria') })).toBeNull();
  });

  it('(d) hides repair for EQUIVALENT and shows it for DIFFERENT with a violated obligation', async () => {
    acceptedComparison = candidateComparison;
    renderApp();
    await awaitVerdict();
    await evaluateObligations();
    await waitFor(() => {
      expect(listingText()).toContain('OB-1');
    }, { timeout: 5000 });
    expect(screen.getByText('Tương đương')).toBeTruthy();
    expect(screen.queryByText(text('quick.repair.title'))).toBeNull();
    expect(screen.queryByText(text('repair.manual.title'))).toBeNull();
  });

  it('(d) shows the contextual repair section for DIFFERENT with a violated obligation', async () => {
    renderApp();
    await awaitVerdict();
    await evaluateObligations();
    expect(await screen.findByText(text('quick.repair.title'), undefined, { timeout: 5000 })).toBeTruthy();
    expect(screen.getByText(text('repair.manual.title'))).toBeTruthy();
  });

  it('(e) reaches deep review mode via the switch and its stages still render', { timeout: 20000 }, async () => {
    renderApp();
    await awaitVerdict();
    fireEvent.click(screen.getByRole('button', { name: text('quick.mode.deep') }));
    expect(await screen.findByRole('navigation', { name: text('nav.aria') }, { timeout: 5000 })).toBeTruthy();
    // The internal stage after the auto-run is the result step.
    expect(screen.getByText(text('result.title'))).toBeTruthy();
    // Stage navigation still works: back to the source step.
    fireEvent.click(screen.getByRole('button', { name: new RegExp(text('nav.source.title')) }));
    expect(await screen.findByText(text('source.title'), undefined, { timeout: 5000 })).toBeTruthy();
    expect(screen.getByText(text('source.action'))).toBeTruthy();
    // And back to quick mode, where the verdict is still there.
    fireEvent.click(screen.getByRole('button', { name: text('quick.mode.quick') }));
    await waitFor(() => {
      expect(screen.queryByRole('navigation', { name: text('nav.aria') })).toBeNull();
    }, { timeout: 5000 });
    expect(screen.getByText('Khác biệt')).toBeTruthy();
  });
});
