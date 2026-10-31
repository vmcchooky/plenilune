import { useEffect, useRef, useState } from 'react';
import { ApiError, isAbort, runtime } from '../lib/api';
import {
  buildObligationSet,
  buildRegions,
  conditionDraft,
  emptyConditionDraft,
  emptyRegionDraft,
  nextConditionId,
  regionDraft,
} from '../lib/conditions';
import type { ConditionDraft, RegionDraft } from '../lib/conditions';
import { toReviewError } from '../lib/errors';
import type { ReviewError } from '../lib/errors';
import { bindingFromAccepted, receiptBindingIssue } from '../lib/receiptBinding';
import { blankSample, findSample, isPristineSample, samples } from '../lib/samples';
import type {
  AIResult,
  Analysis,
  AnalysisObject,
  ChangeReport,
  Comparison,
  CompareOptions,
  DraftEdit,
  ObligationSet,
  NormalizedPolicy,
  ObjectSelection,
  Packet,
  ParseResult,
  PatchEdit,
  PatchRequest,
  Policy,
  Region,
  RepairResult,
  ReplayResult,
  Sample,
  Session,
  Stage,
  TraceResult,
} from '../lib/types';

export type BusyKind =
  | 'parse'
  | 'probe'
  | 'replayAnalysis'
  | 'previewPolicy'
  | 'compare'
  | 'aiProposal'
  | 'aiRepair'
  | 'repair'
  | 'replayComparison'
  | 'replayRepair';

export type BackendState = 'checking' | 'online' | 'offline';

export interface Draft {
  before: string;
  after: string;
  intent: string;
  conditions: ConditionDraft[];
  scopeMode: 'undeclared' | 'none' | 'regions';
  allowedChanges: RegionDraft[];
  options?: CompareOptions;
}

interface ParseSnapshot {
  parsed: ParseResult;
  before: string;
  after: string;
  intent: string;
  // First source line of the analyzed block in each configuration version.
  firstLine: { before: number; after: number };
}

export interface AnalysisSnapshot {
  result: Analysis;
  raw: string;
  before: string;
  after: string;
}

export interface ProbeResult {
  packet: Packet;
  before: TraceResult['decision'];
  after: TraceResult['decision'];
  candidate?: TraceResult['decision'];
}

// Engine input for an ACL object is the exact byte slice the server hashed.
async function sliceObject(source: string, start: number | undefined, end: number | undefined, sha256: string): Promise<string> {
  const bytes = new TextEncoder().encode(source);
  if (start === undefined || end === undefined || start < 0 || end > bytes.length || start > end) {
    throw new ApiError('Object location is outside the configuration.', 'SOURCE_SLICE_MISMATCH');
  }
  const slice = bytes.slice(start, end);
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', slice));
  const hex = Array.from(digest, (b) => b.toString(16).padStart(2, '0')).join('');
  if (hex !== sha256) throw new ApiError('Extracted object bytes do not match the analysis digest.', 'SOURCE_SLICE_MISMATCH');
  return new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(slice);
}

function analyzable(o: AnalysisObject): boolean {
  return Boolean(o.before && o.after && !(o.errors?.length)) && ['changed', 'equivalent', 'unchanged', 'inconclusive'].includes(o.status);
}

// Open one object directly only when nothing else could compete for attention.
function autoTarget(a: Analysis): ObjectSelection | null {
  const candidates = a.objects.filter(analyzable);
  if (candidates.length !== 1 || a.objects.length !== 1) return null;
  return { domain: candidates[0].domain, name: candidates[0].name };
}

export interface AcceptedSnapshot {
  conditions: ObligationSet;
  digest: string;
  policy: Policy;
  policySha256: string;
  before: string;
  after: string;
  intent: string;
  parsed: ParseResult;
  options?: CompareOptions;
  scopeMode: 'undeclared' | 'none' | 'regions';
  allowedChanges: RegionDraft[];
  acceptedAt: string;
}

export interface ComparisonSnapshot {
  comparison: Comparison;
  raw: string;
  options?: CompareOptions;
}

export interface RepairSnapshot {
  receipt: RepairResult;
  raw: string;
  source: 'manual' | 'ai';
  sessionRaw?: string;
}

export interface SessionSnapshot {
  session: Session;
  raw: string;
}

export interface ReplaySnapshot {
  result: ReplayResult;
  raw: string;
}

interface PolicyPreview {
  normalized: NormalizedPolicy;
  draftSignature: string;
}

export interface ReviewDecision {
  choice: 'received' | 'held' | 'rejected';
  receiptSha256: string;
}

type EditSource = 'manual' | 'ai';

