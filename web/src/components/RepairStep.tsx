import { useRef, useState } from 'react';
import { useI18n } from '../i18n';
import type { Review } from '../hooks/useReview';
import { actionLabel, kindLabel, sourceLine, sourceLines } from '../lib/conditions';
import { downloadText } from '../lib/download';
import { fileStamp, prettyJson, shortHash } from '../lib/format';
import {
  aiStatusLabel,
  collateralStatusLabel,
  outcomeLabel,
  repairReasonLabel,
  repairStatusLabel,
  replayLabel,
  scopeStatusLabel,
} from '../lib/labels';
import { changedLines } from '../lib/lineDiff';
import type { AIStatus, CollateralStatus, RepairResult, RepairStatus } from '../lib/types';
import { Icon } from './Icon';
import { WitnessDetail, SourceLines } from './WitnessDetail';
import {
  ActionBar,
  BusyPanel,
  CopyButton,
  HashLine,
  ProvenanceTag,
  SectionHeading,
  StatusMark,
  StepHeader,
  TechnicalDetails,
} from './Ui';
import type { Tone } from './Ui';

function repairTone(status: RepairStatus): Tone {
  return status === 'VERIFIED' ? 'good' : status === 'REJECTED' ? 'bad' : 'warn';
}

function collateralTone(status: CollateralStatus): Tone {
  return status === 'VERIFIED' ? 'good' : status === 'FAILED' ? 'bad' : 'warn';
}

function aiTone(status: AIStatus): Tone {
  return status === 'completed' ? 'info' : status === 'not_configured' || status === 'unavailable' || status === 'partial' ? 'warn' : 'neutral';
}

function unsequenced(text: string): string {
  const trimmed = text.trim();
  const fields = trimmed.split(/\s+/);
  if (fields.length > 1 && /^\d+$/.test(fields[0])) return fields.slice(1).join(' ');
  return trimmed;
}

function candidateMode(receipt: RepairResult): 'undeclared' | 'none' | 'regions' {
  if (!receipt.policy) return 'undeclared';
  if (receipt.policy.allowed_changes === null) return 'undeclared';
  return receipt.policy.allowed_changes.length === 0 ? 'none' : 'regions';
}

function RepairAiConsent({ review }: { review: Review }) {
  const { t } = useI18n();
  const [consent, setConsent] = useState(false);
  const busy = review.busy !== null;
  return (
    <div className="ai-consent">
      <p className="ai-data">{t('repair.ai.data')}</p>
      <div className="ai-consent__controls">
        <label className="checkbox-row">
          <input type="checkbox" checked={consent} disabled={busy} onChange={(event) => setConsent(event.target.checked)} />
          <span>{t('cond.ai.consent')}</span>
        </label>
        <button type="button" className="button button--secondary button--small" disabled={!consent || busy || !review.accepted?.intent.trim()} onClick={review.proposeRepair}>
          <Icon name="info" size={16} />
          {t('repair.ai.send')}
        </button>
      </div>
      <p className="ai-model">{t('cond.ai.model')}</p>
    </div>
  );
}

