import { useEffect, useRef, useState } from 'react';
import { useI18n } from '../i18n';
import type { MessageKey } from '../i18n/messages.en';
import { runtime } from '../lib/api';
import { shortHash } from '../lib/format';
import type { Analysis, ChatAnswer, Comparison } from '../lib/types';
import { SectionHeading } from './Ui';

// P1-10 verdict interrogation chat. The panel exists only on a completed
// artifact and is anchored to that artifact's digest: switching the
// artifact resets the conversation, because answers from another
// artifact would be answers about a different result. The panel can
// only ask — the request carries the sealed artifacts and the question,
// never configuration text, so nothing here can start an analysis,
// change a verdict or trigger a repair. AI phrasing, when it succeeds,
// is shown above the deterministic answer, never instead of the facts:
// if the phrasing contradicts the artifact, the artifact's own data is
// right there and wins.

interface Exchange {
  question: string;
  answer: ChatAnswer;
}

const SUGGESTIONS: MessageKey[] = [
  'chat.suggest.verdict',
  'chat.suggest.obligation',
  'chat.suggest.witness',
  'chat.suggest.undecided',
  'chat.suggest.classes',
];

export function ChatPanel({ comparison, analysis }: { comparison: Comparison | null; analysis: Analysis | null }) {
  const { t, locale } = useI18n();
  const anchor = comparison?.sha256 ?? analysis?.sha256 ?? null;
  const [exchanges, setExchanges] = useState<Exchange[]>([]);
  const [input, setInput] = useState('');
  const [ai, setAi] = useState(false);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const seenAnchor = useRef<string | null>(anchor);

  // Artifact switch: a new digest means a new conversation.
  useEffect(() => {
    if (seenAnchor.current !== anchor) {
      seenAnchor.current = anchor;
      setExchanges([]);
      setInput('');
      setFailed(false);
    }
  }, [anchor]);

  if (!anchor) return null;

  async function ask(question: string) {
    const trimmed = question.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    setFailed(false);
    try {
      const { data } = await runtime.chatAsk(comparison, analysis, trimmed, locale, ai);
      setExchanges((prev) => [...prev, { question: trimmed, answer: data }]);
      setInput('');
    } catch {
      setFailed(true);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="chat-panel" aria-labelledby="chat-title">
      <SectionHeading id="chat-title" title={t('chat.title')} />
      <p className="chat-panel__help">{t('chat.help')}</p>
      <p className="chat-panel__anchor">
        <code>{t('chat.anchor', { digest: shortHash(anchor) })}</code>
      </p>

      <div className="chat-panel__suggestions">
        {SUGGESTIONS.map((key) => (
          <button key={key} type="button" className="badge" onClick={() => setInput(t(key))}>
            {t(key)}
          </button>
        ))}
      </div>

      <div className="chat-panel__log">
        {exchanges.map((exchange, i) => (
          <ChatExchange key={i} exchange={exchange} />
        ))}
      </div>

      {failed && (
        <p className="chat-panel__error" role="alert">
          {t('chat.error')}
        </p>
      )}

      <form
        className="chat-panel__form"
        onSubmit={(event) => {
          event.preventDefault();
          void ask(input);
        }}
      >
        <input
          type="text"
          value={input}
          maxLength={1000}
          onChange={(event) => setInput(event.target.value)}
          placeholder={t('chat.placeholder')}
          aria-label={t('chat.title')}
        />
        <button type="submit" className="button button--primary" disabled={busy || !input.trim()}>
          {busy ? t('chat.busy') : t('chat.send')}
        </button>
      </form>
      <label className="chat-panel__ai">
        <input type="checkbox" checked={ai} onChange={(event) => setAi(event.target.checked)} />
        {t('chat.aiToggle')}
      </label>
      {ai && <p className="chat-panel__ai-note">{t('chat.aiNote')}</p>}
    </section>
  );
}

function ChatExchange({ exchange }: { exchange: Exchange }) {
  const { t } = useI18n();
  const { answer } = exchange;
  const phrased = answer.ai.status === 'ok' && answer.ai.text ? answer.ai.text : null;
  return (
    <article className="chat-exchange">
      <p className="chat-exchange__question">{exchange.question}</p>
      {phrased ? (
        <>
          <p className="chat-exchange__answer">{phrased}</p>
          <details className="chat-exchange__deterministic">
            <summary>{t('chat.deterministic')}</summary>
            <p>{answer.text}</p>
          </details>
        </>
      ) : (
        <p className="chat-exchange__answer">{answer.text}</p>
      )}
      {answer.ai.status === 'not_configured' && <p className="chat-exchange__note">{t('chat.aiNotConfigured')}</p>}
      {(answer.ai.status === 'unavailable' || answer.ai.status === 'error' || answer.ai.status === 'model_unsupported') && (
        <p className="chat-exchange__note">{t('chat.aiUnavailable')}</p>
      )}
      <p className="chat-exchange__facts-title">{t('chat.facts')}</p>
      <dl className="chat-exchange__facts">
        {answer.facts.map((fact, i) => (
          <div key={i} className="chat-exchange__fact">
            <dt>{fact.label}</dt>
            <dd>
              {fact.value} <code>{fact.source}</code>
            </dd>
          </div>
        ))}
      </dl>
    </article>
  );
}
