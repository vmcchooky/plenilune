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
import { AnalysisDownloads, AnalysisReplay } from './EvidenceStep';
import { Icon } from './Icon';
import { ReportView } from './ReportView';
import { BusyPanel, CopyButton, StatusMark, TechnicalDetails } from './Ui';

// The evidence drawer of the quick verdict screen: the same artifacts the
// deep flow's evidence stage offers (translated report, raw JSON, replay,
// downloads), folded into one expandable section of the verdict screen. The
// ticket summary is the share action and leads the download grid whenever an
// accepted policy exists — it is only defined for one, exactly as in the
// deep flow, so a behavior-only verdict offers the raw artifacts instead.
export function QuickEvidence({ review }: { review: Review }) {
  const { t, locale } = useI18n();
  const [open, setOpen] = useState(false);
  const [view, setView] = useState<'report' | 'raw'>('report');
  const [generatedAt] = useState(() => new Date());
  if (!review.analysis && !review.comparison) return null;
  const copy = reviewCopy[locale];
  const stamp = fileStamp(generatedAt);
  const accepted = review.accepted;
  const comparison = review.comparison;
  const parsed = review.parsed;
  const analysis = review.analysis;

  const reportInput: ReportInput | null = accepted && comparison ? {
    locale,
    t,
    accepted,
    comparison,
    repair: review.repair,
    comparisonReplay: review.comparisonReplay,
    repairReplay: review.repairReplay,
    lastAi: review.lastAi,
    sourceKind: review.sourceKind,
    generatedAt,
    reviewDecision: review.decision,
    disputed: review.disputed,
  } : null;

  return (
    <section className="quick-evidence">
      <button
        type="button"
        className="quick-evidence__toggle"
        aria-expanded={open}
        aria-controls="quick-evidence-panel"
        onClick={() => setOpen((current) => !current)}
      >
        <Icon name="chevron" size={16} />
        <span>{t('quick.evidence.title')}</span>
        <span className="field-label">{t('evidence.downloadTicket')}</span>
      </button>
      {open && (
        <div className="quick-evidence__panel" id="quick-evidence-panel">
          {reportInput && comparison ? (
            <>
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
                <>
                  <TechnicalDetails title={t('evidence.downloadComparison')} defaultOpen>
                    <div className="raw-actions">
                      <CopyButton value={comparison.raw} />
                      <button type="button" className="button button--quiet button--small" onClick={() => downloadText('comparison.json', comparison.raw, 'application/json')}>
                        <Icon name="download" size={15} /> comparison.json
                      </button>
                    </div>
                    <pre className="raw-block" tabIndex={0} role="region" aria-label={t('evidence.downloadComparison')}>{prettyJson(comparison.raw)}</pre>
                  </TechnicalDetails>
                  {review.repair && (
                    <TechnicalDetails title={copy.downloadReceipt}>
                      <pre className="raw-block" tabIndex={0} role="region" aria-label={copy.downloadReceipt}>{prettyJson(standaloneRepairRaw(review.repair.receipt, review.repair.raw))}</pre>
                    </TechnicalDetails>
                  )}
                </>
              )}
              <div className="download-grid">
                <button type="button" className="download-item" onClick={() => downloadText(`plenilune-ticket-${locale}.md`, buildTicketSummary(reportInput), 'text/markdown;charset=utf-8')}>
                  <Icon name="file" size={18} /><span><strong>{t('evidence.downloadTicket')}</strong><small>{`plenilune-ticket-${locale}.md`}</small></span><Icon name="download" size={16} />
                </button>
                <button type="button" className="download-item" onClick={() => downloadText(`plenilune-report-${locale}-${stamp}.md`, buildMarkdownReport(reportInput), 'text/markdown; charset=utf-8')}>
                  <Icon name="file" size={18} /><span><strong>{t('evidence.downloadReport', { language: languageNames[locale] })}</strong><small>{`plenilune-report-${locale}-${stamp}.md`}</small></span><Icon name="download" size={16} />
                </button>
                <button type="button" className="download-item" onClick={() => downloadText('before.acl', accepted!.before, 'text/plain;charset=utf-8')}>
                  <Icon name="file" size={18} /><span><strong>{copy.downloadBefore}</strong><small>before.acl</small></span><Icon name="download" size={16} />
                </button>
                <button type="button" className="download-item" onClick={() => downloadText('after.acl', accepted!.after, 'text/plain;charset=utf-8')}>
                  <Icon name="file" size={18} /><span><strong>{copy.downloadAfter}</strong><small>after.acl</small></span><Icon name="download" size={16} />
                </button>
                <AnalysisDownloads review={review} />
                <button type="button" className="download-item" onClick={() => downloadText('replay-instructions.md', replayInstructions(locale, Boolean(review.repair), comparison.comparison.tool.version, Boolean(review.analysis)), 'text/markdown;charset=utf-8')}>
                  <Icon name="file" size={18} /><span><strong>{copy.instructions}</strong><small>replay-instructions.md</small></span><Icon name="download" size={16} />
                </button>
              </div>
            </>
          ) : comparison && parsed ? (
            <>
              <p>{locale === 'vi' ? 'Chỉ so sánh hành vi; chưa có bộ điều kiện được duyệt hoặc kết luận về ý định.'
                : 'Behavior comparison only; no accepted conditions or intent verdict yet.'}</p>
              <p><code>{comparison.comparison.report.verdict}</code></p>
              <div className="download-grid">
                <button type="button" className="download-item" onClick={() => downloadText('before.acl', parsed.before, 'text/plain;charset=utf-8')}>
                  <Icon name="file" size={18} /><span><strong>{copy.downloadBefore}</strong><small>before.acl</small></span><Icon name="download" size={16} />
                </button>
                <button type="button" className="download-item" onClick={() => downloadText('after.acl', parsed.after, 'text/plain;charset=utf-8')}>
                  <Icon name="file" size={18} /><span><strong>{copy.downloadAfter}</strong><small>after.acl</small></span><Icon name="download" size={16} />
                </button>
                <button type="button" className="download-item" onClick={() => downloadText('comparison.json', comparison.raw, 'application/json')}>
                  <Icon name="file" size={18} /><span><strong>{t('evidence.downloadComparison')}</strong><small>comparison.json</small></span><Icon name="download" size={16} />
                </button>
                <AnalysisDownloads review={review} />
                <button type="button" className="download-item" onClick={() => downloadText('replay-instructions.md', replayInstructions(locale, false, comparison.comparison.tool.version, Boolean(review.analysis)), 'text/markdown;charset=utf-8')}>
                  <Icon name="file" size={18} /><span><strong>{copy.instructions}</strong><small>replay-instructions.md</small></span><Icon name="download" size={16} />
                </button>
              </div>
            </>
          ) : analysis ? (
            <div className="download-grid">
              <AnalysisDownloads review={review} />
              <button type="button" className="download-item" onClick={() => downloadText('replay-instructions.md', replayInstructions(locale, false, analysis.result.tool.version, true), 'text/markdown;charset=utf-8')}>
                <Icon name="file" size={18} /><span><strong>{copy.instructions}</strong><small>replay-instructions.md</small></span><Icon name="download" size={16} />
              </button>
            </div>
          ) : null}

          <div className="replay-tools__grid">
            {comparison && (
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
            )}
            <AnalysisReplay review={review} />
          </div>
        </div>
      )}
    </section>
  );
}
