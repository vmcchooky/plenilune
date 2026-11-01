import { useState } from 'react';
import { useI18n } from '../i18n';
import type { Review } from '../hooks/useReview';
import { logicalLineCount } from '../lib/conditions';
import { readReviewFile } from '../lib/reviewArtifacts';

// The before/after editors and the sample picker, shared by the deep flow's
// source step and the quick verdict screen so both entry points read and
// import configurations exactly the same way.
export function SourceEditor({
  id,
  label,
  value,
  help,
  disabled,
  onChange,
}: {
  id: 'before' | 'after';
  label: string;
  value: string;
  help: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useI18n();
  const [fileError, setFileError] = useState(false);
  async function open(file: File | undefined) {
    if (!file) return;
    try {
      const imported = await readReviewFile(file, 4 << 20);
      setFileError(false);
      onChange(imported.text);
    } catch {
      setFileError(true);
    }
  }
  return (
    <div className="source-editor">
      <div className="source-editor__head">
        <label htmlFor={`${id}-acl`}>{label}</label>
        <span>{t('source.lines', { count: logicalLineCount(value) })}</span>
        <label className="button button--quiet button--small source-editor__file">
          {t('source.import')}
          <input type="file" className="sr-only" disabled={disabled} accept=".cfg,.conf,.txt,.acl,text/plain"
            aria-label={`${t('source.import')} — ${label}`}
            onChange={(event) => { void open(event.target.files?.[0]); event.target.value = ''; }} />
        </label>
      </div>
      {fileError && <p className="field-error" role="alert">{t('source.importError')}</p>}
      <textarea
        id={`${id}-acl`}
        className="source-editor__input"
        value={value}
        disabled={disabled}
        spellCheck={false}
        autoCapitalize="off"
        autoCorrect="off"
        wrap="off"
        aria-describedby={`${id}-acl-help`}
        onChange={(event) => onChange(event.target.value)}
      />
      <p className="field-hint" id={`${id}-acl-help`}>{help}</p>
    </div>
  );
}

export function SampleBand({ review }: { review: Review }) {
  const { t } = useI18n();
  const busy = review.busy !== null;
  const sampleDescription = review.sample ? t(review.sample.nameKey) : t('source.sample.blank');
  return (
    <section className="sample-band" aria-labelledby="sample-title">
      <div className="sample-band__intro">
        <p className="field-label" id="sample-title">{t('source.sampleLegend')}</p>
        <p className="sample-band__name">{sampleDescription}</p>
        <p className="field-hint">{t('source.sampleHelp')}</p>
      </div>
      <div className="sample-band__select">
        <label htmlFor="sample-select" className="sr-only">{t('source.sampleLegend')}</label>
        <select
          id="sample-select"
          value={review.sampleId ?? 'blank'}
          disabled={busy}
          onChange={(event) => review.selectSample(event.target.value)}
        >
          {review.samples.map((sample) => (
            <option key={sample.id} value={sample.id}>{t(sample.nameKey)}</option>
          ))}
          <option value="blank">{t('source.sample.blank')}</option>
        </select>
      </div>
    </section>
  );
}
