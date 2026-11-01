import { useEffect, useState } from 'react';
import { useI18n } from '../i18n';
import type { MessageKey } from '../i18n/messages.en';
import type { Review } from '../hooks/useReview';
import {
  obligationSentence,
  regionSentence,
} from '../lib/conditions';
import { aiStatusLabel, outcomeLabel, scopeStatusLabel } from '../lib/labels';
import { formatTimestamp, shortHash } from '../lib/format';
import type { AIResult, AIStatus, ObligationStatus } from '../lib/types';
import { Icon } from './Icon';
import { ConditionEditorList, ScopeEditor, draftCanAccept } from './ConditionEditor';
import {
  ActionBar,
  BusyPanel,
  HashLine,
  ProvenanceTag,
  SectionHeading,
  StatusMark,
  StepHeader,
  TechnicalDetails,
  TermNote,
} from './Ui';
import type { Tone } from './Ui';

function aiTone(status: AIStatus): Tone {
  switch (status) {
    case 'completed': return 'info';
    case 'not_configured':
    case 'unavailable':
    case 'partial':
    case 'clarification_needed': return 'warn';
    default: return 'neutral';
  }
}

function aiNoticeKey(status: AIStatus): MessageKey | null {
  switch (status) {
    case 'not_configured': return 'cond.ai.notConfigured';
    case 'unavailable': return 'cond.ai.unavailable';
    case 'clarification_needed': return 'cond.ai.clarification';
    case 'partial': return 'cond.ai.partial';
    case 'completed': return 'cond.ai.completed';
    case 'canceled': return 'cond.ai.canceled';
    default: return null;
  }
}

function asOutcome(status: string): ObligationStatus {
  return status === 'HOLDS' || status === 'VIOLATED' || status === 'INCONCLUSIVE' ? status : 'INCONCLUSIVE';
}

function AiConsentForm({ review }: { review: Review }) {
  const { t } = useI18n();
  const [consent, setConsent] = useState(false);
  const busy = review.busy !== null;
  return (
    <div className="ai-consent">
      <p className="ai-data">{t('cond.ai.data')}</p>
      <div className="ai-consent__controls">
        <label className="checkbox-row">
          <input type="checkbox" checked={consent} disabled={busy} onChange={(event) => setConsent(event.target.checked)} />
          <span>{t('cond.ai.consent')}</span>
        </label>
        <button
          type="button"
          className="button button--secondary button--small"
          disabled={!consent || busy || !review.parsed?.intent.trim()}
          onClick={review.proposeConditions}
        >
          <Icon name="info" size={16} />
          {t('cond.ai.send')}
        </button>
      </div>
      <p className="ai-model">{t('cond.ai.model')}</p>
    </div>
  );
}

