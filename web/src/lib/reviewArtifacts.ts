import { ApiError } from './api';
import type { Locale } from '../i18n';
import type { RepairResult, ReplayResult } from './types';

export const MAX_SOURCE_BYTES = 2 << 20;
export const MAX_CONFIG_BYTES = 4 << 20;
export const MAX_ANALYSIS_BYTES = 16 << 20;
export const MAX_RECEIPT_BYTES = 16 << 20;
export const MAX_COMPARISON_BYTES = 4 << 20;

export interface ImportedFile {
  name: string;
  text: string;
  bytes: Uint8Array;
}

export interface ImportedReceipt {
  kind: 'comparison' | 'repair' | 'analysis';
  schema: string;
  sha256: string;
  originalOutcome: string;
}

// ignoreBOM=true preserves U+FEFF rather than silently stripping its bytes.
export function decodeUtf8(bytes: Uint8Array): string {
  return new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes);
}

export async function readReviewFile(file: File, maxBytes: number): Promise<ImportedFile> {
  if (file.size > maxBytes) throw new Error('FILE_TOO_LARGE');
  const bytes = new Uint8Array(await file.arrayBuffer());
  if (bytes.byteLength > maxBytes) throw new Error('FILE_TOO_LARGE');
  let text: string;
  try { text = decodeUtf8(bytes); } catch { throw new Error('INVALID_UTF8'); }
  return { name: file.name, text, bytes };
}

export function inspectReceipt(raw: string): ImportedReceipt {
  const value: unknown = JSON.parse(raw);
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('RECEIPT_INVALID');
  const receipt = value as Record<string, unknown>;
  const kind = receipt.schema === 'plenilune.evidence/v1' || receipt.schema === 'plenilune.evidence/v2' ? 'comparison'
    : receipt.schema === 'plenilune.repair/v1' ? 'repair'
      : receipt.schema === 'plenilune.analysis/v1' || receipt.schema === 'plenilune.analysis/v2' || receipt.schema === 'plenilune.analysis/v3' ? 'analysis' : null;
  if (!kind || typeof receipt.sha256 !== 'string' || !/^[0-9a-f]{64}$/.test(receipt.sha256)) {
    throw new Error('RECEIPT_INVALID');
  }
  const cap = kind === 'repair' ? 8 << 20 : kind === 'analysis' ? MAX_ANALYSIS_BYTES : MAX_COMPARISON_BYTES;
  if (new TextEncoder().encode(raw).byteLength > cap) throw new Error('FILE_TOO_LARGE');
  if (kind === 'analysis') {
    const summary = receipt.summary && typeof receipt.summary === 'object' ? receipt.summary as Record<string, unknown> : null;
    if (!summary || !Number.isSafeInteger(summary.objects) || !Number.isSafeInteger(summary.changed)) throw new Error('RECEIPT_INVALID');
    return { kind, schema: receipt.schema as string, sha256: receipt.sha256, originalOutcome: `objects=${summary.objects} changed=${summary.changed}` };
  }
  const report = receipt.report && typeof receipt.report === 'object' ? receipt.report as Record<string, unknown> : {};
  const originalOutcome = kind === 'repair' ? receipt.status : report.verdict;
  const allowed = kind === 'repair' ? ['VERIFIED', 'REJECTED', 'UNVERIFIED'] : ['EQUIVALENT', 'DIFFERENT', 'INCONCLUSIVE'];
  if (typeof originalOutcome !== 'string' || !allowed.includes(originalOutcome)) throw new Error('RECEIPT_INVALID');
  return { kind, schema: receipt.schema as string, sha256: receipt.sha256, originalOutcome };
}

