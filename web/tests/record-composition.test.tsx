import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { RecordComposition } from '../src/design/compositions/RecordComposition';
import { compositionCopy } from '../src/design/compositions/copy';
import type { CompositionModel } from '../src/design/compositions/model';
import { comparison, scenario } from './fixtures';

afterEach(cleanup);
function model(): CompositionModel {
  return {
    report: comparison.report, witness: comparison.report.witnesses[0], changes: null,
    name: 'MGMT', before: scenario.before, after: scenario.after, first: { before: 1, after: 1 },
    policyAccepted: false, conditionsUnresolved: false, incomplete: false, truncated: false,
    coverageGap: false,
  };
}
describe('refined change record safety notices', () => {
  it('shows incomplete and coverage warnings before opening proof details', () => {
    const data = { ...model(), incomplete: true, coverageGap: true, truncated: true };
    render(<RecordComposition model={data} copy={compositionCopy.vi} onConditions={() => {}} />);
    expect(screen.getByRole('heading', { level: 1 }).textContent).toBe(compositionCopy.vi.incomplete);
    const warning = screen.getByRole('status');
    expect(warning.textContent).toContain(compositionCopy.vi.coverage);
    expect(warning.textContent).toContain(compositionCopy.vi.truncated);
    expect(warning.closest('[hidden]')).toBeNull();
    expect(document.getElementById('record-rule-proof')?.hidden).toBe(true);
  });
  it('labels the point as an example and retains both port values', () => {
    const data = model();
    render(<RecordComposition model={data} copy={compositionCopy.en} onConditions={() => {}} />);
    const example = screen.getByRole('region', { name: compositionCopy.en.example });
    expect(example.textContent).toContain(`${data.witness.packet.src_ip}:${data.witness.packet.src_port}`);
    expect(example.textContent).toContain(`${data.witness.packet.dst_ip}:${data.witness.packet.dst_port}`);
    expect(screen.getByText(compositionCopy.en.limit + ' ' + compositionCopy.en.intent)).toBeTruthy();
  });
  it('retains destination port zero as a concrete packet value', () => {
    const data = model();
    data.witness = { ...data.witness, packet: { ...data.witness.packet, protocol: 17, dst_port: 0 } };
    render(<RecordComposition model={data} copy={compositionCopy.vi} onConditions={() => {}} />);
    const example = screen.getByRole('region', { name: compositionCopy.vi.example });
    expect(example.textContent).toContain(`${data.witness.packet.dst_ip}:0`);
    expect(example.textContent).not.toContain('(any)');
  });
});