function AiResultPanel({ ai, review }: { ai: AIResult; review: Review }) {
  const { t } = useI18n();
  const proposed = ai.obligations?.set.obligations ?? [];
  const notice = aiNoticeKey(ai.status);
  return (
    <div className="ai-result">
      <div className="ai-result__head">
        <ProvenanceTag tone={aiTone(ai.status)} icon="info">AI</ProvenanceTag>
        <StatusMark tone={aiTone(ai.status)} label={aiStatusLabel(t, ai.status)} />
      </div>
      <p className="ai-model">{t('ai.model')}: <code>{ai.provenance.model}</code></p>
      {notice && <p className={`notice-line notice-line--${aiTone(ai.status)}`}>{t(notice)}</p>}
      {ai.questions && ai.questions.length > 0 && (
        <div className="ai-questions">
          <h3>{t('cond.ai.questions')}</h3>
          <ul>{ai.questions.map((question, index) => <li key={index}>{question}</li>)}</ul>
        </div>
      )}
      {proposed.length > 0 ? (
        <div className="ai-proposal">
          <div className="ai-proposal__head">
            <h3>{t('cond.ai.proposed')}</h3>
            <span className="field-label">{t('ai.proposalOnly')}</span>
          </div>
          <ol className="ai-proposal__list">
            {proposed.map((obligation) => (
              <li key={obligation.id}>
                <code>{obligation.id}</code>
                <span>{obligationSentence(t, obligation)}</span>
              </li>
            ))}
          </ol>
          <button
            type="button"
            className="button button--secondary button--small"
            disabled={review.proposalAdopted}
            onClick={review.adoptProposal}
          >
            <Icon name="plus" size={15} />
            {review.proposalAdopted ? t('cond.ai.adopted') : t('cond.ai.adopt')}
          </button>
        </div>
      ) : ai.status === 'completed' || ai.status === 'partial' ? (
        <p className="notice-line notice-line--neutral">{t('ai.noObligations')}</p>
      ) : null}
      {ai.summary && (
        <blockquote className="ai-text">
          <p className="ai-text__label">{t('cond.ai.summary')}</p>
          <p>{ai.summary}</p>
        </blockquote>
      )}
      {ai.claims && ai.claims.length > 0 && (
        <div className="ai-claims">
          <h3>{t('cond.ai.claims')}</h3>
          <ul>
            {ai.claims.map((claim, index) => (
              <li key={index}>
                <p>{claim.text}</p>
                <small>{t('cond.ai.evidenceRefs', { ids: claim.evidence_ids.join(', ') })}</small>
              </li>
            ))}
          </ul>
          <p className="field-hint">{t('cond.ai.notEvidence')}</p>
        </div>
      )}
      <TechnicalDetails title={t('ai.artifact')}>
        <p>{t('ai.notVerdict')}</p>
        <dl className="meta-list">
          <div><dt>{t('ai.provider')}</dt><dd><code>{ai.provenance.provider}</code></dd></div>
          <div><dt>{t('ai.model')}</dt><dd><code>{ai.provenance.model}</code></dd></div>
          <div><dt>{t('ai.promptVersion')}</dt><dd><code>{ai.provenance.prompt_version}</code></dd></div>
          <div><dt>{t('ai.requests')}</dt><dd>{ai.provenance.requests}</dd></div>
          <div><dt>{t('ai.toolCalls')}</dt><dd>{ai.provenance.tool_calls}</dd></div>
          <div><dt>{t('ai.elapsed')}</dt><dd>{ai.provenance.elapsed_ms} ms</dd></div>
          <div><dt>{t('common.status')}</dt><dd><code>{ai.status}</code></dd></div>
          {ai.reason && <div><dt>{t('ai.status.reason')}</dt><dd><code>{ai.reason}</code></dd></div>}
        </dl>
        {ai.checks && ai.checks.length > 0 && (
          <div className="ai-checks">
            <h3>{t('cond.ai.checks')}</h3>
            <p className="field-hint">{t('cond.ai.checksHelp')}</p>
            <ul>
              {ai.checks.map((check) => {
                const outcome = asOutcome(check.status);
                return (
                  <li key={check.id}>
                    <code>{check.id}</code>
                    <StatusMark tone={outcome === 'HOLDS' ? 'good' : outcome === 'VIOLATED' ? 'bad' : 'warn'} label={outcomeLabel(t, outcome)} />
                  </li>
                );
              })}
            </ul>
          </div>
        )}
      </TechnicalDetails>
    </div>
  );
}

