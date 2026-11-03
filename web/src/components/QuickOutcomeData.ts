import type { Review } from '../hooks/useReview';
import type { Witness } from '../lib/types';
import { sourceLine } from '../lib/conditions';
import { originalLineInRanges, originalSourceLine } from '../lib/sourceMapping';

// Map already-bound review evidence to presentation data; no verdict is computed here.
export function quickOutcomeData(review: Review) {
  const report = review.comparison?.comparison.report;
  const object = review.selectedObject;
  const acl = object?.domain === 'acl' ? object.acl : undefined;
  const verdict = report?.verdict ?? acl?.verdict;
  if (!verdict) return null;
  const analysis = review.analysis?.result;
  const summary = analysis?.summary;
  const source = review.accepted ?? review.parsed;
  return {
    report, object, verdict, source, draftAfter: review.draft.after,
    name: object?.name ?? review.comparison?.comparison.after.name ?? review.parsed?.parsed.after.acl.name ?? 'ACL',
    witness: report ? report.witnesses[0] : acl?.witnesses[0],
    partial: report ? report.partial : acl?.partial,
    complete: report ? report.search_complete : acl?.search_complete,
    truncated: report ? report.witnesses_truncated : acl?.witnesses_truncated,
    reason: report ? report.reason : acl?.reason ?? acl?.budget.stop_reason,
    coverageGap: Boolean(analysis && [analysis.coverage.before, analysis.coverage.after].some(c => c.unanalyzed_lines + c.unsupported_lines > 0)),
    unresolved: Boolean(summary && summary.unsupported + summary.inconclusive + summary.not_analyzed + summary.unanalyzed_changes > 0),
    firstLine: source?.firstLine?.after ?? review.parsed?.firstLine.after ?? object?.after?.lines[0]?.[0] ?? 1,
    sourceLines: source?.sourceLines?.after ?? review.parsed?.sourceLines?.after,
    extentLines: object?.after?.lines,
    changes: review.aclChanges,
  };
}
export type QuickOutcomeData = NonNullable<ReturnType<typeof quickOutcomeData>>;
export function quickOutcomeCause(data: QuickOutcomeData, witness: Witness | undefined) {
  if (!witness) return undefined;
  const line = data.source
    ? originalSourceLine(witness.after.line, data.sourceLines, data.firstLine)
    : data.extentLines ? originalLineInRanges(witness.after.line, data.extentLines) : originalSourceLine(witness.after.line, undefined, data.firstLine);
  const text = witness.after.implicit ? null : sourceLine(data.source?.after ?? data.draftAfter, data.source ? witness.after.line : line)?.trim();
  return { line, implicit: witness.after.implicit, text };
}
