import { useState } from 'react';
import { useI18n } from '../i18n';
import type { Review } from '../hooks/useReview';
import { actionLabel } from '../lib/conditions';
import { obligationStatusList, verdictBadge, verdictBody, verdictTitle } from '../lib/labels';
import { ChangeClasses } from './ChangeClasses';
import { ChangeRecord } from './ChangeRecord';
import { PartialNotice } from './PartialNotice';
import { QuickChangeGroups } from './QuickChangeGroups';
import { quickOutcomeCause, quickOutcomeData } from './QuickOutcomeData';
import type { QuickOutcomeData } from './QuickOutcomeData';
import { StatusMark } from './Ui';

type Props = { review: Review; onPolicy: () => void; onSource: () => void };
export function QuickOutcome(props: Props) {
  const [detailsOpen, setDetailsOpen] = useState(false);
  const data = quickOutcomeData(props.review);
  if (!data) return null;
  const key = props.review.comparison?.comparison.sha256 ?? props.review.analysis?.result.sha256 ?? 'analysis';
  return <QuickOutcomeRecord {...props} data={data} key={key} detailsOpen={detailsOpen} setDetailsOpen={setDetailsOpen} />;
}
function QuickOutcomeRecord({ review, data, onPolicy, onSource, detailsOpen, setDetailsOpen }: Props & { data: QuickOutcomeData; detailsOpen: boolean; setDetailsOpen: (open: boolean) => void }) {
  const { t } = useI18n();
  const [selected, setSelected] = useState<number | null>(null);
  const { verdict, report, changes, partial, complete } = data;
  const multiple = (changes?.total_classes ?? 0) > 1;
  const group = selected === null ? undefined : changes?.classes[selected];
  const witness = multiple ? group?.witness : data.witness;
  const changed = witness && witness.before.action !== witness.after.action;
  const title = verdict === 'INCONCLUSIVE' ? verdictTitle(t, verdict)
    : multiple && !group ? t('quick.groupsTitle', { count: changes!.total_classes })
      : changed ? t(witness.after.action === 'deny' ? 'quick.newDeny' : 'quick.newPermit') : verdictTitle(t, verdict);
  const violated = report?.obligations.some(o => o.status === 'VIOLATED');
  const pending = report?.obligations.some(o => o.status === 'INCONCLUSIVE');
  // Same condition and handler as the deep result step: a budget-limited
  // inconclusive verdict offers the default-budget retry as the primary
  // action, and the conditions CTA drops to secondary.
  const canRetryDefault = verdict === 'INCONCLUSIVE' && Boolean(review.comparison?.options);
  const copy = {
    aclObject: 'ACL', before: t('changes.before'), after: t('changes.after'),
    permit: actionLabel(t, 'permit'), deny: actionLabel(t, 'deny'),
    example: t('changes.example'), from: t('probe.src'), to: t('probe.dst'),
    implicit: t('changes.implicit'), decidingReason: t('quick.why'), rule: t('quick.ruleLabel'),
    proof: t('quick.ruleProof'), conditions: t('quick.checkIntent'), groups: t('quick.groupLabel'),
  };
  return <ChangeRecord className="quick-outcome" name={data.name} title={title} copy={copy}
    verdictLabel={verdict ? verdictBadge(t, verdict) : undefined}
    headingId="quick-verdict-title" verdict={verdict} groupCount={changes?.total_classes}
    witness={witness} cause={quickOutcomeCause(data, witness)}
    onConditions={review.parsed ? onPolicy : undefined}
    secondaryAction={{ label: t('quick.sources'), run: onSource }}
    primaryAction={canRetryDefault
      ? { label: t('result.primaryRetry'), run: () => { void review.compareWithDefaultBudget(); }, icon: 'replay' }
      : undefined}
    notices={<>
      {(verdict === 'INCONCLUSIVE' || complete === false || partial) && <p className="banner banner--warn" role="status">{verdictBody(t, 'INCONCLUSIVE', data.reason)}</p>}
      {(data.truncated || changes?.classes_truncated || changes?.complete === false) && <p className="banner banner--warn">{t('quick.morePending')}</p>}
      {(data.coverageGap || data.unresolved) && <p className="banner banner--warn">{t('quick.coverageGap')}</p>}
      {partial && <PartialNotice partial={partial} />}
      {report && review.accepted && <p className="record-result-listing"><StatusMark tone={violated ? 'bad' : pending ? 'warn' : 'neutral'} label={obligationStatusList(t, report.obligations)} /></p>}
    </>}
    leadingContent={multiple && changes ? <QuickChangeGroups groups={changes.classes} selected={selected} onSelect={setSelected} /> : undefined}
    scope={<div className="record-refined__scope"><p>{t('quick.shortScope')}</p></div>}
    details={<section className="quick-outcome__details record-refined__details">
      <button type="button" className="button button--quiet" aria-expanded={detailsOpen} aria-controls="quick-outcome-details-panel" onClick={() => setDetailsOpen(!detailsOpen)}>{t('quick.resultDetails')}</button>
      <div id="quick-outcome-details-panel" hidden={!detailsOpen}>
      <p>{review.accepted && report ? obligationStatusList(t, report.obligations) : t('result.noPolicy')}</p>
      <p>{t('result.searchComplete')}: {complete ? t('result.yes') : t('result.no')}</p>
      <p>{t('quick.scopeNote')}</p>
      <ChangeClasses review={review} />
      </div>
    </section>} />;
}
