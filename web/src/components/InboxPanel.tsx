import { useCallback, useEffect, useState } from 'react';
import { useI18n } from '../i18n';
import type { MessageKey } from '../i18n/messages.en';
import { runtime } from '../lib/api';
import { shortHash } from '../lib/format';
import type { InboxEntry, TriggerCondition, Verdict, WatchRule } from '../lib/types';
import { SectionHeading } from './Ui';

// P2-8 verdict inbox: the event-driven entry of the fast flow. The
// server evaluates every completed analysis/comparison against the
// watched objects pinned here; this panel is only the window onto
// that state — entries name the sealed artifact that produced them
// (by content digest), and the artifact remains the record. Verdict
// meaning lives in the badge on each entry, never in a background.
const CONDITIONS: { id: TriggerCondition; key: MessageKey }[] = [
  { id: 'verdict_changed', key: 'inbox.condition.verdict_changed' },
  { id: 'any_different', key: 'inbox.condition.any_different' },
  { id: 'obligation_failed', key: 'inbox.condition.obligation_failed' },
];

function verdictTone(verdict?: Verdict): string {
  switch (verdict) {
    case 'DIFFERENT': return 'badge badge--bad';
    case 'EQUIVALENT': return 'badge badge--good';
    default: return 'badge badge--warn';
  }
}

