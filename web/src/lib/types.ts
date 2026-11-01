export type Stage = 'source' | 'conditions' | 'result' | 'repair' | 'evidence';

export type Side = 'before' | 'after';

export interface Interval {
  lo: number;
  hi: number;
}

export interface Region {
  protocol: string;
  src_cidr: string;
  dst_cidr: string;
  exclude_src: string[];
  exclude_dst: string[];
  src_ports: Interval[];
  dst_ports: Interval[];
}

export type ObligationKind = 'must_permit' | 'must_deny' | 'must_remain';

export interface Obligation {
  id: string;
  kind: ObligationKind;
  match: Region;
}

export interface ObligationSet {
  schema: string;
  scope: string;
  obligations: Obligation[];
}

export interface NormalizeResult {
  set: ObligationSet;
  sha256: string;
}

export interface ParsedRule {
  ordinal: number;
  line: number;
  action: 'permit' | 'deny';
}

export interface ParsedACL {
  name: string;
  kind: string;
  rules: ParsedRule[];
  warnings: string[] | null;
}

export interface ParsedSide {
  acl: ParsedACL;
  sha256: string;
}

export interface ParseResult {
  before: ParsedSide;
  after: ParsedSide;
}

export interface Packet {
  protocol: number;
  src_ip: string;
  dst_ip: string;
  src_port: number;
  dst_port: number;
}

export interface Decision {
  action: 'permit' | 'deny';
  rule_ordinal?: number;
  line?: number;
  implicit: boolean;
}

export interface Witness {
  id: string;
  packet: Packet;
  before: Decision;
  after: Decision;
}

export type ObligationStatus = 'HOLDS' | 'VIOLATED' | 'INCONCLUSIVE';

export interface ObligationResult {
  id: string;
  status: ObligationStatus;
  reason?: string;
  witness?: Witness;
}

export interface BudgetStats {
  operations: number;
  peak_boxes: number;
  stop_reason: string;
  timing_limited: boolean;
}

export interface CompareOptions {
  limits: {
    max_operations: number;
    max_boxes: number;
    timeout_ns: number;
  };
  max_witnesses: number;
}

export type Verdict = 'EQUIVALENT' | 'DIFFERENT' | 'INCONCLUSIVE';

// P1-2 partial result: what a budget-stopped search had already proven
// (identical prefix pairs, decided rule pairs) and the bounded remainder
// it never decided. Present only on incomplete v0.4+ results.
export interface RulePair {
  before: number;
  after: number;
  pieces: number;
}

export interface UndecidedPart {
  direction: 'lost' | 'gained' | 'delta' | 'classes';
  before_rules?: [number, number];
  after_rules?: [number, number];
  work_boxes?: number;
}

export interface PartialResult {
  stage: string;
  before_rule?: number;
  after_rule?: number;
  prefix_pairs: number;
  decided_pairs: RulePair[];
  decided_pairs_truncated?: boolean;
  undecided_pairs?: RulePair[];
  undecided_pairs_truncated?: boolean;
  undecided: UndecidedPart[];
}

export interface ComparisonReport {
  verdict: Verdict;
  search_complete: boolean;
  reason?: string;
  partial?: PartialResult;
  witnesses: Witness[];
  witnesses_truncated: boolean;
  obligations: ObligationResult[];
  budget: BudgetStats;
  options: CompareOptions;
}

export interface ComparisonInput {
  sha256: string;
  name: string;
  kind: string;
}

export interface Comparison {
  schema: 'plenilune.evidence/v1';
  tool: { version: string; commit: string };
  parser_version: string;
  engine_version: string;
  scope: string;
  before: ComparisonInput;
  after: ComparisonInput;
  conditions: ObligationSet | null;
  report: ComparisonReport;
  sha256: string;
}

export interface Policy {
  schema: 'plenilune.policy/v1';
  scope: string;
  operator_accepted: boolean;
  conditions: ObligationSet;
  allowed_changes: Region[] | null;
}

export interface RuleRef {
  base_after_sha256: string;
  ordinal: number;
  line: number;
}

export type PatchEditOp = 'add_before' | 'append' | 'remove' | 'replace';

export interface PatchEdit {
  op: PatchEditOp;
  ref?: RuleRef;
  content?: string;
}

export interface PatchRequest {
  schema: 'plenilune.patch/v1';
  base_before_sha256: string;
  base_after_sha256: string;
  obligations_digest: string;
  scope: string;
  edits: PatchEdit[];
}

export type RepairStatus = 'VERIFIED' | 'REJECTED' | 'UNVERIFIED';
export type CollateralStatus = 'VERIFIED' | 'FAILED' | 'UNCHECKED' | 'INCONCLUSIVE';

