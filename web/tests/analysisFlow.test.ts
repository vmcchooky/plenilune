import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, runtime } from '../src/lib/api';
import type { Artifact } from '../src/lib/api';
import { useReview } from '../src/hooks/useReview';
import { parsedResult, scenario, standaloneAnalysis } from './fixtures';
import type { Analysis, AnalysisObject, ReplayResult, TraceResult } from '../src/lib/types';

vi.mock('../src/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../src/lib/api')>();
  return {
    ...actual,
    runtime: {
      health: vi.fn(), analyze: vi.fn(), analyzeReplay: vi.fn(), trace: vi.fn(),
      parse: vi.fn(), normalize: vi.fn(), normalizePolicy: vi.fn(), compare: vi.fn(),
      replayComparison: vi.fn(), agentSession: vi.fn(), repairVerify: vi.fn(), repairReplay: vi.fn(),
    },
  };
});

const artifact = <T,>(data: T): Artifact<T> => ({ data, raw: JSON.stringify(data) });
const packet = { protocol: 6, src_ip: '192.0.2.10', dst_ip: '198.51.100.10', src_port: 0, dst_port: 22 };
const trace = (action: 'permit' | 'deny'): TraceResult => ({
  schema: 'plenilune.trace/v1', config_sha256: 'a'.repeat(64), packet,
  decision: { action, rule_ordinal: 0, line: 3, implicit: false }, sha256: 'b'.repeat(64),
});

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(runtime.health).mockResolvedValue(artifact({ service: 'plenilune-demo', runtime: {} }));
  vi.mocked(runtime.parse).mockResolvedValue(artifact(parsedResult));
  vi.mocked(runtime.compare).mockResolvedValue(artifact({
    schema: 'plenilune.evidence/v1', sha256: 'c'.repeat(64),
  } as never));
  vi.mocked(runtime.trace).mockImplementation(async (config) => artifact(trace(config.includes('deny tcp') ? 'deny' : 'permit')));
});

function prefixAnalysis(): Analysis {
  const object: AnalysisObject = {
    domain: 'prefix-list', name: 'CUSTOMER', kind: 'ipv4', status: 'changed',
    before: { lines: [[16, 17]], sha256: 'd'.repeat(64), units: 2 },
    after: { lines: [[18, 19]], sha256: 'e'.repeat(64), units: 2 },
    references: { before: [], after: [] },
    prefix_list: {
      parser: 'cisco-prefix-list/v0.1', engine: 'prefix-list-exact/v0.1',
      report: { verdict: 'DIFFERENT', complete: true, classes: [], classes_truncated: false, total_classes: 0, operations: 10 },
    },
  };
  return {
    schema: 'plenilune.analysis/v1', tool: { version: '0.2.0-dev', commit: 'unversioned' }, versions: {},
    request: { select: [], policies: [], options: {} },
    sources: { before: { sha256: '1'.repeat(64), bytes: 10, lines: 2 }, after: { sha256: '2'.repeat(64), bytes: 10, lines: 2 } },
    objects: [object], coverage: {
      before: { lines: 2, noise_lines: 0, analyzed_lines: 2, structural_lines: 0, unsupported_lines: 0, unanalyzed_lines: 0, unanalyzed_sections: [], warnings: [] },
      after: { lines: 2, noise_lines: 0, analyzed_lines: 2, structural_lines: 0, unsupported_lines: 0, unanalyzed_lines: 0, unanalyzed_sections: [], warnings: [] },
    },
    unanalyzed_changes: [], unanalyzed_changes_truncated: false,
    summary: { objects: 1, changed: 1, equivalent: 0, unchanged: 0, added: 0, removed: 0, unsupported: 0, inconclusive: 0, not_analyzed: 0, unanalyzed_changes: 0 },
    limitations: [], sha256: 'f'.repeat(64),
  };
}

