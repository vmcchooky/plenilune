import type { Translate } from './conditions';
import type { ComparisonReport, Witness } from './types';

export interface WitnessEntry {
  key: string;
  witness: Witness;
  label: string;
  conditionId?: string;
}

function packetKey(witness: Witness): string {
  const packet = witness.packet;
  return [
    packet.protocol,
    packet.src_ip,
    packet.src_port,
    packet.dst_ip,
    packet.dst_port,
    witness.before.action,
    witness.before.line ?? 'implicit',
    witness.after.action,
    witness.after.line ?? 'implicit',
  ].join('|');
}

export function collectWitnesses(t: Translate, report: ComparisonReport): WitnessEntry[] {
  const entries: WitnessEntry[] = [];
  const seen = new Set<string>();

  for (const outcome of report.obligations) {
    if (!outcome.witness) continue;
    const key = packetKey(outcome.witness);
    if (seen.has(key)) continue;
    seen.add(key);
    entries.push({
      key: `condition-${outcome.id}`,
      witness: outcome.witness,
      label: t('result.conditionLabel', { id: outcome.id }),
      conditionId: outcome.id,
    });
  }

  for (const witness of report.witnesses) {
    const key = packetKey(witness);
    if (seen.has(key)) continue;
    seen.add(key);
    entries.push({
      key: `difference-${witness.id}`,
      witness,
      label: t('result.differenceWitness'),
    });
  }

  return entries;
}