export function ConditionsStep({ review }: { review: Review }) {
  const { t, locale } = useI18n();
  const [confirmed, setConfirmed] = useState(false);
  const draft = review.draft;
  const signature = JSON.stringify({
    conditions: draft.conditions,
    scopeMode: draft.scopeMode,
    allowedChanges: draft.allowedChanges,
  });
  useEffect(() => {
    setConfirmed(false);
  }, [signature, review.policyPreview]);

  if (!review.parsed) return null;
  const parsed = review.parsed;
  const canAccept = draftCanAccept(draft);
  const busy = review.busy !== null;
  const accepted = review.accepted;
  const ai = review.aiProposal?.session.ai ?? null;

  return (
    <div className="step">
      <StepHeader kicker={t('cond.kicker')} title={t('cond.title')} lede={t('cond.lede')}>
        <ProvenanceTag icon="shield">{t('cond.parse.runtime')}</ProvenanceTag>
      </StepHeader>

      <section className="parse-receipt" aria-label={t('cond.parse.runtime')}>
        <div>
          <span>{t('cond.parse.before')}</span>
          <strong>{parsed.parsed.before.acl.name}</strong>
          <small>{t('cond.parse.rules', { count: parsed.parsed.before.acl.rules.length })} · {shortHash(parsed.parsed.before.sha256)}</small>
        </div>
        <Icon name="arrow" size={18} />
        <div>
          <span>{t('cond.parse.after')}</span>
          <strong>{parsed.parsed.after.acl.name}</strong>
          <small>{t('cond.parse.rules', { count: parsed.parsed.after.acl.rules.length })} · {shortHash(parsed.parsed.after.sha256)}</small>
        </div>
      </section>

      <section className="intent-block">
        <label htmlFor="review-intent">{t('source.intent')}</label>
        <textarea id="review-intent" rows={3} className="intent-block__input" value={draft.intent} disabled={busy}
          placeholder={t('source.intentPlaceholder')} onChange={(event) => review.updateIntent(event.target.value)} />
        <p className="field-hint">{t('source.intentHelp')}</p>
      </section>

      <section className="ai-band" aria-labelledby="ai-band-title">
        <div className="ai-band__head">
          <div>
            <p className="kicker">AI</p>
            <h2 id="ai-band-title">{t('cond.ai.title')}</h2>
            <p>{t('cond.ai.lede')}</p>
          </div>
          <ProvenanceTag tone="info" icon="info">{t('term.ai')}</ProvenanceTag>
        </div>
        {review.busy === 'aiProposal' && (
          <BusyPanel label={t('cond.ai.busy')} onCancel={review.cancel} />
        )}
        {review.canceled === 'aiProposal' && (
          <p className="notice-line notice-line--neutral">{t('cond.ai.canceled')}</p>
        )}
        {ai && <AiResultPanel ai={ai} review={review} />}
        {review.busy !== 'aiProposal' && (
          ai ? (
            <TechnicalDetails title={t('cond.ai.askAgain')}>
              <AiConsentForm review={review} />
            </TechnicalDetails>
          ) : (
            <AiConsentForm review={review} />
          )
        )}
      </section>

      <section className="conditions-section" aria-labelledby="conditions-heading">
        <SectionHeading
          id="conditions-heading"
          title={t('cond.list.title')}
          note={t('cond.list.help')}
          aside={<ProvenanceTag>{t('cond.count', { count: draft.conditions.length })}</ProvenanceTag>}
        />
        <TermNote>{t('cond.term')}</TermNote>
        <ConditionEditorList review={review} />
      </section>

      <section className="scope-section" aria-labelledby="scope-heading">
        <SectionHeading
          id="scope-heading"
          title={t('scope.title')}
          note={t('scope.lede')}
          aside={<ProvenanceTag>{scopeStatusLabel(t, draft.scopeMode, draft.allowedChanges.length)}</ProvenanceTag>}
        />
        <TermNote>{t('scope.term')}</TermNote>
        <ScopeEditor review={review} />
      </section>

      <section className="accept-section" aria-labelledby="accept-heading">
        <SectionHeading id="accept-heading" title={t('accept.title')} note={t('accept.help')} />
        {!accepted && (
          <div className="canonical-preview">
            <p>{t('accept.previewHelp')}</p>
            <button type="button" className="button button--secondary" disabled={!canAccept || busy} onClick={review.previewPolicy}>
              {t('accept.previewAction')}
            </button>
            {review.busy === 'previewPolicy' && <BusyPanel label={t('accept.previewBusy')} onCancel={review.cancel} />}
            {review.policyPreview && (
              <div className="canonical-preview__readback" role="region" aria-label={t('accept.previewTitle')}>
                <h3>{t('accept.previewTitle')}</h3>
                <ol>{review.policyPreview.normalized.policy.conditions.obligations.map((obligation) => (
                  <li key={obligation.id}><code>{obligation.id}</code> {obligationSentence(t, obligation)}</li>
                ))}</ol>
                <p>{scopeStatusLabel(t, draft.scopeMode, review.policyPreview.normalized.policy.allowed_changes?.length ?? 0)}</p>
                {review.policyPreview.normalized.policy.allowed_changes?.map((region, index) => <p key={index}>{regionSentence(t, region)}</p>)}
                <HashLine label={t('accept.previewDigest')} value={review.policyPreview.normalized.policy_sha256} />
              </div>
            )}
          </div>
        )}
        {accepted ? (
          <div className="accepted-stamp">
            <StatusMark tone="good" label={t('ai.acceptedByOperator')} />
            <p>{t('accept.acceptedAt', { time: formatTimestamp(locale, new Date(accepted.acceptedAt)) })}</p>
            <HashLine label={t('accept.digest')} value={accepted.digest} />
          </div>
        ) : (
          <label className="checkbox-row accept-checkbox">
            <input
              type="checkbox"
              checked={confirmed}
              disabled={busy || !review.policyPreview}
              onChange={(event) => setConfirmed(event.target.checked)}
            />
            <span>{t('accept.checkbox')}</span>
          </label>
        )}
        {review.canceled === 'compare' && (
          <p className="notice-line notice-line--neutral">{t('common.canceled')}</p>
        )}
        {review.busy === 'compare' ? (
          <BusyPanel label={t('accept.busy')} onCancel={review.cancel} />
        ) : (
          <ActionBar note={t('accept.note')}>
            <button type="button" className="button button--secondary" disabled={busy} onClick={() => review.navigate('source')}>
              {t('accept.back')}
            </button>
            {accepted ? (
              review.comparison ? (
                <button type="button" className="button button--primary" onClick={() => review.navigate('result')}>
                  <Icon name="arrow" size={17} />
                  {t('accept.viewResult')}
                </button>
              ) : (
                <button type="button" className="button button--primary" onClick={review.retryCompare}>
                  <Icon name="replay" size={17} />
                  {t('accept.retry')}
                </button>
              )
            ) : (
              <button
                type="button"
                className="button button--primary"
                disabled={!canAccept || !confirmed || busy || !review.policyPreview}
                onClick={() => review.acceptAndCompare(confirmed)}
              >
                <Icon name="arrow" size={17} />
                {t('accept.action')}
              </button>
            )}
          </ActionBar>
        )}
      </section>
    </div>
  );
}
