import { languageNames } from '../i18n';
import type { Locale } from '../i18n';
import type {
  AcceptedSnapshot,
  ComparisonSnapshot,
  RepairSnapshot,
  ReplaySnapshot,
  ReviewDecision,
} from '../hooks/useReview';
import {
  actionLabel,
  kindLabel,
  obligationSentence,
  sourceLine,
} from './conditions';
import type { Translate } from './conditions';
import { formatTimestamp } from './format';
import { partialReportLines } from './partial';
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
} from './labels';
import type { AIResult, Decision, Witness } from './types';
import { collectWitnesses } from './witnesses';
import { replayInstructions, reviewCopy } from './reviewArtifacts';

export interface ReportInput {
  locale: Locale;
  t: Translate;
  accepted: AcceptedSnapshot;
  comparison: ComparisonSnapshot;
  repair: RepairSnapshot | null;
  comparisonReplay: ReplaySnapshot | null;
  repairReplay: ReplaySnapshot | null;
  lastAi: AIResult | null;
  sourceKind: 'sample' | 'modified' | 'custom';
  generatedAt: Date;
  reviewDecision?: ReviewDecision | null;
  disputed?: boolean;
}

function indent(value: string): string {
  if (value === '') return '    (empty)';
  return value.split('\n').map((line) => `    ${line}`).join('\n');
}

function packetText(witness: Witness): string {
  const protocol = witness.packet.protocol === 6 ? 'TCP'
    : witness.packet.protocol === 17 ? 'UDP'
      : witness.packet.protocol === 1 ? 'ICMP'
        : String(witness.packet.protocol);
  const src = witness.packet.protocol === 6 || witness.packet.protocol === 17
    ? `${witness.packet.src_ip}:${witness.packet.src_port}`
    : witness.packet.src_ip;
  const dst = witness.packet.protocol === 6 || witness.packet.protocol === 17
    ? `${witness.packet.dst_ip}:${witness.packet.dst_port}`
    : witness.packet.dst_ip;
  return `${protocol} ${src} → ${dst}`;
}

// Artifact-only decision reference for the ticket summary: action and line
// number as recorded in the evidence, never the raw source line.
function decisionRefText(t: Translate, decision: Decision): string {
  const label = actionLabel(t, decision.action);
  if (decision.implicit) return `${label} (${t('common.implicitDeny')})`;
  return `${label} · ${t('result.ruleLine', { line: decision.line ?? 0 })}`;
}