function draftFromSample(sample: Sample): Draft {
  return {
    before: sample.before,
    after: sample.after,
    intent: sample.intent,
    conditions: sample.conditions.map(conditionDraft),
    scopeMode: sample.scopeMode,
    allowedChanges: sample.allowedChanges.map(regionDraft),
    options: sample.options,
  };
}

function blankDraft(): Draft {
  return {
    before: blankSample.before,
    after: blankSample.after,
    intent: blankSample.intent,
    conditions: [],
    scopeMode: blankSample.scopeMode,
    allowedChanges: [],
  };
}

function firstWitnessId(comparison: Comparison): string | null {
  for (const obligation of comparison.report.obligations) {
    if (obligation.status === 'VIOLATED' && obligation.witness) return obligation.witness.id;
  }
  if (comparison.report.witnesses[0]) return comparison.report.witnesses[0].id;
  for (const obligation of comparison.report.obligations) {
    if (obligation.witness) return obligation.witness.id;
  }
  return null;
}

function hasViolation(comparison: Comparison | null): boolean {
  return comparison?.report.obligations.some((obligation) => obligation.status === 'VIOLATED') ?? false;
}

export function useReview() {
  const initial = draftFromSample(samples[0]);
  const [sampleId, setSampleId] = useState<string | null>(samples[0].id);
  const [modified, setModified] = useState(false);
  const [draft, setDraft] = useState<Draft>(initial);
  const [stage, setStage] = useState<Stage>('source');
  const [parsed, setParsed] = useState<ParseSnapshot | null>(null);
  const [accepted, setAccepted] = useState<AcceptedSnapshot | null>(null);
  const [policyPreview, setPolicyPreview] = useState<PolicyPreview | null>(null);
  const [decision, setDecision] = useState<ReviewDecision | null>(null);
  const [disputed, setDisputed] = useState(false);
  const [comparison, setComparison] = useState<ComparisonSnapshot | null>(null);
  const [aiProposal, setAiProposal] = useState<SessionSnapshot | null>(null);
  const [aiSession, setAiSession] = useState<SessionSnapshot | null>(null);
  const [manualRepair, setManualRepair] = useState<RepairSnapshot | null>(null);
  const [repairSource, setRepairSource] = useState<EditSource>('manual');
  const [selectedAiIndex, setSelectedAiIndex] = useState(0);
  const [proposalAdopted, setProposalAdopted] = useState(false);
  const [edits, setEdits] = useState<DraftEdit[]>([]);
  const [selectedWitnessId, setSelectedWitnessId] = useState<string | null>(null);
  const [comparisonReplay, setComparisonReplay] = useState<ReplaySnapshot | null>(null);
  const [analysis, setAnalysis] = useState<AnalysisSnapshot | null>(null);
  const [selection, setSelection] = useState<ObjectSelection | null>(null);
  const [scopedChanges, setScopedChanges] = useState<ChangeReport | null>(null);
  const [probeResult, setProbeResult] = useState<ProbeResult | null>(null);
  const [analysisReplay, setAnalysisReplay] = useState<ReplaySnapshot | null>(null);
  const [repairReplay, setRepairReplay] = useState<ReplaySnapshot | null>(null);
  const [busy, setBusy] = useState<BusyKind | null>(null);
  const [canceled, setCanceled] = useState<BusyKind | null>(null);
  const [error, setError] = useState<ReviewError | null>(null);
  const [backend, setBackend] = useState<BackendState>('checking');
  const generationRef = useRef(0);
  const abortRef = useRef<AbortController | null>(null);
  const editCounter = useRef(0);
  const policyPreviewRef = useRef<PolicyPreview | null>(null);
  const disputedHashesRef = useRef(new Set<string>());

  useEffect(() => {
    const controller = new AbortController();
    runtime.health(controller.signal)
      .then(() => setBackend('online'))
      .catch((cause) => {
        if (!isAbort(cause)) setBackend('offline');
      });
    return () => controller.abort();
  }, []);

  useEffect(() => () => {
    generationRef.current += 1;
    abortRef.current?.abort();
  }, []);

  const sample = sampleId ? findSample(sampleId) : null;
  const sourceKind: 'sample' | 'modified' | 'custom' = sample === null
    ? 'custom'
    : modified
      ? 'modified'
      : 'sample';
  const pristine = sample !== null && !modified && isPristineSample(sample, draft);

  const aiRepairs = aiSession?.session.repairs ?? [];
  const selectedAiReceipt = aiRepairs[selectedAiIndex] ?? null;
  const rawRepair: RepairSnapshot | null = repairSource === 'ai' && selectedAiReceipt && aiSession
    ? { receipt: selectedAiReceipt, raw: aiSession.raw, source: 'ai', sessionRaw: aiSession.raw }
    : manualRepair;
  // A receipt that cannot be bound to the accepted snapshot is never shown,
  // received or exported, even if its transport/request generation is current.
  const repair = rawRepair && accepted
    && receiptBindingIssue(rawRepair.receipt, bindingFromAccepted(accepted)) === null
    ? rawRepair
    : null;
  const lastAi: AIResult | null = aiSession?.session.ai ?? aiProposal?.session.ai ?? null;
  const report = comparison?.comparison.report ?? null;
  const canRepair = Boolean(accepted && report && (report.verdict === 'DIFFERENT' || hasViolation(comparison?.comparison ?? null)));
  const isRepairDisputed = disputed || Boolean(repair?.receipt.sha256 && disputedHashesRef.current.has(repair.receipt.sha256));
  const canReceive = Boolean(accepted && repair && !isRepairDisputed && repair.receipt.status === 'VERIFIED'
    && repair.receipt.policy?.operator_accepted
    && repair.receipt.collateral.status === 'VERIFIED'
    && repair.receipt.comparison?.report.obligations.length
    && repair.receipt.comparison.report.obligations.every((item) => item.status === 'HOLDS')
    && receiptBindingIssue(repair.receipt, bindingFromAccepted(accepted)) === null);
  const canExport = comparison !== null;
  const selectedObject = selection && analysis
    ? analysis.result.objects.find((o) => o.domain === selection.domain && o.name === selection.name) ?? null
    : null;
  const aclChanges = scopedChanges ?? (selectedObject?.domain === 'acl' ? selectedObject.acl?.changes ?? null : null);
  const holdsCount = report ? report.obligations.filter((item) => item.status === 'HOLDS').length : 0;
  const totalConditions = report ? report.obligations.length : 0;

  function abortCurrent() {
    generationRef.current += 1;
    abortRef.current?.abort();
    abortRef.current = null;
    setBusy(null);
  }

  function begin(kind: BusyKind) {
    generationRef.current += 1;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setBusy(kind);
    setCanceled(null);
    setError(null);
    return { generation: generationRef.current, controller };
  }

  function settle(generation: number) {
    if (generationRef.current !== generation) return;
    abortRef.current = null;
    setBusy(null);
  }

  function fail(cause: unknown, generation: number, kind: BusyKind) {
    if (generationRef.current !== generation) return;
    if (isAbort(cause)) {
      setCanceled(kind);
      return;
    }
    setError(toReviewError(cause));
  }

  async function watch<T>(operation: Promise<T>): Promise<T> {
    const generation = generationRef.current;
    try {
      const result = await operation;
      if (generationRef.current === generation) setBackend('online');
      return result;
    } catch (cause) {
      if (generationRef.current === generation && cause instanceof ApiError) {
        setBackend(cause.code === 'RUNTIME_UNAVAILABLE' ? 'offline' : 'online');
      }
      throw cause;
    }
  }

  function clearProofArtifacts() {
    setPolicyPreview(null);
    policyPreviewRef.current = null;
    setDecision(null);
    setDisputed(false);
    setAccepted(null);
    setComparison(null);
    setAiSession(null);
    setManualRepair(null);
    setRepairSource('manual');
    setSelectedAiIndex(0);
    setComparisonReplay(null);
    setRepairReplay(null);
    setEdits([]);
    setSelectedWitnessId(null);
    setScopedChanges(null);
    setProbeResult(null);
  }

  function clearForSourceChange() {
    abortCurrent();
    clearProofArtifacts();
    setAnalysis(null);
    setSelection(null);
    setAnalysisReplay(null);
    setAiProposal(null);
    setParsed(null);
    setProposalAdopted(false);
    setCanceled(null);
    setError(null);
    setStage('source');
  }

  function clearForConditionChange() {
    abortCurrent();
    clearProofArtifacts();
    setProposalAdopted(false);
    setCanceled(null);
    setError(null);
    setStage('conditions');
  }

  function selectSample(id: string) {
    const next = findSample(id);
    clearForSourceChange();
    if (next) {
      setSampleId(next.id);
      setDraft(draftFromSample(next));
      setModified(false);
    } else {
      setSampleId(null);
      setDraft(blankDraft());
      setModified(false);
    }
  }

  function updateSource(field: 'before' | 'after' | 'intent', value: string) {
    if (draft[field] === value) return;
    clearForSourceChange();
    setDraft((current) => ({ ...current, [field]: value, options: undefined }));
    setModified(true);
  }

  function updateIntent(value: string) {
    if (draft.intent === value) return;
    abortCurrent();
    clearProofArtifacts();
    setAiProposal(null);
    setParsed((current) => current ? { ...current, intent: value } : null);
    setDraft((current) => ({ ...current, intent: value }));
    setProposalAdopted(false);
    setStage('conditions');
    setModified(true);
    setError(null);
    setCanceled(null);
  }

  async function parseSource() {
    clearProofArtifacts();
    setAiProposal(null);
    setParsed(null);
    setProposalAdopted(false);
    const { generation, controller } = begin('parse');
    const before = draft.before;
    const after = draft.after;
    try {
      const result = await watch(runtime.parse(before, after, controller.signal));
      if (generationRef.current !== generation) return;
      setParsed({ parsed: result.data, before, after, intent: draft.intent, firstLine: { before: 1, after: 1 } });
      setStage('conditions');
      setCanceled(null);
    } catch (cause) {
      fail(cause, generation, 'parse');
    } finally {
      settle(generation);
    }
  }

  // Opens one object of the current analysis. ACL objects run the exact
  // comparison on the hashed block; other domains use the analysis result.
  async function openObject(snapshot: AnalysisSnapshot, target: ObjectSelection, generation: number, signal: AbortSignal) {
    const object = snapshot.result.objects.find((o) => o.domain === target.domain && o.name === target.name);
    if (!object) throw new ApiError('Selected object is not in the analysis.', 'SELECTION_INVALID');
    if (object.domain !== 'acl' || !object.before || !object.after || object.errors?.length) {
      setSelection(target);
      setSelectedWitnessId(null);
      setStage('result');
      return;
    }
    const before = await sliceObject(snapshot.before, object.before.byte_start, object.before.byte_end, object.before.sha256);
    const after = await sliceObject(snapshot.after, object.after.byte_start, object.after.byte_end, object.after.sha256);
    if (generationRef.current !== generation) return;
    const source = await watch(runtime.parse(before, after, signal));
    if (generationRef.current !== generation) return;
    setParsed({ parsed: source.data, before, after, intent: draft.intent, firstLine: { before: object.before.lines[0][0], after: object.after.lines[0][0] } });
    const result = await watch(runtime.compare(before, after, null, draft.options, signal));
    if (generationRef.current !== generation) return;
    setSelection(target);
    setComparison({ comparison: result.data, raw: result.raw, options: draft.options });
    setSelectedWitnessId(firstWitnessId(result.data));
    setStage('result');
  }

  async function exploreSource() {
    clearProofArtifacts();
    setAiProposal(null);
    setParsed(null);
    setAnalysis(null);
    setSelection(null);
    setAnalysisReplay(null);
    const { generation, controller } = begin('parse');
    const before = draft.before;
    const after = draft.after;
    try {
      const analyzed = await watch(runtime.analyze(before, after, null, null, controller.signal));
      if (generationRef.current !== generation) return;
      const snapshot: AnalysisSnapshot = { result: analyzed.data, raw: analyzed.raw, before, after };
      setAnalysis(snapshot);
      const target = autoTarget(analyzed.data);
      if (!target) {
        setStage('result');
        return;
      }
      await openObject(snapshot, target, generation, controller.signal);
    } catch (cause) {
      fail(cause, generation, 'parse');
    } finally {
      settle(generation);
    }
  }

  async function selectObject(target: ObjectSelection) {
    const snapshot = analysis;
    if (!snapshot) return;
    if (selection?.domain === target.domain && selection.name === target.name && (target.domain !== 'acl' || parsed)) {
      setStage('result');
      return;
    }
    clearProofArtifacts();
    setAiProposal(null);
    setParsed(null);
    setSelection(null);
    setProposalAdopted(false);
    const { generation, controller } = begin('parse');
    try {
      await openObject(snapshot, target, generation, controller.signal);
    } catch (cause) {
      fail(cause, generation, 'parse');
    } finally {
      settle(generation);
    }
  }

  // A point probe evaluates one packet with the independent evaluator on each
  // version; it explains a decision and is never treated as proof.
  async function probe(packet: Packet) {
    const source = accepted ?? parsed;
    if (!source) return;
    const candidate = repair?.receipt.candidate;
    const { generation, controller } = begin('probe');
    try {
      const [b, a, c] = await Promise.all([
        watch(runtime.trace(source.before, packet, controller.signal)),
        watch(runtime.trace(source.after, packet, controller.signal)),
        candidate ? watch(runtime.trace(candidate, packet, controller.signal)) : Promise.resolve(null),
      ]);
      if (generationRef.current !== generation) return;
      setProbeResult({ packet, before: b.data.decision, after: a.data.decision, candidate: c?.data.decision });
    } catch (cause) {
      fail(cause, generation, 'probe');
    } finally {
      settle(generation);
    }
  }

  async function replayAnalysis() {
    const snapshot = analysis;
    if (!snapshot) return;
    const { generation, controller } = begin('replayAnalysis');
    try {
      const result = await watch(runtime.analyzeReplay(snapshot.before, snapshot.after, snapshot.result, controller.signal));
      if (generationRef.current !== generation) return;
      if (result.data.sha256 !== snapshot.result.sha256) throw new ApiError('Replay did not match the analysis.', 'REPLAY_RECEIPT_MISMATCH');
      setAnalysisReplay({ result: result.data, raw: result.raw });
    } catch (cause) {
      fail(cause, generation, 'replayAnalysis');
    } finally {
      settle(generation);
    }
  }

  async function proposeConditions() {
    if (!parsed || !parsed.intent.trim()) return;
    const { generation, controller } = begin('aiProposal');
    try {
      const result = await watch(runtime.agentSession(
        parsed.before,
        parsed.after,
        parsed.intent,
        null,
        true,
        undefined,
        controller.signal,
      ));
      if (generationRef.current !== generation) return;
      setAiProposal({ session: result.data, raw: result.raw });
      setProposalAdopted(false);
      setCanceled(null);
    } catch (cause) {
      fail(cause, generation, 'aiProposal');
    } finally {
      settle(generation);
    }
  }

  function adoptProposal() {
    const proposed = aiProposal?.session.ai.obligations?.set;
    if (!proposed) return;
    clearForConditionChange();
    setDraft((current) => ({ ...current, conditions: proposed.obligations.map(conditionDraft) }));
    setProposalAdopted(true);
    setModified(true);
  }

  function addCondition() {
    const id = nextConditionId(draft.conditions);
    if (!id) return;
    clearForConditionChange();
    setDraft((current) => ({ ...current, conditions: [...current.conditions, emptyConditionDraft(id)] }));
    setModified(true);
  }

  function updateCondition(id: string, next: ConditionDraft) {
    clearForConditionChange();
    setDraft((current) => ({
      ...current,
      conditions: current.conditions.map((condition) => condition.id === id ? next : condition),
    }));
    setModified(true);
  }

  function removeCondition(id: string) {
    clearForConditionChange();
    setDraft((current) => ({
      ...current,
      conditions: current.conditions.filter((condition) => condition.id !== id),
    }));
    setModified(true);
  }

  function setScopeMode(mode: 'undeclared' | 'none' | 'regions') {
    clearForConditionChange();
    setDraft((current) => {
      const allowedChanges = mode === 'regions' && current.allowedChanges.length === 0
        ? [emptyRegionDraft()]
        : current.allowedChanges;
      return { ...current, scopeMode: mode, allowedChanges };
    });
    setModified(true);
  }

  function addRegion() {
    if (draft.allowedChanges.length >= 12) return;
    clearForConditionChange();
    setDraft((current) => ({ ...current, allowedChanges: [...current.allowedChanges, emptyRegionDraft()] }));
    setModified(true);
  }

  function updateRegion(index: number, next: RegionDraft) {
    clearForConditionChange();
    setDraft((current) => ({
      ...current,
      allowedChanges: current.allowedChanges.map((region, i) => i === index ? next : region),
    }));
    setModified(true);
  }

  function removeRegion(index: number) {
    clearForConditionChange();
    setDraft((current) => ({
      ...current,
      allowedChanges: current.allowedChanges.filter((_, i) => i !== index),
    }));
    setModified(true);
  }

  function setErrorCode(code: string, message: string) {
    setError({ code, message });
  }

  function draftSignature(): string {
    return JSON.stringify(draft);
  }

  function draftPolicy(): Policy | null {
    const built = buildObligationSet(draft.conditions);
    if ('error' in built) {
      setErrorCode('CLIENT_CONDITIONS_INVALID', 'The condition set is invalid.');
      return null;
    }
    let allowedChanges: Region[] | null = null;
    if (draft.scopeMode === 'none') allowedChanges = [];
    if (draft.scopeMode === 'regions') {
      const regions = buildRegions(draft.allowedChanges);
      if ('error' in regions || draft.allowedChanges.length === 0) {
        setErrorCode('CLIENT_CONDITIONS_INVALID', 'The allowed change regions are invalid.');
        return null;
      }
      allowedChanges = regions.value;
    }
    return {
      schema: 'plenilune.policy/v1', scope: built.value.scope,
      operator_accepted: false, conditions: built.value, allowed_changes: allowedChanges,
    };
  }

  async function previewPolicy() {
    if (!parsed) return;
    const policy = draftPolicy();
    if (!policy) return;
    abortCurrent();
    clearProofArtifacts();
    const signature = draftSignature();
    const { generation, controller } = begin('previewPolicy');
    try {
      const result = await watch(runtime.normalizePolicy(policy, controller.signal));
      if (generationRef.current !== generation) return;
      if (result.data.policy.operator_accepted) throw new ApiError('Preview was unexpectedly accepted.', 'POLICY_PREVIEW_MISMATCH');
      const preview = { normalized: result.data, draftSignature: signature };
      policyPreviewRef.current = preview;
      setPolicyPreview(preview);
      setStage('conditions');
    } catch (cause) {
      fail(cause, generation, 'previewPolicy');
    } finally { settle(generation); }
  }

  async function publishComparison(snapshot: AcceptedSnapshot, options: CompareOptions | undefined, generation: number, signal: AbortSignal) {
    const result = await watch(runtime.compare(snapshot.before, snapshot.after, snapshot.conditions, options, signal));
    if (generationRef.current !== generation) return;
    setComparison({ comparison: result.data, raw: result.raw, options });
    setSelectedWitnessId(firstWitnessId(result.data));
    setStage('result');
    setCanceled(null);
    await classifyScope(snapshot, generation, signal);
  }

  // Classifies each change class against the accepted allowed scope. The
  // object must still be bound to the accepted inputs and policy hash.
  async function classifyScope(snapshot: AcceptedSnapshot, generation: number, signal: AbortSignal) {
    const source = analysis;
    const target = selection;
    if (!source || !target || target.domain !== 'acl') return;
    const scoped = await watch(runtime.analyze(source.before, source.after, [target], [{ domain: 'acl', name: target.name, policy: snapshot.policy }], signal));
    if (generationRef.current !== generation) return;
    const object = scoped.data.objects.find((o) => o.domain === 'acl' && o.name === target.name);
    if (!object?.acl || object.before?.sha256 !== snapshot.parsed.before.sha256 || object.after?.sha256 !== snapshot.parsed.after.sha256
      || object.acl.policy_sha256 !== snapshot.policySha256) {
      throw new ApiError('Scoped change classes are not bound to the accepted inputs and policy.', 'RECEIPT_BINDING_MISMATCH');
    }
    setScopedChanges(object.acl.changes);
  }

  async function acceptAndCompare(confirmed: boolean) {
    const currentPreview = policyPreviewRef.current;
    if (!confirmed || !parsed || !currentPreview || currentPreview.draftSignature !== draftSignature()) return;
    const preview = currentPreview.normalized;
    const requested: Policy = { ...preview.policy, operator_accepted: true };
    const { generation, controller } = begin('compare');
    try {
      const result = await watch(runtime.normalizePolicy(requested, controller.signal));
      if (generationRef.current !== generation) return;
      const normalized = result.data;
      if (!normalized.policy.operator_accepted
        || normalized.obligations_digest !== preview.obligations_digest
        || JSON.stringify({ ...normalized.policy, operator_accepted: false }) !== JSON.stringify(preview.policy)) {
        setPolicyPreview(null);
        policyPreviewRef.current = null;
        throw new ApiError('Canonical policy changed after preview.', 'POLICY_PREVIEW_MISMATCH');
      }
      const snapshot: AcceptedSnapshot = {
        conditions: normalized.policy.conditions,
        digest: normalized.obligations_digest,
        policy: normalized.policy,
        policySha256: normalized.policy_sha256,
        before: parsed.before, after: parsed.after, intent: parsed.intent,
        parsed: parsed.parsed, options: draft.options,
        scopeMode: draft.scopeMode,
        allowedChanges: (normalized.policy.allowed_changes ?? []).map(regionDraft),
        acceptedAt: new Date().toISOString(),
      };
      setAccepted(snapshot);
      setComparison(null);
      setDraft((current) => ({ ...current, conditions: normalized.policy.conditions.obligations.map(conditionDraft), allowedChanges: normalized.policy.allowed_changes?.map(regionDraft) ?? current.allowedChanges }));
      await publishComparison(snapshot, snapshot.options, generation, controller.signal);
    } catch (cause) {
      fail(cause, generation, 'compare');
    } finally { settle(generation); }
  }

  async function retryCompare() {
    if (!accepted) { await exploreSource(); return; }
    const { generation, controller } = begin('compare');
    try {
      await publishComparison(accepted, accepted.options, generation, controller.signal);
    } catch (cause) {
      fail(cause, generation, 'compare');
    } finally {
      settle(generation);
    }
  }

  async function compareWithDefaultBudget() {
    if (!accepted) {
      if (!parsed) return;
      const { generation, controller } = begin('compare');
      try {
        const result = await watch(runtime.compare(parsed.before, parsed.after, null, undefined, controller.signal));
        if (generationRef.current !== generation) return;
        setComparison({ comparison: result.data, raw: result.raw });
        setDraft((current) => ({ ...current, options: undefined }));
        setSelectedWitnessId(firstWitnessId(result.data));
      } catch (cause) { fail(cause, generation, 'compare'); }
      finally { settle(generation); }
      return;
    }
    const { generation, controller } = begin('compare');
    const nextSnapshot: AcceptedSnapshot = { ...accepted, options: undefined };
    try {
      const result = await watch(runtime.compare(nextSnapshot.before, nextSnapshot.after, nextSnapshot.conditions, undefined, controller.signal));
      if (generationRef.current !== generation) return;
      setAccepted(nextSnapshot);
      setDraft((current) => ({ ...current, options: undefined }));
      setComparison({ comparison: result.data, raw: result.raw, options: undefined });
      setSelectedWitnessId(firstWitnessId(result.data));
      setManualRepair(null);
      setAiSession(null);
      setRepairSource('manual');
      setComparisonReplay(null);
      setRepairReplay(null);
      setStage('result');
      setCanceled(null);
    } catch (cause) {
      fail(cause, generation, 'compare');
    } finally {
      settle(generation);
    }
  }

  function cancel() {
    const current = busy;
    if (!current) return;
    generationRef.current += 1;
    abortRef.current?.abort();
    abortRef.current = null;
    setBusy(null);
    setCanceled(current);
    setError(null);
  }

  function navigate(next: Stage) {
    if (next === 'conditions' && !parsed) return;
    if (next === 'result' && !comparison && !analysis) return;
    if (next === 'repair' && !canRepair) return;
    if (next === 'evidence' && !comparison && !analysis) return;
    setStage(next);
    setError(null);
  }

  function selectWitness(id: string | null) {
    setSelectedWitnessId(id);
  }

  function invalidateCandidate() {
    abortCurrent();
    setDecision(null);
    setDisputed(false);
    setCanceled(null);
    setError(null);
    setRepairReplay(null);
  }

  function decideRepair(choice: ReviewDecision['choice']) {
    if (!repair?.receipt.sha256 || (choice === 'received' && !canReceive)) return;
    setDecision({ choice, receiptSha256: repair.receipt.sha256 });
  }

  function newEditId(): string {
    editCounter.current += 1;
    return `edit-${editCounter.current}`;
  }

  function addEdit(edit: PatchEdit): boolean {
    if (edits.length >= 5) return false;
    if (edit.ref && edits.some((draftEdit) => draftEdit.edit.ref?.line === edit.ref?.line)) return false;
    invalidateCandidate();
    setEdits((current) => [...current, { id: newEditId(), edit }]);
    setManualRepair(null);
    setRepairSource('manual');
    setRepairReplay(null);
    return true;
  }

  function removeEdit(id: string) {
    if (!edits.some((edit) => edit.id === id)) return;
    invalidateCandidate();
    setEdits((current) => current.filter((edit) => edit.id !== id));
    setManualRepair(null);
    setRepairSource('manual');
    setRepairReplay(null);
  }

  function selectAiRepair(index: number) {
    if (!aiSession?.session.repairs[index] || (repairSource === 'ai' && selectedAiIndex === index)) return;
    invalidateCandidate();
    setSelectedAiIndex(index);
    setRepairSource('ai');
    setRepairReplay(null);
    setStage('repair');
  }

  function selectManualRepair() {
    if (repairSource === 'manual') return;
    invalidateCandidate();
    setRepairSource('manual');
    setRepairReplay(null);
    setStage('repair');
  }

  async function verifyManualRepair() {
    if (!accepted || edits.length < 1 || edits.length > 5) return;
    const patch: PatchRequest = {
      schema: 'plenilune.patch/v1',
      base_before_sha256: accepted.parsed.before.sha256,
      base_after_sha256: accepted.parsed.after.sha256,
      obligations_digest: accepted.digest,
      scope: accepted.conditions.scope,
      edits: edits.map((draftEdit) => draftEdit.edit),
    };
    setDecision(null);
    setDisputed(false);
    const { generation, controller } = begin('repair');
    try {
      const result = await watch(runtime.repairVerify(
        accepted.before,
        accepted.after,
        accepted.policy,
        patch,
        accepted.options,
        controller.signal,
      ));
      if (generationRef.current !== generation) return;
      const issue = receiptBindingIssue(result.data, bindingFromAccepted(accepted));
      if (issue) throw new ApiError(`Repair receipt is not bound to the accepted snapshot (${issue}).`, 'RECEIPT_BINDING_MISMATCH');
      if (result.data.sha256) disputedHashesRef.current.delete(result.data.sha256);
      setManualRepair({ receipt: result.data, raw: result.raw, source: 'manual' });
      setRepairSource('manual');
      setRepairReplay(null);
      setStage('repair');
      setCanceled(null);
    } catch (cause) {
      fail(cause, generation, 'repair');
    } finally {
      settle(generation);
    }
  }

  async function proposeRepair() {
    if (!accepted || !accepted.intent.trim()) return;
    setDecision(null);
    setDisputed(false);
    const { generation, controller } = begin('aiRepair');
    try {
      const result = await watch(runtime.agentSession(
        accepted.before,
        accepted.after,
        accepted.intent,
        accepted.policy,
        true,
        accepted.options,
        controller.signal,
      ));
      if (generationRef.current !== generation) return;
      const binding = bindingFromAccepted(accepted);
      const invalid = result.data.repairs.find((receipt) => receiptBindingIssue(receipt, binding) !== null);
      if (invalid) throw new ApiError('AI session returned a repair receipt for a different accepted input or policy.', 'RECEIPT_BINDING_MISMATCH');
      for (const receipt of result.data.repairs) if (receipt.sha256) disputedHashesRef.current.delete(receipt.sha256);
      setAiSession({ session: result.data, raw: result.raw });
      setSelectedAiIndex(0);
      setRepairSource(result.data.repairs.length > 0 ? 'ai' : 'manual');
      setRepairReplay(null);
      setStage('repair');
      setCanceled(null);
    } catch (cause) {
      fail(cause, generation, 'aiRepair');
    } finally {
      settle(generation);
    }
  }

  async function replayComparison() {
    const source = accepted ?? parsed;
    if (!source || !comparison) return;
    const { generation, controller } = begin('replayComparison');
    try {
      const result = await watch(runtime.replayComparison(
        source.before,
        source.after,
        comparison.comparison,
        controller.signal,
      ));
      if (generationRef.current !== generation) return;
      setComparisonReplay({ result: result.data, raw: result.raw });
      setCanceled(null);
    } catch (cause) {
      fail(cause, generation, 'replayComparison');
    } finally {
      settle(generation);
    }
  }

  async function replayRepair() {
    if (!accepted || !repair) return;
    const issue = receiptBindingIssue(repair.receipt, bindingFromAccepted(accepted));
    if (issue) {
      setErrorCode('RECEIPT_BINDING_MISMATCH', `Repair receipt is not bound to the accepted snapshot (${issue}).`);
      return;
    }
    const { generation, controller } = begin('replayRepair');
    try {
      const result = await watch(runtime.repairReplay(
        accepted.before,
        accepted.after,
        repair.receipt,
        controller.signal,
      ));
      if (generationRef.current !== generation) return;
      if (result.data.sha256 !== repair.receipt.sha256) {
        if (repair.receipt.sha256) disputedHashesRef.current.add(repair.receipt.sha256);
        setDisputed(true);
        setDecision(null);
        throw new ApiError('Replay did not match the selected receipt.', 'REPLAY_RECEIPT_MISMATCH');
      }
      if (result.data.status === 'REPAIR_REPLAYED') {
        if (repair.receipt.sha256) disputedHashesRef.current.delete(repair.receipt.sha256);
        setDisputed(false);
      }
      setRepairReplay({ result: result.data, raw: result.raw });
      setCanceled(null);
    } catch (cause) {
      if (generationRef.current === generation && cause instanceof ApiError
        && (cause.code === 'REPLAY_RECEIPT_MISMATCH' || cause.code === 'EVIDENCE_MISMATCH')) {
        if (repair.receipt.sha256) disputedHashesRef.current.add(repair.receipt.sha256);
        setDisputed(true);
        setDecision(null);
        setRepairReplay(null);
      }
      fail(cause, generation, 'replayRepair');
    } finally {
      settle(generation);
    }
  }

  function clearError() {
    setError(null);
  }

  return {
    sample,
    sampleId,
    samples,
    modified,
    pristine,
    sourceKind,
    draft,
    stage,
    parsed,
    accepted,
    policyPreview,
    decision,
    disputed: isRepairDisputed,
    canReceive,
    comparison,
    aiProposal,
    aiSession,
    aiRepairs,
    selectedAiIndex,
    lastAi,
    report,
    repair,
    manualRepair,
    repairSource,
    edits,
    selectedWitnessId,
    comparisonReplay,
    repairReplay,
    analysis,
    selection,
    selectedObject,
    aclChanges,
    probeResult,
    analysisReplay,
    busy,
    canceled,
    error,
    backend,
    canRepair,
    canExport,
    holdsCount,
    totalConditions,
    proposalAdopted,
    selectSample,
    updateSource,
    parseSource,
    exploreSource,
    updateIntent,
    previewPolicy,
    decideRepair,
    proposeConditions,
    adoptProposal,
    addCondition,
    updateCondition,
    removeCondition,
    setScopeMode,
    addRegion,
    updateRegion,
    removeRegion,
    acceptAndCompare,
    retryCompare,
    compareWithDefaultBudget,
    cancel,
    navigate,
    selectWitness,
    addEdit,
    removeEdit,
    verifyManualRepair,
    proposeRepair,
    selectAiRepair,
    selectManualRepair,
    replayComparison,
    replayRepair,
    selectObject,
    probe,
    replayAnalysis,
    clearError,
  };
}

export type Review = ReturnType<typeof useReview>;
