import { useI18n } from '../i18n';
import type { MessageKey } from '../i18n/messages.en';
import type { BackendState } from '../hooks/useReview';
import type { AIResult } from '../lib/types';
import { Icon } from './Icon';

type BadgeTone = 'neutral' | 'info' | 'warn';

function aiBadge(lastAi: AIResult | null): { key: MessageKey; tone: BadgeTone } {
  if (!lastAi) return { key: 'ai.badge.idle', tone: 'neutral' };
  switch (lastAi.status) {
    case 'completed': return { key: 'ai.badge.used', tone: 'info' };
    case 'not_configured': return { key: 'ai.badge.not_configured', tone: 'warn' };
    case 'unavailable': return { key: 'ai.badge.unavailable', tone: 'warn' };
    case 'partial': return { key: 'ai.badge.partial', tone: 'warn' };
    case 'clarification_needed': return { key: 'ai.badge.clarification_needed', tone: 'warn' };
    case 'canceled': return { key: 'ai.badge.canceled', tone: 'neutral' };
    default: return { key: 'ai.badge.idle', tone: 'neutral' };
  }
}

export function TopBar({
  backend,
  lastAi,
  aiBusy,
}: {
  backend: BackendState;
  lastAi: AIResult | null;
  aiBusy: boolean;
}) {
  const { t, locale, setLocale } = useI18n();
  const badge = aiBadge(lastAi);

  return (
    <header className="topbar">
      <div className="wordmark">
        <span className="wordmark__mark" aria-hidden="true"><Icon name="moon" size={24} /></span>
        <span className="wordmark__text">
          <strong>Plenilune</strong>
          <span>{t('app.tagline')}</span>
        </span>
      </div>
      <div className="topbar__right">
        <span className={`runtime-indicator runtime-indicator--${backend}`}>
          <span className="runtime-indicator__dot" aria-hidden="true" />
          {backend === 'online' ? t('runtime.online') : backend === 'offline' ? t('runtime.offline') : t('runtime.checking')}
        </span>
        <span className={`ai-indicator ai-indicator--${badge.tone}`} title={t('ai.badge.tooltip')}>
          <Icon name={aiBusy ? 'clock' : 'lock'} size={14} />
          {aiBusy ? t('common.loading') : t(badge.key)}
        </span>
        <div className="language-switch" role="group" aria-label={t('lang.group')}>
          <button
            type="button"
            className={locale === 'vi' ? 'is-active' : ''}
            aria-pressed={locale === 'vi'}
            aria-label={`VI — ${t('lang.vi')}`}
            title={t('lang.vi')}
            onClick={() => setLocale('vi')}
          >
            VI
          </button>
          <button
            type="button"
            className={locale === 'en' ? 'is-active' : ''}
            aria-pressed={locale === 'en'}
            aria-label={`EN — ${t('lang.en')}`}
            title={t('lang.en')}
            onClick={() => setLocale('en')}
          >
            EN
          </button>
        </div>
      </div>
    </header>
  );
}
