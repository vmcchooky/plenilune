import { describe, expect, it } from 'vitest';
import { translate } from '../src/i18n';
import { decisionRefLabel, resultNextKey } from '../src/lib/labels';

const t = (key: Parameters<typeof translate>[1], vars?: Parameters<typeof translate>[2]) => translate('en', key, vars);

describe('result priority line', () => {
  it('names the deciding rule and line in each version', () => {
    expect(decisionRefLabel(t, { action: 'deny', line: 3, implicit: false })).toBe('Deny · Line 3');
    expect(decisionRefLabel(t, { action: 'permit', line: 4, implicit: false }, ' 20 permit tcp any any eq 443 '))
      .toBe('Permit · Line 4 — 20 permit tcp any any eq 443');
  });

  it('names an implicit deny instead of inventing a line', () => {
    expect(decisionRefLabel(t, { action: 'deny', implicit: true })).toBe('Deny · Implicit deny');
  });

  it('points to policy, repair, retry or evidence from the current state', () => {
    expect(resultNextKey({ accepted: false, canRepair: false, hasRepair: false, inconclusive: false })).toBe('result.next.policy');
    expect(resultNextKey({ accepted: true, canRepair: true, hasRepair: false, inconclusive: false })).toBe('result.next.repair');
    expect(resultNextKey({ accepted: true, canRepair: false, hasRepair: false, inconclusive: true })).toBe('result.next.retry');
    expect(resultNextKey({ accepted: true, canRepair: true, hasRepair: true, inconclusive: false })).toBe('result.next.evidence');
    expect(resultNextKey({ accepted: true, canRepair: false, hasRepair: false, inconclusive: false })).toBe('result.next.evidence');
  });
});