// Embedded mode renders the same repair workspace without the step chrome
// (header and stage navigation) for the quick verdict screen, where repair
// is a contextual section rather than a stage. The verify action stays.
export function RepairStep({ review, embedded = false }: { review: Review; embedded?: boolean }) {
  const { t } = useI18n();
  const accepted = review.accepted;
  const comparison = review.comparison;
  const aiSession = review.aiSession;
  const receipt = review.repair;
  const [pending, setPending] = useState<{ line: number; ordinal: number; op: 'replace' | 'add_before'; content: string } | null>(null);
  const [appendContent, setAppendContent] = useState('');
  const [formError, setFormError] = useState<string | null>(null);
  const builderRef = useRef<HTMLDivElement>(null);
  if (!accepted || !comparison) return null;
  const busy = review.busy !== null;
  const rules = accepted.parsed.after.acl.rules;
  const ai = aiSession?.session.ai ?? null;
  const aiRepairs = aiSession?.session.repairs ?? [];
  const manualEdits = new Set(review.edits.map((edit) => edit.edit.ref?.line).filter((line): line is number => line !== undefined));
  const scopeMode = accepted.scopeMode;

  function startEdit(line: number, ordinal: number, op: 'replace' | 'add_before') {
    const text = sourceLine(accepted!.after, line) ?? '';
    setPending({ line, ordinal, op, content: op === 'replace' ? unsequenced(text) : '' });
    setFormError(null);
    builderRef.current?.scrollIntoView({ block: 'nearest' });
  }

  function commitPending() {
    if (!pending) return;
    if (pending.content.trim() === '') {
      setFormError(t('repair.manual.contentRequired'));
      return;
    }
    const ok = review.addEdit({
      op: pending.op,
      ref: { base_after_sha256: accepted!.parsed.after.sha256, ordinal: pending.ordinal, line: pending.line },
      content: pending.content.trim(),
    });
    if (!ok) {
      setFormError(t('repair.manual.addFailed'));
      return;
    }
    setPending(null);
    setFormError(null);
  }

  function addRemove(line: number, ordinal: number) {
    const ok = review.addEdit({
      op: 'remove',
      ref: { base_after_sha256: accepted!.parsed.after.sha256, ordinal, line },
    });
    setFormError(ok ? null : t('repair.manual.addFailed'));
  }

  function commitAppend() {
    if (appendContent.trim() === '') {
      setFormError(t('repair.manual.contentRequired'));
      return;
    }
    const ok = review.addEdit({ op: 'append', content: appendContent.trim() });
    if (!ok) {
      setFormError(t('repair.manual.addFailed'));
      return;
    }
    setAppendContent('');
    setFormError(null);
  }

  function editLabel(edit: { op: string; ref?: { line: number }; content?: string }): string {
    const line = edit.ref?.line ?? 0;
    switch (edit.op) {
      case 'remove': return t('repair.manual.edit.remove', { line });
      case 'replace': return t('repair.manual.edit.replace', { line });
      case 'add_before': return t('repair.manual.edit.addBefore', { line });
      default: return t('repair.manual.edit.append', { content: edit.content ?? '' });
    }
  }

  const aiNotice = ai
    ? ai.status === 'not_configured' ? t('repair.ai.notConfigured')
      : ai.status === 'unavailable' ? t('repair.ai.unavailable')
        : ai.status === 'partial' ? t('repair.ai.partial')
          : ai.status === 'canceled' ? t('repair.ai.canceled')
            : ai.status === 'completed' && aiRepairs.length === 0 ? t('repair.ai.noRepair')
              : null
    : null;

  const candidateLines = receipt?.receipt.candidate ? sourceLines(receipt.receipt.candidate) : [];
  const collateralChanged = receipt?.receipt.candidate
    ? changedLines(sourceLines(accepted.before), sourceLines(receipt.receipt.candidate))
    : { before: new Set<number>(), after: new Set<number>() };
  const rawIsSession = receipt?.source === 'ai';
  const rawValue = receipt?.raw ?? '';
  const evidenceReady = Boolean(receipt);

  return (
    <div className={embedded ? 'repair-embedded' : 'step'}>
      {!embedded && (
        <StepHeader kicker={t('repair.kicker')} title={t('repair.title')} lede={t('repair.lede')}>
          <ProvenanceTag icon="lock">{t('repair.applyNone')}</ProvenanceTag>
        </StepHeader>
      )}

      {scopeMode === 'undeclared'  && <p className="banner banner--warn">{t('repair.scope.undeclaredWarn')}</p>}
      {scopeMode === 'none' && <p className="banner banner--neutral">{t('repair.scope.noneNote')}</p>}
      {scopeMode === 'regions' && <p className="banner banner--neutral">{t('repair.scope.regionsNote', { count: accepted.allowedChanges.length })}</p>}

      <section className="ai-band" aria-labelledby="repair-ai-title">
        <div className="ai-band__head">
          <div>
            <p className="kicker">AI</p>
            <h2 id="repair-ai-title">{t('repair.ai.title')}</h2>
            <p>{t('repair.ai.lede')}</p>
          </div>
          <ProvenanceTag tone="info" icon="info">{t('term.ai')}</ProvenanceTag>
        </div>
        {review.busy === 'aiRepair' && <BusyPanel label={t('repair.ai.busy')} onCancel={review.cancel} />}
        {review.canceled === 'aiRepair' && <p className="banner banner--neutral">{t('repair.ai.canceled')}</p>}
        {ai && (
          <div className="ai-result">
            <div className="ai-result__head">
              <ProvenanceTag tone={aiTone(ai.status)} icon="info">AI</ProvenanceTag>
              <StatusMark tone={aiTone(ai.status)} label={aiStatusLabel(t, ai.status)} />
            </div>
            <p className="ai-model">{t('ai.model')}: <code>{ai.provenance.model}</code></p>
            {aiNotice && <p className={`banner banner--${aiTone(ai.status)}`}>{aiNotice}</p>}
            {aiRepairs.length > 0 && (
              <div className="candidate-switch" role="group" aria-label={t('repair.ai.candidates')}>
                <span className="field-label">{t('repair.ai.candidates')}</span>
                <div className="candidate-switch__buttons">
                  {aiRepairs.map((candidate, index) => {
                    const selected = review.repairSource === 'ai' && review.selectedAiIndex === index;
                    return (
                      <button
                        type="button"
                        key={candidate.sha256 ?? index}
                        className={`candidate-button ${selected ? 'candidate-button--selected' : ''}`}
                        aria-pressed={selected}
                        onClick={() => review.selectAiRepair(index)}
                      >
                        <span>{t('repair.oneCandidate', { index: index + 1 })}</span>
                        <StatusMark tone={repairTone(candidate.status)} label={repairStatusLabel(t, candidate.status)} />
                        <code>{candidate.sha256 ? shortHash(candidate.sha256) : shortHash(candidate.candidate_sha256 ?? '')}</code>
                      </button>
                    );
                  })}
                </div>
              </div>
            )}
            {ai.repair?.rationale && (
              <blockquote className="ai-text">
                <p className="ai-text__label">{t('repair.ai.rationale')}</p>
                <p>{ai.repair.rationale}</p>
              </blockquote>
            )}
            {ai.summary && (
              <blockquote className="ai-text">
                <p className="ai-text__label">{t('cond.ai.summary')}</p>
                <p>{ai.summary}</p>
              </blockquote>
            )}
            <TechnicalDetails title={t('ai.artifact')}>
              <p>{t('ai.notVerdict')}</p>
              <dl className="meta-list">
                <div><dt>{t('ai.provider')}</dt><dd><code>{ai.provenance.provider}</code></dd></div>
                <div><dt>{t('ai.model')}</dt><dd><code>{ai.provenance.model}</code></dd></div>
                <div><dt>{t('ai.requests')}</dt><dd>{ai.provenance.requests}</dd></div>
                <div><dt>{t('ai.toolCalls')}</dt><dd>{ai.provenance.tool_calls}</dd></div>
                <div><dt>{t('ai.elapsed')}</dt><dd>{ai.provenance.elapsed_ms} ms</dd></div>
                <div><dt>{t('common.status')}</dt><dd><code>{ai.status}</code></dd></div>
              </dl>
            </TechnicalDetails>
          </div>
        )}
        {review.busy !== 'aiRepair' && (
          ai ? (
            <TechnicalDetails title={t('cond.ai.askAgain')}>
              <RepairAiConsent review={review} />
            </TechnicalDetails>
          ) : (
            <RepairAiConsent review={review} />
          )
        )}
      </section>

      <section className="manual-repair" aria-labelledby="manual-repair-title" ref={builderRef}>
        <SectionHeading
          id="manual-repair-title"
          title={t('repair.manual.title')}
          note={t('repair.manual.lede')}
          aside={<ProvenanceTag>{t('repair.manual.editCount', { count: review.edits.length })}</ProvenanceTag>}
        />
        {rules.length === 0 ? (
          <p className="banner banner--neutral">{t('repair.manual.emptyRules')}</p>
        ) : (
          <div className="rule-builder" role="group" aria-label={t('repair.manual.afterRules')}>
            {rules.map((rule) => {
              const text = sourceLine(accepted.after, rule.line) ?? '';
              const inDraft = manualEdits.has(rule.line);
              return (
                <div className={`rule-builder__row ${inDraft ? 'rule-builder__row--in-draft' : ''}`} key={`${rule.ordinal}-${rule.line}`}>
                  <div className="rule-builder__line">
                    <span className="source-pane__number">{rule.line}</span>
                    <span className="rule-builder__action">
                      <code>{rule.action}</code>
                      <span>{actionLabel(t, rule.action)}</span>
                    </span>
                    <code className="rule-builder__text">{text}</code>
                    {inDraft && <span className="badge badge--info">{t('repair.manual.inDraft')}</span>}
                  </div>
                  <div className="rule-builder__actions">
                    <button type="button" className="button button--quiet button--small button--danger" disabled={busy || inDraft || review.edits.length >= 5} onClick={() => addRemove(rule.line, rule.ordinal)}>
                      <Icon name="trash" size={15} /> {t('repair.manual.op.remove')}
                    </button>
                    <button type="button" className="button button--quiet button--small" disabled={busy || inDraft || review.edits.length >= 5} onClick={() => startEdit(rule.line, rule.ordinal, 'replace')}>
                      <Icon name="repair" size={15} /> {t('repair.manual.op.replace')}
                    </button>
                    <button type="button" className="button button--quiet button--small" disabled={busy || inDraft || review.edits.length >= 5} onClick={() => startEdit(rule.line, rule.ordinal, 'add_before')}>
                      <Icon name="plus" size={15} /> {t('repair.manual.op.addBefore')}
                    </button>
                  </div>
                  {pending?.line === rule.line && (
                    <div className="rule-builder__form">
                      <label htmlFor={`edit-${rule.line}`}>{t('repair.manual.content')}</label>
                      <div className="rule-builder__form-row">
                        <input
                          id={`edit-${rule.line}`}
                          value={pending.content}
                          disabled={busy}
                          spellCheck={false}
                          placeholder={t('repair.manual.contentPlaceholder')}
                          onChange={(event) => setPending({ ...pending, content: event.target.value })}
                        />
                        <button type="button" className="button button--secondary button--small" disabled={busy} onClick={commitPending}>{t('repair.manual.addEdit')}</button>
                        <button type="button" className="button button--quiet button--small" onClick={() => { setPending(null); setFormError(null); }}>{t('repair.manual.cancelEdit')}</button>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}

        <div className="append-block">
          <h3>{t('repair.manual.appendTitle')}</h3>
          <p className="field-hint">{t('repair.manual.appendHelp')}</p>
          <div className="rule-builder__form-row">
            <label className="sr-only" htmlFor="append-rule">{t('repair.manual.content')}</label>
            <input
              id="append-rule"
              value={appendContent}
              disabled={busy || review.edits.length >= 5}
              spellCheck={false}
              placeholder={t('repair.manual.contentPlaceholder')}
              onChange={(event) => setAppendContent(event.target.value)}
            />
            <button type="button" className="button button--secondary button--small" disabled={busy || review.edits.length >= 5} onClick={commitAppend}>
              <Icon name="plus" size={15} /> {t('repair.manual.addEdit')}
            </button>
          </div>
        </div>

        <div className="edit-draft">
          <h3>{t('repair.manual.edits')}</h3>
          {review.edits.length === 0 ? (
            <p className="field-hint">{t('repair.manual.noEdits')}</p>
          ) : (
            <ol className="edit-draft__list">
              {review.edits.map((edit) => (
                <li key={edit.id}>
                  <span>{editLabel(edit.edit)}</span>
                  {edit.edit.op !== 'remove' && edit.edit.content && <code>{edit.edit.content}</code>}
                  {edit.edit.op === 'remove' && <code>{sourceLine(accepted.after, edit.edit.ref?.line) ?? ''}</code>}
                  <button type="button" className="button button--quiet button--small button--danger" aria-label={t('repair.manual.removeEdit')} disabled={busy} onClick={() => review.removeEdit(edit.id)}>
                    <Icon name="trash" size={15} />
                  </button>
                </li>
              ))}
            </ol>
          )}
          {formError && <p className="field-error" role="alert">{formError}</p>}
        </div>

        {review.canceled === 'repair' && <p className="banner banner--neutral">{t('common.canceled')}</p>}
      </section>

      {receipt && (
        <section className="receipt" aria-labelledby="receipt-title">
          <SectionHeading
            id="receipt-title"
            title={t('repair.result.title')}
            aside={<StatusMark tone={repairTone(receipt.receipt.status)} label={repairStatusLabel(t, receipt.receipt.status)} />}
          />
          <div className="receipt__source">
            <ProvenanceTag tone={receipt.source === 'ai' ? 'info' : 'neutral'} icon={receipt.source === 'ai' ? 'info' : 'repair'}>
              {receipt.source === 'ai' ? t('repair.actor.ai') : t('repair.actor.manual')}
            </ProvenanceTag>
            {receipt.source === 'ai' && review.manualRepair && (
              <button type="button" className="button button--quiet button--small" onClick={review.selectManualRepair}>{t('repair.manual.receipt')}</button>
            )}
            {receipt.source === 'manual' && aiRepairs.length > 0 && (
              <button type="button" className="button button--quiet button--small" onClick={() => review.selectAiRepair(review.selectedAiIndex)}>{t('repair.ai.candidates')}</button>
            )}
          </div>
          {receipt.receipt.status !== 'VERIFIED' && (
            <p className="banner banner--warn">{repairReasonLabel(t, receipt.receipt.reason)}</p>
          )}
          <p className="field-hint">{receipt.receipt.status === 'VERIFIED' ? t('repair.verifiedHint') : t('repair.notVerifiedHint')}</p>

          <div className="receipt-grid">
            <div className="receipt-block">
              <p className="field-hint">{t('repair.candidateHelp')}</p>
              {receipt.receipt.candidate ? (
                <SourceLines title={t('repair.candidate')} lines={candidateLines} changed={new Set<number>()} />
              ) : (
                <p className="banner banner--neutral">{t('common.noData')}</p>
              )}
              <p className="field-hint">{t('repair.applyNone')}</p>
            </div>

            <div className="receipt-block">
              <h3>{t('repair.obligations')}</h3>
              {receipt.receipt.comparison ? (
                <ul className="receipt-obligations">
                  {receipt.receipt.comparison.report.obligations.map((outcome) => {
                    const definition = receipt.receipt.conditions.obligations.find((item) => item.id === outcome.id);
                    return (
                      <li key={outcome.id}>
                        <span><code>{outcome.id}</code>{definition && <span>{kindLabel(t, definition.kind)}</span>}</span>
                        <StatusMark
                          tone={outcome.status === 'HOLDS' ? 'good' : outcome.status === 'VIOLATED' ? 'bad' : 'warn'}
                          label={outcomeLabel(t, outcome.status)}
                        />
                      </li>
                    );
                  })}
                </ul>
              ) : (
                <p className="banner banner--neutral">{t('repair.comparisonMissing')}</p>
              )}
            </div>

            <div className="receipt-block">
              <h3>{t('repair.collateral')}</h3>
              <div className="receipt-collateral__head">
                <StatusMark tone={collateralTone(receipt.receipt.collateral.status)} label={collateralStatusLabel(t, receipt.receipt.collateral.status)} />
                <span className="field-label">{scopeStatusLabel(t, candidateMode(receipt.receipt), receipt.receipt.policy?.allowed_changes?.length ?? 0)}</span>
              </div>
              <p>{t(`repair.collateral.${collateralMode(receipt.receipt.collateral.status)}`)}</p>
              {receipt.receipt.collateral.reason && <p className="field-hint">{t('common.reason')}: <code>{receipt.receipt.collateral.reason}</code></p>}
            </div>
          </div>

          {receipt.receipt.collateral.witness && (
            <div className="collateral-witness">
              <WitnessDetail
                witness={receipt.receipt.collateral.witness}
                title={t('repair.collateralWitness')}
                before={accepted.before}
                after={receipt.receipt.candidate ?? accepted.after}
                changedBefore={collateralChanged.before}
                changedAfter={collateralChanged.after}
              />
            </div>
          )}

          <div className="receipt-policy">
            <h3>{t('repair.policy')}</h3>
            {receipt.receipt.policy ? (
              <>
                <p className="field-hint">{receipt.receipt.policy.operator_accepted ? t('repair.policyAccepted') : t('repair.reason.policy_not_accepted')}</p>
                <dl className="meta-list">
                  <div><dt>{t('scope.title')}</dt><dd>{scopeStatusLabel(t, candidateMode(receipt.receipt), receipt.receipt.policy.allowed_changes?.length ?? 0)}</dd></div>
                  <div><dt>{t('accept.digest')}</dt><dd><code>{receipt.receipt.obligations_digest ?? t('common.notAvailable')}</code></dd></div>
                </dl>
                {receipt.receipt.policy_sha256 && <HashLine label={t('repair.policyHash')} value={receipt.receipt.policy_sha256} />}
              </>
            ) : (
              <p className="banner banner--warn">{t('repair.reason.policy_missing')}</p>
            )}
          </div>

          <div className="replay-block">
            <h3>{t('repair.replay')}</h3>
            <p className="field-hint">{t('repair.replayHelp')}</p>
            {review.repairReplay ? (
              <div className="replay-result">
                <StatusMark tone={review.repairReplay.result.status === 'TIMING_SENSITIVE' ? 'warn' : 'good'} label={replayLabel(t, review.repairReplay.result.status)} />
                <span>{t('repair.replayCount', { count: review.repairReplay.result.rechecked_witnesses })}</span>
                {review.repairReplay.result.status === 'TIMING_SENSITIVE' && <span>{t('repair.replayStatus.TIMING_SENSITIVE')}</span>}
              </div>
            ) : (
              <p className="field-hint">{t('report.noReplay')}</p>
            )}
            {review.canceled === 'replayRepair' && <p className="banner banner--neutral">{t('common.canceled')}</p>}
            {review.busy === 'replayRepair' ? (
              <BusyPanel label={t('repair.replayBusy')} onCancel={review.cancel} />
            ) : (
              <button type="button" className="button button--secondary button--small" onClick={review.replayRepair} disabled={busy || !receipt}>
                <Icon name="replay" size={15} /> {t('repair.primaryReplay')}
              </button>
            )}
          </div>

          <TechnicalDetails title={t('repair.details')}>
            <dl className="meta-list">
              <div><dt>{t('common.status')}</dt><dd><code>{receipt.receipt.status}</code>{receipt.receipt.reason ? <> · <code>{receipt.receipt.reason}</code></> : null}</dd></div>
              <div><dt>{t('repair.candidateHash')}</dt><dd><code>{receipt.receipt.candidate_sha256 ?? t('common.notAvailable')}</code></dd></div>
              <div><dt>{t('repair.receiptHash')}</dt><dd><code>{receipt.receipt.sha256 ?? t('common.notAvailable')}</code></dd></div>
              <div><dt>{t('repair.conditionsDigest')}</dt><dd><code>{receipt.receipt.obligations_digest ?? t('common.notAvailable')}</code></dd></div>
              <div><dt>{t('repair.collateral')}</dt><dd><code>{receipt.receipt.collateral.status}</code> · {t('result.operations', { count: receipt.receipt.collateral.stats.operations })}</dd></div>
              <div><dt>{t('result.stopReason')}</dt><dd>{receipt.receipt.collateral.stats.stop_reason ? <code>{receipt.receipt.collateral.stats.stop_reason}</code> : t('common.none')}</dd></div>
            </dl>
            <HashLine label={t('result.inputHash', { side: t('common.before') })} value={receipt.receipt.before_sha256} />
            <HashLine label={t('result.inputHash', { side: t('common.after') })} value={receipt.receipt.base_after_sha256} />
            <div className="raw-actions">
              <CopyButton value={rawValue} label={t('common.copy')} />
              <button
                type="button"
                className="button button--quiet button--small"
                onClick={() => downloadText(
                  rawIsSession ? `plenilune-session-${fileStamp()}.json` : `plenilune-repair-${fileStamp()}.json`,
                  rawValue,
                  'application/json',
                )}
              >
                <Icon name="download" size={15} />
                {rawIsSession ? t('repair.sessionRaw') : t('repair.rawJson')}
              </button>
            </div>
            <p className="field-hint">{t('common.rawDisplay')}</p>
            <pre className="raw-block" tabIndex={0} role="region" aria-label={rawIsSession ? t('repair.sessionRaw') : t('repair.rawJson')}>{prettyJson(rawValue)}</pre>
          </TechnicalDetails>
        </section>
      )}

      {receipt && (
        <section className="accept-section" aria-label={t('decision.title')}>
          <h2>{t('decision.title')}</h2>
          <p>{t('decision.help')}</p>
          <p className="field-hint">{t('decision.afterReceipt')}</p>
          {review.disputed && <p className="banner banner--warn" role="alert">{t('decision.disputed')}</p>}
          {review.decision
            ? <p role="status">{t(`decision.${review.decision.choice}`)}</p>
            : !review.canReceive && !review.disputed && <p className="banner banner--neutral">{t('decision.blocked')}</p>}
          <div className="raw-actions">
            <button type="button" className="button button--primary" disabled={busy || !review.canReceive} onClick={() => review.decideRepair('received')}>{t('decision.receive')}</button>
            <button type="button" className="button button--secondary" disabled={busy} onClick={() => review.decideRepair('held')}>{t('decision.hold')}</button>
            <button type="button" className="button button--secondary" disabled={busy} onClick={() => review.decideRepair('rejected')}>{t('decision.reject')}</button>
          </div>
        </section>
      )}

      {review.busy === 'repair' ? (
        <BusyPanel label={t('repair.verifyBusy')} onCancel={review.cancel} />
      ) : embedded ? (
        receipt ? null : (
          <ActionBar note={t('repair.noReceipt')}>
            <button type="button" className="button button--primary" onClick={review.verifyManualRepair} disabled={busy || review.edits.length === 0}>
              <Icon name="shield" size={17} /> {t('repair.verify')}
            </button>
          </ActionBar>
        )
      ) : (
        <ActionBar note={receipt ? (receipt.receipt.status === 'VERIFIED' ? t('repair.verifiedHint') : t('repair.notVerifiedHint')) : t('repair.noReceipt')}>
          <button type="button" className="button button--secondary" onClick={() => review.navigate('result')}>{t('repair.back')}</button>
          {evidenceReady ? (
            <button type="button" className="button button--primary" onClick={() => review.navigate('evidence')}>
              <Icon name="arrow" size={17} /> {t('repair.primaryEvidence')}
            </button>
          ) : (
            <button type="button" className="button button--primary" onClick={review.verifyManualRepair} disabled={busy || review.edits.length === 0}>
              <Icon name="shield" size={17} /> {t('repair.verify')}
            </button>
          )}
        </ActionBar>
      )}
    </div>
  );
}

function collateralMode(status: CollateralStatus): 'verified' | 'failed' | 'unchecked' | 'inconclusive' {
  switch (status) {
    case 'VERIFIED': return 'verified';
    case 'FAILED': return 'failed';
    case 'INCONCLUSIVE': return 'inconclusive';
    default: return 'unchecked';
  }
}
