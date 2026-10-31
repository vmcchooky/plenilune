import type { MessageKey } from '../i18n/messages.en';
import type { MessageVars } from '../i18n';
import type {
  Interval,
  Obligation,
  ObligationKind,
  ObligationSet,
  Region,
} from './types';

export const OBLIGATION_SCHEMA = 'plenilune.obligations/v1';
export const SCOPE = 'ipv4-unfragmented-or-initial-with-complete-l4/v1';

export type Translate = (key: MessageKey, vars?: MessageVars) => string;

export interface RegionDraft {
  protocol: string;
  src_cidr: string;
  dst_cidr: string;
  exclude_src_text: string;
  exclude_dst_text: string;
  src_ports_text: string;
  dst_ports_text: string;
}

export interface ConditionDraft {
  id: string;
  kind: ObligationKind;
  region: RegionDraft;
}

export type DraftError =
  | { kind: 'count' }
  | { kind: 'condition'; conditionId: string }
  | { kind: 'region'; index: number };

export type BuildResult<T> = { value: T } | { error: DraftError };

const protocolWithPorts = new Set(['tcp', 'udp']);

export function emptyRegionDraft(protocol = 'ip'): RegionDraft {
  return {
    protocol,
    src_cidr: '0.0.0.0/0',
    dst_cidr: '0.0.0.0/0',
    exclude_src_text: '',
    exclude_dst_text: '',
    src_ports_text: '',
    dst_ports_text: '',
  };
}

export function emptyConditionDraft(id: string): ConditionDraft {
  return { id, kind: 'must_remain', region: emptyRegionDraft() };
}

export function regionDraft(region: Region): RegionDraft {
  return {
    protocol: region.protocol,
    src_cidr: region.src_cidr,
    dst_cidr: region.dst_cidr,
    exclude_src_text: region.exclude_src.join(', '),
    exclude_dst_text: region.exclude_dst.join(', '),
    src_ports_text: formatPortsInput(region.src_ports),
    dst_ports_text: formatPortsInput(region.dst_ports),
  };
}

export function conditionDraft(obligation: Obligation): ConditionDraft {
  return { id: obligation.id, kind: obligation.kind, region: regionDraft(obligation.match) };
}

export function parsePorts(text: string): Interval[] | null {
  const trimmed = text.trim();
  if (trimmed === '') return [];
  const intervals: Interval[] = [];
  for (const part of trimmed.split(',')) {
    const piece = part.trim();
    if (piece === '') return null;
    const match = /^(\d{1,5})(?:\s*-\s*(\d{1,5}))?$/.exec(piece);
    if (!match) return null;
    const lo = Number(match[1]);
    const hi = match[2] === undefined ? lo : Number(match[2]);
    if (lo > 65535 || hi > 65535 || lo > hi) return null;
    intervals.push({ lo, hi });
  }
  return intervals;
}

export function formatPortsInput(intervals: Interval[]): string {
  if (isFullPorts(intervals)) return '';
  return intervals.map((interval) => interval.lo === interval.hi ? `${interval.lo}` : `${interval.lo}-${interval.hi}`).join(', ');
}

export function formatIntervals(intervals: Interval[]): string | null {
  if (intervals.length === 0 || isFullPorts(intervals)) return null;
  return intervals.map((interval) => interval.lo === interval.hi ? `${interval.lo}` : `${interval.lo}–${interval.hi}`).join(', ');
}

export function isFullPorts(intervals: Interval[]): boolean {
  return intervals.length === 1 && intervals[0].lo === 0 && intervals[0].hi === 65535;
}

export function parsePrefixList(text: string): string[] {
  return text.split(',').map((item) => item.trim()).filter((item) => item !== '');
}

export function regionFromDraft(draft: RegionDraft): Region | null {
  const portsAllowed = protocolWithPorts.has(draft.protocol);
  const srcPorts = portsAllowed ? parsePorts(draft.src_ports_text) : [];
  const dstPorts = portsAllowed ? parsePorts(draft.dst_ports_text) : [];
  if (!srcPorts || !dstPorts) return null;
  return {
    protocol: draft.protocol,
    src_cidr: draft.src_cidr.trim(),
    dst_cidr: draft.dst_cidr.trim(),
    exclude_src: parsePrefixList(draft.exclude_src_text),
    exclude_dst: parsePrefixList(draft.exclude_dst_text),
    src_ports: srcPorts,
    dst_ports: dstPorts,
  };
}

export function conditionFromDraft(draft: ConditionDraft): Obligation | null {
  const region = regionFromDraft(draft.region);
  if (!region) return null;
  return { id: draft.id, kind: draft.kind, match: region };
}

export function buildObligationSet(drafts: ConditionDraft[]): BuildResult<ObligationSet> {
  if (drafts.length < 1 || drafts.length > 12) return { error: { kind: 'count' } };
  const ids = new Set<string>();
  const obligations: Obligation[] = [];
  for (const draft of drafts) {
    if (ids.has(draft.id)) return { error: { kind: 'condition', conditionId: draft.id } };
    ids.add(draft.id);
    const obligation = conditionFromDraft(draft);
    if (!obligation) return { error: { kind: 'condition', conditionId: draft.id } };
    obligations.push(obligation);
  }
  return {
    value: {
      schema: OBLIGATION_SCHEMA,
      scope: SCOPE,
      obligations: obligations.slice().sort((a, b) => a.id.localeCompare(b.id, 'en', { numeric: true })),
    },
  };
}

