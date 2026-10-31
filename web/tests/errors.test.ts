import { describe, expect, it } from 'vitest';
import { translate } from '../src/i18n';
import type { MessageKey } from '../src/i18n/messages.en';
import { errorCopy } from '../src/lib/errors';

const vi = (key: MessageKey) => translate('vi', key);
const en = (key: MessageKey) => translate('en', key);

describe('review error copy', () => {
  it('explains a changed canonical policy in both locales', () => {
    const copyVi = errorCopy(vi, { code: 'POLICY_PREVIEW_MISMATCH', message: 'canonical policy changed' });
    const copyEn = errorCopy(en, { code: 'POLICY_PREVIEW_MISMATCH', message: 'canonical policy changed' });
    expect(copyVi.title).toContain('Policy đã thay đổi');
    expect(copyVi.hint).toContain('Xem lại');
    expect(copyEn.title).toContain('policy changed');
    expect(copyEn.hint).toContain('Review');
  });

  it('explains typed analysis request and deadline failures in both locales', () => {
    const inputVi = errorCopy(vi, { code: 'INPUT_TOO_LARGE', message: 'too large' });
    const inputEn = errorCopy(en, { code: 'INPUT_TOO_LARGE', message: 'too large' });
    expect(inputVi.title).toContain('Không phân tích');
    expect(inputVi.hint).toContain('Không có kết quả');
    expect(inputEn.title).toContain('Could not analyze');
    expect(inputEn.hint).toContain('No analysis result');
    const deadlineVi = errorCopy(vi, { code: 'DEADLINE_EXCEEDED', message: 'deadline' });
    const deadlineEn = errorCopy(en, { code: 'DEADLINE_EXCEEDED', message: 'deadline' });
    expect(deadlineVi.title).toContain('chưa hoàn tất');
    expect(deadlineEn.title).toContain('did not finish');
  });

  it('explains a receipt that is not bound to the accepted policy', () => {
    const copyVi = errorCopy(vi, { code: 'RECEIPT_BINDING_MISMATCH', message: 'foreign receipt' });
    const copyEn = errorCopy(en, { code: 'RECEIPT_BINDING_MISMATCH', message: 'foreign receipt' });
    expect(copyVi.title).toContain('không khớp policy');
    expect(copyVi.hint).toContain('Không nhận hoặc xuất');
    expect(copyEn.title).toContain('does not match the accepted policy');
    expect(copyEn.hint).toContain('Do not receive or export');
  });
});
