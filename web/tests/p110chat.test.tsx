import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatPanel } from '../src/components/ChatPanel';
import { LocaleProvider } from '../src/i18n';
import enMessages from '../src/i18n/messages.en';
import { runtime } from '../src/lib/api';
import type { Artifact } from '../src/lib/api';
import type { ChatAnswer, Comparison } from '../src/lib/types';
import { comparison } from './fixtures';

vi.mock('../src/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../src/lib/api')>();
  return {
    ...actual,
    runtime: {
      health: vi.fn(), analyze: vi.fn(), analyzeReplay: vi.fn(), trace: vi.fn(),
      parse: vi.fn(), normalize: vi.fn(), normalizePolicy: vi.fn(), compare: vi.fn(),
      replayComparison: vi.fn(), agentSession: vi.fn(), repairVerify: vi.fn(), repairReplay: vi.fn(),
      chatAsk: vi.fn(),
    },
  };
});

const text = (key: keyof typeof enMessages): string => enMessages[key];
const artifact = <T,>(data: T): Artifact<T> => ({ data, raw: JSON.stringify(data) });

const verdictAnswer: ChatAnswer = {
  schema: 'plenilune.chat/v1',
  class: 'verdict',
  text: 'The verdict is DIFFERENT.',
  facts: [{ label: 'Verdict', value: 'DIFFERENT', source: 'comparison.report.verdict' }],
  artifact_sha256: comparison.sha256,
  locale: 'en',
  ai: { status: 'off' },
};

function renderPanel(comp: Comparison | null = comparison) {
  return render(
    <LocaleProvider>
      <ChatPanel comparison={comp} analysis={null} />
    </LocaleProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  window.localStorage.setItem('plenilune.locale', 'en');
});

afterEach(() => cleanup());

describe('P1-10 interrogation chat panel', () => {
  it('is absent before any artifact exists', () => {
    const { container } = renderPanel(null);
    expect(container.querySelector('.chat-panel')).toBeNull();
    expect(vi.mocked(runtime.chatAsk)).not.toHaveBeenCalled();
  });

  it('anchors to the artifact digest and never asks on render', () => {
    renderPanel();
    expect(screen.getByText(text('chat.title'))).toBeTruthy();
    expect(screen.getByText(new RegExp(comparison.sha256.slice(0, 10)))).toBeTruthy();
    expect(vi.mocked(runtime.chatAsk)).not.toHaveBeenCalled();
  });

  it('answers with the deterministic text and its artifact facts', async () => {
    vi.mocked(runtime.chatAsk).mockResolvedValue(artifact(verdictAnswer));
    renderPanel();
    fireEvent.change(screen.getByPlaceholderText(text('chat.placeholder')), { target: { value: 'Why DIFFERENT?' } });
    fireEvent.click(screen.getByRole('button', { name: text('chat.send') }));
    await waitFor(() => expect(screen.getByText('The verdict is DIFFERENT.')).toBeTruthy());
    expect(screen.getByText('comparison.report.verdict')).toBeTruthy();
    expect(vi.mocked(runtime.chatAsk)).toHaveBeenCalledWith(comparison, null, 'Why DIFFERENT?', 'en', false);
  });

  it('fills the input from a suggestion chip without asking', () => {
    renderPanel();
    fireEvent.click(screen.getByRole('button', { name: text('chat.suggest.witness') }));
    const input = screen.getByPlaceholderText(text('chat.placeholder')) as HTMLInputElement;
    expect(input.value).toBe(text('chat.suggest.witness'));
    expect(vi.mocked(runtime.chatAsk)).not.toHaveBeenCalled();
  });

  it('resets the conversation when the artifact digest changes', async () => {
    vi.mocked(runtime.chatAsk).mockResolvedValue(artifact(verdictAnswer));
    const { rerender } = renderPanel();
    fireEvent.change(screen.getByPlaceholderText(text('chat.placeholder')), { target: { value: 'Why DIFFERENT?' } });
    fireEvent.click(screen.getByRole('button', { name: text('chat.send') }));
    await waitFor(() => expect(screen.getByText('The verdict is DIFFERENT.')).toBeTruthy());
    const other: Comparison = { ...comparison, sha256: 'f'.repeat(64) };
    rerender(
      <LocaleProvider>
        <ChatPanel comparison={other} analysis={null} />
      </LocaleProvider>,
    );
    expect(screen.queryByText('The verdict is DIFFERENT.')).toBeNull();
    expect(screen.getByText(new RegExp('f'.repeat(10)))).toBeTruthy();
  });

  it('shows the not-configured notice and keeps the deterministic answer primary', async () => {
    vi.mocked(runtime.chatAsk).mockResolvedValue(artifact({ ...verdictAnswer, ai: { status: 'not_configured' } }));
    renderPanel();
    fireEvent.click(screen.getByLabelText(text('chat.aiToggle')));
    fireEvent.change(screen.getByPlaceholderText(text('chat.placeholder')), { target: { value: 'Why DIFFERENT?' } });
    fireEvent.click(screen.getByRole('button', { name: text('chat.send') }));
    await waitFor(() => expect(screen.getByText(text('chat.aiNotConfigured'))).toBeTruthy());
    expect(screen.getByText('The verdict is DIFFERENT.')).toBeTruthy();
    expect(vi.mocked(runtime.chatAsk)).toHaveBeenCalledWith(comparison, null, 'Why DIFFERENT?', 'en', true);
  });

  it('shows AI phrasing above — never instead of — the facts', async () => {
    vi.mocked(runtime.chatAsk).mockResolvedValue(
      artifact({ ...verdictAnswer, ai: { status: 'ok', model: 'muse-spark-1.3-contributor', text: 'Phrased: the ACLs differ.' } }),
    );
    renderPanel();
    fireEvent.change(screen.getByPlaceholderText(text('chat.placeholder')), { target: { value: 'Why DIFFERENT?' } });
    fireEvent.click(screen.getByRole('button', { name: text('chat.send') }));
    await waitFor(() => expect(screen.getByText('Phrased: the ACLs differ.')).toBeTruthy());
    // The deterministic answer and the facts stay on screen: if the
    // phrasing contradicted the artifact, the artifact wins by display.
    expect(screen.getByText('The verdict is DIFFERENT.')).toBeTruthy();
    expect(screen.getByText('comparison.report.verdict')).toBeTruthy();
  });
});
