import { useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { useI18n } from '../i18n';
import { copyText } from '../lib/download';
import { errorCopy, errorLocation } from '../lib/errors';
import type { ReviewError } from '../lib/errors';
import { shortHash } from '../lib/format';
import { Icon } from './Icon';
import type { IconName } from './Icon';

export type Tone = 'good' | 'bad' | 'warn' | 'neutral' | 'info';

const toneIcons: Record<Tone, IconName> = {
  good: 'check',
  bad: 'cross',
  warn: 'clock',
  neutral: 'dash',
  info: 'info',
};

export function StatusMark({ tone, label, icon, raw }: { tone: Tone; label: string; icon?: IconName; raw?: string }) {
  return (
    <span className={`status-mark status-mark--${tone}`}>
      <Icon name={icon ?? toneIcons[tone]} size={16} />
      <span>{label}</span>
      {raw && <code className="status-mark__raw">{raw}</code>}
    </span>
  );
}

export function ProvenanceTag({ tone = 'neutral', icon = 'info', children }: { tone?: Tone; icon?: IconName; children: ReactNode }) {
  return (
    <span className={`provenance provenance--${tone}`}>
      <Icon name={icon} size={14} />
      <span>{children}</span>
    </span>
  );
}

export function StepHeader({
  kicker,
  title,
  lede,
  children,
}: {
  kicker: string;
  title: string;
  lede: string;
  children?: ReactNode;
}) {
  return (
    <header className="step-header">
      <p className="kicker">{kicker}</p>
      <h1 id="step-heading" tabIndex={-1}>{title}</h1>
      <p className="step-lede">{lede}</p>
      {children && <div className="step-header__meta">{children}</div>}
    </header>
  );
}

export function SectionHeading({
  title,
  note,
  aside,
  id,
}: {
  title: string;
  note?: string;
  aside?: ReactNode;
  id?: string;
}) {
  return (
    <div className="section-heading">
      <div>
        <h2 id={id}>{title}</h2>
        {note && <p>{note}</p>}
      </div>
      {aside && <div className="section-heading__aside">{aside}</div>}
    </div>
  );
}

export function TermNote({ children }: { children: ReactNode }) {
  return (
    <p className="term-note">
      <Icon name="info" size={15} />
      <span>{children}</span>
    </p>
  );
}

export function ActionBar({ note, children }: { note?: ReactNode; children: ReactNode }) {
  return (
    <div className="action-bar">
      {note && <div className="action-bar__note">{note}</div>}
      <div className="action-bar__buttons">{children}</div>
    </div>
  );
}

export function Spinner() {
  return <span className="spinner" aria-hidden="true" />;
}

export function BusyPanel({ label, onCancel }: { label: string; onCancel?: () => void }) {
  const { t } = useI18n();
  return (
    <div className="busy-panel" role="status" aria-live="polite">
      <Spinner />
      <p>{label}</p>
      {onCancel && <button type="button" className="button button--quiet button--small" onClick={onCancel}>{t('common.cancel')}</button>}
    </div>
  );
}

export function EmptyState({
  icon = 'shield',
  title,
  body,
  children,
}: {
  icon?: IconName;
  title: string;
  body: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <Icon name={icon} size={26} />
      <h2>{title}</h2>
      <p>{body}</p>
      {children && <div className="empty-state__actions">{children}</div>}
    </div>
  );
}

export function TechnicalDetails({
  title,
  children,
  defaultOpen = false,
  className = '',
}: {
  title: string;
  children: ReactNode;
  defaultOpen?: boolean;
  className?: string;
}) {
  return (
    <details className={`technical-details ${className}`.trim()} open={defaultOpen}>
      <summary>
        <Icon name="chevron" size={15} className="technical-details__chevron" />
        <span>{title}</span>
      </summary>
      <div className="technical-details__body">{children}</div>
    </details>
  );
}

export function CopyButton({ value, label }: { value: string; label?: string }) {
  const { t } = useI18n();
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle');
  const timer = useRef<number | null>(null);

  useEffect(() => () => {
    if (timer.current !== null) window.clearTimeout(timer.current);
  }, []);

  async function copy() {
    const ok = await copyText(value);
    setState(ok ? 'copied' : 'failed');
    if (timer.current !== null) window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setState('idle'), 2200);
  }

  return (
    <>
      <button type="button" className="button button--quiet button--small" onClick={copy}>
        <Icon name={state === 'copied' ? 'check' : 'copy'} size={15} />
        {state === 'copied' ? t('common.copied') : label ?? t('common.copy')}
      </button>
      <span className="sr-only" role="status" aria-live="polite">
        {state === 'failed' ? t('common.copyFailed') : state === 'copied' ? t('common.copiedAnnounce') : ''}
      </span>
    </>
  );
}

export function HashLine({ label, value, short = true }: { label: string; value: string; short?: boolean }) {
  return (
    <div className="hash-line">
      <span>{label}</span>
      <code title={value}>{short ? shortHash(value) : value}</code>
      <CopyButton value={value} />
    </div>
  );
}

export function InlineError({
  error,
  onRetry,
  onDismiss,
}: {
  error: ReviewError;
  onRetry?: () => void;
  onDismiss?: () => void;
}) {
  const { t } = useI18n();
  const copy = errorCopy(t, error);
  const location = errorLocation(t, error);
  return (
    <section className="error-panel" role="alert" aria-labelledby="error-title">
      <div className="error-panel__body">
        <p id="error-title" className="error-panel__title">{copy.title}</p>
        <p>{copy.body}</p>
        {location && <p className="error-panel__location">{location}</p>}
        <p className="error-panel__hint">{copy.hint}</p>
        <details className="error-panel__raw">
          <summary>{t('error.details')}</summary>
          <dl>
            <div><dt>{t('error.code')}</dt><dd><code>{error.code}</code></dd></div>
            {error.side && <div><dt>{t('error.side')}</dt><dd><code>{error.side}</code></dd></div>}
            {error.line !== undefined && <div><dt>{t('error.line')}</dt><dd><code>{error.line}</code></dd></div>}
            <div><dt>{t('error.raw')}</dt><dd><code>{error.message}</code></dd></div>
          </dl>
        </details>
      </div>
      <div className="error-panel__actions">
        {onRetry && <button type="button" className="button button--secondary button--small" onClick={onRetry}>{t('common.retry')}</button>}
        {onDismiss && <button type="button" className="button button--quiet button--small" onClick={onDismiss}>{t('common.close')}</button>}
      </div>
    </section>
  );
}

export function FieldHint({ id, children }: { id?: string; children: ReactNode }) {
  return <p className="field-hint" id={id}>{children}</p>;
}