export interface RepairCollateral {
  status: CollateralStatus;
  reason?: string;
  witness?: Witness;
  stats: BudgetStats;
}

export interface RepairResult {
  schema: 'plenilune.repair/v1';
  version: string;
  status: RepairStatus;
  reason?: string;
  before_sha256: string;
  base_after_sha256: string;
  policy?: Policy | null;
  policy_sha256?: string;
  request: PatchRequest;
  options: CompareOptions;
  candidate?: string;
  candidate_sha256?: string;
  sensitive?: boolean;
  conditions: ObligationSet;
  obligations_digest?: string;
  comparison?: Comparison | null;
  collateral: RepairCollateral;
  sha256?: string;
}

export type AIStatus =
  | 'disabled'
  | 'not_configured'
  | 'unavailable'
  | 'clarification_needed'
  | 'partial'
  | 'completed'
  | 'canceled';

export interface AgentRuleRef {
  base_after_sha256: string;
  ordinal: number;
  line: number;
}

export interface AgentEdit {
  op: string;
  ref?: AgentRuleRef;
  content?: string;
}

export interface AgentObligationResult {
  id: string;
  status: string;
  evidence_ids: string[];
  reason?: string;
}

export interface AgentRepair {
  attempts: number;
  status: string;
  edits?: AgentEdit[];
  rationale?: string;
  obligations_digest?: string;
  obligations?: AgentObligationResult[];
  collateral_status?: string;
  evidence_ids?: string[];
  reason?: string;
}

export interface AcceptedObligations {
  set: ObligationSet;
  sha256: string;
  source: string;
  operator_accepted: boolean;
}

export interface AgentClaim {
  text: string;
  obligation_ids?: string[];
  evidence_ids: string[];
  ai_generated: boolean;
}

export interface AIUsage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface AIProvenance {
  provider: string;
  model: string;
  prompt_version: string;
  requests: number;
  retries: number;
  tool_calls: number;
  repair_proposals: number;
  usage: AIUsage;
  request_sha256: string[];
  response_sha256: string[];
  elapsed_ms: number;
}

export interface AIResult {
  schema: 'plenilune.ai/v1';
  status: AIStatus;
  reason?: string;
  obligations?: AcceptedObligations;
  checks?: AgentObligationResult[];
  repair?: AgentRepair;
  claims?: AgentClaim[];
  dropped_claims?: number;
  questions?: string[];
  summary?: string;
  provenance: AIProvenance;
}

export interface Session {
  schema: 'plenilune.session/v1';
  comparison: Comparison;
  ai: AIResult;
  repairs: RepairResult[];
}

export type ReplayStatus = 'COMPARISON_REPLAYED' | 'REPAIR_REPLAYED' | 'ANALYSIS_REPLAYED' | 'TIMING_SENSITIVE';

export interface ReplayResult {
  status: ReplayStatus;
  sha256: string;
  rechecked_witnesses: number;
}

export interface Health {
  service: string;
  runtime: Record<string, string>;
}

export interface NormalizedPolicy {
  policy: Policy;
  policy_sha256: string;
  obligations_digest: string;
}

export interface DraftEdit {
  id: string;
  edit: PatchEdit;
}

export interface Sample {
  id: string;
  nameKey: 'source.sample.ssh' | 'source.sample.budget' | 'source.sample.unsupported' | 'source.sample.fullRegression' | 'source.sample.fullRouting' | 'source.sample.fullRefactor';
  before: string;
  after: string;
  intent: string;
  conditions: Obligation[];
  scopeMode: 'undeclared' | 'none' | 'regions';
  allowedChanges: Region[];
  options?: CompareOptions;
}

export type AnalysisDomain = 'acl' | 'prefix-list';
export type ObjectStatus = 'changed' | 'equivalent' | 'unchanged' | 'added' | 'removed' | 'unsupported' | 'inconclusive' | 'not_analyzed';

export interface Extent {
  lines: [number, number][];
  byte_start?: number;
  byte_end?: number;
  sha256: string;
  units: number;
}

export interface ObjectError {
  side: 'before' | 'after';
  code: string;
  message: string;
  line?: number;
}

export interface DecidingRule {
  action: 'permit' | 'deny';
  ordinal?: number;
  line?: number;
  implicit: boolean;
}

export interface BoxText {
  protocol: string;
  src: string;
  dst: string;
  src_ports: string;
  dst_ports: string;
}

export type ChangeScope = 'undeclared' | 'inside_allowed' | 'outside_allowed' | 'mixed';

