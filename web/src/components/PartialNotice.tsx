import { useI18n } from '../i18n';
import {
  partialPairLine,
  partialStageLabel,
  partialUndecidedLine,
} from '../lib/partial';
import type { PartialResult } from '../lib/types';

// P1-2: renders a budget-stopped (partial) result honestly — the badge is
// unmistakable, only proven facts are listed (identical prefix pairs,
// decided rule pairs whose evaluator-checked witnesses appear elsewhere
// on the screen), and the undecided remainder is shown as bounded rule
// ranges that are characterized no further than "not yet decided".

export function PartialNotice({
  partial,
  showDecidedPairs = true,
  showUndecidedPairs = false,
}: {
  partial: PartialResult;
  showDecidedPairs?: boolean;
  showUndecidedPairs?: boolean;
}) {
  const { t } = useI18n();
  const undecidedPairs = partial.undecided_pairs ?? [];
  return (
    <div className="partial-result" role="note">
      <p className="partial-result__head">
        <strong>{t('partial.badge')}</strong>
        <span>{t('partial.stoppedAt', { stage: partialStageLabel(t, partial.stage) })}</span>
      </p>
      <p>{t('partial.note')}</p>
      <ul className="partial-result__facts">
        {partial.prefix_pairs > 0 && <li>{t('partial.prefixPairs', { count: partial.prefix_pairs })}</li>}
        {showDecidedPairs && partial.decided_pairs.length > 0 && (
          <li>{t('partial.decidedPairs', { count: partial.decided_pairs.length })}</li>
        )}
        {showUndecidedPairs && undecidedPairs.length > 0 && (
          <li>{t('partial.undecidedPairs', { count: undecidedPairs.length })}</li>
        )}
        {(partial.decided_pairs_truncated || partial.undecided_pairs_truncated) && (
          <li>{t('partial.pairsTruncated')}</li>
        )}
      </ul>
      {showDecidedPairs && partial.decided_pairs.length > 0 && (
        <ul className="partial-result__pairs">
          {partial.decided_pairs.map((pair, i) => (
            <li key={i}><code>{partialPairLine(t, pair)}</code></li>
          ))}
        </ul>
      )}
      {showUndecidedPairs && undecidedPairs.length > 0 && (
        <ul className="partial-result__pairs">
          {undecidedPairs.map((pair, i) => (
            <li key={i}><code>{partialPairLine(t, pair)}</code></li>
          ))}
        </ul>
      )}
      <p className="partial-result__undecided-title">{t('partial.undecidedTitle')}</p>
      <ul className="partial-result__undecided">
        {partial.undecided.map((part, i) => (
          <li key={i}>{partialUndecidedLine(t, part)}</li>
        ))}
      </ul>
    </div>
  );
}
