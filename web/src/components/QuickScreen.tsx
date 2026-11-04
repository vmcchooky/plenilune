import { useEffect, useRef, useState } from 'react';
import type { DragEvent } from 'react';
import { useI18n } from '../i18n';
import type { Review } from '../hooks/useReview';
import { kindLabel, obligationSentence } from '../lib/conditions';
import {
  obligationStatusList,
  outcomeLabel,
  scopeStatusLabel,
} from '../lib/labels';
import { readReviewFile } from '../lib/reviewArtifacts';
import type { ObligationStatus } from '../lib/types';
import { ChatPanel } from './ChatPanel';
import { ConditionEditorList, ScopeEditor, draftCanAccept } from './ConditionEditor';
import { ConfigOverview } from './ConfigOverview';
import { Icon } from './Icon';
import { ObjectDetail } from './PrefixListDetail';
import { QuickOutcome } from './QuickOutcome';
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
  TechnicalDetails,
} from './Ui';
import type { Tone } from './Ui';

function outcomeTone(status: ObligationStatus): Tone {
  return status === 'HOLDS' ? 'good' : status === 'VIOLATED' ? 'bad' : 'warn';
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
        <SectionHeading id="quick-obligations-title" title={t('quick.obligations.title')} note={t('quick.obligations.help')} srOnlyTitle />
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
        srOnlyTitle
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
  const sourceRef = useRef<HTMLDivElement>(null);
  const policyRef = useRef<HTMLDivElement>(null);
  const [sourceOpen, setSourceOpen] = useState(false);
  const [policyOpen, setPolicyOpen] = useState(false);
  const [toolsOpen, setToolsOpen] = useState(false);
  const [questionsOpen, setQuestionsOpen] = useState(false);
  function reveal(ref: { current: HTMLDivElement | null }, setOpen: (open: boolean) => void) {
    setOpen(true);
    requestAnimationFrame(() => {
      ref.current?.focus();
      ref.current?.scrollIntoView?.({ block: 'start' });
    });
  }
  const [dragOver, setDragOver] = useState(false);
  const [dropError, setDropError] = useState(false);

  // Auto-run: as soon as both configurations are present and no analysis
  // covers them, run the same explore path as the deep flow's first action.
  // A short debounce keeps typing in an editor from re-analyzing per
  // keystroke; each attempted input pair is recorded so a cancel or an
  // error never loops — the manual button below is the retry path.
  // Switching samples is an explicit request even when the configuration
  // text is identical (a budget scenario reuses the same ACLs), so a sample
  // change always grants one fresh auto-run attempt (audit F11).
  const sampleIdRef = useRef(review.sampleId);
  useEffect(() => {
    if (sampleIdRef.current !== review.sampleId) {
      sampleIdRef.current = review.sampleId;
      attemptedRef.current = null;
    }
    if (!bothPresent || review.analysis || review.busy || review.error) return;
    if (attemptedRef.current === signature) return;
    const timer = setTimeout(() => {
      attemptedRef.current = signature;
      void exploreRef.current();
    }, 400);
    return () => clearTimeout(timer);
  }, [bothPresent, signature, review.analysis, review.busy, review.error, review.sampleId]);

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
  const showObjectSelection = Boolean(review.analysis && (
    review.analysis.result.summary.objects !== 1 || review.analysis.result.summary.unsupported > 0
    || !review.selectedObject || (!review.comparison && !review.selectedObject.acl)
  ));
  const showRepair = Boolean(
    review.accepted && report && report.verdict === 'DIFFERENT'
    && report.obligations.some((obligation) => obligation.status === 'VIOLATED'),
  );

  return (
    <div className="step quick-screen">
      <header className="quick-header">
        <h1 id="step-heading" tabIndex={-1}>{t('quick.title')}</h1>
        {review.pristine && <ProvenanceTag icon="file">{t('source.sample.pristine')}</ProvenanceTag>}
        {review.sample && !review.pristine && <ProvenanceTag icon="file">{t('source.sample.modified')}</ProvenanceTag>}
      </header>

      <QuickOutcome review={review} onPolicy={() => reveal(policyRef, setPolicyOpen)} onSource={() => reveal(sourceRef, setSourceOpen)} />
      {review.analysis && showObjectSelection && <ConfigOverview review={review} />}
      {review.analysis && !showObjectSelection && (
        <section className="review-disclosure quick-tools">
          <button type="button" className="button button--quiet" aria-expanded={toolsOpen} aria-controls="quick-tools-panel" onClick={() => setToolsOpen(!toolsOpen)}>{t('quick.tools')}</button>
          <section id="quick-tools-panel" className="quick-overview" aria-label={t('quick.objects')} hidden={!toolsOpen}>
          <ConfigOverview review={review} />
          </section>
        </section>
      )}
      {/* Hidden as a whole while the sources panel is collapsed — otherwise the
          section renders a border and padding around nothing (a dead strip
          between the tools and obligations disclosures). */}
      <section className="review-disclosure quick-sources" hidden={Boolean(review.analysis) && !sourceOpen}>
      <div id="quick-sources-panel" ref={sourceRef} tabIndex={-1} hidden={Boolean(review.analysis) && !sourceOpen}>
      <h2>{t('quick.sources')}</h2>
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

      </div>
      </section>

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

      {!review.comparison && review.selectedObject && (
        <ObjectDetail object={review.selectedObject} />
      )}
      <section className="review-disclosure quick-policy">
        <button type="button" className="button button--quiet" aria-expanded={policyOpen} aria-controls="quick-policy-panel" onClick={() => setPolicyOpen(!policyOpen)}>{t('quick.obligations.title')}</button>
        <div id="quick-policy-panel" ref={policyRef} tabIndex={-1} hidden={!policyOpen}>
        <QuickObligations review={review} />
        </div>
      </section>
      {showRepair && (
        <section className="quick-repair" aria-labelledby="quick-repair-title">
          <SectionHeading id="quick-repair-title" title={t('quick.repair.title')} />
          <RepairStep review={review} embedded />
        </section>
      )}
      <QuickEvidence review={review} />
      {review.analysis && <section className="review-disclosure quick-questions">
        <button type="button" className="button button--quiet" aria-expanded={questionsOpen} aria-controls="quick-questions-panel" onClick={() => setQuestionsOpen(!questionsOpen)}>{t('chat.title')}</button>
        <div id="quick-questions-panel" hidden={!questionsOpen}>
        <ChatPanel comparison={review.comparison?.comparison ?? null} analysis={review.analysis?.result ?? null} headingSrOnly />
        </div>
      </section>}
    </div>
  );
}
