import type {
  AcceptedSnapshot,
  ComparisonSnapshot,
  RepairSnapshot,
} from '../src/hooks/useReview';
import { samples } from '../src/lib/samples';
import type {
  Analysis,
  ChangeClass,
  Comparison,
  ObligationResult,
  ObligationSet,
  ParseResult,
  Policy,
  RepairResult,
  Session,
  Witness,
} from '../src/lib/types';

export const scenario = samples[0];

export const parsedResult: ParseResult = {
  before: {
    acl: {
      name: 'MGMT',
      kind: 'named-extended',
      rules: [
        { ordinal: 0, line: 3, action: 'permit' },
        { ordinal: 1, line: 4, action: 'permit' },
        { ordinal: 2, line: 5, action: 'deny' },
      ],
      warnings: [],
    },
    sha256: 'a'.repeat(64),
  },
  after: {
    acl: {
      name: 'MGMT',
      kind: 'named-extended',
      rules: [
        { ordinal: 0, line: 3, action: 'deny' },
        { ordinal: 1, line: 4, action: 'permit' },
        { ordinal: 2, line: 5, action: 'permit' },
        { ordinal: 3, line: 6, action: 'deny' },
      ],
      warnings: [],
    },
    sha256: 'b'.repeat(64),
  },
};

export const normalizedSet: ObligationSet = {
  schema: 'plenilune.obligations/v1',
  scope: 'ipv4-unfragmented-or-initial-with-complete-l4/v1',
  obligations: scenario.conditions,
};

export const normalizedDigest = 'e'.repeat(64);

export const witness: Witness = {
  id: 'wit-test-1',
  packet: {
    protocol: 6,
    src_ip: '192.0.2.10',
    dst_ip: '198.51.100.10',
    src_port: 0,
    dst_port: 22,
  },
  before: { action: 'permit', rule_ordinal: 0, line: 3, implicit: false },
  after: { action: 'deny', rule_ordinal: 0, line: 3, implicit: false },
};

const options = {
  limits: {
    max_operations: 2_000_000,
    max_boxes: 50_000,
    timeout_ns: 2_000_000_000,
  },
  max_witnesses: 8,
};

function makeComparison(
  verdict: Comparison['report']['verdict'],
  obligations: ObligationResult[],
  witnesses: Witness[],
  beforeSha = parsedResult.before.sha256,
  afterSha = parsedResult.after.sha256,
  sha = 'c'.repeat(64),
): Comparison {
  return {
    schema: 'plenilune.evidence/v1',
    tool: { version: '0.2.0-dev', commit: 'unversioned' },
    parser_version: 'cisco-acl/v0.2',
    engine_version: 'acl-exact/v0.1',
    scope: normalizedSet.scope,
    before: { sha256: beforeSha, name: 'MGMT', kind: 'named-extended' },
    after: { sha256: afterSha, name: 'MGMT', kind: 'named-extended' },
    conditions: normalizedSet,
    report: {
      verdict,
      search_complete: true,
      witnesses,
      witnesses_truncated: false,
      obligations,
      budget: { operations: 10, peak_boxes: 2, stop_reason: '', timing_limited: false },
      options,
    },
    sha256: sha,
  };
}

export const comparison: Comparison = makeComparison(
  'DIFFERENT',
  [
    { id: 'OB-1', status: 'VIOLATED', witness },
    { id: 'OB-2', status: 'HOLDS' },
    { id: 'OB-3', status: 'HOLDS' },
  ],
  [witness],
);

export const candidateComparison: Comparison = makeComparison(
  'EQUIVALENT',
  [
    { id: 'OB-1', status: 'HOLDS' },
    { id: 'OB-2', status: 'HOLDS' },
    { id: 'OB-3', status: 'HOLDS' },
  ],
  [],
  parsedResult.before.sha256,
  'f'.repeat(64),
  'f'.repeat(63) + 'a',
);

export const comparisonSnapshot: ComparisonSnapshot = {
  comparison,
  raw: JSON.stringify(comparison),
  options: undefined,
};

export const policy: Policy = {
  schema: 'plenilune.policy/v1',
  scope: normalizedSet.scope,
  operator_accepted: true,
  conditions: normalizedSet,
  allowed_changes: [],
};

export const accepted: AcceptedSnapshot = {
  conditions: normalizedSet,
  digest: normalizedDigest,
  policy,
  policySha256: 'd'.repeat(64),
  before: scenario.before,
  after: scenario.after,
  intent: scenario.intent,
  parsed: parsedResult,
  options: undefined,
  scopeMode: 'none',
  allowedChanges: [],
  acceptedAt: '2026-10-30T10:00:00.000Z',
};

export const repairReceipt: RepairResult = {
  schema: 'plenilune.repair/v1',
  version: 'acl-repair/v0.1',
  status: 'VERIFIED',
  before_sha256: parsedResult.before.sha256,
  base_after_sha256: parsedResult.after.sha256,
  policy,
  policy_sha256: 'd'.repeat(64),
  request: {
    schema: 'plenilune.patch/v1',
    base_before_sha256: parsedResult.before.sha256,
    base_after_sha256: parsedResult.after.sha256,
    obligations_digest: normalizedDigest,
    scope: normalizedSet.scope,
    edits: [{ op: 'remove', ref: { base_after_sha256: parsedResult.after.sha256, ordinal: 0, line: 3 } }],
  },
  options,
  candidate: scenario.before,
  candidate_sha256: 'f'.repeat(64),
  conditions: normalizedSet,
  obligations_digest: normalizedDigest,
  comparison: candidateComparison,
  collateral: {
    status: 'VERIFIED',
    stats: { operations: 4, peak_boxes: 1, stop_reason: '', timing_limited: false },
  },
  sha256: '8'.repeat(64),
};

