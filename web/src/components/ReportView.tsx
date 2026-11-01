import { useI18n } from '../i18n';
import { languageNames } from '../i18n';
import {
  actionLabel,
  kindLabel,
  obligationSentence,
  sourceLine,
  sourceLines,
} from '../lib/conditions';
import { changedLines } from '../lib/lineDiff';
import { formatTimestamp } from '../lib/format';
import type { ReportInput } from '../lib/report';
import {
  aiStatusLabel,
  collateralStatusLabel,
  obligationStatusList,
  outcomeLabel,
  repairReasonLabel,
  repairStatusLabel,
  replayLabel,
  scopeStatusLabel,
  stopReasonLabel,
  verdictBody,
  verdictTitle,
} from '../lib/labels';
import type { Witness } from '../lib/types';
import { collectWitnesses } from '../lib/witnesses';
import { PacketRoute } from './PacketRoute';
import { SourceLines } from './WitnessDetail';
import { HashLine, StatusMark } from './Ui';

function ReportWitness({
  witness,
  label,
  before,
  after,
}: {
  witness: Witness;
  label: string;
  before: string;
  after: string;
}) {
  const { t } = useI18n();
  const beforeRule = sourceLine(before, witness.before.line);
  const afterRule = sourceLine(after, witness.after.line);
  return (
    <div className="report-witness">
      <div className="report-witness__head">
        <h4>{label}</h4>
        <code title={witness.id}>{witness.id}</code>
      </div>
      <PacketRoute packet={witness.packet} />
      <dl className="report-decisions">
        <div>
          <dt>{t('result.decisionBefore')}</dt>
          <dd>
            {actionLabel(t, witness.before.action)}
            {witness.before.implicit ? ` · ${t('common.implicitDeny')}` : ` · ${t('result.ruleLine', { line: witness.before.line ?? 0 })}`}
            {beforeRule && <code className="report-rule">{beforeRule.trim()}</code>}
          </dd>
        </div>
        <div>
          <dt>{t('result.decisionAfter')}</dt>
          <dd>
            {actionLabel(t, witness.after.action)}
            {witness.after.implicit ? ` · ${t('common.implicitDeny')}` : ` · ${t('result.ruleLine', { line: witness.after.line ?? 0 })}`}
            {afterRule && <code className="report-rule">{afterRule.trim()}</code>}
          </dd>
        </div>
      </dl>
    </div>
  );
}

