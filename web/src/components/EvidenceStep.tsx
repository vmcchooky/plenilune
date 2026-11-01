import { useState } from 'react';
import { useI18n } from '../i18n';
import { languageNames } from '../i18n';
import type { Review } from '../hooks/useReview';
import { downloadText } from '../lib/download';
import { fileStamp, prettyJson } from '../lib/format';
import { replayLabel } from '../lib/labels';
import { buildMarkdownReport, buildTicketSummary } from '../lib/report';
import type { ReportInput } from '../lib/report';
import { replayInstructions, reviewCopy, standaloneRepairRaw } from '../lib/reviewArtifacts';
import { Icon } from './Icon';
import { ReportView } from './ReportView';
import {
  ActionBar,
  BusyPanel,
  CopyButton,
  ProvenanceTag,
  SectionHeading,
  StatusMark,
  StepHeader,
  TechnicalDetails,
} from './Ui';

interface ArtifactItem {
  key: string;
  title: string;
  raw: string;
  filename: string;
}

export function EvidenceStep({ review }: { review: Review }) {
  const { t, locale } = useI18n();
  const [view, setView] = useState<'report' | 'raw'>('report');
  const [generatedAt] = useState(() => new Date());
  const accepted = review.accepted;
  const comparison = review.comparison;
  if (!comparison) return review.analysis ? <AnalysisEvidence review={review} /> : null;
  if (!accepted) return <DeltaEvidence review={review} />;
  const repair = review.repair;
  const stamp = fileStamp(generatedAt);
  const copy = reviewCopy[locale];

  const reportInput: ReportInput = {
    locale,
    t,
    accepted,
    comparison,
    repair,
    comparisonReplay: review.comparisonReplay,
    repairReplay: review.repairReplay,
    lastAi: review.lastAi,
    sourceKind: review.sourceKind,
    generatedAt,
    reviewDecision: review.decision,
    disputed: review.disputed,
  };
  const markdown = buildMarkdownReport(reportInput);
  const reportName = `plenilune-report-${locale}-${stamp}.md`;
  const ticketSummary = buildTicketSummary(reportInput);
  const ticketName = `plenilune-ticket-${locale}.md`;

  const artifacts: ArtifactItem[] = [
    {
      key: 'comparison',
      title: t('evidence.downloadComparison'),
      raw: comparison.raw,
      filename: 'comparison.json',
    },
  ];
  if (repair) {
    artifacts.push({
      key: 'repair',
      title: copy.downloadReceipt,
      raw: standaloneRepairRaw(repair.receipt, repair.raw),
      filename: 'repair.json',
    });
  }
  if (review.aiProposal) {
    artifacts.push({
      key: 'ai-proposal',
      title: t('evidence.downloadProposalSession'),
      raw: review.aiProposal.raw,
      filename: `plenilune-session-proposal-${stamp}.json`,
    });
  }
  if (review.aiSession) {
    artifacts.push({
      key: 'ai-repair',
      title: t('evidence.downloadRepairSession'),
      raw: review.aiSession.raw,
      filename: `plenilune-session-repair-${stamp}.json`,
    });
  }
  if (review.comparisonReplay) {
    artifacts.push({
      key: 'comparison-replay',
      title: t('evidence.replayComparison'),
      raw: review.comparisonReplay.raw,
      filename: `plenilune-compare-replay-${stamp}.json`,
    });
  }
  if (review.repairReplay) {
    artifacts.push({
      key: 'repair-replay',
      title: t('evidence.replayRepair'),
      raw: review.repairReplay.raw,
      filename: `plenilune-repair-replay-${stamp}.json`,
    });
  }

  function download(content: string, filename: string) {
    downloadText(filename, content, 'application/json');
  }

  return (
    <div className="step">
      <StepHeader kicker={t('evidence.kicker')} title={t('evidence.title')} lede={t('evidence.lede')}>
        <ProvenanceTag icon="file">{t('evidence.reportView')}</ProvenanceTag>
        {review.sourceKind === 'sample'
          ? <ProvenanceTag icon="file">{t('evidence.sample')}</ProvenanceTag>
          : <ProvenanceTag icon="file">{t('evidence.custom')}</ProvenanceTag>}
      </StepHeader>

      <div className="evidence-toolbar" role="group" aria-label={t('evidence.reportView')}>
        <button
          type="button"
          className={`segmented ${view === 'report' ? 'segmented--active' : ''}`}
          aria-pressed={view === 'report'}
          onClick={() => setView('report')}
        >
          {t('evidence.reportView')}
        </button>
        <button
          type="button"
          className={`segmented ${view === 'raw' ? 'segmented--active' : ''}`}
          aria-pressed={view === 'raw'}
          onClick={() => setView('raw')}
        >
          {t('evidence.rawView')}
        </button>
      </div>

      {view === 'report' ? (
        <ReportView input={reportInput} />
      ) : (
        <section className="raw-view" aria-labelledby="raw-view-title">
          <SectionHeading
            id="raw-view-title"
            title={t('evidence.rawView')}
            note={t('evidence.downloadNotice')}
          />
          <p className="field-hint">{t('common.rawDisplay')}</p>
          {artifacts.map((artifact) => (
            <TechnicalDetails title={artifact.title} key={artifact.key}>
              <div className="raw-actions">
                <CopyButton value={artifact.raw} />
                <button
                  type="button"
                  className="button button--quiet button--small"
                  onClick={() => download(artifact.raw, artifact.filename)}
                >
                  <Icon name="download" size={15} />
                  {artifact.filename}
                </button>
              </div>
              <pre className="raw-block" tabIndex={0} role="region" aria-label={artifact.title}>{prettyJson(artifact.raw)}</pre>
            </TechnicalDetails>
          ))}
        </section>
      )}

      <section className="replay-tools">
        <h2>{copy.decision}</h2>
        <p>{review.disputed ? copy.disputed : review.decision && review.decision.receiptSha256 === repair?.receipt.sha256
          ? copy[review.decision.choice] : copy.undecided}</p>
        <p className="field-hint">{copy.decisionNote}</p>
      </section>

      <section className="replay-tools" aria-labelledby="replay-tools-title">
        <SectionHeading id="replay-tools-title" title={t('evidence.replay')} note={t('repair.replayHelp')} />
        <div className="replay-tools__grid">
          <div className="replay-tool">
            <h3>{t('evidence.replayComparison')}</h3>
            {review.comparisonReplay ? (
              <p>
                <StatusMark
                  tone={review.comparisonReplay.result.status === 'TIMING_SENSITIVE' ? 'warn' : 'good'}
                  label={replayLabel(t, review.comparisonReplay.result.status)}
                />
                <code>{review.comparisonReplay.result.status}</code>
                <span>{t('repair.replayCount', { count: review.comparisonReplay.result.rechecked_witnesses })}</span>
              </p>
            ) : <p className="field-hint">{t('report.noReplay')}</p>}
            {review.busy === 'replayComparison' ? (
              <BusyPanel label={t('repair.replayBusy')} onCancel={review.cancel} />
            ) : (
              <button type="button" className="button button--secondary button--small" onClick={review.replayComparison} disabled={review.busy !== null}>
                <Icon name="replay" size={15} /> {t('evidence.replayComparison')}
              </button>
            )}
            {review.canceled === 'replayComparison' && <p className="banner banner--neutral">{t('common.canceled')}</p>}
          </div>
          <div className="replay-tool">
            <h3>{t('evidence.replayRepair')}</h3>
            {review.repairReplay ? (
              <p>
                <StatusMark
                  tone={review.repairReplay.result.status === 'TIMING_SENSITIVE' ? 'warn' : 'good'}
                  label={replayLabel(t, review.repairReplay.result.status)}
                />
                <code>{review.repairReplay.result.status}</code>
                <span>{t('repair.replayCount', { count: review.repairReplay.result.rechecked_witnesses })}</span>
              </p>
            ) : <p className="field-hint">{repair ? t('report.noReplay') : t('evidence.noRepair')}</p>}
            {review.busy === 'replayRepair' ? (
              <BusyPanel label={t('repair.replayBusy')} onCancel={review.cancel} />
            ) : (
              <button type="button" className="button button--secondary button--small" onClick={review.replayRepair} disabled={!repair || review.busy !== null}>
                <Icon name="replay" size={15} /> {t('evidence.replayRepair')}
              </button>
            )}
            {review.canceled === 'replayRepair' && <p className="banner banner--neutral">{t('common.canceled')}</p>}
          </div>
        </div>
      </section>

      <section className="download-section" aria-labelledby="download-title">
        <SectionHeading
          id="download-title"
          title={t('evidence.downloadTitle')}
          note={t('evidence.downloadNotice')}
        />
        <p className="field-hint">{copy.artifactHelp}</p>
        <div className="download-grid">
          <button type="button" className="download-item" onClick={() => downloadText('before.acl', accepted.before, 'text/plain;charset=utf-8')}>
            <Icon name="file" size={18} /><span><strong>{copy.downloadBefore}</strong><small>before.acl</small></span><Icon name="download" size={16} />
          </button>
          <button type="button" className="download-item" onClick={() => downloadText('after.acl', accepted.after, 'text/plain;charset=utf-8')}>
            <Icon name="file" size={18} /><span><strong>{copy.downloadAfter}</strong><small>after.acl</small></span><Icon name="download" size={16} />
          </button>
          <AnalysisDownloads review={review} />
          <button type="button" className="download-item" onClick={() => downloadText('replay-instructions.md', replayInstructions(locale, Boolean(repair), comparison.comparison.tool.version, Boolean(review.analysis)), 'text/markdown;charset=utf-8')}>
            <Icon name="file" size={18} /><span><strong>{copy.instructions}</strong><small>replay-instructions.md</small></span><Icon name="download" size={16} />
          </button>
          <button type="button" className="download-item" onClick={() => downloadText(ticketName, ticketSummary, 'text/markdown;charset=utf-8')}>
            <Icon name="file" size={18} /><span><strong>{t('evidence.downloadTicket')}</strong><small>{ticketName}</small></span><Icon name="download" size={16} />
          </button>
          {repair?.receipt.candidate && <button type="button" className="download-item" onClick={() => downloadText('candidate.acl', repair.receipt.candidate!, 'text/plain;charset=utf-8')}>
            <Icon name="file" size={18} /><span><strong>{copy.candidate}</strong><small>candidate.acl</small></span><Icon name="download" size={16} />
          </button>}
          {artifacts.map((artifact) => (
            <button
              type="button"
              className="download-item"
              key={artifact.key}
              onClick={() => download(artifact.raw, artifact.filename)}
            >
              <Icon name="file" size={18} />
              <span>
                <strong>{artifact.title}</strong>
                <small>{artifact.filename}</small>
              </span>
              <Icon name="download" size={16} />
            </button>
          ))}
          <button type="button" className="download-item" onClick={() => window.print()}>
            <Icon name="print" size={18} />
            <span>
              <strong>{t('evidence.print')}</strong>
              <small>{t('evidence.reportView')}</small>
            </span>
            <Icon name="arrow" size={16} />
          </button>
        </div>
      </section>

      <ActionBar note={t('evidence.downloadNotice')}>
        <button type="button" className="button button--secondary" onClick={() => review.navigate(repair ? 'repair' : 'result')}>
          {t('evidence.back')}
        </button>
        <button
          type="button"
          className="button button--primary"
          onClick={() => downloadText(reportName, markdown, 'text/markdown; charset=utf-8')}
        >
          <Icon name="download" size={17} />
          {t('evidence.downloadReport', { language: languageNames[locale] })}
        </button>
      </ActionBar>
    </div>
  );
}

