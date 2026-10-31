import { describe, expect, it } from 'vitest';
import {
  LOCALE_STORAGE_KEY,
  readStoredLocale,
  translate,
} from '../src/i18n';
import en from '../src/i18n/messages.en';
import vi from '../src/i18n/messages.vi';

function placeholders(value: string): string[] {
  return [...value.matchAll(/\{([a-z_]+)\}/gi)].map((match) => match[1]).sort();
}

describe('VI/EN dictionary', () => {
  it('has exactly the same key set in both languages', () => {
    expect(Object.keys(vi).sort()).toEqual(Object.keys(en).sort());
  });

  it('has no empty messages', () => {
    for (const [key, value] of Object.entries(vi)) expect(value.trim(), key).not.toBe('');
    for (const [key, value] of Object.entries(en)) expect(value.trim(), key).not.toBe('');
  });

  it('keeps the same interpolation placeholders in both languages', () => {
    for (const key of Object.keys(en) as (keyof typeof en)[]) {
      expect(placeholders(vi[key]), key).toEqual(placeholders(en[key]));
    }
  });

  it('interpolates values without touching the raw evidence terms', () => {
    expect(translate('vi', 'result.conditionsSummary', { holds: 2, total: 3 })).toBe('Điều kiện đạt: 2/3');
    expect(translate('en', 'result.conditionsSummary', { holds: 2, total: 3 })).toBe('Conditions passed: 2/3');
    expect(translate('vi', 'result.verdictRaw')).toBe('Mã kết luận');
    expect(translate('vi', 'repair.status.VERIFIED')).toBe('Đã kiểm chứng');
  });

  it('defaults to Vietnamese and remembers a valid stored choice', () => {
    window.localStorage.removeItem(LOCALE_STORAGE_KEY);
    expect(readStoredLocale()).toBe('vi');
    window.localStorage.setItem(LOCALE_STORAGE_KEY, 'en');
    expect(readStoredLocale()).toBe('en');
    window.localStorage.setItem(LOCALE_STORAGE_KEY, 'fr');
    expect(readStoredLocale()).toBe('vi');
  });
});