function decisionText(t: Translate, action: 'permit' | 'deny', line: number | undefined, implicit: boolean, source: string): string {
  const label = actionLabel(t, action);
  if (implicit) return `${label} (${t('common.implicitDeny')})`;
  const rule = sourceLine(source, line);
  return `${label} · ${t('result.ruleLine', { line: line ?? 0 })}${rule ? ` — \`${rule.trim()}\`` : ''}`;
}

export function buildMarkdownReport(input: ReportInput): string {
  const { t, locale, accepted, comparison, repair, comparisonReplay, repairReplay, lastAi, sourceKind, generatedAt } = input;
  const report = comparison.comparison.report;
  const lines: string[] = [];
  const push = (line = '') => lines.push(line);

  push(`# ${t('report.title')}`);
  push('');
  push(`> ${t('report.note')}`);
  push('');
  push(`- ${t('report.generated', { time: formatTimestamp(locale, generatedAt) })}`);
  push(`- ${t('report.language')}: ${languageNames[locale]}`);
  push(`- ${t('report.source')}: ${sourceKind === 'sample' ? t('evidence.sample') : t('evidence.custom')}`);
  push(`- ${t('report.advisory')}`);
  push('');

  push(`## ${t('report.section.verdict')}`);
  push('');
  push(`**${verdictTitle(t, report.verdict)}** \`${report.verdict}\``);
  push('');
  push(verdictBody(t, report.verdict, report.reason));
  push('');
  push(`- ${obligationStatusList(t, report.obligations)}`);
  push(`- ${t('result.searchComplete')}: ${report.search_complete ? t('result.yes') : t('result.no')}`);
  if (report.partial) for (const line of partialReportLines(t, report.partial)) push(line);
  if (report.witnesses_truncated) push(`- ${t('result.truncated')}`);
  push('');

  push(`## ${t('report.section.intent')}`);
  push('');
  push(`_${t('evidence.intent')}_`);
  push('');
  push(indent(accepted.intent));
  push('');

  push(`## ${t('report.section.conditions')}`);
  push('');
  for (const outcome of report.obligations) {
    const definition = accepted.conditions.obligations.find((item) => item.id === outcome.id);
    const kind = definition ? `${kindLabel(t, definition.kind)} \`${definition.kind}\`` : '';
    const sentence = definition ? obligationSentence(t, definition) : outcome.id;
    push(`- **${outcome.id}** · ${kind}: ${sentence}`);
    push(`  - ${outcomeLabel(t, outcome.status)} \`${outcome.status}\`${outcome.reason ? ` — ${stopReasonLabel(t, outcome.reason)} (\`${outcome.reason}\`)` : ''}`);
  }
  push('');

  push(`## ${t('report.section.witnesses')}`);
  push('');
  const entries = collectWitnesses(t, report);
  if (entries.length === 0) {
    push(t('report.noWitness'));
    push('');
  } else {
    for (const entry of entries) {
      push(`### ${entry.label} · \`${entry.witness.id}\``);
      push('');
      push(`- ${t('packet.route')}: \`${packetText(entry.witness)}\``);
      push(`- ${t('result.decisionBefore')}: ${decisionText(t, entry.witness.before.action, entry.witness.before.line, entry.witness.before.implicit, accepted.before)}`);
      push(`- ${t('result.decisionAfter')}: ${decisionText(t, entry.witness.after.action, entry.witness.after.line, entry.witness.after.implicit, accepted.after)}`);
      push('');
    }
  }
  push(`> ${t('result.traceNote')}`);
  push('');

  push(`## ${t('report.section.repair')}`);
  push('');
  if (repair) {
    const receipt = repair.receipt;
    push(`**${repairStatusLabel(t, receipt.status)}** \`${receipt.status}\``);
    push('');
    push(`- ${t('report.source')}: ${repair.source === 'ai' ? t('repair.actor.ai') : t('repair.actor.manual')}`);
    if (receipt.reason) push(`- ${t('common.reason')}: ${repairReasonLabel(t, receipt.reason)} (\`${receipt.reason}\`)`);
    if (receipt.policy) {
      const mode = receipt.policy.allowed_changes === null ? 'undeclared' : receipt.policy.allowed_changes.length === 0 ? 'none' : 'regions';
      push(`- ${t('scope.title')}: ${scopeStatusLabel(t, mode, receipt.policy.allowed_changes?.length ?? 0)}`);
      push(`- ${t('accept.digest')}: \`${receipt.obligations_digest ?? ''}\``);
      push(`- ${t('repair.policyHash')}: \`${receipt.policy_sha256 ?? ''}\``);
    } else {
      push(`- ${t('repair.reason.policy_missing')}`);
    }
    push('');
    if (receipt.candidate) {
      push(`### ${t('repair.candidate')}`);
      push('');
      push(indent(receipt.candidate));
      push('');
    }
    if (receipt.comparison) {
      push(`### ${t('repair.obligations')}`);
      push('');
      for (const outcome of receipt.comparison.report.obligations) {
        const definition = receipt.conditions.obligations.find((item) => item.id === outcome.id);
        push(`- **${outcome.id}**${definition ? ` · ${kindLabel(t, definition.kind)} \`${definition.kind}\`` : ''}: ${outcomeLabel(t, outcome.status)} \`${outcome.status}\``);
      }
      push('');
    } else {
      push(t('repair.comparisonMissing'));
      push('');
    }
    push(`### ${t('repair.collateral')}`);
    push('');
    push(`${collateralStatusLabel(t, receipt.collateral.status)} \`${receipt.collateral.status}\``);
    push('');
    if (receipt.collateral.reason) push(`${t('common.reason')}: \`${receipt.collateral.reason}\``);
    push('');
    push(`### ${t('repair.replay')}`);
    push('');
    push(repairReplay ? `${replayLabel(t, repairReplay.result.status)} \`${repairReplay.result.status}\` · ${t('repair.replayCount', { count: repairReplay.result.rechecked_witnesses })}` : t('report.noReplay'));
    push('');
  } else {
    push(t('report.noRepair'));
    push('');
  }

  push(`## ${t('report.section.ai')}`);
  push('');
  if (lastAi) {
    push(`- ${t('common.status')}: ${aiStatusLabel(t, lastAi.status)} \`${lastAi.status}\``);
    push(`- ${t('ai.model')}: \`${lastAi.provenance.model}\``);
    push(`- ${t('ai.provider')}: \`${lastAi.provenance.provider}\``);
    push(`- ${lastAi.status === 'completed' || lastAi.status === 'partial' || lastAi.status === 'clarification_needed' ? t('report.aiUsed') : t('report.aiNotUsed')}`);
    if (lastAi.summary && lastAi.status !== 'not_configured' && lastAi.status !== 'unavailable' && lastAi.status !== 'canceled') {
      push('');
      push(`> ${t('cond.ai.summary')}: ${lastAi.summary.replace(/\s+/g, ' ').trim()}`);
    }
  } else {
    push(t('report.aiNotUsed'));
  }
  push('');

  push(`## ${t('report.section.trace')}`);
  push('');
  push(`- ${t('result.scope')}: \`${comparison.comparison.scope}\``);
  push(`- ${t('evidence.parser')}: \`${comparison.comparison.parser_version}\``);
  push(`- ${t('evidence.engine')}: \`${comparison.comparison.engine_version}\``);
  push(`- ${t('result.verdictRaw')}: \`${report.verdict}\``);
  push(`- ${t('result.comparisonHash')}: \`${comparison.comparison.sha256}\``);
  push(`- ${t('result.inputHash', { side: t('common.before') })}: \`${comparison.comparison.before.sha256}\``);
  push(`- ${t('result.inputHash', { side: t('common.after') })}: \`${comparison.comparison.after.sha256}\``);
  if (repair) {
    push(`- ${t('repair.receiptHash')}: \`${repair.receipt.sha256 ?? ''}\``);
    push(`- ${t('repair.candidateHash')}: \`${repair.receipt.candidate_sha256 ?? ''}\``);
  }
  if (comparisonReplay) {
    push(`- ${t('evidence.replayComparison')}: ${replayLabel(t, comparisonReplay.result.status)} \`${comparisonReplay.result.status}\``);
  }
  push('');

  push(`## ${t('report.section.limits')}`);
  push('');
  push(`- ${t('evidence.limit.trace')}`);
  push(`- ${t('evidence.limit.intent')}`);
  push(`- ${t('evidence.limit.ai')}`);
  push(`- ${t('evidence.limit.device')}`);
  push(`- ${t('evidence.limit.scope')}`);
  push('');
  const copy = reviewCopy[locale];
  const decision = input.reviewDecision?.receiptSha256 === repair?.receipt.sha256 ? input.reviewDecision : null;
  push(`## ${copy.decision}`);
  push('');
  push(input.disputed ? copy.disputed : decision ? copy[decision.choice] : copy.undecided);
  if (decision) push(`${copy.hash}: \`${decision.receiptSha256}\``);
  push(copy.decisionNote);
  push('');
  push(replayInstructions(locale, Boolean(repair), comparison.comparison.tool.version));
  return lines.join('\n');
}

