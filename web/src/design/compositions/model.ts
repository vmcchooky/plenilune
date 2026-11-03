import type { Review } from '../../hooks/useReview';
import { sourceLine } from '../../lib/conditions';
import { originalSourceLine } from '../../lib/sourceMapping';
import type { Decision, Witness } from '../../lib/types';

export function compositionModel(review: Review) {
  const report = review.comparison?.comparison.report;
  const source = review.accepted ?? review.parsed;
  if (!report || !source) return null;
  const witness = report.witnesses[0];
  if (!witness) return null;
  const first = source.firstLine ?? review.parsed?.firstLine ?? { before: 1, after: 1 };
  const sourceLines = source.sourceLines ?? review.parsed?.sourceLines;
  const changes = review.aclChanges;
  const analysis = review.analysis?.result;
  return {
    report, witness, changes, name: review.selection?.name ?? source.parsed.after.acl.name,
    before: source.before, after: source.after, first,
    ...(sourceLines ? { sourceLines } : {}),
    policyAccepted: Boolean(review.accepted),
    conditionsUnresolved: report.obligations.some(o => o.status !== 'HOLDS'),
    incomplete: report.verdict === 'INCONCLUSIVE' || !report.search_complete || Boolean(report.partial),
    truncated: report.witnesses_truncated || changes?.classes_truncated || changes?.complete === false,
    coverageGap: Boolean(analysis && (
      analysis.summary.unsupported + analysis.summary.not_analyzed + analysis.summary.inconclusive > 0
      || analysis.summary.unanalyzed_changes > 0
      || [analysis.coverage.before, analysis.coverage.after].some(c => c.unsupported_lines + c.unanalyzed_lines > 0))),
  };
}
export type CompositionModel = NonNullable<ReturnType<typeof compositionModel>>;
export function decidingText(model: CompositionModel, side: 'before' | 'after', decision: Decision) {
  return decision.implicit ? null : sourceLine(model[side], decision.line)?.trim() ?? null;
}
export function decidingNumber(model: CompositionModel, side: 'before' | 'after', decision: Decision) {
  return originalSourceLine(decision.line, model.sourceLines?.[side], model.first[side]);
}
export function protocolText(witness: Witness) {
  const p = witness.packet;
  return `${p.protocol === 6 ? 'TCP' : p.protocol === 17 ? 'UDP' : p.protocol === 1 ? 'ICMP' : p.protocol}${p.protocol === 6 || p.protocol === 17 ? `/${p.dst_port}` : ''}`;
}
