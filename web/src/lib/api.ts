import type {
  Analysis,
  ChatAnswer,
  Comparison,
  CompareOptions,
  Health,
  NormalizeResult,
  NormalizedPolicy,
  ObligationSet,
  Packet,
  ParseResult,
  PatchRequest,
  Policy,
  RepairResult,
  ReplayResult,
  Session,
  TraceResult,
} from './types';

interface ErrorPayload {
  error?: {
    code?: string;
    message?: string;
    line?: number;
    side?: string;
  };
}

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly code = 'REQUEST_FAILED',
    public readonly line?: number,
    public readonly side?: string,
    public readonly status?: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export interface Artifact<T> {
  data: T;
  raw: string;
}

export function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError';
}

async function request<T>(path: string, init: RequestInit): Promise<Artifact<T>> {
  let response: Response;
  try {
    response = await fetch(path, init);
  } catch (cause) {
    if (isAbort(cause)) throw cause;
    throw new ApiError('The local Plenilune runtime is unavailable.', 'RUNTIME_UNAVAILABLE');
  }
  const text = await response.text();
  if (!response.ok) {
    let payload: ErrorPayload = {};
    try {
      payload = JSON.parse(text) as ErrorPayload;
    } catch {
      // Keep the generic server message when the error body is not JSON.
    }
    throw new ApiError(
      payload.error?.message ?? `Runtime request failed with HTTP ${response.status}.`,
      payload.error?.code ?? 'REQUEST_FAILED',
      payload.error?.line,
      payload.error?.side,
      response.status,
    );
  }
  let data: T;
  try {
    data = JSON.parse(text) as T;
  } catch {
    throw new ApiError('The runtime returned malformed JSON.', 'RESPONSE_INVALID', undefined, undefined, response.status);
  }
  return { data, raw: text };
}

function post(body: unknown, signal?: AbortSignal): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  };
}

export const runtime = {
  health(signal?: AbortSignal) {
    return request<Health>('/api/health', { signal });
  },
  analyze(before: string, after: string, select: { domain: string; name: string }[] | null, policies: { domain: string; name: string; policy: Policy }[] | null, signal?: AbortSignal) {
    return request<Analysis>('/api/analyze', post({
      schema: 'plenilune.analysis-request/v1',
      before: { text: before },
      after: { text: after },
      ...(select ? { select } : {}),
      ...(policies ? { policies } : {}),
    }, signal));
  },
  analyzeReplay(before: string, after: string, evidence: Analysis, signal?: AbortSignal) {
    return request<ReplayResult>('/api/analyze/replay', post({ before, after, evidence }, signal));
  },
  trace(config: string, packet: Packet, signal?: AbortSignal) {
    return request<TraceResult>('/api/trace', post({ config, packet }, signal));
  },
  parse(before: string, after: string, signal?: AbortSignal) {
    return request<ParseResult>('/api/parse', post({ before, after }, signal));
  },
  normalize(set: ObligationSet, signal?: AbortSignal) {
    return request<NormalizeResult>('/api/obligations/normalize', post(set, signal));
  },
  normalizePolicy(policy: Policy, signal?: AbortSignal) {
    return request<NormalizedPolicy>('/api/policy/normalize', post({ policy }, signal));
  },
  compare(before: string, after: string, conditions: ObligationSet | null, options?: CompareOptions, signal?: AbortSignal) {
    return request<Comparison>('/api/compare', post({ before, after, conditions, options }, signal));
  },
  replayComparison(before: string, after: string, evidence: Comparison, signal?: AbortSignal) {
    return request<ReplayResult>('/api/compare/replay', post({ before, after, evidence }, signal));
  },
  agentSession(
    before: string,
    after: string,
    intent: string,
    policy: Policy | null,
    enabled: boolean,
    options?: CompareOptions,
    signal?: AbortSignal,
  ) {
    return request<Session>('/api/agent/session', post({ before, after, intent, policy, enabled, options }, signal));
  },
  repairVerify(
    before: string,
    after: string,
    policy: Policy,
    patch: PatchRequest,
    options?: CompareOptions,
    signal?: AbortSignal,
  ) {
    return request<RepairResult>('/api/repair/verify', post({ before, after, policy, patch, options }, signal));
  },
  repairReplay(before: string, after: string, evidence: RepairResult, signal?: AbortSignal) {
    return request<ReplayResult>('/api/repair/replay', post({ before, after, evidence }, signal));
  },
  // P1-10: interrogation over a completed artifact. The request carries
  // only the sealed artifacts and the question — no configuration text,
  // so this client method cannot start an analysis or a repair.
  chatAsk(comparison: Comparison | null, analysis: Analysis | null, question: string, locale: string, ai: boolean, signal?: AbortSignal) {
    return request<ChatAnswer>('/api/chat/ask', post({
      ...(comparison ? { comparison } : {}),
      ...(analysis ? { analysis } : {}),
      question,
      locale,
      ai,
    }, signal));
  },
};