// Deterministic ticket summary: a compact Markdown digest generated only
// from the evidence artifacts (comparison + optional repair receipt). It
// carries no timestamp, no session state, no AI prose and no raw
// configuration text — deciding rules are cited by line number, exactly as
// recorded in the artifact. Same artifact in, same summary out. The JSON
// artifact remains authoritative.
export function buildTicketSummary(input: ReportInput): string {
  const { t, comparison, repair } = input;
  const report = comparison.comparison.report;
  const lines: string[] = [];
  const push = (line = '') => lines.push(line);

  push(`# ${t('ticket.title')}`);
  push('');
  push(`> ${t('ticket.note')}`);
  push('');
  push(`**${verdictTitle(t, report.verdict)}** \`${report.verdict}\``);
  push('');
  push(verdictBody(t, report.verdict, report.reason));
  push('');
  push(`- ${obligationStatusList(t, report.obligations)}`);
  push(`- ${t('result.searchComplete')}: ${report.search_complete ? t('result.yes') : t('result.no')}`);
  if (report.partial) for (const line of partialReportLines(t, report.partial)) push(line);
  if (report.witnesses_truncated) push(`- ${t('result.truncated')}`);
  push('');

  push(`## ${t('report.section.witnesses')}`);
  push('');
  const entries = collectWitnesses(t, report);
  if (entries.length === 0) {
    push(t('report.noWitness'));
  } else {
    for (const entry of entries) {
      push(`- ${entry.label} · \`${packetText(entry.witness)}\`: ${t('result.decisionBefore')}: ${decisionRefText(t, entry.witness.before)} · ${t('result.decisionAfter')}: ${decisionRefText(t, entry.witness.after)}`);
    }
  }
  push('');

  push(`## ${t('report.section.repair')}`);
  push('');
  if (repair) {
    const receipt = repair.receipt;
    push(`**${repairStatusLabel(t, receipt.status)}** \`${receipt.status}\``);
    push('');
    push(`- ${t('repair.collateral')}: ${collateralStatusLabel(t, receipt.collateral.status)} \`${receipt.collateral.status}\``);
  } else {
    push(t('report.noRepair'));
  }
  push('');

  push(`## ${t('report.section.limits')}`);
  push('');
  push(`- ${t('evidence.limit.trace')}`);
  push(`- ${t('evidence.limit.intent')}`);
  push(`- ${t('evidence.limit.ai')}`);
  push(`- ${t('evidence.limit.device')}`);
  push(`- ${t('evidence.limit.scope')}`);
  push('');

  push(`## ${t('ticket.hashAndReplay')}`);
  push('');
  push(`- ${t('result.comparisonHash')}: \`${comparison.comparison.sha256}\``);
  push(`- ${t('result.inputHash', { side: t('common.before') })}: \`${comparison.comparison.before.sha256}\``);
  push(`- ${t('result.inputHash', { side: t('common.after') })}: \`${comparison.comparison.after.sha256}\``);
  if (repair) {
    push(`- ${t('repair.receiptHash')}: \`${repair.receipt.sha256 ?? ''}\``);
  }
  push('');
  push('```sh');
  push('plenilune replay --before before.acl --after after.acl --evidence comparison.json');
  if (repair) {
    push('plenilune repair-replay --before before.acl --after after.acl --evidence repair.json');
  }
  push('```');
  return lines.join('\n');
}
