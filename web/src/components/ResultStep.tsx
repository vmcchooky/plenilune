import { useRef } from 'react';
import { useI18n } from '../i18n';
import type { Review } from '../hooks/useReview';
import { actionLabel, kindLabel, obligationSentence, sourceLine, sourceLines } from '../lib/conditions';
import { changedLines } from '../lib/lineDiff';
import { downloadText } from '../lib/download';
import { fileStamp, prettyJson, shortHash } from '../lib/format';
import { outcomeLabel, decisionRefLabel, resultNextKey, stopReasonLabel, verdictBody, verdictTitle } from '../lib/labels';
import type { ObligationResult, Packet } from '../lib/types';
import { collectWitnesses } from '../lib/witnesses';
import { ChangeClasses } from './ChangeClasses';
import { ConfigOverview } from './ConfigOverview';
import { Icon } from './Icon';
import { ObjectDetail } from './PrefixListDetail';
import { WitnessDetail } from './WitnessDetail';
import {
  ActionBar,
  CopyButton,
  EmptyState,
  HashLine,
  ProvenanceTag,
  SectionHeading,
  StatusMark,
  StepHeader,
  TechnicalDetails,
  TermNote,
} from './Ui';
import type { Tone } from './Ui';

function outcomeTone(status: ObligationResult['status']): Tone {
  return status === 'HOLDS' ? 'good' : status === 'VIOLATED' ? 'bad' : 'warn';
}

function packetSummary(packet: Packet): string {
  const protocol = packet.protocol === 6 ? 'TCP' : packet.protocol === 17 ? 'UDP' : packet.protocol === 1 ? 'ICMP' : String(packet.protocol);
  const port = packet.protocol === 6 || packet.protocol === 17 ? `:${packet.dst_port}` : '';
  return `${protocol} ${packet.src_ip} → ${packet.dst_ip}${port}`;
}

function AnalysisOnly({ review }: { review: Review }) {
  const { t } = useI18n();
  const object = review.selectedObject;
  return (
    <div className="step">
      <StepHeader kicker={t('result.kicker')} title={t('result.title')} lede={object?.domain === 'prefix-list' ? t('result.prefixLede') : t('result.lede')}>
        <ProvenanceTag tone="info" icon="shield">{t('result.runtime')}</ProvenanceTag>
      </StepHeader>
      <ConfigOverview review={review} />
      {object && <ObjectDetail object={object} />}
      <ActionBar note={object ? t('result.next.object') : t('overview.pick')}>
        <button type="button" className="button button--secondary" onClick={() => review.navigate('source')}>{t('accept.back')}</button>
        <button type="button" className="button button--primary" onClick={() => review.navigate('evidence')}>
          <Icon name="arrow" size={17} />
          {t('result.primaryEvidence')}
        </button>
      </ActionBar>
    </div>
  );
}

