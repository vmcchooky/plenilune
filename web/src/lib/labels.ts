import type { MessageKey } from '../i18n/messages.en';
import type { Translate } from './conditions';
import type {
  AIStatus,
  CollateralStatus,
  Decision,
  ObligationStatus,
  RepairStatus,
  ReplayStatus,
  Verdict,
} from './types';

const outcomeKeys: Record<ObligationStatus, MessageKey> = {
  HOLDS: 'result.outcome.HOLDS',
  VIOLATED: 'result.outcome.VIOLATED',
  INCONCLUSIVE: 'result.outcome.INCONCLUSIVE',
};

const repairKeys: Record<RepairStatus, MessageKey> = {
  VERIFIED: 'repair.status.VERIFIED',
  REJECTED: 'repair.status.REJECTED',
  UNVERIFIED: 'repair.status.UNVERIFIED',
};

const collateralKeys: Record<CollateralStatus, MessageKey> = {
  VERIFIED: 'repair.collateralStatus.VERIFIED',
  FAILED: 'repair.collateralStatus.FAILED',
  UNCHECKED: 'repair.collateralStatus.UNCHECKED',
  INCONCLUSIVE: 'repair.collateralStatus.INCONCLUSIVE',
};

const aiKeys: Record<AIStatus, MessageKey> = {
  disabled: 'ai.status.disabled',
  not_configured: 'ai.status.not_configured',
  unavailable: 'ai.status.unavailable',
  clarification_needed: 'ai.status.clarification_needed',
  partial: 'ai.status.partial',
  completed: 'ai.status.completed',
  canceled: 'ai.status.canceled',
};

const replayKeys: Record<ReplayStatus, MessageKey> = {
  COMPARISON_REPLAYED: 'repair.replayStatus.REPAIR_REPLAYED',
  REPAIR_REPLAYED: 'repair.replayStatus.REPAIR_REPLAYED',
  ANALYSIS_REPLAYED: 'repair.replayStatus.REPAIR_REPLAYED',
  TIMING_SENSITIVE: 'repair.replayStatus.TIMING_SENSITIVE',
};

const stopReasonKeys: Record<string, MessageKey> = {
  operations: 'result.reason.operations',
  boxes: 'result.reason.boxes',
  wall_clock: 'result.reason.wall_clock',
  context_deadline: 'result.reason.context_deadline',
  not_evaluated: 'result.reason.not_evaluated',
  search_budget_exhausted: 'result.reason.search_budget_exhausted',
};

const repairReasonKeys: Record<string, MessageKey> = {
  base_digest_mismatch: 'repair.reason.base_digest_mismatch',
  obligations_digest_mismatch: 'repair.reason.obligations_digest_mismatch',
  policy_missing: 'repair.reason.policy_missing',
  obligations_violated: 'repair.reason.obligations_violated',
  obligations_inconclusive: 'repair.reason.obligations_inconclusive',
  allowed_change_region_missing: 'repair.reason.allowed_change_region_missing',
  collateral_changed: 'repair.reason.collateral_changed',
  collateral_inconclusive: 'repair.reason.collateral_inconclusive',
  policy_not_accepted: 'repair.reason.policy_not_accepted',
  invalid_edit_count: 'repair.reason.invalid_edit_count',
  request_pins_mismatch: 'repair.reason.request_pins_mismatch',
  frozen_obligations_mismatch: 'repair.reason.frozen_obligations_mismatch',
  operator_policy_not_accepted: 'repair.reason.operator_policy_not_accepted',
};

export function outcomeLabel(t: Translate, status: ObligationStatus): string {
  return t(outcomeKeys[status] ?? 'result.outcome.INCONCLUSIVE');
}

export function repairStatusLabel(t: Translate, status: RepairStatus): string {
  return t(repairKeys[status] ?? 'repair.status.UNVERIFIED');
}

export function collateralStatusLabel(t: Translate, status: CollateralStatus): string {
  return t(collateralKeys[status] ?? 'repair.collateralStatus.UNCHECKED');
}

export function aiStatusLabel(t: Translate, status: AIStatus): string {
  return t(aiKeys[status] ?? 'ai.status.unavailable');
}

export function replayLabel(t: Translate, status: ReplayStatus): string {
  return t(replayKeys[status] ?? 'repair.replayStatus.TIMING_SENSITIVE');
}

export function stopReasonLabel(t: Translate, reason: string | undefined): string {
  if (!reason) return t('common.notAvailable');
  const key = stopReasonKeys[reason];
  return key ? t(key) : reason;
}

export function repairReasonLabel(t: Translate, reason: string | undefined): string {
  if (!reason) return t('common.notAvailable');
  const key = repairReasonKeys[reason];
  if (key) return t(key);
  if (reason.startsWith('PATCH_')) return t('error.repair.body');
  return reason;
}

export function verdictTitle(t: Translate, verdict: Verdict): string {
  switch (verdict) {
    case 'DIFFERENT': return t('result.verdict.different.title');
    case 'EQUIVALENT': return t('result.verdict.equivalent.title');
    default: return t('result.verdict.inconclusive.title');
  }
}

export function verdictBody(t: Translate, verdict: Verdict, reason?: string): string {
  switch (verdict) {
    case 'DIFFERENT': return t('result.verdict.different.body');
    case 'EQUIVALENT': return t('result.verdict.equivalent.body');
    default: return t('result.verdict.inconclusive.body', { reason: stopReasonLabel(t, reason) });
  }
}

// The verdict plate names the deciding rule in each version. It carries no
// proof claim beyond the witness itself.
export function decisionRefLabel(t: Translate, decision: Decision, rule?: string | null): string {
  const action = t(decision.action === 'permit' ? 'common.permit' : 'common.deny');
  if (decision.implicit) return `${action} · ${t('common.implicitDeny')}`;
  const text = rule?.trim();
  return `${action} · ${t('result.ruleLine', { line: decision.line ?? 0 })}${text ? ` — ${text}` : ''}`;
}

export function resultNextKey(input: {
  accepted: boolean;
  canRepair: boolean;
  hasRepair: boolean;
  inconclusive: boolean;
}): MessageKey {
  if (!input.accepted) return 'result.next.policy';
  if (input.inconclusive) return 'result.next.retry';
  if (input.canRepair && !input.hasRepair) return 'result.next.repair';
  return 'result.next.evidence';
}

export function scopeModeLabel(t: Translate, mode: 'undeclared' | 'none' | 'regions'): string {
  return t(`scope.mode.${mode}`);
}

export function scopeStatusLabel(t: Translate, mode: 'undeclared' | 'none' | 'regions', count: number): string {
  if (mode === 'regions') return t('scope.status.regions', { count });
  return t(`scope.status.${mode}`);
}
