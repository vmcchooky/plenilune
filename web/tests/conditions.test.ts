import { describe, expect, it } from 'vitest';
import {
  buildObligationSet,
  conditionDraft,
  conditionPortError,
  emptyConditionDraft,
  formatPortsInput,
  isWideRegion,
  nextConditionId,
  obligationSentence,
  parsePorts,
  regionFromDraft,
  sourceLine,
  sourceLines,
} from '../src/lib/conditions';
import { translate } from '../src/i18n';
import { scenario } from './fixtures';

const t = (key: Parameters<typeof translate>[1], vars?: Parameters<typeof translate>[2]) => translate('vi', key, vars);

describe('port and region drafts', () => {
  it('parses single ports, ranges and multiple intervals', () => {
    expect(parsePorts('22')).toEqual([{ lo: 22, hi: 22 }]);
    expect(parsePorts('8000-8080')).toEqual([{ lo: 8000, hi: 8080 }]);
    expect(parsePorts('22, 443')).toEqual([{ lo: 22, hi: 22 }, { lo: 443, hi: 443 }]);
    expect(parsePorts('')).toEqual([]);
    expect(parsePorts('70000')).toBeNull();
    expect(parsePorts('90-80')).toBeNull();
    expect(parsePorts('ssh')).toBeNull();
  });

  it('treats the full port range as all ports in display and input', () => {
    expect(formatPortsInput([{ lo: 0, hi: 65535 }])).toBe('');
    expect(formatPortsInput([{ lo: 22, hi: 22 }])).toBe('22');
    expect(obligationSentence(t, scenario.conditions[0])).toBe('Cho phép TCP từ 192.0.2.0/24 đến 198.51.100.10/32 ở cổng đích 22.');
  });

  it('builds a canonical set and rejects invalid port text', () => {
    const draft = scenario.conditions.map(conditionDraft);
    const built = buildObligationSet(draft);
    expect('value' in built).toBe(true);
    if ('value' in built) {
      expect(built.value.schema).toBe('plenilune.obligations/v1');
      expect(built.value.obligations.map((item) => item.id)).toEqual(['OB-1', 'OB-2', 'OB-3']);
    }

    const invalid = [{ ...draft[0], region: { ...draft[0].region, dst_ports_text: 'not-a-port' } }];
    expect('error' in buildObligationSet(invalid)).toBe(true);
    expect(conditionPortError(invalid[0])).toBe(true);
  });

  it('keeps excluded and non-TCP regions valid for the server', () => {
    const draft = {
      ...emptyConditionDraft('OB-9'),
      region: {
        ...emptyConditionDraft('OB-9').region,
        protocol: 'ip',
        src_ports_text: '22',
        exclude_src_text: '192.0.2.99/32',
      },
    };
    const region = regionFromDraft(draft.region);
    expect(region).not.toBeNull();
    expect(region?.protocol).toBe('ip');
    expect(region?.src_ports).toEqual([]);
    expect(region?.exclude_src).toEqual(['192.0.2.99/32']);
    expect(conditionPortError(draft)).toBe(false);
  });

  it('distinguishes a wide region and allocates the next free ID', () => {
    expect(isWideRegion(emptyConditionDraft('OB-1').region)).toBe(true);
    expect(nextConditionId([emptyConditionDraft('OB-1'), emptyConditionDraft('OB-3')])).toBe('OB-2');
    expect(nextConditionId(Array.from({ length: 12 }, (_, index) => emptyConditionDraft(`OB-${index + 1}`)))).toBeNull();
  });

  it('splits CRLF and trailing newlines without shifting source lines', () => {
    const source = 'line one\r\nline two\r\n';
    expect(sourceLines(source)).toEqual(['line one', 'line two']);
    expect(sourceLine(source, 2)).toBe('line two');
    expect(sourceLine(source, 9)).toBeNull();
  });
});
