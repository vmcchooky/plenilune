import { useEffect, useRef } from 'react';
import { useI18n } from '../i18n';
import type { Analysis, ObjectSelection, ParseResult, Stage } from '../lib/types';
import { shortHash } from '../lib/format';

interface RailProps {
  stage: Stage;
  parsed: ParseResult | null;
  analysis: Analysis | null;
  selection: ObjectSelection | null;
  canConditions: boolean;
  canResult: boolean;
  canRepair: boolean;
  canEvidence: boolean;
  busy?: boolean;
  onNavigate: (stage: Stage) => void;
}

export function StepRail(props: RailProps) {
  const { t } = useI18n();
  const navRef = useRef<HTMLElement>(null);
  const activeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const nav = navRef.current;
    const active = activeRef.current;
    if (!nav || !active) return;
    const reveal = () => {
      if (nav.scrollWidth <= nav.clientWidth) return;
      const navBounds = nav.getBoundingClientRect();
      const activeBounds = active.getBoundingClientRect();
      if (activeBounds.left >= navBounds.left && activeBounds.right <= navBounds.right) return;
      const left = nav.scrollLeft + activeBounds.left - navBounds.left - (nav.clientWidth - activeBounds.width) / 2;
      nav.scrollTo({ left: Math.max(0, left), behavior: 'auto' });
    };
    reveal();
    const observer = new ResizeObserver(reveal);
    observer.observe(nav);
    return () => observer.disconnect();
  }, [props.stage]);

  const steps: { id: Stage; number: string; title: string; detail: string; enabled: boolean }[] = [
    { id: 'source', number: '01', title: t('nav.source.title'), detail: t('nav.source.detail'), enabled: true },
    { id: 'conditions', number: '02', title: t('nav.conditions.title'), detail: t('nav.conditions.detail'), enabled: props.canConditions },
    { id: 'result', number: '03', title: t('nav.result.title'), detail: t('nav.result.detail'), enabled: props.canResult },
    { id: 'repair', number: '04', title: t('nav.repair.title'), detail: t('nav.repair.detail'), enabled: props.canRepair },
    { id: 'evidence', number: '05', title: t('nav.evidence.title'), detail: t('nav.evidence.detail'), enabled: props.canEvidence },
  ];

  const acl = props.parsed?.after.acl;
  const selectedObject = props.analysis && props.selection
    ? props.analysis.objects.find((object) => object.domain === props.selection!.domain && object.name === props.selection!.name) ?? null
    : null;
  const ticketHash = selectedObject?.after?.sha256 ?? selectedObject?.before?.sha256 ?? props.analysis?.sha256 ?? '';

  return (
    <nav className="step-rail" aria-label={t('nav.aria')} ref={navRef}>
      <div className="ticket">
        <p className="ticket__label">{t('nav.ticket')}</p>
        {acl ? (
          <>
            <p className="ticket__acl">{acl.name}</p>
            <p className="ticket__meta">{t('cond.parse.rules', { count: acl.rules.length })}</p>
            <code className="ticket__hash" title={props.parsed?.after.sha256}>{shortHash(props.parsed?.after.sha256 ?? '')}</code>
          </>
        ) : selectedObject ? (
          <>
            <p className="ticket__acl">{t('result.objectHeader', { domain: t(`overview.domain.${selectedObject.domain}`), name: selectedObject.name })}</p>
            <p className="ticket__meta">{t(`overview.status.${selectedObject.status}`)}</p>
            <code className="ticket__hash" title={ticketHash}>{shortHash(ticketHash)}</code>
          </>
        ) : props.analysis ? (
          <>
            <p className="ticket__acl">{t('nav.analysisObjects', { count: props.analysis.objects.length })}</p>
            <p className="ticket__meta">{t(props.analysis.summary.changed === 1 ? 'nav.analysisChangedOne' : 'nav.analysisChanged', { count: props.analysis.summary.changed })}</p>
            <code className="ticket__hash" title={props.analysis.sha256}>{shortHash(props.analysis.sha256)}</code>
          </>
        ) : (
          <p className="ticket__empty">{t('nav.noAcl')}</p>
        )}
      </div>
      <ol className="rail-list">
        {steps.map((step) => {
          const active = step.id === props.stage;
          const disabled = !step.enabled || (Boolean(props.busy) && !active);
          return (
            <li key={step.id}>
              <button
                type="button"
                ref={active ? activeRef : undefined}
                className={`rail-step ${active ? 'rail-step--active' : ''}`}
                disabled={disabled}
                title={!step.enabled ? t('nav.unavailable') : disabled ? t('nav.busy') : undefined}
                aria-current={active ? 'step' : undefined}
                onClick={() => props.onNavigate(step.id)}
              >
                <span className="rail-step__number">{step.number}</span>
                <span className="rail-step__text">
                  <strong>{step.title}</strong>
                  <small>{step.detail}</small>
                </span>
              </button>
            </li>
          );
        })}
      </ol>
      <p className="rail-foot">{t('nav.privacy')}</p>
    </nav>
  );
}
