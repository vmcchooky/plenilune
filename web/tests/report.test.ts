import { describe, expect, it } from 'vitest';
import { translate } from '../src/i18n';
import { buildMarkdownReport } from '../src/lib/report';
import { reviewCopy } from '../src/lib/reviewArtifacts';
import type { ReportInput } from '../src/lib/report';
import {
  accepted,
  comparisonSnapshot,
  repairSnapshot,
  scenario,
} from './fixtures';

const generatedAt = new Date('2026-10-30T10:00:00Z');

function input(overrides: Partial<ReportInput> = {}): ReportInput {
  return {
    locale: 'vi',
    t: (key, vars) => translate('vi', key, vars),
    accepted,
    comparison: comparisonSnapshot,
    repair: repairSnapshot,
    comparisonReplay: null,
    repairReplay: null,
    lastAi: null,
    sourceKind: 'sample',
    generatedAt,
    ...overrides,
  };
}

describe('translated Markdown report', () => {
  it('reads the conclusion and conditions in Vietnamese while keeping raw data intact', () => {
    const markdown = buildMarkdownReport(input());
    expect(markdown).toContain('# Plenilune — Báo cáo kiểm chứng thay đổi ACL');
    expect(markdown).toContain('## Kết luận');
    expect(markdown).toContain('Có thay đổi hành vi');
    expect(markdown).toContain('OB-1 không đạt · OB-2 đạt · OB-3 đạt');
    expect(markdown).not.toContain('Điều kiện đạt:');
    expect(markdown).toContain('Cho phép TCP từ 192.0.2.0/24 đến 198.51.100.10/32 ở cổng đích 22.');
    expect(markdown).toContain('Đã kiểm chứng');
    expect(markdown).toContain('192.0.2.10');
    expect(markdown).toContain(comparisonSnapshot.comparison.sha256);
    expect(markdown).toContain('`VIOLATED`');
    expect(markdown).toContain('DIFFERENT');
    expect(markdown).toContain('AI không được dùng.');
  });

  it('keeps the operator intent untouched, as an indented data block', () => {
    const markdown = buildMarkdownReport(input());
    const intentBlock = scenario.intent.split('\n').map((line) => `    ${line}`).join('\n');
    expect(markdown).toContain(intentBlock);
    expect(markdown).toContain('Ý định (dữ liệu gốc, không dịch)');
  });

  it('renders the English reader view without translating the ACL or IPs', () => {
    const markdown = buildMarkdownReport(input({
      locale: 'en',
      t: (key, vars) => translate('en', key, vars),
    }));
    expect(markdown).toContain('# Plenilune — ACL change verification report');
    expect(markdown).toContain('## Conclusion');
    expect(markdown).toContain('Behavior changes');
    expect(markdown).toContain('OB-1 violated · OB-2 holds · OB-3 holds');
    expect(markdown).not.toContain('Conditions passed:');
    expect(markdown).toContain('Permit TCP from 192.0.2.0/24');
    expect(markdown).toContain('Verified');
    expect(markdown).toContain('192.0.2.10:0 → 198.51.100.10:22');
  });

  it('states that no witness or repair exists when the evidence has none', () => {
    const noWitness = {
      ...comparisonSnapshot,
      comparison: {
        ...comparisonSnapshot.comparison,
        report: {
          ...comparisonSnapshot.comparison.report,
          verdict: 'EQUIVALENT' as const,
          witnesses: [],
          obligations: comparisonSnapshot.comparison.report.obligations.map((item) => ({ id: item.id, status: 'HOLDS' as const })),
        },
      },
    };
    const markdown = buildMarkdownReport(input({ comparison: noWitness, repair: null }));
    expect(markdown).toContain('Không có gói tin minh chứng.');
    expect(markdown).toContain('Không có biên bản sửa.');
    expect(markdown).toContain('Không thay đổi hành vi');
    expect(markdown).not.toContain('## Bản sửa\n\n**Đã kiểm chứng**');
  });

  it('keeps a human decision bound to its receipt and records disputes without upgrading the engine result', () => {
    const stale = buildMarkdownReport(input({ reviewDecision: { choice: 'received', status: 'current', receiptSha256: '0'.repeat(64) } }));
    expect(stale).toContain('Chưa có quyết định');
    expect(stale).not.toContain('Đã nhận để review');
    const disputed = buildMarkdownReport(input({
      reviewDecision: { choice: 'received', status: 'current', receiptSha256: repairSnapshot.receipt.sha256! }, disputed: true,
    }));
    expect(disputed).toContain('Bằng chứng đang có sai lệch; chưa được nhận candidate');
    expect(disputed).toContain('không xác thực danh tính người duyệt');
    expect(disputed).toContain('replay --before before.acl --after after.acl --evidence comparison.json');
  });
});


it('P3-4 exports a stale warning even if the receipt digest is unchanged', () => {
  const markdown = buildMarkdownReport(input({ reviewDecision: {
    choice: 'received', status: 'stale', receiptSha256: repairSnapshot.receipt.sha256!,
  } }));
  expect(markdown).toContain('Quyết định trước đã hết hiệu lực');
  expect(markdown).not.toContain('Đã nhận để review');
});

for (const locale of ['vi', 'en'] as const) {
  for (const choice of ['received', 'held', 'rejected'] as const) {
    it(`P3-4 ${locale} reports stale ${choice} without presenting it as current`, () => {
      const markdown = buildMarkdownReport(input({ locale, t: (key, vars) => translate(locale, key, vars),
        reviewDecision: { choice, status: 'stale', receiptSha256: repairSnapshot.receipt.sha256! },
      }));
      expect(markdown).toContain(reviewCopy[locale].stale);
      expect(markdown).not.toContain(`\n${reviewCopy[locale][choice]}\n`);
      expect(markdown).toContain(reviewCopy[locale].decisionNote);
    });
  }
}