function DeltaEvidence({ review }: { review: Review }) {
  const { t, locale } = useI18n();
  const copy = reviewCopy[locale];
  const comparison = review.comparison;
  const parsed = review.parsed;
  if (!comparison || !parsed) return null;
  return <div className="step">
    <StepHeader kicker={t('evidence.kicker')} title={t('evidence.title')} lede={copy.artifactHelp} />
    <p>{locale === 'vi' ? 'Chỉ so sánh hành vi; chưa có bộ điều kiện được duyệt hoặc kết luận về ý định.'
      : 'Behavior comparison only; no accepted conditions or intent verdict yet.'}</p>
    <p><code>{comparison.comparison.report.verdict}</code></p>
    <div className="download-grid">
      <button type="button" className="download-item" onClick={() => downloadText('before.acl', parsed.before, 'text/plain;charset=utf-8')}>{copy.downloadBefore}</button>
      <button type="button" className="download-item" onClick={() => downloadText('after.acl', parsed.after, 'text/plain;charset=utf-8')}>{copy.downloadAfter}</button>
      <button type="button" className="download-item" onClick={() => downloadText('comparison.json', comparison.raw, 'application/json')}>{t('evidence.downloadComparison')}</button>
      <AnalysisDownloads review={review} />
      <button type="button" className="download-item" onClick={() => downloadText('replay-instructions.md', replayInstructions(locale, false, comparison.comparison.tool.version, Boolean(review.analysis)), 'text/markdown;charset=utf-8')}>{copy.instructions}</button>
    </div>
    <AnalysisReplay review={review} />
    {review.comparisonReplay ? <p role="status">{replayLabel(t, review.comparisonReplay.result.status)} <code>{review.comparisonReplay.result.status}</code></p> : <p>{copy.unchecked}</p>}
    {review.busy === 'replayComparison' ? <BusyPanel label={copy.busy} onCancel={review.cancel} />
      : <button type="button" className="button button--secondary" disabled={review.busy !== null} onClick={review.replayComparison}>{t('evidence.replayComparison')}</button>}
    <ActionBar><button type="button" className="button button--secondary" onClick={() => review.navigate('result')}>{t('evidence.back')}</button></ActionBar>
  </div>;
}