describe('configuration review flow', () => {
  it('opens a single prefix-list object without ACL parse/compare', async () => {
    vi.mocked(runtime.analyze).mockResolvedValue(artifact(prefixAnalysis()));
    const { result } = renderHook(() => useReview());
    await act(async () => {});
    await act(async () => { await result.current.exploreSource(); });
    expect(runtime.parse).not.toHaveBeenCalled();
    expect(runtime.compare).not.toHaveBeenCalled();
    expect(result.current.selection).toEqual({ domain: 'prefix-list', name: 'CUSTOMER' });
    expect(result.current.selectedObject?.prefix_list?.report.verdict).toBe('DIFFERENT');
    expect(result.current.stage).toBe('result');
  });

  it('auto-opens a single standalone ACL on the exact bytes', async () => {
    const analysis = await standaloneAnalysis(scenario.before, scenario.after);
    vi.mocked(runtime.analyze).mockResolvedValue(artifact(analysis));
    const { result } = renderHook(() => useReview());
    await act(async () => {});
    await act(async () => { await result.current.exploreSource(); });
    expect(runtime.parse).toHaveBeenCalledWith(scenario.before, scenario.after, expect.any(AbortSignal));
    expect(result.current.selection).toEqual({ domain: 'acl', name: 'MGMT' });
    expect(result.current.parsed?.firstLine).toEqual({ before: 1, after: 1 });
  });

  it('rejects an object whose slice digest does not match the analysis', async () => {
    const analysis = await standaloneAnalysis('ip access-list extended MGMT\n permit ip any any\n', 'ip access-list extended MGMT\n deny ip any any\n');
    analysis.objects[0].before!.sha256 = '0'.repeat(64);
    vi.mocked(runtime.analyze).mockResolvedValue(artifact(analysis));
    const { result } = renderHook(() => useReview());
    await act(async () => {});
    await act(async () => { await result.current.exploreSource(); });
    expect(result.current.error?.code).toBe('SOURCE_SLICE_MISMATCH');
    expect(result.current.comparison).toBeNull();
  });

  it('probes a packet on before, after and the candidate', async () => {
    const { result } = renderHook(() => useReview());
    await act(async () => {});
    await act(async () => { await result.current.parseSource(); });
    await act(async () => { await result.current.probe(packet); });
    expect(runtime.trace).toHaveBeenCalledTimes(2);
    expect(result.current.probeResult?.before.action).toBe('permit');
    expect(result.current.probeResult?.after.action).toBe('deny');
  });

  it('replays the analysis record and rejects a digest mismatch', async () => {
    const analysis = await standaloneAnalysis('ip access-list extended MGMT\n permit ip any any\n', 'ip access-list extended MGMT\n deny ip any any\n');
    vi.mocked(runtime.analyze).mockResolvedValue(artifact(analysis));
    vi.mocked(runtime.analyzeReplay).mockResolvedValue(artifact<ReplayResult>({ status: 'ANALYSIS_REPLAYED', sha256: analysis.sha256, rechecked_witnesses: 2 }));
    const { result } = renderHook(() => useReview());
    await act(async () => {});
    await act(async () => { await result.current.exploreSource(); });
    await act(async () => { await result.current.replayAnalysis(); });
    expect(result.current.analysisReplay?.result.status).toBe('ANALYSIS_REPLAYED');
    vi.mocked(runtime.analyzeReplay).mockResolvedValueOnce(artifact<ReplayResult>({ status: 'ANALYSIS_REPLAYED', sha256: '9'.repeat(64), rechecked_witnesses: 2 }));
    await act(async () => { await result.current.replayAnalysis(); });
    expect(result.current.error?.code).toBe('REPLAY_RECEIPT_MISMATCH');
    expect(result.current.analysisReplay?.result.sha256).toBe(analysis.sha256);
  });

  it('surfaces a typed analysis error without a partial artifact', async () => {
    vi.mocked(runtime.analyze).mockRejectedValueOnce(new ApiError('too large', 'INPUT_TOO_LARGE', undefined, undefined, 400));
    const { result } = renderHook(() => useReview());
    await act(async () => {});
    await act(async () => { await result.current.exploreSource(); });
    expect(result.current.error?.code).toBe('INPUT_TOO_LARGE');
    expect(result.current.analysis).toBeNull();
    expect(result.current.stage).toBe('source');
  });
});
