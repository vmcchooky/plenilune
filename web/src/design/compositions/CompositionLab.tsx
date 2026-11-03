import { useEffect, useRef, useState } from 'react';
import { useReview } from '../../hooks/useReview';
import { useI18n } from '../../i18n';
import { ConditionsStep } from '../../components/ConditionsStep';
import { TopBar } from '../../components/TopBar';
import { ChangeClasses } from '../../components/ChangeClasses';
import { InlineError } from '../../components/Ui';
import { compositionCopy } from './copy';
import { compositionModel } from './model';
import { RecordComposition } from './RecordComposition';
import { InlineComposition } from './InlineComposition';
import { WorkbenchComposition } from './WorkbenchComposition';
import './compositions.css';

type Layout = 'a' | 'b' | 'c';
export default function CompositionLab() {
  const review = useReview();
  const { locale, t } = useI18n();
  const copy = compositionCopy[locale];
  const initial = new URLSearchParams(window.location.search).get('design');
  const focused = new URLSearchParams(window.location.search).get('focus') === '1';
  const [layout, setLayout] = useState<Layout>(initial === 'b' || initial === 'c' ? initial : 'a');
  const [conditions, setConditions] = useState(false);
  const attempted = useRef(false);
  const run = useRef(review.exploreSource);
  run.current = review.exploreSource;
  const model = compositionModel(review);
  useEffect(() => {
    if (attempted.current) return;
    const timer = setTimeout(() => { attempted.current = true; void run.current(); }, 80);
    return () => clearTimeout(timer);
  }, []);
  useEffect(() => {
    if (conditions && review.stage !== 'conditions') setConditions(false);
  }, [conditions, review.stage]);
  useEffect(() => {
    if (conditions) document.querySelector<HTMLElement>('.composition-condition-panel h1')?.focus();
  }, [conditions]);
  function choose(value: Layout) {
    setConditions(false);
    setLayout(value);
    const url = new URL(window.location.href);
    url.searchParams.set('design', value);
    window.history.replaceState(null, '', url);
  }
  const props = model ? { model, copy, onConditions: () => {
    review.navigate('conditions'); setConditions(true);
  } } : null;
  return <div className={`composition-lab ${focused ? 'composition-lab--focused' : ''}`}>
    <a className="skip-link" href="#main">{t('app.skip')}</a>
    <TopBar backend={review.backend} lastAi={review.lastAi} aiBusy={false} />
    {!focused && <div className="study-bar">
      <div className="study-choices" role="group" aria-label={copy.layouts}>
        {(['a', 'b', 'c'] as const).map(value => <button type="button" key={value} aria-pressed={layout === value} onClick={() => choose(value)}>{copy[value]}</button>)}
      </div>
      <a href="/">{copy.back}</a>
    </div>}
    <main id="main" className="study-main">
      {focused ? <div className="study-focus-context"><a className="study-focus-back" href="/?design=a">← {copy.lab}</a><span>{copy.sample}</span></div> : <div className="study-intro"><p>{copy[`${layout}Note`]}</p><span>{copy.sample}</span></div>}
      {review.error && <InlineError error={review.error} onDismiss={review.clearError} />}
      {conditions ? <section className="composition-condition-panel">
        <button type="button" className="button button--quiet" onClick={() => setConditions(false)}>← {copy.close}</button>
        <ConditionsStep review={review} />
      </section> : props ? <div className={`study-canvas study-canvas--${layout}`} data-layout={layout}>
        {layout === 'a' ? <RecordComposition {...props} /> : layout === 'b' ? <InlineComposition {...props} /> : <WorkbenchComposition {...props} />}
        <details className="study-evidence"><summary>{copy.evidence}</summary>
          <ChangeClasses review={review} />
          <p>{copy.limit}</p>
          <code>{review.comparison?.comparison.sha256}</code>
        </details>
      </div> : <div className="study-wait" role="status">
        <p>{review.busy || !attempted.current ? copy.loading : review.error ? copy.unavailable : copy.noWitness}</p>
        {!review.busy && <button type="button" className="button button--secondary" onClick={() => { void review.exploreSource(); }}>{copy.retry}</button>}
      </div>}
    </main>
  </div>;
}
