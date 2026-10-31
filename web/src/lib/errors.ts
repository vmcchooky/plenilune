import { ApiError } from './api';
import type { Translate } from './conditions';

export interface ReviewError {
  code: string;
  message: string;
  line?: number;
  side?: string;
  status?: number;
}

export function toReviewError(cause: unknown): ReviewError {
  if (cause instanceof ApiError) {
    return {
      code: cause.code,
      message: cause.message,
      line: cause.line,
      side: cause.side,
      status: cause.status,
    };
  }
  if (cause instanceof Error) {
    return { code: 'CLIENT_ERROR', message: cause.message };
  }
  return { code: 'CLIENT_ERROR', message: 'Unexpected failure.' };
}

export function errorCopy(t: Translate, error: ReviewError): { title: string; body: string; hint: string } {
  const code = error.code;
  if (code === 'RUNTIME_UNAVAILABLE') {
    return { title: t('error.runtime.title'), body: t('error.runtime.body'), hint: t('error.runtime.hint') };
  }
  if (code.startsWith('PARSE_') || code === 'INPUT_INVALID' || code === 'PARSE_FAILED') {
    return { title: t('error.parse.title'), body: t('error.parse.body'), hint: t('error.parse.hint') };
  }
  if (code === 'OBLIGATIONS_INVALID' || code === 'CLIENT_CONDITIONS_INVALID') {
    return { title: t('error.conditions.title'), body: t('error.conditions.body'), hint: t('error.conditions.hint') };
  }
  if (code.startsWith('REPAIR_') || code.startsWith('PATCH_') || code === 'REPAIR_INPUT_INVALID') {
    return { title: t('error.repair.title'), body: t('error.repair.body'), hint: t('error.repair.hint') };
  }
  if (code === 'EVIDENCE_MISMATCH' || code === 'COMPARISON_INVALID' || code === 'REPLAY_RECEIPT_MISMATCH' || code === 'SOURCE_SLICE_MISMATCH') {
    return { title: t('error.evidence.title'), body: t('error.evidence.body'), hint: t('error.evidence.hint') };
  }
  if (code === 'POLICY_PREVIEW_MISMATCH') {
    return { title: t('error.policyPreview.title'), body: t('error.policyPreview.body'), hint: t('error.policyPreview.hint') };
  }
  if (code === 'RECEIPT_BINDING_MISMATCH') {
    return { title: t('error.binding.title'), body: t('error.binding.body'), hint: t('error.binding.hint') };
  }
  if (code === 'AI_MODEL_UNSUPPORTED') {
    return { title: t('error.model.title'), body: t('error.model.body'), hint: t('error.model.hint') };
  }
  if (code === 'LOCAL_ORIGIN_REQUIRED') {
    return { title: t('error.origin.title'), body: t('error.origin.body'), hint: t('error.origin.hint') };
  }
  if (code.startsWith('AI_') || code === 'SESSION_INPUT_INVALID') {
    return { title: t('error.ai.title'), body: t('error.ai.body'), hint: t('error.ai.hint') };
  }
  if (code === 'DEADLINE_EXCEEDED' || code === 'CANCELED') {
    return { title: t('error.deadline.title'), body: t('error.deadline.body'), hint: t('error.deadline.hint') };
  }
  if (code.startsWith('ANALYSIS_') || code.startsWith('INPUT_') || code.startsWith('REQUEST_')
    || code.startsWith('OPTIONS_') || code.startsWith('SELECT_') || code.startsWith('POLICY_')
    || code.startsWith('SOURCE_REF_') || code.startsWith('RESULT_')
    || code === 'INVALID_JSON' || code === 'JSON_INVALID' || code === 'BODY_TOO_LARGE' || code === 'CONTENT_TYPE') {
    return { title: t('error.analysis.title'), body: t('error.analysis.body'), hint: t('error.analysis.hint') };
  }
  return { title: t('error.generic.title'), body: t('error.generic.body'), hint: t('error.generic.hint') };
}

export function errorLocation(t: Translate, error: ReviewError): string | null {
  if (!error.side && error.line === undefined) return null;
  const side = error.side === 'before' ? t('common.before')
    : error.side === 'after' ? t('common.after')
      : error.side === 'config' ? t('common.config')
        : error.side;
  if (error.line !== undefined && error.line > 0) {
    return side
      ? t('error.parse.at', { side, line: error.line })
      : `${t('error.line')} ${error.line}`;
  }
  return side ?? null;
}
