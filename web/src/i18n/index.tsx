import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import en from './messages.en';
import type { MessageKey } from './messages.en';
import vi from './messages.vi';

export type Locale = 'vi' | 'en';

export type MessageVars = Record<string, string | number>;

const dictionaries: Record<Locale, Record<MessageKey, string>> = { vi, en };

export const LOCALE_STORAGE_KEY = 'plenilune.locale';

export const languageNames: Record<Locale, string> = {
  vi: 'Tiếng Việt',
  en: 'English',
};

export function isLocale(value: unknown): value is Locale {
  return value === 'vi' || value === 'en';
}

export function readStoredLocale(): Locale {
  try {
    const value = window.localStorage.getItem(LOCALE_STORAGE_KEY);
    return isLocale(value) ? value : 'vi';
  } catch {
    return 'vi';
  }
}

export function translate(locale: Locale, key: MessageKey, vars?: MessageVars): string {
  const template = dictionaries[locale][key] ?? en[key];
  if (!vars) return template;
  return template.replace(/\{([a-z_]+)\}/gi, (match, name: string) => {
    const value = vars[name];
    return value === undefined ? match : String(value);
  });
}

interface I18nValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: MessageKey, vars?: MessageVars) => string;
}

const I18nContext = createContext<I18nValue | null>(null);

export function LocaleProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(readStoredLocale);

  useEffect(() => {
    try {
      window.localStorage.setItem(LOCALE_STORAGE_KEY, locale);
    } catch {
      // A blocked storage area must not break the review.
    }
    document.documentElement.lang = locale;
    document.title = `Plenilune — ${translate(locale, 'app.tagline')}`;
    const description = document.querySelector('meta[name="description"]');
    if (description) description.setAttribute('content', translate(locale, 'app.description'));
  }, [locale]);

  const setLocale = useCallback((next: Locale) => setLocaleState(next), []);
  const t = useCallback((key: MessageKey, vars?: MessageVars) => translate(locale, key, vars), [locale]);
  const value = useMemo(() => ({ locale, setLocale, t }), [locale, setLocale, t]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const value = useContext(I18nContext);
  if (!value) throw new Error('useI18n must be used inside LocaleProvider');
  return value;
}