export const foreignRepairReceipt: RepairResult = {
  ...repairReceipt,
  before_sha256: '1'.repeat(64),
  request: { ...repairReceipt.request, base_before_sha256: '1'.repeat(64) },
  sha256: '7'.repeat(64),
};

// Same obligations digest, different accepted policy hash (scope differs only).
export const scopeOnlyRepairReceipt: RepairResult = {
  ...repairReceipt,
  policy: {
    ...policy,
    allowed_changes: [{
      protocol: 'tcp', src_cidr: '192.0.2.0/24', dst_cidr: '198.51.100.10/32',
      exclude_src: [], exclude_dst: [], src_ports: [], dst_ports: [{ lo: 22, hi: 22 }],
    }],
  },
  policy_sha256: '6'.repeat(64),
  sha256: '5'.repeat(64),
};

export const repairSnapshot: RepairSnapshot = {
  receipt: repairReceipt,
  raw: JSON.stringify(repairReceipt),
  source: 'manual',
};

export const provenance = {
  provider: 'meta-model-api',
  model: 'muse-spark-1.3',
  prompt_version: 'meta-orchestrator/v2',
  requests: 1,
  retries: 0,
  tool_calls: 1,
  repair_proposals: 0,
  usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
  request_sha256: [],
  response_sha256: [],
  elapsed_ms: 12,
};

export const sessionNotConfigured: Session = {
  schema: 'plenilune.session/v1',
  comparison,
  ai: {
    schema: 'plenilune.ai/v1',
    status: 'not_configured',
    reason: 'provider_not_configured',
    provenance,
  },
  repairs: [],
};

export const sessionProposal: Session = {
  schema: 'plenilune.session/v1',
  comparison,
  ai: {
    schema: 'plenilune.ai/v1',
    status: 'completed',
    obligations: {
      set: normalizedSet,
      sha256: normalizedDigest,
      source: 'ai_proposed_validated',
      operator_accepted: false,
    },
    summary: 'These conditions are a draft.',
    provenance,
  },
  repairs: [],
};

async function sha256Hex(bytes: Uint8Array<ArrayBuffer>): Promise<string> {
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes));
  return Array.from(digest, (b) => b.toString(16).padStart(2, '0')).join('');
}

// A standalone single-ACL analysis whose extents hash the exact input bytes.
export async function standaloneAnalysis(before: string, after: string): Promise<Analysis> {
  const extentOf = async (text: string) => {
    const bytes = new TextEncoder().encode(text);
    return { lines: [[1, text.split('\n').length - 1] as [number, number]], byte_start: 0, byte_end: bytes.length, sha256: await sha256Hex(bytes), units: 3 };
  };
  const b = await extentOf(before);
  const a = await extentOf(after);
  const extent = (text: string) => (text === before ? b : a);
  const changed = before !== after;
  return {
    schema: 'plenilune.analysis/v1',
    tool: { version: '0.2.0-dev', commit: 'unversioned' },
    versions: { intake: 'plenilune-intake/v0.1', acl_engine: 'acl-exact/v0.2' },
    request: { select: [], policies: [], options: {} },
    sources: { before: { sha256: extent(before).sha256, bytes: before.length, lines: 7 }, after: { sha256: extent(after).sha256, bytes: after.length, lines: 8 } },
    objects: [{
      domain: 'acl', name: 'MGMT', kind: 'named-extended', status: changed ? 'changed' : 'unchanged',
      before: extent(before), after: extent(after), references: { before: [], after: [] },
      acl: changed ? {
        engine: 'acl-exact/v0.2', verdict: 'DIFFERENT', search_complete: true, witnesses: [witness], witnesses_truncated: false,
        budget: { operations: 10, peak_boxes: 2, stop_reason: '', timing_limited: false },
        changes: { schema: 'plenilune.acl-changes/v1', engine: 'acl-exact/v0.2', complete: true, scope_declared: false, classes: [changeClass], classes_truncated: false, total_classes: 1, budget: { operations: 10, peak_boxes: 2, stop_reason: '', timing_limited: false } },
      } : undefined,
    }],
    coverage: { before: emptyCoverage, after: emptyCoverage },
    unanalyzed_changes: [], unanalyzed_changes_truncated: false,
    summary: { objects: 1, changed: changed ? 1 : 0, equivalent: 0, unchanged: changed ? 0 : 1, added: 0, removed: 0, unsupported: 0, inconclusive: 0, not_analyzed: 0, unanalyzed_changes: 0 },
    limitations: ['behavior_evaluated_per_object_without_binding'],
    sha256: '3'.repeat(64),
  };
}

const emptyCoverage = { lines: 7, noise_lines: 1, analyzed_lines: 6, structural_lines: 0, unsupported_lines: 0, unanalyzed_lines: 0, unanalyzed_sections: [], warnings: [] };

export const changeClass: ChangeClass = {
  effect: 'newly_denied',
  before: { action: 'permit', ordinal: 0, line: 3, implicit: false },
  after: { action: 'deny', ordinal: 0, line: 3, implicit: false },
  witness, pieces: 1,
  examples: [{ protocol: 'tcp', src: '192.0.2.10/32', dst: '198.51.100.10/32', src_ports: 'any', dst_ports: '22' }],
  examples_truncated: false, scope: 'undeclared',
};
