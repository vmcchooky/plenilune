import type { Policy, RepairResult } from './types';

export type ReceiptBindingIssue =
  | 'schema'
  | 'before'
  | 'after'
  | 'request_before'
  | 'request_after'
  | 'obligation_digest'
  | 'request_obligation_digest'
  | 'policy_missing'
  | 'policy_not_accepted'
  | 'policy_hash'
  | 'scope';

export interface AcceptedReceiptBinding {
  beforeSha256: string;
  afterSha256: string;
  obligationsDigest: string;
  policySha256: string;
  scope: string;
}

export function bindingFromAccepted(accepted: {
  digest: string;
  policySha256: string;
  policy: Policy;
  parsed: { before: { sha256: string }; after: { sha256: string } };
}): AcceptedReceiptBinding {
  return {
    beforeSha256: accepted.parsed.before.sha256,
    afterSha256: accepted.parsed.after.sha256,
    obligationsDigest: accepted.digest,
    policySha256: accepted.policySha256,
    scope: accepted.policy.scope,
  };
}

// A receipt is displayed, receivable and exportable only when every pin matches
// the immutable accepted snapshot. Server validation is not a UI trust boundary.
export function receiptBindingIssue(receipt: RepairResult, accepted: AcceptedReceiptBinding): ReceiptBindingIssue | null {
  if (receipt.schema !== 'plenilune.repair/v1') return 'schema';
  if (receipt.before_sha256 !== accepted.beforeSha256) return 'before';
  if (receipt.base_after_sha256 !== accepted.afterSha256) return 'after';
  if (receipt.request?.base_before_sha256 !== accepted.beforeSha256) return 'request_before';
  if (receipt.request?.base_after_sha256 !== accepted.afterSha256) return 'request_after';
  if (receipt.obligations_digest !== accepted.obligationsDigest) return 'obligation_digest';
  if (receipt.request?.obligations_digest !== accepted.obligationsDigest) return 'request_obligation_digest';
  if (!receipt.policy) return 'policy_missing';
  if (!receipt.policy.operator_accepted) return 'policy_not_accepted';
  if (receipt.policy_sha256 !== accepted.policySha256) return 'policy_hash';
  if (receipt.policy.scope !== accepted.scope
    || receipt.conditions?.scope !== accepted.scope
    || receipt.request?.scope !== accepted.scope) return 'scope';
  return null;
}
