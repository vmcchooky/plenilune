import { useI18n } from '../i18n';
import type { MessageKey } from '../i18n/messages.en';
import type { Review } from '../hooks/useReview';
import type { AnalysisObject, ObjectStatus } from '../lib/types';
import { SectionHeading, StatusMark, TechnicalDetails } from './Ui';
import type { Tone } from './Ui';

export function statusTone(status: ObjectStatus): Tone {
  switch (status) {
    case 'changed': return 'bad';
    case 'equivalent':
    case 'unchanged': return 'good';
    case 'unsupported':
    case 'inconclusive':
    case 'not_analyzed': return 'warn';
    default: return 'neutral';
  }
}

function linesText(o: AnalysisObject, side: 'before' | 'after', absent: string): string {
  const extent = o[side];
  if (!extent) return absent;
  return extent.lines.map(([a, b]) => (a === b ? `${a}` : `${a}–${b}`)).join(', ');
}

export function ObjectErrors({ object }: { object: AnalysisObject }) {
  const { t } = useI18n();
  if (!object.errors?.length) return null;
  return (
    <ul className="object-errors">
      {object.errors.map((e, i) => {
        const side = t(e.side === 'before' ? 'common.before' : 'common.after');
        return (
          <li key={i}>
            {e.line
              ? t('overview.error', { side, line: e.line, message: e.message, code: e.code })
              : t('overview.errorNoLine', { side, message: e.message, code: e.code })}
          </li>
        );
      })}
    </ul>
  );
}

// The overview answers "what changed and where should I look" for the whole
// configuration, and states explicitly what was not analyzed.
export function ConfigOverview({ review }: { review: Review }) {
  const { t } = useI18n();
  const analysis = review.analysis?.result;
  if (!analysis) return null;
  const s = analysis.summary;
  const headline: MessageKey = s.objects === 0 ? 'overview.headline.empty'
    : s.changed > 0 ? (s.changed === 1 ? 'overview.headline.changedOne' : 'overview.headline.changed')
      : s.unsupported + s.inconclusive > 0 ? 'overview.headline.unsupported'
        : 'overview.headline.none';
  const unanalyzedKey: MessageKey = s.unanalyzed_changes === 1 ? 'overview.unanalyzedOne' : 'overview.unanalyzed';
  const busy = review.busy !== null;
  return (
    <section className="config-overview" aria-labelledby="overview-title">
      <SectionHeading id="overview-title" title={t('overview.title')} />
      <p className="config-overview__headline">{t(headline, { count: s.changed })}</p>
      <p className={`notice-line notice-line--${s.unanalyzed_changes > 0 ? 'warn' : 'neutral'}`}>
        {s.unanalyzed_changes > 0 ? t(unanalyzedKey, { count: s.unanalyzed_changes }) : t('overview.unanalyzedNone')}
      </p>
      {analysis.objects.length > 0 && (
        <ul className="object-list" aria-label={t('overview.objects')}>
          {analysis.objects.map((o) => {
            const selected = review.selection?.domain === o.domain && review.selection.name === o.name;
            return (
              <li key={`${o.domain}:${o.kind}:${o.name}`} className={`object-row ${selected ? 'object-row--selected' : ''}`}>
                <div className="object-row__identity">
                  <span className="object-row__domain">{t(`overview.domain.${o.domain}`)}</span>
                  <strong>{o.name}</strong>
                  <StatusMark tone={statusTone(o.status)} label={t(`overview.status.${o.status}`)} />
                </div>
                <p className="object-row__meta">
                  {t('overview.lines', { before: linesText(o, 'before', t('overview.absent')), after: linesText(o, 'after', t('overview.absent')) })}
                  {o.reason && <> · {t(`overview.reason.${o.reason}` as MessageKey)}</>}
                </p>
                <ObjectErrors object={o} />
                {selected ? (
                  <span className="field-label">{t('overview.selected')}</span>
                ) : (
                  <button type="button" className="button button--quiet button--small" disabled={busy}
                    aria-label={`${t('overview.open')}: ${t(`overview.domain.${o.domain}`)} ${o.name}`}
                    onClick={() => { void review.selectObject({ domain: o.domain, name: o.name }); }}>
                    {t('overview.open')}
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {(s.added + s.removed > 0) && <p className="field-hint">{t('overview.addedNote')}</p>}
      {!review.selection && analysis.objects.length > 0 && <p className="field-hint">{t('overview.pick')}</p>}
      <TechnicalDetails title={t('overview.unanalyzedTitle')}>
        {analysis.unanalyzed_changes.length > 0 && (
          <ul className="section-changes">
            {analysis.unanalyzed_changes.map((c, i) => (
              <li key={i}>
                <code>{t(`overview.change.${c.change}`)}</code> {c.header}
                <small> · {[c.before_line ? `${t('common.before')} ${c.before_line}` : '', c.after_line ? `${t('common.after')} ${c.after_line}` : ''].filter(Boolean).join(' · ')}</small>
              </li>
            ))}
          </ul>
        )}
        {(['before', 'after'] as const).map((side) => {
          const c = analysis.coverage[side];
          return <p key={side} className="field-hint">{t('overview.coverage', { side: t(side === 'before' ? 'common.before' : 'common.after'), analyzed: c.analyzed_lines, structural: c.structural_lines, unsupported: c.unsupported_lines, unanalyzed: c.unanalyzed_lines })}</p>;
        })}
        <h3>{t('overview.limitations')}</h3>
        <ul>{analysis.limitations.map((code) => <li key={code}>{t(`limit.${code}` as MessageKey)}</li>)}</ul>
      </TechnicalDetails>
    </section>
  );
}