export function ResultStep({ review }: { review: Review }) {
  const { t } = useI18n();
  const detailRef = useRef<HTMLDivElement>(null);
  const snapshot = review.comparison;
  const accepted = review.accepted;
  const source = accepted ?? review.parsed;
  if (!snapshot || !source) return review.analysis ? <AnalysisOnly review={review} /> : null;
  const showOverview = Boolean(review.analysis && (review.analysis.result.objects.length > 1 || review.analysis.result.summary.unanalyzed_changes > 0));
  const firstLine = review.parsed?.firstLine ?? { before: 1, after: 1 };
  const acceptedConditions = accepted?.conditions;
  const comparison = snapshot.comparison;
  const report = comparison.report;
  const changedPacket = report.witnesses[0];
  const beforeLines = sourceLines(source.before);
  const afterLines = sourceLines(source.after);
  const changed = changedLines(beforeLines, afterLines);

  const entries = collectWitnesses(t, report);
  const selected = entries.find((entry) => entry.witness.id === review.selectedWitnessId) ?? entries[0] ?? null;

  function choose(witnessId: string) {
    review.selectWitness(witnessId);
    detailRef.current?.scrollIntoView({ block: 'nearest' });
  }

  function conditionFor(id: string) {
    return acceptedConditions?.obligations.find((obligation) => obligation.id === id) ?? null;
  }

  const verdictTone: Tone = report.verdict === 'DIFFERENT' ? 'bad' : report.verdict === 'EQUIVALENT' ? 'neutral' : 'warn';
  const witnessCount = entries.length;
  const inconclusive = report.verdict === 'INCONCLUSIVE';
  const canRetryDefault = inconclusive && Boolean(snapshot.options);
  const note = !accepted ? t('result.noPolicy') : review.canRepair
    ? t('result.repairAvailable')
    : report.verdict === 'EQUIVALENT'
      ? t('result.noRepairNeeded')
      : t('result.noWitness.inconclusive');
  const primaryIsEvidence = !review.canRepair && !canRetryDefault && !inconclusive;
  const why = changedPacket && changedPacket.before.action !== changedPacket.after.action
    ? t('result.why', {
      after: decisionRefLabel(t, changedPacket.after, sourceLine(source.after, changedPacket.after.line)),
      before: decisionRefLabel(t, changedPacket.before, sourceLine(source.before, changedPacket.before.line)),
    })
    : null;
  const nextKey = resultNextKey({
    accepted: Boolean(accepted),
    canRepair: review.canRepair,
    hasRepair: Boolean(review.repair),
    inconclusive,
  });

  return (
    <div className="step">
      <StepHeader kicker={t('result.kicker')} title={t('result.title')} lede={t('result.lede')}>
        <ProvenanceTag tone="info" icon="shield">{t('result.runtime')}</ProvenanceTag>
        {review.sourceKind === 'sample' && <ProvenanceTag icon="file">{t('evidence.sample')}</ProvenanceTag>}
        {(review.sourceKind === 'modified' || review.sourceKind === 'custom') && <ProvenanceTag icon="file">{t('evidence.custom')}</ProvenanceTag>}
      </StepHeader>

      {showOverview && <ConfigOverview review={review} />}
      {review.selection && (
        <p className="object-heading">
          <strong>{t('result.objectHeader', { domain: t(`overview.domain.${review.selection.domain}`), name: review.selection.name })}</strong>
          {(firstLine.before > 1 || firstLine.after > 1) && <span> · {t('overview.objectLines', { before: firstLine.before, after: firstLine.after })}</span>}
        </p>
      )}

      <section className={`verdict-plate verdict-plate--${report.verdict.toLowerCase()}`} aria-labelledby="verdict-title">
        <div className="verdict-plate__head">
          <ProvenanceTag tone={verdictTone === 'bad' ? 'info' : verdictTone} icon="shield">{t('result.runtime')}</ProvenanceTag>
          <code className="verdict-plate__code">{report.verdict}</code>
          {report.witnesses_truncated && <span className="field-label">{t('result.truncated')}</span>}
        </div>
        <h2 id="verdict-title">{changedPacket ? t('result.behaviorHeadline', { packet: packetSummary(changedPacket.packet), before: actionLabel(t, changedPacket.before.action), after: actionLabel(t, changedPacket.after.action) }) : verdictTitle(t, report.verdict)}</h2>
        <p>{verdictBody(t, report.verdict, report.reason)}</p>
        <div className="verdict-plate__facts">
          <span>{accepted ? t('result.conditionsSummary', { holds: review.holdsCount, total: review.totalConditions }) : t('result.noPolicy')}</span>
          <span>{t('result.witnessCount', { count: witnessCount })}</span>
          <span>{t('result.searchComplete')}: {report.search_complete ? t('result.yes') : t('result.no')}</span>
        </div>
        {why && <p className="verdict-plate__why">{why}</p>}
        <p className="verdict-plate__next">{t(nextKey)}</p>
      </section>

      {inconclusive && <TermNote>{t('result.noWitness.inconclusive')}</TermNote>}

      <ChangeClasses review={review} />

      {accepted && <section className="condition-results" aria-labelledby="condition-results-title">
        <SectionHeading
          id="condition-results-title"
          title={t('result.conditionsTitle')}
          note={t('result.conditionsHelp')}
        />
        <div className="condition-result-list">
          {report.obligations.map((outcome) => {
            const obligation = conditionFor(outcome.id);
            const content = (
              <>
                <span className="condition-result__id">
                  <code>{outcome.id}</code>
                  {obligation && <span>{kindLabel(t, obligation.kind)}</span>}
                </span>
                <span className="condition-result__body">
                  <strong>{obligation ? obligationSentence(t, obligation) : outcome.id}</strong>
                  {outcome.reason && <small>{t('common.reason')}: {stopReasonLabel(t, outcome.reason)}</small>}
                </span>
                <span className="condition-result__outcome">
                  <StatusMark tone={outcomeTone(outcome.status)} label={outcomeLabel(t, outcome.status)} />
                  {outcome.witness && <small>{t('result.openWitness')} →</small>}
                </span>
              </>
            );
            return outcome.witness ? (
              <button
                type="button"
                key={outcome.id}
                className={`condition-result ${selected?.conditionId === outcome.id ? 'condition-result--selected' : ''}`}
                aria-pressed={selected?.conditionId === outcome.id}
                onClick={() => choose(outcome.witness!.id)}
              >
                {content}
              </button>
            ) : (
              <div className="condition-result condition-result--static" key={outcome.id}>{content}</div>
            );
          })}
        </div>
      </section>}

      <section className="witness-section" aria-labelledby="witness-section-title" ref={detailRef}>
        <SectionHeading
          id="witness-section-title"
          title={t('result.witnessTitle')}
          note={t('result.witnessHelp')}
          aside={entries.length > 0 ? <ProvenanceTag>{t('result.witnessCount', { count: entries.length })}</ProvenanceTag> : undefined}
        />
        {selected ? (
          <div className="witness-layout">
            <div className="witness-index" role="group" aria-label={t('result.witnessTitle')}>
              {entries.map((entry) => {
                const isSelected = entry.witness.id === selected.witness.id;
                return (
                  <button
                    type="button"
                    key={entry.key}
                    className={`witness-index__item ${isSelected ? 'witness-index__item--selected' : ''}`}
                    aria-pressed={isSelected}
                    onClick={() => choose(entry.witness.id)}
                  >
                    <span className="witness-index__top">
                      <strong>{entry.label}</strong>
                      <code>{shortHash(entry.witness.id)}</code>
                    </span>
                    <span className="witness-index__packet">{packetSummary(entry.witness.packet)}</span>
                  </button>
                );
              })}
            </div>
            <div className="witness-layout__detail">
              <WitnessDetail
                witness={selected.witness}
                title={selected.label}
                before={source.before}
                after={source.after}
                changedBefore={changed.before}
                changedAfter={changed.after}
              />
            </div>
          </div>
        ) : (
          <EmptyState
            icon="packet"
            title={t('result.noWitness')}
            body={inconclusive ? t('result.noWitness.inconclusive') : t('result.noWitness.equivalent')}
          />
        )}
      </section>

      <TechnicalDetails title={t('result.comparisonDetails')}>
        <dl className="meta-list">
          <div><dt>{t('result.scope')}</dt><dd><code>{comparison.scope}</code></dd></div>
          <div><dt>{t('result.verdictRaw')}</dt><dd><code>{report.verdict}</code></dd></div>
          <div><dt>{t('result.searchComplete')}</dt><dd>{report.search_complete ? t('result.yes') : t('result.no')}</dd></div>
          <div><dt>{t('evidence.parser')}</dt><dd><code>{comparison.parser_version}</code></dd></div>
          <div><dt>{t('evidence.engine')}</dt><dd><code>{comparison.engine_version}</code></dd></div>
          <div><dt>{t('evidence.tool')}</dt><dd><code>{comparison.tool.version} · {comparison.tool.commit}</code></dd></div>
          <div><dt>{t('result.budget')}</dt><dd>{t('result.operations', { count: report.budget.operations })} · {t('result.peakBoxes', { count: report.budget.peak_boxes })}</dd></div>
          <div><dt>{t('result.stopReason')}</dt><dd>{report.budget.stop_reason ? <>{stopReasonLabel(t, report.budget.stop_reason)} · <code>{report.budget.stop_reason}</code></> : t('common.none')}</dd></div>
          <div><dt>{t('result.truncated')}</dt><dd>{report.witnesses_truncated ? t('result.yes') : t('result.no')}</dd></div>
          {accepted && <div><dt>{t('accept.digest')}</dt><dd><code>{accepted.digest}</code></dd></div>}
        </dl>
        <div className="hash-stack">
          <HashLine label={t('result.comparisonHash')} value={comparison.sha256} />
          <HashLine label={t('result.inputHash', { side: t('common.before') })} value={comparison.before.sha256} />
          <HashLine label={t('result.inputHash', { side: t('common.after') })} value={comparison.after.sha256} />
        </div>
        <div className="raw-actions">
          <CopyButton value={snapshot.raw} label={t('result.copyJson')} />
          <button
            type="button"
            className="button button--quiet button--small"
            onClick={() => downloadText(`plenilune-comparison-${fileStamp()}.json`, snapshot.raw, 'application/json')}
          >
            <Icon name="download" size={15} />
            {t('result.downloadJson')}
          </button>
        </div>
        <p className="field-hint">{t('common.rawDisplay')}</p>
        <pre className="raw-block" tabIndex={0} role="region" aria-label={t('result.rawJson')}>{prettyJson(snapshot.raw)}</pre>
      </TechnicalDetails>

      <ActionBar note={note}>
        <button type="button" className="button button--secondary" onClick={() => review.navigate('conditions')}>
          {t('result.back')}
        </button>
        {!primaryIsEvidence && (
          <button type="button" className="button button--secondary" onClick={() => review.navigate('evidence')}>
            {t('result.primaryEvidence')}
          </button>
        )}
        {!accepted && !inconclusive ? (
          <button type="button" className="button button--primary" onClick={() => review.navigate('conditions')}>
            {t('result.definePolicy')}
          </button>
        ) : review.canRepair ? (
          <button type="button" className="button button--primary" onClick={() => review.navigate('repair')}>
            <Icon name="repair" size={17} />
            {t('result.primaryRepair')}
          </button>
        ) : canRetryDefault ? (
          <button type="button" className="button button--primary" onClick={review.compareWithDefaultBudget}>
            <Icon name="replay" size={17} />
            {t('result.primaryRetry')}
          </button>
        ) : inconclusive ? (
          <button type="button" className="button button--primary" onClick={review.retryCompare}>
            <Icon name="replay" size={17} />
            {t('accept.retry')}
          </button>
        ) : (
          <button type="button" className="button button--primary" onClick={() => review.navigate('evidence')}>
            <Icon name="arrow" size={17} />
            {t('result.primaryEvidence')}
          </button>
        )}
      </ActionBar>
    </div>
  );
}