export function ReportView({ input }: { input: ReportInput }) {
  const { t, locale } = useI18n();
  const { accepted, comparison, repair, comparisonReplay, repairReplay, lastAi, sourceKind, generatedAt } = input;
  const report = comparison.comparison.report;
  const entries = collectWitnesses(t, report);
  const candidateLines = repair?.receipt.candidate ? sourceLines(repair.receipt.candidate) : [];
  const candidateChanged = repair?.receipt.candidate
    ? changedLines(sourceLines(accepted.after), candidateLines).after
    : new Set<number>();

  return (
    <article className="report-view" aria-labelledby="report-view-title">
      <header className="report-view__head">
        <p className="kicker">{t('evidence.reportView')}</p>
        <h2 id="report-view-title">{t('report.title')}</h2>
        <p>{t('report.note')}</p>
        <p className="field-hint">{t('evidence.generated', { time: formatTimestamp(locale, generatedAt) })}</p>
        <dl className="report-meta">
          <div><dt>{t('report.language')}</dt><dd>{languageNames[locale]}</dd></div>
          <div><dt>{t('report.source')}</dt><dd>{sourceKind === 'sample' ? t('evidence.sample') : t('evidence.custom')}</dd></div>
        </dl>
      </header>

      <section className="report-section" aria-labelledby="report-verdict">
        <h3 id="report-verdict">{t('report.section.verdict')}</h3>
        <p className="report-verdict">
          <strong>{verdictTitle(t, report.verdict)}</strong>
          <code className="badge">{report.verdict}</code>
        </p>
        <p>{verdictBody(t, report.verdict, report.reason)}</p>
        <p className="field-hint">
          {obligationStatusList(t, report.obligations)}
          {' · '}
          {t('result.searchComplete')}: {report.search_complete ? t('result.yes') : t('result.no')}
          {report.witnesses_truncated ? ` · ${t('result.truncated')}` : ''}
        </p>
      </section>

      <section className="report-section" aria-labelledby="report-intent">
        <h3 id="report-intent">{t('report.section.intent')}</h3>
        <p className="field-hint">{t('evidence.intent')}</p>
        <pre className="report-intent">{accepted.intent}</pre>
      </section>

      <section className="report-section" aria-labelledby="report-conditions">
        <h3 id="report-conditions">{t('report.section.conditions')}</h3>
        <ul className="report-list">
          {report.obligations.map((outcome) => {
            const definition = accepted.conditions.obligations.find((item) => item.id === outcome.id);
            return (
              <li key={outcome.id}>
                <span className="report-list__id"><code>{outcome.id}</code>{definition && <span>{kindLabel(t, definition.kind)}</span>}</span>
                <span className="report-list__body">{definition ? obligationSentence(t, definition) : outcome.id}{outcome.reason ? <small> · {t('common.reason')}: {stopReasonLabel(t, outcome.reason)}</small> : null}</span>
                <span className="report-list__outcome">
                  <StatusMark tone={outcome.status === 'HOLDS' ? 'good' : outcome.status === 'VIOLATED' ? 'bad' : 'warn'} label={outcomeLabel(t, outcome.status)} />
                </span>
              </li>
            );
          })}
        </ul>
      </section>

      <section className="report-section" aria-labelledby="report-witnesses">
        <h3 id="report-witnesses">{t('report.section.witnesses')}</h3>
        {entries.length === 0 ? <p>{t('report.noWitness')}</p> : (
          <div className="report-witnesses">
            {entries.map((entry) => (
              <ReportWitness
                key={`${entry.label}-${entry.witness.id}`}
                witness={entry.witness}
                label={entry.label}
                before={accepted.before}
                after={accepted.after}
              />
            ))}
          </div>
        )}
      </section>

      <section className="report-section" aria-labelledby="report-repair">
        <h3 id="report-repair">{t('report.section.repair')}</h3>
        {repair ? (
          <>
            <p className="report-verdict">
              <strong>{repairStatusLabel(t, repair.receipt.status)}</strong>
              <span className="field-label">{repair.source === 'ai' ? t('repair.actor.ai') : t('repair.actor.manual')}</span>
            </p>
            {repair.receipt.reason && <p>{repairReasonLabel(t, repair.receipt.reason)}</p>}
            {repair.receipt.policy && (
              <p className="field-hint">
                {t('scope.title')}: {scopeStatusLabel(
                  t,
                  repair.receipt.policy.allowed_changes === null ? 'undeclared' : repair.receipt.policy.allowed_changes.length === 0 ? 'none' : 'regions',
                  repair.receipt.policy.allowed_changes?.length ?? 0,
                )}
              </p>
            )}
            {repair.receipt.comparison ? (
              <ul className="report-list report-list--compact">
                {repair.receipt.comparison.report.obligations.map((outcome) => (
                  <li key={outcome.id}>
                    <span><code>{outcome.id}</code></span>
                    <span><StatusMark tone={outcome.status === 'HOLDS' ? 'good' : outcome.status === 'VIOLATED' ? 'bad' : 'warn'} label={outcomeLabel(t, outcome.status)} /></span>
                  </li>
                ))}
              </ul>
            ) : <p>{t('repair.comparisonMissing')}</p>}
            <p>
              {t('repair.collateral')}: <StatusMark
                tone={repair.receipt.collateral.status === 'VERIFIED' ? 'good' : repair.receipt.collateral.status === 'FAILED' ? 'bad' : 'warn'}
                label={collateralStatusLabel(t, repair.receipt.collateral.status)}
              />
            </p>
            {repair.receipt.collateral.reason && <p className="field-hint">{t('common.reason')}: {repair.receipt.collateral.reason}</p>}
            {repair.receipt.collateral.witness && (
              <ReportWitness
                witness={repair.receipt.collateral.witness}
                label={t('repair.collateralWitness')}
                before={accepted.before}
                after={repair.receipt.candidate ?? accepted.after}
              />
            )}
            <p className="field-hint">
              {t('repair.replay')}: {repairReplay ? replayLabel(t, repairReplay.result.status) : t('report.noReplay')}
            </p>
            {repair.receipt.candidate && (
              <details className="report-details">
                <summary>{t('repair.candidate')}</summary>
                <SourceLines title={t('repair.candidate')} lines={candidateLines} changed={candidateChanged} />
                <p className="field-hint">{t('repair.candidateHelp')}</p>
              </details>
            )}
          </>
        ) : <p>{t('report.noRepair')}</p>}
      </section>

      <section className="report-section" aria-labelledby="report-ai">
        <h3 id="report-ai">{t('report.section.ai')}</h3>
        {lastAi ? (
          <>
            <p>{aiResponded(lastAi.status) ? t('report.aiUsed') : t('report.aiNotUsed')}</p>
            <dl className="report-meta">
              <div><dt>{t('common.status')}</dt><dd>{aiStatusLabel(t, lastAi.status)}</dd></div>
              <div><dt>{t('ai.model')}</dt><dd><code>{lastAi.provenance.model}</code></dd></div>
              <div><dt>{t('ai.provider')}</dt><dd><code>{lastAi.provenance.provider}</code></dd></div>
            </dl>
            {aiResponded(lastAi.status) && lastAi.summary && <blockquote className="report-quote">{lastAi.summary}</blockquote>}
            <p className="field-hint">{t('ai.notVerdict')}</p>
          </>
        ) : <p>{t('report.aiNotUsed')}</p>}
      </section>

      <section className="report-section" aria-labelledby="report-trace">
        <h3 id="report-trace">{t('report.section.trace')}</h3>
        <dl className="report-meta">
          <div><dt>{t('result.scope')}</dt><dd><code>{comparison.comparison.scope}</code></dd></div>
          <div><dt>{t('evidence.parser')}</dt><dd><code>{comparison.comparison.parser_version}</code></dd></div>
          <div><dt>{t('evidence.engine')}</dt><dd><code>{comparison.comparison.engine_version}</code></dd></div>
          <div><dt>{t('result.verdictRaw')}</dt><dd><code>{report.verdict}</code></dd></div>
        </dl>
        <div className="hash-stack">
          <HashLine label={t('result.comparisonHash')} value={comparison.comparison.sha256} short={false} />
          <HashLine label={t('result.inputHash', { side: t('common.before') })} value={comparison.comparison.before.sha256} short={false} />
          <HashLine label={t('result.inputHash', { side: t('common.after') })} value={comparison.comparison.after.sha256} short={false} />
          {repair?.receipt.sha256 && <HashLine label={t('repair.receiptHash')} value={repair.receipt.sha256} short={false} />}
        </div>
        {comparisonReplay && (
          <p className="field-hint">{t('evidence.replayComparison')}: {replayLabel(t, comparisonReplay.result.status)}</p>
        )}
      </section>

      <section className="report-section" aria-labelledby="report-limits">
        <h3 id="report-limits">{t('report.section.limits')}</h3>
        <ul className="report-limits">
          <li>{t('evidence.limit.trace')}</li>
          <li>{t('evidence.limit.intent')}</li>
          <li>{t('evidence.limit.ai')}</li>
          <li>{t('evidence.limit.device')}</li>
          <li>{t('evidence.limit.scope')}</li>
        </ul>
      </section>
    </article>
  );
}

function aiResponded(status: string): boolean {
  return status === 'completed' || status === 'partial' || status === 'clarification_needed';
}

