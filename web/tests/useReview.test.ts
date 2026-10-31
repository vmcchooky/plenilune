import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, runtime } from '../src/lib/api';
import type { Artifact } from '../src/lib/api';
import { useReview } from '../src/hooks/useReview';
import { LocaleProvider, LOCALE_STORAGE_KEY, useI18n } from '../src/i18n';
import type { Comparison, ReplayResult, RepairResult, NormalizedPolicy } from '../src/lib/types';
import {
  comparison,
  foreignRepairReceipt,
  normalizedDigest,
  normalizedSet,
  parsedResult,
  repairReceipt,
  scenario,
  scopeOnlyRepairReceipt,
  sessionNotConfigured,
  sessionProposal,
  standaloneAnalysis,
} from './fixtures';

vi.mock('../src/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../src/lib/api')>();
  return {
    ...actual,
    runtime: {
      health: vi.fn(),
      analyze: vi.fn(),
      analyzeReplay: vi.fn(),
      trace: vi.fn(),
      parse: vi.fn(),
      normalize: vi.fn(),
      normalizePolicy: vi.fn(),
      compare: vi.fn(),
      replayComparison: vi.fn(),
      agentSession: vi.fn(),
      repairVerify: vi.fn(),
      repairReplay: vi.fn(),
    },
  };
});

function artifact<T>(data: T, raw = JSON.stringify(data)): Artifact<T> {
  return { data, raw };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function apiCalls(): number {
  return [
    runtime.health,
    runtime.analyze,
    runtime.analyzeReplay,
    runtime.trace,
    runtime.parse,
    runtime.normalize,
    runtime.normalizePolicy,
    runtime.compare,
    runtime.replayComparison,
    runtime.agentSession,
    runtime.repairVerify,
    runtime.repairReplay,
  ].reduce((total, fn) => total + vi.mocked(fn).mock.calls.length, 0);
}

type Review = ReturnType<typeof useReview>;

async function renderReview() {
  const rendered = renderHook(() => useReview());
  await act(async () => {});
  return rendered;
}

async function prepare(review: Review) {
  await act(async () => { await review.parseSource(); });
}

async function acceptAll(review: Review) {
  await act(async () => { await review.previewPolicy(); });
  await act(async () => { await review.acceptAndCompare(true); });
}

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  vi.mocked(runtime.health).mockResolvedValue(artifact({ service: 'plenilune-demo', runtime: {} }));
  vi.mocked(runtime.parse).mockResolvedValue(artifact(parsedResult));
  vi.mocked(runtime.analyze).mockImplementation(async (before, after) => artifact(await standaloneAnalysis(before, after)));
  vi.mocked(runtime.normalize).mockResolvedValue(artifact({ set: normalizedSet, sha256: normalizedDigest }));
  vi.mocked(runtime.normalizePolicy).mockImplementation(async (policy) => artifact({ policy: { ...policy, conditions: normalizedSet }, policy_sha256: (policy.operator_accepted ? 'd' : 'c').repeat(64), obligations_digest: normalizedDigest }));
  vi.mocked(runtime.compare).mockResolvedValue(artifact(comparison));
  vi.mocked(runtime.replayComparison).mockResolvedValue(artifact<ReplayResult>({ status: 'COMPARISON_REPLAYED', sha256: '6'.repeat(64), rechecked_witnesses: 1 }));
  vi.mocked(runtime.agentSession).mockResolvedValue(artifact(sessionNotConfigured));
  vi.mocked(runtime.repairVerify).mockResolvedValue(artifact(repairReceipt));
  vi.mocked(runtime.repairReplay).mockResolvedValue(artifact<ReplayResult>({ status: 'REPAIR_REPLAYED', sha256: repairReceipt.sha256!, rechecked_witnesses: 1 }));
});

afterEach(() => cleanup());

