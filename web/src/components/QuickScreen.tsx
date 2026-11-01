import { useEffect, useRef, useState } from 'react';
import type { DragEvent } from 'react';
import { useI18n } from '../i18n';
import type { Review } from '../hooks/useReview';
import { kindLabel, obligationSentence } from '../lib/conditions';
import {
  obligationStatusList,
  outcomeLabel,
  scopeStatusLabel,
  verdictBody,
  verdictTitle,
} from '../lib/labels';
import { readReviewFile } from '../lib/reviewArtifacts';
import type { ObligationStatus, Verdict } from '../lib/types';
import { collectWitnesses } from '../lib/witnesses';
import { ChatPanel } from './ChatPanel';
import { ConditionEditorList, ScopeEditor, draftCanAccept } from './ConditionEditor';
import { ConfigOverview } from './ConfigOverview';
import { Icon } from './Icon';
import { ObjectDetail } from './PrefixListDetail';
import { PartialNotice } from './PartialNotice';
import { QuickEvidence } from './QuickEvidence';
import { RepairStep } from './RepairStep';
import { SampleBand, SourceEditor } from './SourceInputs';
import {
  BusyPanel,
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

function outcomeTone(status: ObligationStatus): Tone {
  return status === 'HOLDS' ? 'good' : status === 'VIOLATED' ? 'bad' : 'warn';
}

// The verdict plate of the quick screen. It prefers the exact comparison
// when one exists; while obligations are being edited inline (which clears
// the comparison by design) it falls back to the verdict recorded in the
// analysis artifact for the opened ACL object, labeled as unchecked against
// obligations. It never invents a configuration-level verdict: with several
// objects and none opened, the overview headline below is the summary.
function QuickVerdictBanner({ review }: { review: Review }) {
  const { t } = useI18n();
  const report = review.comparison?.comparison.report ?? null;
  const object = review.selectedObject;
  const fromAnalysis = !report && object?.domain === 'acl' ? object.acl ?? null : null;
  const verdict: Verdict | null = report?.verdict ?? fromAnalysis?.verdict ?? null;
  if (!verdict) return null;
  const tone: Tone = verdict === 'DIFFERENT' ? 'bad' : verdict === 'EQUIVALENT' ? 'neutral' : 'warn';
  const reason = report ? report.reason : fromAnalysis?.budget.stop_reason;
  const witnessCount = report
    ? collectWitnesses(t, report).length
    : fromAnalysis?.witnesses.length ?? 0;
  const searchComplete = report ? report.search_complete : fromAnalysis?.search_complete;
  const partial = report?.partial ?? fromAnalysis?.partial ?? null;
  return (
    <>
      <section className={`card verdict-plate verdict-plate--${verdict.toLowerCase()}`} aria-labelledby="quick-verdict-title">
        <div className="verdict-plate__head">
          <ProvenanceTag tone={tone === 'bad' ? 'info' : tone} icon="shield">{t('result.runtime')}</ProvenanceTag>
          <code className="badge">{verdict}</code>
          {partial && <span className="field-label">{t('partial.badge')}</span>}
          {report?.witnesses_truncated && <span className="field-label">{t('result.truncated')}</span>}
        </div>
        <h2 id="quick-verdict-title">{verdictTitle(t, verdict)}</h2>
        <p>{verdictBody(t, verdict, reason)}</p>
        <div className="verdict-plate__facts">
          {report ? (
            <span>{review.accepted ? obligationStatusList(t, report.obligations) : t('result.noPolicy')}</span>
          ) : (
            <span>{t('quick.unevaluated')}</span>
          )}
          <span>{t('result.witnessCount', { count: witnessCount })}</span>
          {searchComplete !== undefined && (
            <span>{t('result.searchComplete')}: {searchComplete ? t('result.yes') : t('result.no')}</span>
          )}
        </div>
        <p className="verdict-plate__limits">{t('quick.scopeNote')}</p>
      </section>
      {partial && <PartialNotice partial={partial} />}
      {verdict === 'INCONCLUSIVE' && <TermNote>{t('result.noWitness.inconclusive')}</TermNote>}
    </>
  );
}

// Obligations live on the verdict screen: the same draft editor as the deep
// flow's conditions step, the canonical preview readback before acceptance,
// and the per-obligation status listing rendered in place once accepted.
// Nothing here navigates; the deep flow's stage changes are ignored by this
// screen, which reads only the review state.
function QuickObligations({ review }: { review: Review }) {
  const { t } = useI18n();
  const draft = review.draft;
  const busy = review.busy !== null;
  const accepted = review.accepted;
  const report = review.comparison?.comparison.report ?? null;

  if (!review.parsed) {
    if (!review.analysis) return null;
    const object = review.selectedObject;
    return (
      <section className="quick-obligations" aria-labelledby="quick-obligations-title">
        <SectionHeading id="quick-obligations-title" title={t('quick.obligations.title')} note={t('quick.obligations.help')} />
        <p className="field-hint">
          {object && object.domain !== 'acl' ? t('quick.obligations.aclOnly') : t('quick.obligations.needObject')}
        </p>
      </section>
    );
  }

  return (
    <section className="quick-obligations" aria-labelledby="quick-obligations-title">
      <SectionHeading
        id="quick-obligations-title"
        title={t('quick.obligations.title')}
        note={t('quick.obligations.help')}
        aside={<ProvenanceTag>{t('cond.count', { count: draft.conditions.length })}</ProvenanceTag>}
      />

      {accepted && report && (
        <div className="quick-obligations__results">
          <p className="quick-obligations__listing">{obligationStatusList(t, report.obligations)}</p>
          <div className="condition-result-list">
            {report.obligations.map((outcome) => {
              const obligation = accepted.conditions.obligations.find((item) => item.id === outcome.id) ?? null;
              return (
                <div className="condition-result" key={outcome.id}>
                  <span className="condition-result__id">
                    <code>{outcome.id}</code>
                    {obligation && <span>{kindLabel(t, obligation.kind)}</span>}
                  </span>
                  <span className="condition-result__body">
                    <strong>{obligation ? obligationSentence(t, obligation) : outcome.id}</strong>
                  </span>
                  <span className="condition-result__outcome">
                    <StatusMark tone={outcomeTone(outcome.status)} label={outcomeLabel(t, outcome.status)} />
                  </span>
                </div>
              );
            })}
          </div>
          <div className="accepted-stamp">
            <StatusMark tone="good" label={t('ai.acceptedByOperator')} />
            <HashLine label={t('accept.digest')} value={accepted.digest} />
          </div>
        </div>
      )}

      <div className="intent-block">
        <label htmlFor="quick-intent">{t('source.intent')}</label>
        <textarea
          id="quick-intent"
          rows={2}
          className="intent-block__input"
          value={draft.intent}
          disabled={busy}
          placeholder={t('source.intentPlaceholder')}
          onChange={(event) => review.updateIntent(event.target.value)}
        />
      </div>

      <ConditionEditorList review={review} />

      <TechnicalDetails title={t('scope.title')}>
        <ScopeEditor review={review} />
      </TechnicalDetails>

      {!accepted && (
        <div className="quick-obligations__accept">
          {review.busy === 'previewPolicy' ? (
            <BusyPanel label={t('accept.previewBusy')} onCancel={review.cancel} />
          ) : review.busy === 'compare' ? (
            <BusyPanel label={t('accept.busy')} onCancel={review.cancel} />
          ) : review.policyPreview ? (
            <div className="canonical-preview__readback" role="region" aria-label={t('accept.previewTitle')}>
              <h3>{t('accept.previewTitle')}</h3>
              <ol>
                {review.policyPreview.normalized.policy.conditions.obligations.map((obligation) => (
                  <li key={obligation.id}><code>{obligation.id}</code> {obligationSentence(t, obligation)}</li>
                ))}
              </ol>
              <p>{scopeStatusLabel(t, draft.scopeMode, review.policyPreview.normalized.policy.allowed_changes?.length ?? 0)}</p>
              <HashLine label={t('accept.previewDigest')} value={review.policyPreview.normalized.policy_sha256} />
              <div className="raw-actions">
                <button type="button" className="button button--primary" onClick={() => review.acceptAndCompare(true)}>
                  <Icon name="arrow" size={16} /> {t('accept.action')}
                </button>
              </div>
            </div>
          ) : (
            <button
              type="button"
              className="button button--secondary"
              disabled={!draftCanAccept(draft) || busy}
              onClick={review.previewPolicy}
            >
              {t('accept.previewAction')}
            </button>
          )}
          {review.canceled === 'compare' && (
            <p className="banner banner--neutral">{t('common.canceled')}</p>
          )}
        </div>
      )}
    </section>
  );
}

export function QuickScreen({ review }: { review: Review }) {
  const { t } = useI18n();
  const draft = review.draft;
  const busy = review.busy !== null;
  const bothPresent = Boolean(draft.before.trim() && draft.after.trim());
  const signature = `${draft.before}\u0000${draft.after}`;
  const attemptedRef = useRef<string | null>(null);
  const exploreRef = useRef(review.exploreSource);
  exploreRef.current = review.exploreSource;
  const [dragOver, setDragOver] = useState(false);
  const [dropError, setDropError] = useState(false);

  // Auto-run: as soon as both configurations are present and no analysis
  // covers them, run the same explore path as the deep flow's first action.
  // A short debounce keeps typing in an editor from re-analyzing per
  // keystroke; each attempted input pair is recorded so a cancel or an
  // error never loops — the manual button below is the retry path.
  useEffect(() => {
    if (!bothPresent || review.analysis || review.busy || review.error) return;
    if (attemptedRef.current === signature) return;
    const timer = setTimeout(() => {
      attemptedRef.current = signature;
      void exploreRef.current();
    }, 400);
    return () => clearTimeout(timer);
  }, [bothPresent, signature, review.analysis, review.busy, review.error]);

  function runNow() {
    attemptedRef.current = signature;
    void exploreRef.current();
  }

  function onDrop(event: DragEvent) {
    event.preventDefault();
    setDragOver(false);
    if (busy) return;
    const files = Array.from(event.dataTransfer.files ?? []);
    if (files.length === 0) return;
    void (async () => {
      try {
        const texts: string[] = [];
        for (const file of files.slice(0, 2)) {
          texts.push((await readReviewFile(file, 4 << 20)).text);
        }
        setDropError(false);
        if (texts.length >= 2) {
          review.updateSource('before', texts[0]);
          review.updateSource('after', texts[1]);
        } else {
          const target = draft.before.trim() && !draft.after.trim() ? 'after' : 'before';
          review.updateSource(target, texts[0]);
        }
      } catch {
        setDropError(true);
      }
    })();
  }

  const report = review.comparison?.comparison.report ?? null;
  const showRepair = Boolean(
    review.accepted && report && report.verdict === 'DIFFERENT'
    && report.obligations.some((obligation) => obligation.status === 'VIOLATED'),
  );

  return (
    <div className="step quick-screen">
      <StepHeader kicker={t('quick.kicker')} title={t('quick.title')} lede={t('quick.lede')}>
        {review.pristine && <ProvenanceTag icon="file">{t('source.sample.pristine')}</ProvenanceTag>}
        {review.sample && !review.pristine && <ProvenanceTag icon="file">{t('source.sample.modified')}</ProvenanceTag>}
      </StepHeader>

      <div
        className={`quick-dropzone ${dragOver ? 'quick-dropzone--over' : ''}`}
        onDragOver={(event) => { event.preventDefault(); if (!busy) setDragOver(true); }}
        onDragLeave={() => setDragOver(false)}
        onDrop={onDrop}
      >
        <SampleBand review={review} />
        <div className="card source-spread">
          <SourceEditor
            id="before"
            label={t('source.before')}
            value={draft.before}
            help={t('source.beforeHelp')}
            disabled={busy}
            onChange={(value) => review.updateSource('before', value)}
          />
          <SourceEditor
            id="after"
            label={t('source.after')}
            value={draft.after}
            help={t('source.beforeHelp')}
            disabled={busy}
            onChange={(value) => review.updateSource('after', value)}
          />
        </div>
        <p className="field-hint">{t('quick.dropHint')} {t('source.privacy')}</p>
        {dropError && <p className="field-error" role="alert">{t('source.importError')}</p>}
      </div>

      {review.busy === 'parse' && !review.analysis && (
        <BusyPanel label={t('source.busy')} onCancel={review.cancel} />
      )}
      {review.canceled === 'parse' && !review.analysis && (
        <p className="banner banner--neutral">{t('common.canceled')}</p>
      )}
      {!bothPresent && !review.analysis && (
        <EmptyState icon="file" title={t('quick.empty.title')} body={t('quick.empty.body')} />
      )}
      {bothPresent && !review.analysis && !busy && (
        <div className="quick-run">
          <button type="button" className="button button--primary" onClick={runNow}>
            <Icon name="arrow" size={17} />
            {t('source.action')}
          </button>
        </div>
      )}

      <QuickVerdictBanner review={review} />
      {review.analysis && <ConfigOverview review={review} />}
      {!review.comparison && review.selectedObject && (
        <ObjectDetail object={review.selectedObject} />
      )}
      <QuickObligations review={review} />
      {showRepair && (
        <section className="quick-repair" aria-labelledby="quick-repair-title">
          <SectionHeading id="quick-repair-title" title={t('quick.repair.title')} />
          <RepairStep review={review} embedded />
        </section>
      )}
      <QuickEvidence review={review} />
      <ChatPanel comparison={review.comparison?.comparison ?? null} analysis={review.analysis?.result ?? null} />
    </div>
  );
}
