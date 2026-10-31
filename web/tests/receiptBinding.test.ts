import { describe, expect, it } from 'vitest';
import { bindingFromAccepted, receiptBindingIssue } from '../src/lib/receiptBinding';
import { accepted, foreignRepairReceipt, repairReceipt, scopeOnlyRepairReceipt } from './fixtures';

describe('receipt binding', () => {
  const binding = bindingFromAccepted(accepted);

  it('accepts a receipt pinned to the accepted input pair, obligation digest and policy hash', () => {
    expect(receiptBindingIssue(repairReceipt, binding)).toBeNull();
  });

  it('rejects a different before input', () => {
    expect(receiptBindingIssue(foreignRepairReceipt, binding)).toBe('before');
  });

  it('rejects a different after input', () => {
    expect(receiptBindingIssue({ ...repairReceipt, base_after_sha256: '2'.repeat(64) }, binding)).toBe('after');
  });

  it('rejects a different request input pin', () => {
    expect(receiptBindingIssue({ ...repairReceipt, request: { ...repairReceipt.request, base_before_sha256: '3'.repeat(64) } }, binding)).toBe('request_before');
  });

  it('rejects a different obligation digest', () => {
    expect(receiptBindingIssue({ ...repairReceipt, obligations_digest: '4'.repeat(64) }, binding)).toBe('obligation_digest');
  });

  it('rejects a scope-only policy change even when the obligation digest is unchanged', () => {
    expect(scopeOnlyRepairReceipt.obligations_digest).toBe(accepted.digest);
    expect(receiptBindingIssue(scopeOnlyRepairReceipt, binding)).toBe('policy_hash');
  });

  it('rejects an unaccepted or missing policy', () => {
    expect(receiptBindingIssue({ ...repairReceipt, policy: null }, binding)).toBe('policy_missing');
    expect(receiptBindingIssue({ ...repairReceipt, policy: { ...repairReceipt.policy!, operator_accepted: false } }, binding)).toBe('policy_not_accepted');
  });

  it('rejects a different policy scope', () => {
    expect(receiptBindingIssue({ ...repairReceipt, policy: { ...repairReceipt.policy!, scope: 'other-scope' } }, binding)).toBe('scope');
  });
});
