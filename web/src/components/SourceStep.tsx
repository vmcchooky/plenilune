import { useI18n } from '../i18n';
import type { Review } from '../hooks/useReview';
import { ActionBar, BusyPanel, ProvenanceTag, StepHeader, TechnicalDetails } from './Ui';
import { OfflineReviewPanel } from './OfflineReviewPanel';
import { SampleBand, SourceEditor } from './SourceInputs';
import { Icon } from './Icon';

export function SourceStep({ review }: { review: Review }) {
  const { t } = useI18n();
  const busy = review.busy !== null;
  const empty = !review.draft.before.trim() || !review.draft.after.trim();

  return (
    <div className="step">
      <StepHeader
        kicker={t('source.kicker')}
        title={t('source.title')}
        lede={t('source.lede')}
      >
        {review.pristine && <ProvenanceTag icon="file">{t('source.sample.pristine')}</ProvenanceTag>}
        {review.sample && !review.pristine && <ProvenanceTag icon="file">{t('source.sample.modified')}</ProvenanceTag>}
      </StepHeader>

      <SampleBand review={review} />

      <div className="card source-spread">
        <SourceEditor
          id="before"
          label={t('source.before')}
          value={review.draft.before}
          help={t('source.beforeHelp')}
          disabled={busy}
          onChange={(value) => review.updateSource('before', value)}
        />
        <SourceEditor
          id="after"
          label={t('source.after')}
          value={review.draft.after}
          help={t('source.beforeHelp')}
          disabled={busy}
          onChange={(value) => review.updateSource('after', value)}
        />
      </div>

      <section className="intent-block">
        <div className="intent-block__head">
          <label htmlFor="intent-input">{t('source.intent')}</label>
          <span className="field-label">{t('source.optionalIntent')}</span>
        </div>
        <textarea
          id="intent-input"
          className="intent-block__input"
          value={review.draft.intent}
          disabled={busy}
          rows={3}
          placeholder={t('source.intentPlaceholder')}
          aria-describedby="intent-help"
          onChange={(event) => review.updateSource('intent', event.target.value)}
        />
        <p className="field-hint" id="intent-help">
          {review.sample && review.draft.intent === review.sample.intent ? t('source.intentHint') : t('source.intentHelp')}
        </p>
      </section>

      <p className="field-hint">{t('source.privacy')}</p>

      <OfflineReviewPanel />

      <TechnicalDetails title={t('source.advanced')}>
        <p>{t('source.limits')}</p>
        {review.draft.options && <p>{t('source.budgetLimited')}</p>}
      </TechnicalDetails>

      {review.canceled === 'parse' && (
        <div className="banner banner--neutral" role="status">
          <Icon name="info" size={16} />
          <p>{t('common.canceled')}</p>
        </div>
      )}

      {review.busy === 'parse' ? (
        <BusyPanel label={t('source.busy')} onCancel={review.cancel} />
      ) : (
        <ActionBar note={<><strong>{t('source.actionNote')}</strong>{empty && <> {t('source.emptyHint')}</>}</>}>
          <button type="button" className="button button--primary" disabled={empty} onClick={review.exploreSource}>
            <Icon name="arrow" size={17} />
            {t('source.action')}
          </button>
        </ActionBar>
      )}
    </div>
  );
}
