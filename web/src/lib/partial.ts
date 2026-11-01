import type { MessageKey } from '../i18n/messages.en';
import type { PartialResult, RulePair, UndecidedPart } from './types';

// P1-2 shared formatting for partial (budget-stopped) results, used by the
// PartialNotice component and the plain-text report/ticket builders so
// every surface describes the proven part and the undecided remainder in
// the same words.

export type Translate = (key: MessageKey, vars?: Record<string, string | number>) => string;

export const PARTIAL_STAGE_KEYS: Record<string, MessageKey> = {
  prefix: 'partial.stage.prefix',
  permit_before: 'partial.stage.permit_before',
  permit_after: 'partial.stage.permit_after',
  decide_lost: 'partial.stage.decide_lost',
  decide_gained: 'partial.stage.decide_gained',
  witnesses: 'partial.stage.witnesses',
  scope_index: 'partial.stage.scope_index',
  classes: 'partial.stage.classes',
};

export const PARTIAL_DIRECTION_KEYS: Record<UndecidedPart['direction'], MessageKey> = {
  delta: 'partial.direction.delta',
  lost: 'partial.direction.lost',
  gained: 'partial.direction.gained',
  classes: 'partial.direction.classes',
};

export function partialStageLabel(t: Translate, stage: string): string {
  const key = PARTIAL_STAGE_KEYS[stage];
  return key ? t(key) : stage;
}

export function partialOrdinalLabel(t: Translate, n: number): string {
  return n < 0 ? t('partial.implicit') : t('partial.ruleN', { n });
}

export function partialPairLine(t: Translate, pair: RulePair): string {
  return t('partial.decidedPairLine', {
    before: partialOrdinalLabel(t, pair.before),
    after: partialOrdinalLabel(t, pair.after),
    pieces: pair.pieces,
  });
}

export function partialRangeText(t: Translate, part: UndecidedPart): string {
  if (part.before_rules && part.after_rules) {
    return t('partial.rangeBoth', {
      bfrom: part.before_rules[0],
      bto: part.before_rules[1],
      afrom: part.after_rules[0],
      ato: part.after_rules[1],
    });
  }
  if (part.before_rules) return t('partial.rangeBefore', { bfrom: part.before_rules[0], bto: part.before_rules[1] });
  if (part.after_rules) return t('partial.rangeAfter', { afrom: part.after_rules[0], ato: part.after_rules[1] });
  return '';
}

export function partialUndecidedLine(t: Translate, part: UndecidedPart): string {
  const range = partialRangeText(t, part);
  let line = t(PARTIAL_DIRECTION_KEYS[part.direction]);
  if (range) line += ` — ${range}`;
  if (part.work_boxes !== undefined && part.work_boxes > 0) {
    line += ` · ${t('partial.workBoxes', { count: part.work_boxes })}`;
  }
  return line;
}

// The bullet lines the plain-text outputs add under the search-complete
// line when a comparison stopped with a partial explanation.
export function partialReportLines(t: Translate, partial: PartialResult): string[] {
  const lines = [
    `- **${t('partial.badge')}** — ${t('partial.stoppedAt', { stage: partialStageLabel(t, partial.stage) })}`,
  ];
  if (partial.prefix_pairs > 0) {
    lines.push(`- ${t('partial.prefixPairs', { count: partial.prefix_pairs })}`);
  }
  if (partial.decided_pairs.length > 0) {
    lines.push(`- ${t('partial.decidedPairs', { count: partial.decided_pairs.length })}`);
  }
  lines.push(`- ${t('partial.undecidedTitle')}: ${partial.undecided.map((part) => partialUndecidedLine(t, part)).join('; ')}`);
  return lines;
}
