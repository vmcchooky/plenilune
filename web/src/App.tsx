import { useEffect, useRef, useState } from 'react';
import { useI18n } from './i18n';
import { useReview } from './hooks/useReview';
import { ConditionsStep } from './components/ConditionsStep';
import { EvidenceStep } from './components/EvidenceStep';
import { InboxPanel } from './components/InboxPanel';
import { RepairStep } from './components/RepairStep';
import { QuickScreen } from './components/QuickScreen';
import { ResultStep } from './components/ResultStep';
import { SourceStep } from './components/SourceStep';
import { StepRail } from './components/StepRail';
import { TopBar } from './components/TopBar';
import { InlineError } from './components/Ui';

export function App() {
  const { t } = useI18n();
  const review = useReview();
  // The quick verdict screen is the default entry; the five-stage flow
  // remains fully available as deep review mode over the same review state.
  const [unread, setUnread] = useState(0);
  const [mode, setMode] = useState<'quick' | 'deep'>('quick');
  const parsed = review.parsed?.parsed ?? null;
  const aiBusy = review.busy === 'aiProposal' || review.busy === 'aiRepair';
  const initialStage = useRef(true);
  const inboxRef = useRef<HTMLDetailsElement>(null);

  useEffect(() => {
    if (initialStage.current) {
      initialStage.current = false;
      return;
    }
    if (mode !== 'deep') return;
    document.getElementById('step-heading')?.focus();
    window.scrollTo({ top: 0, behavior: 'auto' });
  }, [review.stage, mode]);

  const watchDrawer = (
    <details className="watch-drawer" ref={inboxRef} key="watch-drawer">
      <summary>{t('inbox.title')}{unread > 0 && <span className="badge badge--warn watch-drawer__count">{t('inbox.unread', { count: unread })}</span>}</summary>
      <InboxPanel onUnreadChange={setUnread} refreshKey={review.comparison?.comparison.sha256 ?? review.analysis?.result.sha256 ?? null} />
    </details>
  );

  return (
    <div className="app">
      <a className="skip-link" href="#main">{t('app.skip')}</a>
      <TopBar backend={review.backend} lastAi={review.lastAi} aiBusy={aiBusy} />
      <div className={`mode-bar ${mode === 'quick' ? 'mode-bar--quick' : ''}`} role="region" aria-label={t('quick.workspace')}>
        <p className="workspace-label">{t('quick.workspace')}</p>
        <div className="workspace-actions">
          {unread > 0 && <div role="status"><button type="button" className="button button--quiet" onClick={() => {
            if (!inboxRef.current) return;
            inboxRef.current.open = true;
            inboxRef.current.querySelector('summary')?.focus();
            inboxRef.current.scrollIntoView?.({ block: 'start' });
          }}>{t('inbox.unread', { count: unread })}</button></div>}
        <div className="mode-switch" role="group" aria-label={t('quick.modeLabel')}>
          <button
            type="button"
            className={mode === 'quick' ? 'is-active' : ''}
            aria-pressed={mode === 'quick'}
            onClick={() => setMode('quick')}
          >
            {t('quick.mode.quick')}
          </button>
          <button
            type="button"
            className={mode === 'deep' ? 'is-active' : ''}
            aria-pressed={mode === 'deep'}
            onClick={() => setMode('deep')}
          >
            {t('quick.mode.deep')}
          </button>
        </div>
        </div>
      </div>
      <div className={`workspace ${mode === 'quick' ? 'workspace--quick' : ''}`}>
        {mode === 'deep' && <StepRail
          stage={review.stage}
          parsed={parsed}
          analysis={review.analysis?.result ?? null}
          selection={review.selection}
          canConditions={Boolean(parsed)}
          canResult={Boolean(review.comparison)}
          canRepair={review.canRepair}
          canEvidence={Boolean(review.comparison)}
          busy={review.busy !== null}
          onNavigate={review.navigate}
        />}
        <main className="main" id="main" tabIndex={-1}>
          {review.decision?.status === 'stale' && <p className="banner banner--warn" role="status">{t('decision.stale')}</p>}
          {review.error && (
            <InlineError error={review.error} onRetry={review.retryCompare} onDismiss={review.clearError} />
          )}
          {/* Keep the drawer mounted as unread changes: moving it between
              parents erased the watch draft and focus. The explicit unread
              action above surfaces new notifications without reordering work. */}
          {mode === 'deep' && watchDrawer}
          {mode === 'quick' ? (
            <QuickScreen review={review} />
          ) : (
            <>
              {review.stage === 'source' && <SourceStep review={review} />}
              {review.stage === 'conditions' && <ConditionsStep review={review} />}
              {review.stage === 'result' && <ResultStep review={review} />}
              {review.stage === 'repair' && <RepairStep review={review} />}
              {review.stage === 'evidence' && <EvidenceStep review={review} />}
            </>
          )}
          {mode === 'quick' && watchDrawer}
        </main>
      </div>
    </div>
  );
}
