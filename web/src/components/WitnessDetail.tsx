import { useI18n } from '../i18n';
import { actionLabel, sourceLine, sourceLines } from '../lib/conditions';
import type { Decision, Witness } from '../lib/types';
import { Icon } from './Icon';
import { PacketRoute } from './PacketRoute';
import { HashLine, ProvenanceTag, StatusMark, TechnicalDetails } from './Ui';

function DecisionBlock({
  label,
  decision,
  source,
  changed,
  tag,
}: {
  label: string;
  decision: Decision;
  source: string;
  changed: Set<number>;
  tag: string;
}) {
  const { t } = useI18n();
  const text = sourceLine(source, decision.line);
  const lineChanged = decision.line !== undefined && changed.has(decision.line);
  return (
    <div className={`card decision-block decision-block--${decision.action}`}>
      <div className="decision-block__head">
        <span>{label}</span>
        <StatusMark
          tone="neutral"
          icon={decision.action === 'permit' ? 'check' : 'cross'}
          label={actionLabel(t, decision.action)}
        />
      </div>
      <p className="decision-block__raw"><code>{decision.action}</code>{!decision.implicit && decision.line !== undefined && <span>{t('result.ruleLine', { line: decision.line })}</span>}</p>
      {decision.implicit ? (
        <p className="decision-block__rule decision-block__rule--implicit">{t('result.implicit')}</p>
      ) : (
        <div className="rule-slip">
          <span className="rule-slip__tag">{t('result.decisionLine')}</span>
          <code className="rule-slip__text">{text ?? t('result.ruleMissing')}</code>
          {lineChanged && <span className="rule-slip__note">{tag}</span>}
        </div>
      )}
    </div>
  );
}

export function SourceLines({
  title,
  lines,
  changed,
  matchedLine,
}: {
  title: string;
  lines: string[];
  changed: Set<number>;
  matchedLine?: number;
}) {
  const { t } = useI18n();
  return (
    <div className="card source-pane">
      <div className="source-pane__head">
        <h3>{title}</h3>
        <span>{t('source.lines', { count: lines.length })}</span>
      </div>
      <div className="source-pane__scroll" tabIndex={0} role="region" aria-label={`${title}. ${t('result.sourceCompare')}`}>
        <ol className="source-pane__lines">
          {lines.map((line, index) => {
            const number = index + 1;
            const matched = number === matchedLine;
            const isChanged = changed.has(number);
            return (
              <li key={number} className={`${isChanged ? 'is-changed' : ''} ${matched ? 'is-matched' : ''}`.trim()}>
                <span className="source-pane__number">{number}</span>
                <code>{line === '' ? ' ' : line}</code>
                {matched && <span className="source-pane__label">{t('result.decisionLine')}</span>}
                {isChanged && !matched && <span className="source-pane__label source-pane__label--change">{t('result.changedLine')}</span>}
              </li>
            );
          })}
        </ol>
      </div>
    </div>
  );
}

export function WitnessDetail({
  witness,
  title,
  before,
  after,
  changedBefore,
  changedAfter,
}: {
  witness: Witness;
  title: string;
  before: string;
  after: string;
  changedBefore: Set<number>;
  changedAfter: Set<number>;
}) {
  const { t } = useI18n();
  const beforeLines = sourceLines(before);
  const afterLines = sourceLines(after);
  const actionsDiffer = witness.before.action !== witness.after.action;
  return (
    <div className="witness-detail">
      <div className="witness-detail__head">
        <div>
          <p className="kicker">{title}</p>
          <h2 id="witness-detail-title">{t('result.witnessTitle')}</h2>
        </div>
        <ProvenanceTag tone="info" icon="shield">{t('result.runtime')}</ProvenanceTag>
      </div>
      <div className="witness-detail__id">
        <span>{t('packet.witnessId')}</span>
        <code title={witness.id}>{witness.id.slice(0, 18)}…</code>
      </div>
      <PacketRoute packet={witness.packet} />
      <div className="decision-path">
        <DecisionBlock
          label={t('result.decisionBefore')}
          decision={witness.before}
          source={before}
          changed={changedBefore}
          tag={t('result.changedLine')}
        />
        <div className="decision-path__between" aria-hidden="true">
          <Icon name="arrow" size={18} />
        </div>
        <DecisionBlock
          label={t('result.decisionAfter')}
          decision={witness.after}
          source={after}
          changed={changedAfter}
          tag={t('result.newLine')}
        />
      </div>
      {actionsDiffer && (
        <p className="decision-path__summary">
          {t('result.decisionChange', {
            before: actionLabel(t, witness.before.action),
            after: actionLabel(t, witness.after.action),
          })}
        </p>
      )}
      <p className="field-hint">{t('result.traceNote')}</p>

      <TechnicalDetails title={t('result.fullSource')}>
        <p className="field-hint">{t('result.sourceCompare')}</p>
        <div className="source-panes">
          <SourceLines title={t('common.before')} lines={beforeLines} changed={changedBefore} matchedLine={witness.before.line} />
          <SourceLines title={t('common.after')} lines={afterLines} changed={changedAfter} matchedLine={witness.after.line} />
        </div>
      </TechnicalDetails>

      <TechnicalDetails title={t('result.witnessDetails')}>
        <dl className="meta-list">
          <div><dt>{t('packet.protocol')}</dt><dd><code>{witness.packet.protocol}</code></dd></div>
          <div><dt>{t('packet.src')}</dt><dd><code>{witness.packet.src_ip}:{witness.packet.src_port}</code></dd></div>
          <div><dt>{t('packet.dst')}</dt><dd><code>{witness.packet.dst_ip}:{witness.packet.dst_port}</code></dd></div>
          <div><dt>{t('common.before')}</dt><dd><code>{witness.before.action}</code>{witness.before.implicit ? ` · ${t('common.implicitDeny')}` : ` · ${t('result.ordinal', { ordinal: witness.before.rule_ordinal ?? 0 })}`}</dd></div>
          <div><dt>{t('common.after')}</dt><dd><code>{witness.after.action}</code>{witness.after.implicit ? ` · ${t('common.implicitDeny')}` : ` · ${t('result.ordinal', { ordinal: witness.after.rule_ordinal ?? 0 })}`}</dd></div>
        </dl>
        <HashLine label={t('packet.witnessId')} value={witness.id} short={false} />
      </TechnicalDetails>
    </div>
  );
}
