import { useI18n } from '../i18n';
import { actionLabel } from '../lib/conditions';
import type { Translate } from '../lib/conditions';
import { stopReasonLabel, verdictTitle } from '../lib/labels';
import type { AnalysisObject, EntryRef, PrefixClass, PrefixReport } from '../lib/types';
import { ObjectErrors, ObjectReferences } from './ConfigOverview';
import { ProvenanceTag, SectionHeading, StatusMark, TechnicalDetails } from './Ui';

function Entry({ entry }: { entry: EntryRef }) {
  const { t } = useI18n();
  if (entry.implicit) return <span>{actionLabel(t, 'deny')} · {t('prefix.implicit')}</span>;
  return (
    <span>
      {actionLabel(t, entry.action)} · {t('prefix.entry', { seq: entry.seq ?? 0 })} · {t('changes.rule', { line: entry.line ?? 0 })}
      {entry.text && <> — <code>{entry.text}</code></>}
    </span>
  );
}

function entryRefLabel(t: Translate, entry: EntryRef): string {
  const action = actionLabel(t, entry.action);
  return entry.implicit ? `${action} · ${t('prefix.implicit')}` : `${action} · ${t('prefix.entry', { seq: entry.seq ?? 0 })}`;
}

function ClassRow({ c }: { c: PrefixClass }) {
  const { t } = useI18n();
  return (
    <li className={`change-class change-class--${c.effect}`}>
      <div className="change-class__head">
        <StatusMark tone={c.effect === 'newly_denied' ? 'bad' : 'warn'} label={t(`prefix.effect.${c.effect}`)} />
        <strong>{t(c.routes === 1 ? 'prefix.routesOne' : 'prefix.routes', { count: c.routes.toLocaleString() })}</strong>
        <span className="field-label">{t('prefix.lengths', { lengths: c.lengths.join(', /') })}</span>
      </div>
      <dl className="change-class__rules">
        <div><dt>{t('changes.before')}</dt><dd><Entry entry={c.before} /></dd></div>
        <div><dt>{t('changes.after')}</dt><dd><Entry entry={c.after} /></dd></div>
        <div><dt>{t('prefix.example')}</dt><dd><code>{c.witness.route}</code></dd></div>
      </dl>
      <ul className="route-ranges">
        {c.ranges.map((r, i) => (
          <li key={i}><code>{t(r.routes === 1 ? 'prefix.rangeOne' : 'prefix.range', { first: r.first, last: r.last, count: r.routes.toLocaleString() })}</code></li>
        ))}
      </ul>
      {c.ranges_truncated && <p className="field-hint">{t('prefix.moreRanges', { count: c.total_ranges })}</p>}
    </li>
  );
}

function prefixVerdictBody(t: Translate, report: PrefixReport): string {
  if (report.verdict === 'DIFFERENT') return t('prefix.verdict.different.body');
  if (report.verdict === 'EQUIVALENT') return t('prefix.verdict.equivalent.body');
  return t('result.verdict.inconclusive.body', { reason: stopReasonLabel(t, report.reason) });
}

// Route-filter view: speaks in routes and sequence entries, not packets.
export function PrefixListDetail({ object }: { object: AnalysisObject }) {
  const { t } = useI18n();
  const report = object.prefix_list?.report;
  return (
    <section className="object-detail" aria-labelledby="object-title">
      <SectionHeading id="object-title" title={t('prefix.title', { name: object.name })} note={t('prefix.lede')}
        aside={<ProvenanceTag icon="shield">{t('result.runtime')}</ProvenanceTag>} />
      <ObjectErrors object={object} />
      {!report ? (
        <ObjectStatusNote object={object} />
      ) : (
        <>
          <div className={`verdict-plate verdict-plate--${report.verdict.toLowerCase()}`}>
            <code className="verdict-plate__code">{report.verdict}</code>
            <h2>{verdictTitle(t, report.verdict)}</h2>
            <p>{prefixVerdictBody(t, report)}</p>
            {report.classes[0] && (
              <p className="verdict-plate__why">
                {t('prefix.why', {
                  after: entryRefLabel(t, report.classes[0].after),
                  before: entryRefLabel(t, report.classes[0].before),
                })}
              </p>
            )}
            <p className="verdict-plate__next">{t('prefix.next')}</p>
          </div>
          {report.classes.length === 0 ? (
            report.verdict === 'EQUIVALENT' && report.complete
              ? <p className="notice-line notice-line--neutral">{t('prefix.none')}</p>
              : report.verdict === 'INCONCLUSIVE'
                ? <p className="notice-line notice-line--warn">{t('prefix.incomplete', { reason: stopReasonLabel(t, report.reason) })}</p>
                : <p className="notice-line notice-line--warn">{t('prefix.noClasses')}</p>
          ) : (
            <ol className="change-class-list">{report.classes.map((c, i) => <ClassRow key={i} c={c} />)}</ol>
          )}
          {report.classes_truncated && <p className="field-hint">{t('prefix.truncated', { shown: report.classes.length, total: report.total_classes })}</p>}
          <p className="field-hint">{t('prefix.modelScope')}</p>
          <p className="notice-line notice-line--neutral">{t('prefix.noIntent')}</p>
          <TechnicalDetails title={t('result.comparisonDetails')}>
            <dl className="meta-list">
              <div><dt>{t('evidence.parser')}</dt><dd><code>{object.prefix_list?.parser}</code></dd></div>
              <div><dt>{t('evidence.engine')}</dt><dd><code>{object.prefix_list?.engine}</code></dd></div>
              <div><dt>{t('result.budget')}</dt><dd>{t('result.operations', { count: report.operations })}</dd></div>
              <div><dt>{t('prefix.complete')}</dt><dd>{report.complete ? t('result.yes') : t('result.no')}</dd></div>
              {report.reason && <div><dt>{t('result.stopReason')}</dt><dd>{stopReasonLabel(t, report.reason)} · <code>{report.reason}</code></dd></div>}
            </dl>
          </TechnicalDetails>
        </>
      )}
      <ObjectReferences object={object} />
    </section>
  );
}

// Explains why an object has no behavior comparison, by status.
function ObjectStatusNote({ object }: { object: AnalysisObject }) {
  const { t } = useI18n();
  if (object.errors?.length) return <p className="notice-line notice-line--warn">{t('object.unsupported')}</p>;
  switch (object.status) {
    case 'unchanged': return <p className="notice-line notice-line--neutral">{t('object.unchanged')}</p>;
    case 'added':
    case 'removed': return <p className="notice-line notice-line--warn">{t('object.structural')}</p>;
    case 'not_analyzed': return <p className="notice-line notice-line--warn">{t('object.notAnalyzed')}</p>;
    default: return <p className="notice-line notice-line--warn">{t('object.unsupported')}</p>;
  }
}

export function ObjectDetail({ object }: { object: AnalysisObject }) {
  const { t } = useI18n();
  if (object.domain === 'prefix-list') return <PrefixListDetail object={object} />;
  return (
    <section className="object-detail" aria-labelledby="object-title">
      <SectionHeading id="object-title" title={t('result.objectHeader', { domain: t(`overview.domain.${object.domain}`), name: object.name })} />
      <ObjectStatusNote object={object} />
      <ObjectErrors object={object} />
      <ObjectReferences object={object} />
    </section>
  );
}