describe('runtime review flow', () => {
  it('never calls AI unless the user asks for a proposal', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);

    expect(result.current.comparison?.comparison.report.verdict).toBe('DIFFERENT');
    expect(runtime.agentSession).not.toHaveBeenCalled();

    await act(async () => { await result.current.proposeConditions(); });
    expect(runtime.agentSession).toHaveBeenCalledTimes(1);
    const call = vi.mocked(runtime.agentSession).mock.calls[0];
    expect(call[3]).toBeNull();
    expect(call[4]).toBe(true);
    expect(result.current.aiProposal?.session.ai.status).toBe('not_configured');
    expect(result.current.lastAi?.status).toBe('not_configured');
  });

  it('adopts an AI proposal as a draft without accepting it', async () => {
    vi.mocked(runtime.agentSession).mockResolvedValueOnce(artifact(sessionProposal));
    const { result } = await renderReview();
    await prepare(result.current);
    await act(async () => { await result.current.proposeConditions(); });

    expect(result.current.aiProposal?.session.ai.obligations?.set.obligations.length).toBe(3);
    act(() => result.current.adoptProposal());

    expect(result.current.draft.conditions.length).toBe(3);
    expect(result.current.proposalAdopted).toBe(true);
    expect(result.current.modified).toBe(true);
    expect(result.current.accepted).toBeNull();
    expect(result.current.comparison).toBeNull();
  });

  it('preserves input, acceptance and results across a language switch without requests', async () => {
    const { result } = renderHook(() => ({ review: useReview(), i18n: useI18n() }), { wrapper: LocaleProvider });
    await act(async () => {});
    await prepare(result.current.review);
    await acceptAll(result.current.review);

    const before = {
      draft: result.current.review.draft.before,
      intent: result.current.review.draft.intent,
      conditions: result.current.review.draft.conditions.length,
      verdict: result.current.review.comparison?.comparison.report.verdict,
      stage: result.current.review.stage,
      calls: apiCalls(),
    };

    act(() => result.current.i18n.setLocale('en'));

    expect(result.current.review.draft.before).toBe(before.draft);
    expect(result.current.review.draft.intent).toBe(before.intent);
    expect(result.current.review.draft.conditions.length).toBe(before.conditions);
    expect(result.current.review.comparison?.comparison.report.verdict).toBe(before.verdict);
    expect(result.current.review.stage).toBe(before.stage);
    expect(apiCalls()).toBe(before.calls);
    expect(window.localStorage.getItem(LOCALE_STORAGE_KEY)).toBe('en');
  });

  it('invalidates every downstream artifact when the source changes', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    expect(result.current.accepted).not.toBeNull();

    act(() => result.current.updateSource('intent', 'A changed intent'));

    expect(result.current.stage).toBe('source');
    expect(result.current.parsed).toBeNull();
    expect(result.current.accepted).toBeNull();
    expect(result.current.comparison).toBeNull();
    expect(result.current.repair).toBeNull();
    expect(result.current.aiProposal).toBeNull();
    act(() => result.current.navigate('result'));
    expect(result.current.stage).toBe('source');
  });

  it('invalidates acceptance and evidence when a condition changes', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    act(() => result.current.navigate('conditions'));
    act(() => result.current.removeCondition('OB-3'));

    expect(result.current.stage).toBe('conditions');
    expect(result.current.accepted).toBeNull();
    expect(result.current.comparison).toBeNull();
    act(() => result.current.navigate('result'));
    expect(result.current.stage).toBe('conditions');
  });

  it('keeps the accepted policy on cancel and allows an explicit retry', async () => {
    const pending = deferred<Artifact<Comparison>>();
    vi.mocked(runtime.compare).mockReturnValueOnce(pending.promise);
    const { result } = await renderReview();
    await prepare(result.current);

    await act(async () => { await result.current.previewPolicy(); });
    let run!: Promise<void>;
    act(() => { run = result.current.acceptAndCompare(true); });
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });
    expect(result.current.accepted).not.toBeNull();
    expect(result.current.comparison).toBeNull();

    act(() => result.current.cancel());
    pending.resolve(artifact(comparison));
    await act(async () => { await run; });

    expect(result.current.canceled).toBe('compare');
    expect(result.current.comparison).toBeNull();
    expect(result.current.accepted).not.toBeNull();

    vi.mocked(runtime.compare).mockResolvedValue(artifact(comparison));
    await act(async () => { await result.current.retryCompare(); });

    expect(result.current.comparison?.comparison.sha256).toBe(comparison.sha256);
    expect(result.current.stage).toBe('result');
    expect(result.current.canceled).toBeNull();
  });

  it('ignores an older comparison that resolves after a newer one', async () => {
    const first = deferred<Artifact<Comparison>>();
    const second = deferred<Artifact<Comparison>>();
    const oldComparison: Comparison = { ...comparison, sha256: '6'.repeat(64) };
    const newComparison: Comparison = { ...comparison, sha256: '7'.repeat(64) };
    vi.mocked(runtime.compare)
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const { result } = await renderReview();
    await prepare(result.current);

    await act(async () => { await result.current.previewPolicy(); });
    let firstRun!: Promise<void>;
    act(() => { firstRun = result.current.acceptAndCompare(true); });
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });

    act(() => result.current.removeCondition('OB-3'));
    await act(async () => { await result.current.previewPolicy(); });
    let secondRun!: Promise<void>;
    act(() => { secondRun = result.current.acceptAndCompare(true); });
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });

    second.resolve(artifact(newComparison));
    await act(async () => { await secondRun; });
    expect(result.current.comparison?.comparison.sha256).toBe(newComparison.sha256);

    first.resolve(artifact(oldComparison));
    await act(async () => { await firstRun; });
    expect(result.current.comparison?.comparison.sha256).toBe(newComparison.sha256);
  });

  it('surfaces a comparison error, keeps the accepted policy and retries', async () => {
    vi.mocked(runtime.compare).mockRejectedValueOnce(new ApiError('invalid input', 'PARSE_INVALID'));
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);

    expect(result.current.error?.code).toBe('PARSE_INVALID');
    expect(result.current.accepted).not.toBeNull();
    expect(result.current.comparison).toBeNull();
    expect(result.current.stage).toBe('conditions');

    vi.mocked(runtime.compare).mockResolvedValueOnce(artifact(comparison));
    await act(async () => { await result.current.retryCompare(); });
    expect(result.current.comparison).not.toBeNull();
    expect(result.current.stage).toBe('result');
  });

  it('marks the runtime offline on transport failure and online on recovery', async () => {
    vi.mocked(runtime.parse).mockRejectedValueOnce(new ApiError('offline', 'RUNTIME_UNAVAILABLE'));
    const { result } = await renderReview();
    await prepare(result.current);
    expect(result.current.backend).toBe('offline');
    expect(result.current.error?.code).toBe('RUNTIME_UNAVAILABLE');

    await prepare(result.current);
    expect(result.current.backend).toBe('online');
  });

  it('verifies a manual repair with the accepted policy and replays the receipt', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    act(() => result.current.navigate('repair'));

    act(() => {
      const accepted = result.current.addEdit({
        op: 'remove',
        ref: { base_after_sha256: parsedResult.after.sha256, ordinal: 0, line: 3 },
      });
      expect(accepted).toBe(true);
    });
    expect(result.current.edits.length).toBe(1);

    await act(async () => { await result.current.verifyManualRepair(); });

    expect(runtime.repairVerify).toHaveBeenCalledTimes(1);
    const call = vi.mocked(runtime.repairVerify).mock.calls[0];
    expect(call[2].operator_accepted).toBe(true);
    expect(call[3].edits[0]).toEqual({
      op: 'remove',
      ref: { base_after_sha256: parsedResult.after.sha256, ordinal: 0, line: 3 },
    });
    expect(result.current.repair?.receipt.status).toBe('VERIFIED');
    expect(result.current.repair?.source).toBe('manual');

    await act(async () => { await result.current.replayRepair(); });
    expect(result.current.repairReplay?.result.status).toBe('REPAIR_REPLAYED');

    act(() => {
      result.current.addEdit({
        op: 'replace',
        ref: { base_after_sha256: parsedResult.after.sha256, ordinal: 1, line: 4 },
        content: 'permit tcp any any eq 443',
      });
    });
    expect(result.current.repair).toBeNull();
    expect(result.current.repairReplay).toBeNull();
  });

  it('adopts an engine-verified AI repair receipt as a proposal', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    vi.mocked(runtime.agentSession).mockResolvedValueOnce(artifact({ ...sessionProposal, repairs: [repairReceipt] }));

    await act(async () => { await result.current.proposeRepair(); });

    expect(runtime.agentSession).toHaveBeenCalledTimes(1);
    expect(result.current.repair?.source).toBe('ai');
    expect(result.current.repair?.receipt.status).toBe('VERIFIED');
    expect(result.current.repairSource).toBe('ai');
    expect(result.current.aiSession?.session.ai.status).toBe('completed');
  });

  it('marks a budget-exhausted comparison as inconclusive without a repair path', async () => {
    const inconclusive: Comparison = {
      ...comparison,
      report: {
        ...comparison.report,
        verdict: 'INCONCLUSIVE',
        search_complete: false,
        witnesses: [],
        witnesses_truncated: false,
        obligations: comparison.report.obligations.map((item) => ({ id: item.id, status: 'INCONCLUSIVE' })),
        budget: { operations: 50, peak_boxes: 17, stop_reason: 'operations', timing_limited: false },
      },
      sha256: '5'.repeat(64),
    };
    vi.mocked(runtime.compare).mockResolvedValueOnce(artifact(inconclusive));
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);

    expect(result.current.comparison?.comparison.report.verdict).toBe('INCONCLUSIVE');
    expect(result.current.canRepair).toBe(false);
    expect(result.current.canExport).toBe(true);
    act(() => result.current.navigate('repair'));
    expect(result.current.stage).toBe('result');
  });

  it('keeps the sample designation until the user edits the draft', async () => {
    const { result } = await renderReview();
    expect(result.current.sourceKind).toBe('sample');
    expect(result.current.sample?.id).toBe(scenario.id);
    act(() => result.current.updateSource('before', `${scenario.before}\n`));
    expect(result.current.sourceKind).toBe('modified');
    expect(result.current.sample?.id).toBe(scenario.id);
  });
  it('explores the exact delta with no intent or accepted conditions', async () => {
    const { result } = await renderReview();
    act(() => result.current.updateSource('intent', ''));
    await act(async () => { await result.current.exploreSource(); });
    expect(runtime.compare).toHaveBeenLastCalledWith(scenario.before, scenario.after, null, undefined, expect.any(AbortSignal));
    expect(result.current.stage).toBe('result');
    expect(result.current.accepted).toBeNull();
    expect(result.current.canRepair).toBe(false);
    expect(runtime.agentSession).not.toHaveBeenCalled();
  });

  it('requires a canonical full-policy preview before explicit acceptance', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await act(async () => { await result.current.acceptAndCompare(true); });
    expect(runtime.compare).not.toHaveBeenCalled();
    await act(async () => { await result.current.previewPolicy(); });
    expect(result.current.policyPreview?.normalized.policy.operator_accepted).toBe(false);
    expect(result.current.accepted).toBeNull();
    const preview = result.current.policyPreview!.normalized;
    await act(async () => { await result.current.acceptAndCompare(true); });
    const requests = vi.mocked(runtime.normalizePolicy).mock.calls;
    expect(requests[0][0].operator_accepted).toBe(false);
    expect(requests[1][0]).toEqual({ ...preview.policy, operator_accepted: true });
    expect(result.current.accepted?.policy.conditions).toEqual(preview.policy.conditions);
  });

  it('invalidates preview when a condition changes and rejects late preview publication', async () => {
    const pending = deferred<Artifact<NormalizedPolicy>>();
    const { result } = await renderReview();
    await prepare(result.current);
    await act(async () => { await result.current.previewPolicy(); });
    expect(result.current.policyPreview).not.toBeNull();
    act(() => result.current.removeCondition('OB-3'));
    expect(result.current.policyPreview).toBeNull();
    vi.mocked(runtime.normalizePolicy).mockReturnValueOnce(pending.promise);
    let run!: Promise<void>;
    act(() => { run = result.current.previewPolicy(); });
    const submitted = vi.mocked(runtime.normalizePolicy).mock.calls.at(-1)!;
    act(() => result.current.updateSource('before', scenario.before + '\n'));
    expect(submitted[1]?.aborted).toBe(true);
    pending.resolve(artifact({ policy: submitted[0], policy_sha256: 'e'.repeat(64), obligations_digest: normalizedDigest }));
    await act(async () => { await run; });
    expect(result.current.policyPreview).toBeNull();
    expect(result.current.accepted).toBeNull();
    expect(result.current.stage).toBe('source');
  });

  it('rejects changed policy definitions returned at acceptance', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await act(async () => { await result.current.previewPolicy(); });
    vi.mocked(runtime.normalizePolicy).mockImplementationOnce(async (policy) => artifact({
      policy: { ...policy, allowed_changes: null },
      policy_sha256: 'd'.repeat(64), obligations_digest: normalizedDigest,
    }));
    await act(async () => { await result.current.acceptAndCompare(true); });
    expect(result.current.error?.code).toBe('POLICY_PREVIEW_MISMATCH');
    expect(result.current.accepted).toBeNull();
    expect(runtime.compare).not.toHaveBeenCalled();
  });

  it('drops a late verification after removing the submitted edit despite ignored abort', async () => {
    const pending = deferred<Artifact<RepairResult>>();
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    act(() => result.current.addEdit({ op: 'remove', ref: { base_after_sha256: parsedResult.after.sha256, ordinal: 0, line: 3 } }));
    vi.mocked(runtime.repairVerify).mockReturnValueOnce(pending.promise);
    let run!: Promise<void>;
    act(() => { run = result.current.verifyManualRepair(); });
    const signal = vi.mocked(runtime.repairVerify).mock.calls.at(-1)![5];
    act(() => result.current.removeEdit(result.current.edits[0].id));
    expect(signal?.aborted).toBe(true);
    pending.resolve(artifact(repairReceipt));
    await act(async () => { await run; });
    expect(result.current.edits).toHaveLength(0);
    expect(result.current.repair).toBeNull();
    expect(result.current.busy).toBeNull();
    expect(result.current.repairReplay).toBeNull();
  });

  it('ignores a stale verification error after adding a new edit', async () => {
    const pending = deferred<Artifact<RepairResult>>();
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    act(() => result.current.addEdit({ op: 'remove', ref: { base_after_sha256: parsedResult.after.sha256, ordinal: 0, line: 3 } }));
    vi.mocked(runtime.repairVerify).mockReturnValueOnce(pending.promise);
    let run!: Promise<void>;
    act(() => { run = result.current.verifyManualRepair(); });
    act(() => result.current.addEdit({ op: 'append', content: 'deny ip any any' }));
    pending.reject(new ApiError('stale transport failure', 'RUNTIME_UNAVAILABLE'));
    await act(async () => { await run; });
    expect(result.current.edits).toHaveLength(2);
    expect(result.current.repair).toBeNull();
    expect(result.current.error).toBeNull();
    expect(result.current.backend).toBe('online');
  });

  it('drops a replay when the selected candidate changes', async () => {
    const pending = deferred<Artifact<ReplayResult>>();
    const other = { ...repairReceipt, sha256: '4'.repeat(64) };
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    vi.mocked(runtime.agentSession).mockResolvedValueOnce(artifact({ ...sessionProposal, repairs: [repairReceipt, other] }));
    await act(async () => { await result.current.proposeRepair(); });
    vi.mocked(runtime.repairReplay).mockReturnValueOnce(pending.promise);
    let run!: Promise<void>;
    act(() => { run = result.current.replayRepair(); });
    act(() => result.current.selectAiRepair(1));
    pending.resolve(artifact({ status: 'REPAIR_REPLAYED', sha256: repairReceipt.sha256!, rechecked_witnesses: 0 }));
    await act(async () => { await run; });
    expect(result.current.repair?.receipt.sha256).toBe(other.sha256);
    expect(result.current.repairReplay).toBeNull();
    expect(result.current.busy).toBeNull();
  });

  it('binds human receipt decisions and disputes to the receipt hash', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    vi.mocked(runtime.agentSession).mockResolvedValueOnce(artifact({ ...sessionProposal, repairs: [repairReceipt, { ...repairReceipt, sha256: '4'.repeat(64) }] }));
    await act(async () => { await result.current.proposeRepair(); });
    expect(result.current.canReceive).toBe(true);
    act(() => result.current.decideRepair('received'));
    expect(result.current.decision?.receiptSha256).toBe(repairReceipt.sha256);
    vi.mocked(runtime.repairReplay).mockRejectedValueOnce(new ApiError('mismatch', 'EVIDENCE_MISMATCH', undefined, undefined, 400));
    await act(async () => { await result.current.replayRepair(); });
    expect(result.current.disputed).toBe(true);
    expect(result.current.decision).toBeNull();
    expect(result.current.canReceive).toBe(false);
    act(() => result.current.selectAiRepair(1));
    expect(result.current.canReceive).toBe(true);
    act(() => result.current.selectAiRepair(0));
    expect(result.current.disputed).toBe(true);
    expect(result.current.canReceive).toBe(false);
    vi.mocked(runtime.repairReplay).mockResolvedValueOnce(artifact({ status: 'TIMING_SENSITIVE', sha256: repairReceipt.sha256!, rechecked_witnesses: 0 }));
    await act(async () => { await result.current.replayRepair(); });
    expect(result.current.disputed).toBe(true);
    expect(result.current.canReceive).toBe(false);
    await act(async () => { await result.current.replayRepair(); });
    expect(result.current.disputed).toBe(false);
    expect(result.current.canReceive).toBe(true);
  });

  it('does not dispute a receipt for validation or transport errors', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    vi.mocked(runtime.agentSession).mockResolvedValueOnce(artifact({ ...sessionProposal, repairs: [repairReceipt] }));
    await act(async () => { await result.current.proposeRepair(); });

    vi.mocked(runtime.repairReplay).mockRejectedValueOnce(new ApiError('malformed', 'JSON_INVALID', undefined, undefined, 400));
    await act(async () => { await result.current.replayRepair(); });
    expect(result.current.disputed).toBe(false);
    expect(result.current.error?.code).toBe('JSON_INVALID');
    expect(result.current.canReceive).toBe(true);
    act(() => result.current.clearError());

    vi.mocked(runtime.repairReplay).mockRejectedValueOnce(new ApiError('offline', 'RUNTIME_UNAVAILABLE'));
    await act(async () => { await result.current.replayRepair(); });
    expect(result.current.disputed).toBe(false);
    expect(result.current.error?.code).toBe('RUNTIME_UNAVAILABLE');
    expect(result.current.canReceive).toBe(true);
  });

  it('does not dispute when a replay is canceled', async () => {
    const pending = deferred<Artifact<ReplayResult>>();
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    vi.mocked(runtime.agentSession).mockResolvedValueOnce(artifact({ ...sessionProposal, repairs: [repairReceipt] }));
    await act(async () => { await result.current.proposeRepair(); });
    vi.mocked(runtime.repairReplay).mockReturnValueOnce(pending.promise);
    let run!: Promise<void>;
    act(() => { run = result.current.replayRepair(); });
    act(() => result.current.cancel());
    pending.reject(new DOMException('Aborted', 'AbortError'));
    await act(async () => { await run; });
    expect(result.current.disputed).toBe(false);
    expect(result.current.canReceive).toBe(true);
    expect(result.current.canceled).toBe('replayRepair');
  });

  it('rejects a manual receipt returned for a different input pair', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    act(() => result.current.navigate('repair'));
    act(() => result.current.addEdit({ op: 'remove', ref: { base_after_sha256: parsedResult.after.sha256, ordinal: 0, line: 3 } }));
    vi.mocked(runtime.repairVerify).mockResolvedValueOnce(artifact(foreignRepairReceipt));
    await act(async () => { await result.current.verifyManualRepair(); });
    expect(result.current.error?.code).toBe('RECEIPT_BINDING_MISMATCH');
    expect(result.current.manualRepair).toBeNull();
    expect(result.current.repair).toBeNull();
    expect(result.current.canReceive).toBe(false);
  });

  it('rejects an AI session containing a foreign receipt before showing candidates', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    vi.mocked(runtime.agentSession).mockResolvedValueOnce(artifact({ ...sessionProposal, repairs: [repairReceipt, foreignRepairReceipt] }));
    await act(async () => { await result.current.proposeRepair(); });
    expect(result.current.error?.code).toBe('RECEIPT_BINDING_MISMATCH');
    expect(result.current.aiSession).toBeNull();
    expect(result.current.repair).toBeNull();
    expect(result.current.canReceive).toBe(false);
  });

  it('rejects a receipt whose allowed scope changes while the obligation digest matches', async () => {
    const { result } = await renderReview();
    await prepare(result.current);
    await acceptAll(result.current);
    act(() => result.current.navigate('repair'));
    act(() => result.current.addEdit({ op: 'remove', ref: { base_after_sha256: parsedResult.after.sha256, ordinal: 0, line: 3 } }));
    vi.mocked(runtime.repairVerify).mockResolvedValueOnce(artifact(scopeOnlyRepairReceipt));
    await act(async () => { await result.current.verifyManualRepair(); });
    expect(result.current.error?.code).toBe('RECEIPT_BINDING_MISMATCH');
    expect(result.current.repair).toBeNull();
    expect(result.current.canReceive).toBe(false);
    expect(scopeOnlyRepairReceipt.obligations_digest).toBe(result.current.accepted?.digest);
  });

  it('adds intent after delta exploration without losing the parsed source identity', async () => {
    const { result } = await renderReview();
    act(() => result.current.updateSource('intent', ''));
    await act(async () => { await result.current.exploreSource(); });
    act(() => result.current.navigate('conditions'));
    await act(async () => { await result.current.proposeConditions(); });
    expect(runtime.agentSession).not.toHaveBeenCalled();
    const inputHash = result.current.parsed!.parsed.before.sha256;
    act(() => result.current.updateIntent('Preserve SSH access.'));
    expect(result.current.parsed?.parsed.before.sha256).toBe(inputHash);
    expect(result.current.parsed?.intent).toBe('Preserve SSH access.');
    expect(result.current.accepted).toBeNull();
    await act(async () => { await result.current.proposeConditions(); });
    expect(vi.mocked(runtime.agentSession).mock.calls[0][2]).toBe('Preserve SSH access.');
  });
});