export function buildRegions(drafts: RegionDraft[]): BuildResult<Region[]> {
  const regions: Region[] = [];
  for (let index = 0; index < drafts.length; index++) {
    const region = regionFromDraft(drafts[index]);
    if (!region) return { error: { kind: 'region', index } };
    regions.push(region);
  }
  return { value: regions };
}

export function isWideRegion(draft: RegionDraft): boolean {
  const srcAny = draft.src_cidr === '0.0.0.0/0' || draft.src_cidr === 'any';
  const dstAny = draft.dst_cidr === '0.0.0.0/0' || draft.dst_cidr === 'any';
  return draft.protocol === 'ip'
    && srcAny
    && dstAny
    && draft.exclude_src_text.trim() === ''
    && draft.exclude_dst_text.trim() === ''
    && draft.src_ports_text.trim() === ''
    && draft.dst_ports_text.trim() === '';
}

export function conditionPortError(draft: ConditionDraft): boolean {
  if (!protocolWithPorts.has(draft.region.protocol)) return false;
  return parsePorts(draft.region.src_ports_text) === null || parsePorts(draft.region.dst_ports_text) === null;
}

export function nextConditionId(conditions: ConditionDraft[]): string | null {
  if (conditions.length >= 12) return null;
  const used = new Set(conditions.map((condition) => condition.id));
  for (let n = 1; n <= 99; n++) {
    const id = `OB-${n}`;
    if (!used.has(id)) return id;
  }
  return null;
}

export function kindLabel(t: Translate, kind: ObligationKind): string {
  return t(`cond.kind.${kind}`);
}

export function kindHelp(t: Translate, kind: ObligationKind): string {
  return t(`cond.kindHelp.${kind}`);
}

export function protocolLabel(t: Translate, protocol: string): string {
  switch (protocol) {
    case 'ip': return 'IP';
    case 'tcp': return 'TCP';
    case 'udp': return 'UDP';
    case 'icmp': return 'ICMP';
    default:
      return /^\d+$/.test(protocol) ? t('format.protocolNumber', { n: protocol }) : protocol;
  }
}

export function actionLabel(t: Translate, action: 'permit' | 'deny'): string {
  return action === 'permit' ? t('common.permit') : t('common.deny');
}

export function portsPhrase(t: Translate, region: Region): string {
  const src = formatIntervals(region.src_ports);
  const dst = formatIntervals(region.dst_ports);
  if (src && dst) return t('format.obligation.portsBoth', { srcPorts: src, dstPorts: dst });
  if (src) return t('format.obligation.srcPorts', { ports: src });
  if (dst) return t('format.obligation.ports', { ports: dst });
  return '';
}

export function excludesPhrase(t: Translate, region: Region): string {
  const src = region.exclude_src.join(', ');
  const dst = region.exclude_dst.join(', ');
  if (src && dst) return t('format.obligation.excludesBoth', { src, dst });
  if (src) return t('format.obligation.excludesSrc', { src });
  if (dst) return t('format.obligation.excludesDst', { dst });
  return '';
}

export function obligationSentence(t: Translate, obligation: Obligation): string {
  const vars = {
    protocol: protocolLabel(t, obligation.match.protocol),
    src: obligation.match.src_cidr,
    dst: obligation.match.dst_cidr,
    ports: portsPhrase(t, obligation.match),
    excludes: excludesPhrase(t, obligation.match),
  };
  if (obligation.kind === 'must_permit') return t('format.obligation.permit', vars);
  if (obligation.kind === 'must_deny') return t('format.obligation.deny', vars);
  return t('format.obligation.remain', vars);
}

export function regionSentence(t: Translate, region: Region): string {
  return t('format.region.sentence', {
    protocol: protocolLabel(t, region.protocol),
    src: region.src_cidr,
    dst: region.dst_cidr,
    ports: portsPhrase(t, region),
    excludes: excludesPhrase(t, region),
  });
}

export function conditionSentence(t: Translate, draft: ConditionDraft): string {
  const obligation = conditionFromDraft(draft);
  if (obligation) return obligationSentence(t, obligation);
  return regionSentence(t, {
    protocol: draft.region.protocol,
    src_cidr: draft.region.src_cidr,
    dst_cidr: draft.region.dst_cidr,
    exclude_src: [],
    exclude_dst: [],
    src_ports: [],
    dst_ports: [],
  });
}

export function sourceLines(source: string): string[] {
  const stable = source.replace(/\r\n/g, '\n');
  const withoutFinal = stable.endsWith('\n') ? stable.slice(0, -1) : stable;
  return withoutFinal === '' ? [] : withoutFinal.split('\n');
}

export function sourceLine(source: string, line: number | undefined): string | null {
  if (!line) return null;
  const lines = sourceLines(source);
  return lines[line - 1] ?? null;
}

export function logicalLineCount(value: string): number {
  if (value === '') return 0;
  const count = value.split('\n').length;
  return value.endsWith('\n') ? count - 1 : count;
}
