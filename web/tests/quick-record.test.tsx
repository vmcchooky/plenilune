import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { QuickOutcome } from '../src/components/QuickOutcome';
import type { Review } from '../src/hooks/useReview';
import { LocaleProvider } from '../src/i18n';
import messages from '../src/i18n/messages.vi';
import type { ChangeClass } from '../src/lib/types';
import { changeClass, scenario, witness } from './fixtures';

afterEach(cleanup);

// Presentation fixture: SSH is newly denied; a separate RDP rule is newly
// permitted. This test checks selection, not the engine's proof of either class.
const rdp: ChangeClass = {
  ...changeClass, effect: 'newly_permitted',
  before: { action: 'deny', ordinal: 2, line: 5, implicit: false },
  after: { action: 'permit', ordinal: 3, line: 6, implicit: false },
  witness: { ...witness, id: 'wit-rdp', packet: { ...witness.packet, dst_port: 3389 },
    before: { action: 'deny', rule_ordinal: 2, line: 5, implicit: false },
    after: { action: 'permit', rule_ordinal: 3, line: 6, implicit: false } },
  examples: [{ ...changeClass.examples[0], dst_ports: '3389' }],
};
const budget = { operations: 10, peak_boxes: 2, stop_reason: '', timing_limited: false };

function presentationReview(): Review {
  // Only the read surface used by this presentation is provided; no runtime or
  // acceptance operation belongs in the fixture.
  return {
    comparison: null, analysis: null, parsed: null, accepted: null,
    draft: { after: scenario.after.replace(' 50 deny', ' 45 permit tcp any any eq 3389\n 50 deny') },
    selectedObject: { domain: 'acl', name: 'MGMT', acl: {
      verdict: 'DIFFERENT', search_complete: true, witnesses: [witness],
      budget,
    } },
    aclChanges: { schema: 'plenilune.acl-changes/v1', engine: 'acl-exact/v0.2',
      complete: true, scope_declared: false, total_classes: 2, classes_truncated: false,
      classes: [changeClass, rdp], budget },
  } as Review;
}

describe('quick record change selection', () => {
  it('requires an explicit group choice and displays that group’s actual example', () => {
    const policy = vi.fn();
    const source = vi.fn();
    render(<LocaleProvider><QuickOutcome review={presentationReview()} onPolicy={policy} onSource={source} /></LocaleProvider>);
    const list = screen.getByRole('region', { name: messages['quick.groupList'] });
    const choices = within(list).getAllByRole('button');
    expect(choices.every(button => button.getAttribute('aria-pressed') === 'false')).toBe(true);
    expect(document.querySelector('.quick-outcome__example')).toBeNull();

    fireEvent.click(choices[1]);
    expect(choices[1].getAttribute('aria-pressed')).toBe('true');
    expect(document.querySelector('.quick-outcome__example')?.textContent).toContain(':3389');
    expect(screen.getByRole('heading', { level: 2, name: messages['quick.newPermit'] })).toBeTruthy();
    fireEvent.click(choices[0]);
    expect(choices[1].getAttribute('aria-pressed')).toBe('false');
    expect(document.querySelector('.quick-outcome__example')?.textContent).toContain(':22');
    expect(policy).not.toHaveBeenCalled();
    expect(source).not.toHaveBeenCalled();
  });
});