export interface ChangeClass {
  effect: 'newly_permitted' | 'newly_denied';
  before: DecidingRule;
  after: DecidingRule;
  witness: Witness;
  pieces: number;
  examples: BoxText[];
  examples_truncated: boolean;
  scope: ChangeScope;
  outside_witness?: Witness;
}

export interface ChangeReport {
  schema: string;
  engine: string;
  complete: boolean;
  reason?: string;
  partial?: PartialResult;
  scope_declared: boolean;
  classes: ChangeClass[];
  classes_truncated: boolean;
  total_classes: number;
  budget: BudgetStats;
}

export interface ACLAnalysis {
  engine: string;
  verdict: Verdict;
  search_complete: boolean;
  reason?: string;
  partial?: PartialResult;
  witnesses: Witness[];
  witnesses_truncated: boolean;
  budget: BudgetStats;
  changes: ChangeReport;
  policy_sha256?: string;
}

export interface PrefixDecision {
  action: 'permit' | 'deny';
  seq?: number;
  line?: number;
  implicit: boolean;
}

export interface EntryRef extends PrefixDecision {
  text?: string;
}

export interface RouteRange {
  length: number;
  first: string;
  last: string;
  routes: number;
}

export interface PrefixClass {
  effect: 'newly_permitted' | 'newly_denied';
  before: EntryRef;
  after: EntryRef;
  witness: { route: string; before: PrefixDecision; after: PrefixDecision };
  routes: number;
  lengths: number[];
  ranges: RouteRange[];
  ranges_truncated: boolean;
  total_ranges: number;
}

export interface PrefixReport {
  verdict: Verdict;
  complete: boolean;
  reason?: string;
  classes: PrefixClass[];
  classes_truncated: boolean;
  total_classes: number;
  operations: number;
}

export interface AnalysisObject {
  domain: AnalysisDomain;
  name: string;
  kind: string;
  status: ObjectStatus;
  reason?: string;
  before?: Extent;
  after?: Extent;
  references: { before: { line: number; kind: string; name: string }[]; after: { line: number; kind: string; name: string }[] };
  errors?: ObjectError[];
  acl?: ACLAnalysis;
  prefix_list?: { parser: string; engine: string; report: PrefixReport };
}

export interface AnalysisCoverage {
  lines: number;
  noise_lines: number;
  analyzed_lines: number;
  structural_lines: number;
  unsupported_lines: number;
  unanalyzed_lines: number;
  unanalyzed_sections: { kind: string; count: number }[];
  warnings: string[];
}

export interface SectionChange {
  kind: string;
  header_kind: string;
  lines: number;
  section_sha256: string;
  change: 'added' | 'removed' | 'modified';
  before_line?: number;
  after_line?: number;
}

export interface AnalysisSummary {
  objects: number;
  changed: number;
  equivalent: number;
  unchanged: number;
  added: number;
  removed: number;
  unsupported: number;
  inconclusive: number;
  not_analyzed: number;
  unanalyzed_changes: number;
}

export interface Analysis {
  schema: 'plenilune.analysis/v1' | 'plenilune.analysis/v2';
  tool: { version: string; commit: string };
  versions: Record<string, string>;
  request: { select: { domain: string; name: string }[]; policies: { domain: string; name: string; policy_sha256: string }[]; options: unknown };
  sources: { before: { sha256: string; bytes: number; lines: number; ref?: string }; after: { sha256: string; bytes: number; lines: number; ref?: string } };
  objects: AnalysisObject[];
  coverage: { before: AnalysisCoverage; after: AnalysisCoverage };
  unanalyzed_changes: SectionChange[];
  unanalyzed_changes_truncated: boolean;
  summary: AnalysisSummary;
  limitations: string[];
  sha256: string;
}

export interface ObjectSelection {
  domain: AnalysisDomain;
  name: string;
}

export interface TraceResult {
  schema: string;
  config_sha256: string;
  packet: Packet;
  decision: Decision;
  sha256: string;
}

// P1-10 interrogation chat: every answer is rendered from these facts;
// the optional AI phrasing is presentation only and never replaces them.
export interface ChatFact {
  label: string;
  value: string;
  source: string;
}

export interface ChatPhrasing {
  status: 'off' | 'ok' | 'not_configured' | 'model_unsupported' | 'unavailable' | 'error';
  model?: string;
  text?: string;
}

export interface ChatAnswer {
  schema: string;
  class: string;
  text: string;
  facts: ChatFact[];
  artifact_sha256: string;
  locale: string;
  ai: ChatPhrasing;
}