export async function replayImported(
  before: ImportedFile,
  after: ImportedFile,
  receipt: ImportedFile,
  signal?: AbortSignal,
): Promise<ReplayResult> {
  const info = inspectReceipt(receipt.text);
  const sourceCap = info.kind === 'analysis' ? MAX_CONFIG_BYTES : MAX_SOURCE_BYTES;
  if (before.bytes.byteLength > sourceCap || after.bytes.byteLength > sourceCap) throw new Error('FILE_TOO_LARGE');
  // Keep receipt JSON verbatim so duplicate keys/unknown fields are rejected by
  // the Go strict decoder rather than being hidden by JSON.parse/stringify.
  const body = `{"before":${JSON.stringify(before.text)},"after":${JSON.stringify(after.text)},"evidence":${receipt.text}}`;
  const cap = info.kind === 'repair' ? 16 << 20 : info.kind === 'analysis' ? 26 << 20 : 12 << 20;
  if (new TextEncoder().encode(body).byteLength > cap) throw new ApiError('Imported replay exceeds request limit.', 'BODY_TOO_LARGE');
  let response: Response;
  try {
    const path = info.kind === 'repair' ? '/api/repair/replay' : info.kind === 'analysis' ? '/api/analyze/replay' : '/api/compare/replay';
    response = await fetch(path, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body, signal,
    });
  } catch (cause) {
    if (cause instanceof DOMException && cause.name === 'AbortError') throw cause;
    throw new ApiError('Local runtime is unavailable.', 'RUNTIME_UNAVAILABLE');
  }
  const text = await response.text();
  let value: unknown;
  try { value = JSON.parse(text); } catch { throw new ApiError('Invalid replay response.', 'RESPONSE_INVALID'); }
  if (!response.ok) {
    const error = value && typeof value === 'object' ? (value as { error?: { code?: string } }).error : undefined;
    throw new ApiError('Replay did not complete.', error?.code ?? 'REQUEST_FAILED', undefined, undefined, response.status);
  }
  const result = value as ReplayResult;
  const expected = info.kind === 'repair' ? 'REPAIR_REPLAYED' : info.kind === 'analysis' ? 'ANALYSIS_REPLAYED' : 'COMPARISON_REPLAYED';
  if (!result || (result.status !== expected && result.status !== 'TIMING_SENSITIVE')
    || result.sha256 !== info.sha256 || !Number.isSafeInteger(result.rechecked_witnesses) || result.rechecked_witnesses < 0) {
    throw new ApiError('Replay response does not match imported receipt.', 'RESPONSE_INVALID');
  }
  return result;
}

// The repair object is already Go-sealed. Extraction never recomputes its hash.
export function standaloneRepairRaw(receipt: RepairResult, raw: string): string {
  try {
    const parsed = JSON.parse(raw) as { schema?: string; sha256?: string };
    if (parsed.schema === 'plenilune.repair/v1' && parsed.sha256 === receipt.sha256) return raw;
  } catch { /* Fall back to serializing the server's parsed receipt object. */ }
  return JSON.stringify(receipt);
}

export function replayInstructions(locale: Locale, hasRepair: boolean, comparisonVersion: string, hasAnalysis = false): string {
  const vi = locale === 'vi';
  return [
    vi ? '# Kiểm tra lại độc lập — không cần AI' : '# Independent replay — no AI required', '',
    vi ? 'Giữ nguyên byte của before.acl và after.acl (kể cả CRLF, BOM và newline cuối).'
      : 'Preserve every byte of before.acl and after.acl, including CRLF, BOM and final newline.',
    `${vi ? 'Phiên bản tool của comparison' : 'Comparison tool version'}: ${comparisonVersion}.`,
    vi ? 'Dùng binary tương thích với các phiên bản trong receipt. Hash không xác thực tác giả.'
      : 'Use a binary compatible with the versions recorded in the receipt. Hashes do not authenticate the author.', '',
    'plenilune replay --before before.acl --after after.acl --evidence comparison.json',
    ...(hasRepair ? ['plenilune repair-replay --before before.acl --after after.acl --evidence repair.json'] : []),
    ...(hasAnalysis ? ['plenilune analyze-replay --before before.cfg --after after.cfg --evidence analysis.json'] : []), '',
    vi ? 'Hoặc mở Plenilune trong phiên sạch → Nhập hồ sơ để phát lại → chọn hai ACL và receipt → Phát lại.'
      : 'Or open a fresh Plenilune session → Import files for replay → select both ACLs and a receipt → Replay.',
    vi ? 'Receipt sửa từ AI là repair.json độc lập, không phải wrapper session. Replay dựng lại candidate từ after + patch.'
      : 'An AI repair receipt is a standalone repair.json, not the session wrapper. Replay rebuilds the candidate from after + patch.',
    vi ? 'Replay matched chỉ tái lập kết quả gốc; không biến REJECTED thành VERIFIED. TIMING_SENSITIVE chưa phải replay đầy đủ.'
      : 'Replay matched reproduces the original outcome; it does not upgrade REJECTED to VERIFIED. TIMING_SENSITIVE is not a full replay match.',
    vi ? 'Sai byte/hash/version: lấy đúng nguồn/build, không sửa receipt cho khớp. Các tệp có thể chứa cấu hình nhạy cảm.'
      : 'On byte/hash/version mismatch, use the correct source/build; do not edit the receipt to match. Files may contain sensitive configuration.', '',
  ].join('\n');
}

