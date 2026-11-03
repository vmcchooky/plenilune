import { lazy, StrictMode, Suspense } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import { LocaleProvider } from './i18n';
import './styles/tokens.css';
import './styles/app.css';
import './styles/components.css';
import './styles/review-desk.css';

const CompositionLab = lazy(() => import('./design/compositions/CompositionLab'));
const showCompositions = new URLSearchParams(window.location.search).has('design');

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <LocaleProvider>
      {showCompositions ? <Suspense fallback={<p role="status">Plenilune…</p>}><CompositionLab /></Suspense> : <App />}
    </LocaleProvider>
  </StrictMode>,
);