// Source configurations and the domain-neutral analysis record. Source files
// are only offered when they differ from the analyzed object block.
export function AnalysisDownloads({ review }: { review: Review }) {
  const { t } = useI18n();
  const analysis = review.analysis;
  if (!analysis) return null;
  const parsed = review.parsed;
  const distinct = !parsed || parsed.before !== analysis.before || parsed.after !== analysis.after;
  return (
    <>
      {distinct && (
        <>
          <button type="button" className="download-item" onClick={() => downloadText('before.cfg', analysis.before, 'text/plain;charset=utf-8')}>
            <Icon name="file" size={18} /><span><strong>{t('evidence.sourceBefore')}</strong><small>before.cfg</small></span><Icon name="download" size={16} />
          </button>
          <button type="button" className="download-item" onClick={() => downloadText('after.cfg', analysis.after, 'text/plain;charset=utf-8')}>
            <Icon name="file" size={18} /><span><strong>{t('evidence.sourceAfter')}</strong><small>after.cfg</small></span><Icon name="download" size={16} />
          </button>
        </>
      )}
      <button type="button" className="download-item" onClick={() => downloadText('analysis.json', analysis.raw, 'application/json')}>
        <Icon name="file" size={18} /><span><strong>{t('evidence.analysis')}</strong><small>analysis.json</small></span><Icon name="download" size={16} />
      </button>
    </>
  );
}