export const reviewCopy = {
  vi: {
    title: 'Nhập hồ sơ để phát lại',
    help: 'Dùng trong phiên sạch, không cần AI. Chọn đúng hai nguồn gốc và một receipt độc lập; nội dung tệp được giữ nguyên byte, không đưa qua ô soạn thảo.',
    before: 'Tệp cấu hình trước (ACL: 2 MiB; cấu hình đầy đủ: 4 MiB)', after: 'Tệp cấu hình sau (ACL: 2 MiB; cấu hình đầy đủ: 4 MiB)',
    receipt: 'Receipt độc lập (comparison 4 MiB; repair 8 MiB; analysis 16 MiB)',
    unchecked: 'Chưa kiểm tra lại', loading: 'Đang đọc tệp…', run: 'Phát lại hồ sơ', busy: 'Đang phát lại…', cancel: 'Hủy',
    matched: 'Đã tái lập kết quả gốc', timing: 'Kết quả phụ thuộc thời gian — chưa xác nhận replay đầy đủ',
    mismatch: 'Hồ sơ không khớp nguồn, hash hoặc phiên bản. Chưa xác nhận được bằng chứng.',
    error: 'Không hoàn tất replay. Giữ tệp và kiểm tra máy chủ/giới hạn trước khi thử lại.',
    canceled: 'Đã hủy; chưa có kết quả replay mới.', outcome: 'Kết luận gốc (chưa được nâng cấp bởi replay)',
    invalid: 'Tệp receipt không hợp lệ hoặc sai schema; không nhận session wrapper.',
    file: 'Không đọc được tệp. Chỉ nhận UTF-8 hợp lệ và tệp trong giới hạn.',
    hash: 'Hash receipt', downloadBefore: 'Tải nguồn trước nguyên byte', downloadAfter: 'Tải nguồn sau nguyên byte',
    downloadReceipt: 'Tải receipt sửa độc lập', instructions: 'Tải hướng dẫn replay', candidate: 'Tải candidate nguyên byte',
    artifactHelp: 'Giữ các tệp nguồn cùng receipt để đồng nghiệp có thể kiểm tra lại. Báo cáo dịch chỉ là view, không thay receipt.',
    decision: 'Quyết định của người review', received: 'Đã nhận để review', held: 'Giữ lại để bổ sung', rejected: 'Đã từ chối',
    undecided: 'Chưa có quyết định', disputed: 'Bằng chứng đang có sai lệch; chưa được nhận candidate',
    decisionNote: 'Quyết định là ghi chú của phiên giao diện, không xác thực danh tính người duyệt và không thay kết luận engine.',
  },
  en: {
    title: 'Import files for replay',
    help: 'Works in a fresh session without AI. Select both original sources and one standalone receipt; file bytes bypass text editors and remain unchanged.',
    before: 'Before configuration file (ACL 2 MiB; full configuration 4 MiB)', after: 'After configuration file (ACL 2 MiB; full configuration 4 MiB)',
    receipt: 'Standalone receipt (comparison 4 MiB; repair 8 MiB; analysis 16 MiB)',
    unchecked: 'Not replayed', loading: 'Reading file…', run: 'Replay imported files', busy: 'Replaying…', cancel: 'Cancel',
    matched: 'Original outcome reproduced', timing: 'Timing-sensitive — full replay not confirmed',
    mismatch: 'Receipt does not match source bytes, hash or version. Evidence has not been confirmed.',
    error: 'Replay did not complete. Keep the files and check the local runtime/limits before retrying.',
    canceled: 'Canceled; no new replay result.', outcome: 'Original outcome (replay does not upgrade it)',
    invalid: 'Invalid receipt or schema; session wrappers are not accepted.',
    file: 'Cannot read this file. Valid UTF-8 and file-size limits are required.',
    hash: 'Receipt hash', downloadBefore: 'Download exact before bytes', downloadAfter: 'Download exact after bytes',
    downloadReceipt: 'Download standalone repair receipt', instructions: 'Download replay instructions', candidate: 'Download exact candidate bytes',
    artifactHelp: 'Keep source files with the receipts so another reviewer can replay them. Translated reports are views, not replacement receipts.',
    decision: 'Human review decision', received: 'Received for review', held: 'Held for more information', rejected: 'Rejected by reviewer',
    undecided: 'No review decision', disputed: 'Evidence is disputed; candidate cannot be received',
    decisionNote: 'This decision is a UI-session annotation, not authenticated reviewer identity or a replacement engine verdict.',
  },
} as const;