export function InboxPanel({ refreshKey }: { refreshKey: string | null }) {
  const { t } = useI18n();
  const [entries, setEntries] = useState<InboxEntry[]>([]);
  const [unread, setUnread] = useState(0);
  const [watches, setWatches] = useState<WatchRule[]>([]);
  const [failed, setFailed] = useState(false);
  const [watchFailed, setWatchFailed] = useState(false);
  const [name, setName] = useState('');
  const [domain, setDomain] = useState('acl');
  const [conditions, setConditions] = useState<TriggerCondition[]>(['verdict_changed']);

  const load = useCallback(async () => {
    try {
      const [inboxDoc, watchDoc] = await Promise.all([runtime.listInbox(), runtime.listWatches()]);
      setEntries(inboxDoc.data.entries);
      setUnread(inboxDoc.data.unread);
      setWatches(watchDoc.data.watches);
      setFailed(false);
    } catch {
      setFailed(true);
    }
  }, []);

  // Refetch on mount and whenever a new artifact completes: a just
  // finished run is exactly when a trigger may have fired.
  useEffect(() => {
    void load();
  }, [load, refreshKey]);

  async function saveWatches(next: WatchRule[]) {
    try {
      const doc = await runtime.replaceWatches(next);
      setWatches(doc.data.watches);
      setWatchFailed(false);
    } catch {
      setWatchFailed(true);
    }
  }

  async function addWatch() {
    const trimmed = name.trim();
    if (!trimmed || conditions.length === 0) return;
    const rest = watches.filter((w) => !(w.domain === domain && w.name === trimmed));
    await saveWatches([...rest, { domain, name: trimmed, conditions }]);
    setName('');
  }

  async function markRead(id: number | 'all') {
    try {
      const doc = await runtime.markInboxRead(id);
      setEntries(doc.data.entries);
      setUnread(doc.data.unread);
    } catch {
      setFailed(true);
    }
  }

  return (
    <section className="inbox-panel card" aria-labelledby="inbox-title">
      <SectionHeading
        id="inbox-title"
        title={t('inbox.title')}
        note={t('inbox.help')}
        aside={unread > 0 ? <span className="badge badge--warn">{t('inbox.unread', { count: unread })}</span> : undefined}
      />
      {failed && <p className="banner banner--warn" role="alert">{t('inbox.error')}</p>}
      <div className="inbox-panel__actions">
        <button type="button" className="button button--quiet button--small" onClick={() => void load()}>
          {t('inbox.refresh')}
        </button>
        {unread > 0 && (
          <button type="button" className="button button--quiet button--small" onClick={() => void markRead('all')}>
            {t('inbox.markAllRead')}
          </button>
        )}
      </div>
      {entries.length === 0 ? (
        <p className="inbox-panel__empty">{t('inbox.empty')}</p>
      ) : (
        <ul className="inbox-list">
          {entries.map((entry) => (
            <li key={entry.id} className={`inbox-entry${entry.read ? '' : ' inbox-entry--unread'}`}>
              <div className="inbox-entry__head">
                <strong className="inbox-entry__name">{entry.name}</strong>
                <span className="badge">{t(`inbox.condition.${entry.condition}` as MessageKey)}</span>
                {entry.verdict && <code className={verdictTone(entry.verdict)}>{entry.verdict}</code>}
                {entry.stale && <span className="badge badge--warn">{t('inbox.stale')}</span>}
              </div>
              <div className="inbox-entry__meta">
                {entry.condition === 'verdict_changed' && entry.previous_verdict && entry.verdict && (
                  <span>{t('inbox.change', { from: entry.previous_verdict, to: entry.verdict })}</span>
                )}
                {entry.violated_obligations && entry.violated_obligations.length > 0 && (
                  <span>{t('inbox.violated', { ids: entry.violated_obligations.join(', ') })}</span>
                )}
                <code>{t('inbox.artifact', { digest: shortHash(entry.content_sha256) })}</code>
                <time dateTime={entry.time}>{new Date(entry.time).toLocaleString()}</time>
              </div>
              {!entry.read && (
                <button type="button" className="button button--quiet button--small" onClick={() => void markRead(entry.id)}>
                  {t('inbox.markRead')}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      <div className="inbox-watch">
        <h3>{t('inbox.watch.title')}</h3>
        <p className="inbox-watch__help">{t('inbox.watch.help')}</p>
        {watchFailed && <p className="banner banner--warn" role="alert">{t('inbox.watch.error')}</p>}
        {watches.length === 0 ? (
          <p className="inbox-panel__empty">{t('inbox.watch.none')}</p>
        ) : (
          <ul className="watch-list">
            {watches.map((watch) => (
              <li key={`${watch.domain}/${watch.name}`} className="watch-list__item">
                <strong>{watch.name}</strong>
                <span className="badge">{t(`inbox.watch.domain.${watch.domain}` as MessageKey)}</span>
                {watch.conditions.map((condition) => (
                  <span key={condition} className="badge">{t(`inbox.condition.${condition}` as MessageKey)}</span>
                ))}
                <button
                  type="button"
                  className="button button--quiet button--small"
                  aria-label={t('inbox.watch.remove', { name: watch.name })}
                  onClick={() => void saveWatches(watches.filter((w) => !(w.domain === watch.domain && w.name === watch.name)))}
                >
                  ×
                </button>
              </li>
            ))}
          </ul>
        )}
        <form
          className="inbox-watch__form"
          onSubmit={(event) => {
            event.preventDefault();
            void addWatch();
          }}
        >
          <label className="field">
            <span>{t('inbox.watch.name')}</span>
            <input value={name} onChange={(event) => setName(event.target.value)} />
          </label>
          <label className="field">
            <span>{t('inbox.watch.domain.acl')} / {t('inbox.watch.domain.prefix-list')}</span>
            <select value={domain} onChange={(event) => setDomain(event.target.value)}>
              <option value="acl">{t('inbox.watch.domain.acl')}</option>
              <option value="prefix-list">{t('inbox.watch.domain.prefix-list')}</option>
            </select>
          </label>
          <fieldset className="inbox-watch__conditions">
            {CONDITIONS.map(({ id, key }) => (
              <label key={id} className="inbox-watch__condition">
                <input
                  type="checkbox"
                  checked={conditions.includes(id)}
                  onChange={(event) => setConditions(
                    event.target.checked ? [...conditions, id] : conditions.filter((c) => c !== id),
                  )}
                />
                {t(key)}
              </label>
            ))}
          </fieldset>
          <button type="submit" className="button button--secondary button--small" disabled={!name.trim() || conditions.length === 0}>
            {t('inbox.watch.add')}
          </button>
        </form>
      </div>
    </section>
  );
}