export function AnalysisReplay({ review }: { review: Review }) {
  const { t } = useI18n();
  if (!review.analysis) return null;
  const replay = review.analysisReplay;
  return (
    <div className="replay-tool">
      <h3>{t('evidence.replayAnalysis')}</h3>
      {replay ? (
        <p>
          <StatusMark tone={replay.result.status === 'TIMING_SENSITIVE' ? 'warn' : 'good'} label={replayLabel(t, replay.result.status)} />
          <code>{replay.result.status}</code>
        </p>
      ) : <p className="field-hint">{t('report.noReplay')}</p>}
      {review.busy === 'replayAnalysis' ? (
        <BusyPanel label={t('repair.replayBusy')} onCancel={review.cancel} />
      ) : (
        <button type="button" className="button button--secondary button--small" disabled={review.busy !== null} onClick={review.replayAnalysis}>
          <Icon name="replay" size={15} /> {t('evidence.replayAnalysis')}
        </button>
      )}
      {review.canceled === 'replayAnalysis' && <p className="banner banner--neutral">{t('common.canceled')}</p>}
    </div>
  );
}

function AnalysisEvidence({ review }: { review: Review }) {
  const { t, locale } = useI18n();
  const copy = reviewCopy[locale];
  const analysis = review.analysis;
  if (!analysis) return null;
  return (
    <div className="step">
      <StepHeader kicker={t('evidence.kicker')} title={t('evidence.analysisTitle')} lede={t('evidence.analysisHelp')} />
      <section className="download-section" aria-labelledby="download-title">
        <SectionHeading id="download-title" title={t('evidence.downloadTitle')} note={t('evidence.downloadNotice')} />
        <div className="download-grid">
          <AnalysisDownloads review={review} />
          <button type="button" className="download-item" onClick={() => downloadText('replay-instructions.md', replayInstructions(locale, false, analysis.result.tool.version, true), 'text/markdown;charset=utf-8')}>
            <Icon name="file" size={18} /><span><strong>{copy.instructions}</strong><small>replay-instructions.md</small></span><Icon name="download" size={16} />
          </button>
        </div>
      </section>
      <section className="replay-tools"><AnalysisReplay review={review} /></section>
      <TechnicalDetails title={t('evidence.analysis')}>
        <div className="raw-actions"><CopyButton value={analysis.raw} /></div>
        <pre className="raw-block" tabIndex={0} role="region" aria-label={t('evidence.analysis')}>{prettyJson(analysis.raw)}</pre>
      </TechnicalDetails>
      <ActionBar><button type="button" className="button button--secondary" onClick={() => review.navigate('result')}>{t('evidence.back')}</button></ActionBar>
    </div>
  );
}
