import { useI18n } from '../i18n';
import type { Draft, Review } from '../hooks/useReview';
import {
  conditionPortError,
  conditionSentence,
  isWideRegion,
  kindHelp,
  kindLabel,
  regionFromDraft,
} from '../lib/conditions';
import { Icon } from './Icon';
import { RegionFields } from './RegionFields';
import { EmptyState } from './Ui';

// The condition list and allowed-change scope editors, shared by the deep
// flow's conditions step and the quick verdict screen's inline obligations
// panel. Both surfaces drive the same draft in useReview, so an edit made in
// one is the same edit in the other.
export function regionInvalid(region: { src_ports_text: string; dst_ports_text: string; protocol: string }): boolean {
  if (region.protocol !== 'tcp' && region.protocol !== 'udp') return false;
  return regionFromDraft({ ...region, src_cidr: '', dst_cidr: '', exclude_src_text: '', exclude_dst_text: '' }) === null;
}

export function draftCanAccept(draft: Draft): boolean {
  const portError = draft.conditions.some(conditionPortError);
  const scopeRegionError = draft.scopeMode === 'regions' && draft.allowedChanges.some(regionInvalid);
  return draft.conditions.length > 0
    && draft.conditions.length <= 12
    && !portError
    && (draft.scopeMode !== 'regions' || draft.allowedChanges.length > 0)
    && !scopeRegionError;
}

export function ConditionEditorList({ review }: { review: Review }) {
  const { t } = useI18n();
  const draft = review.draft;
  const busy = review.busy !== null;
  if (draft.conditions.length === 0) {
    return (
      <EmptyState
        icon="shield"
        title={t('cond.empty')}
        body={t('cond.starterHelp')}
      >
        <button type="button" className="button button--secondary" onClick={review.addCondition}>
          <Icon name="plus" size={16} />
          {t('cond.starter')}
        </button>
      </EmptyState>
    );
  }
  return (
    <div className="condition-list">
      {draft.conditions.map((condition) => {
        const titleId = `${condition.id}-title`;
        return (
          <article className="condition-row" aria-labelledby={titleId} key={condition.id}>
            <div className="condition-row__head">
              <div className="condition-row__identity">
                <code id={titleId} title={t('cond.idLocked')}>{condition.id}</code>
              </div>
              <div className="field condition-row__kind">
                <label htmlFor={`${condition.id}-kind`}>{t('cond.kind')}</label>
                <select
                  id={`${condition.id}-kind`}
                  value={condition.kind}
                  disabled={busy}
                  onChange={(event) => review.updateCondition(condition.id, { ...condition, kind: event.target.value as typeof condition.kind })}
                >
                  {(['must_permit', 'must_deny', 'must_remain'] as const).map((kind) => (
                    <option key={kind} value={kind}>{kindLabel(t, kind)}</option>
                  ))}
                </select>
              </div>
              <button
                type="button"
                className="button button--quiet button--small button--danger condition-row__remove"
                disabled={busy}
                aria-label={t('cond.remove', { id: condition.id })}
                onClick={() => review.removeCondition(condition.id)}
              >
                <Icon name="trash" size={16} />
              </button>
            </div>
            <p className="condition-row__help">{kindHelp(t, condition.kind)}</p>
            <RegionFields
              idPrefix={`condition-${condition.id}`}
              value={condition.region}
              disabled={busy}
              onChange={(next) => review.updateCondition(condition.id, { ...condition, region: next })}
            />
            <p className="condition-row__summary">
              <span className="field-label">{t('cond.summary')}</span>
              {conditionSentence(t, condition)}
            </p>
          </article>
        );
      })}
      <button
        type="button"
        className="button button--secondary button--small condition-add"
        disabled={busy || draft.conditions.length >= 12}
        onClick={review.addCondition}
      >
        <Icon name="plus" size={15} />
        {t('cond.add')}
      </button>
    </div>
  );
}

export function ScopeEditor({ review }: { review: Review }) {
  const { t } = useI18n();
  const draft = review.draft;
  const busy = review.busy !== null;
  return (
    <>
      <div className="scope-options" role="radiogroup" aria-label={t('scope.title')}>
        {(['undeclared', 'none', 'regions'] as const).map((mode) => {
          const help = mode === 'undeclared' ? t('scope.undeclaredHelp') : mode === 'none' ? t('scope.noneHelp') : t('scope.regionsHelp');
          return (
            <label className={`scope-option ${draft.scopeMode === mode ? 'scope-option--selected' : ''}`} key={mode}>
              <input
                type="radio"
                name="scope-mode"
                value={mode}
                checked={draft.scopeMode === mode}
                disabled={busy}
                onChange={() => review.setScopeMode(mode)}
              />
              <span>
                <strong>{t(`scope.mode.${mode}`)}</strong>
                <small>{help}</small>
              </span>
            </label>
          );
        })}
      </div>
      {draft.scopeMode === 'regions' && (
        <div className="scope-regions">
          {draft.allowedChanges.map((region, index) => (
            <article className="scope-region" key={index}>
              <div className="scope-region__head">
                <h3>{t('scope.region', { index: index + 1 })}</h3>
                <button
                  type="button"
                  className="button button--quiet button--small button--danger"
                  disabled={busy}
                  aria-label={t('scope.remove', { index: index + 1 })}
                  onClick={() => review.removeRegion(index)}
                >
                  <Icon name="trash" size={16} />
                </button>
              </div>
              <RegionFields
                idPrefix={`scope-${index}`}
                value={region}
                disabled={busy}
                onChange={(next) => review.updateRegion(index, next)}
              />
              {isWideRegion(region) && <p className="field-warning" role="status">{t('scope.wideWarning')}</p>}
            </article>
          ))}
          <button
            type="button"
            className="button button--secondary button--small"
            disabled={busy || draft.allowedChanges.length >= 12}
            onClick={review.addRegion}
          >
            <Icon name="plus" size={15} />
            {t('scope.add')}
          </button>
        </div>
      )}
    </>
  );
}
