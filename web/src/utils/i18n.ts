import { useTranslation } from "react-i18next";
import i18n, { DEFAULT_LOCALE, locales } from "@/i18n";
import enTranslation from "@/locales/en.json";

const LOCALE_STORAGE_KEY = "memos-locale";
const LOCALE_DIRECTION_CHANGE_EVENT = "memos:locale-direction-change";

export type LocaleDirection = "ltr" | "rtl";

export const getLocaleDirection = (): LocaleDirection =>
  typeof document !== "undefined" && document.documentElement.dir === "rtl" ? "rtl" : "ltr";

export const subscribeToLocaleDirection = (listener: () => void): (() => void) => {
  if (typeof window === "undefined") return () => {};
  window.addEventListener(LOCALE_DIRECTION_CHANGE_EVENT, listener);
  return () => window.removeEventListener(LOCALE_DIRECTION_CHANGE_EVENT, listener);
};

export const applyDocumentLocale = (locale: string): LocaleDirection => {
  const direction = i18n.dir(locale);
  if (typeof document === "undefined") return direction;

  const previousDirection = document.documentElement.getAttribute("dir");
  document.documentElement.lang = locale;
  document.documentElement.dir = direction;
  if (previousDirection !== direction && typeof window !== "undefined") {
    window.dispatchEvent(new Event(LOCALE_DIRECTION_CHANGE_EVENT));
  }
  return direction;
};

const getStoredLocale = (): Locale | null => {
  try {
    const stored = localStorage.getItem(LOCALE_STORAGE_KEY);
    return stored && locales.includes(stored) ? (stored as Locale) : null;
  } catch {
    return null;
  }
};

const setStoredLocale = (locale: Locale): void => {
  try {
    localStorage.setItem(LOCALE_STORAGE_KEY, locale);
  } catch {
    // localStorage might not be available
  }
};

export const findNearestMatchedLanguage = (language: string): Locale => (/^en(?:-|$)/i.test(language) ? "en" : DEFAULT_LOCALE);

type NestedKeyOf<T, K = keyof T> = K extends keyof T & (string | number)
  ? `${K}` | (T[K] extends object ? `${K}.${NestedKeyOf<T[K]>}` : never)
  : never;

// A plural message is stored as `key_one`, `key_other`, … and looked up by its base `key`
// with a `count`, so i18next picks each language's own plural form.
type PluralBase<K> = K extends `${infer Base}_other` ? Base : never;

// Represents the keys of nested translation objects.
export type Translations = NestedKeyOf<typeof enTranslation> | PluralBase<NestedKeyOf<typeof enTranslation>>;

// Represents a typed translation function.
type TypedT = (key: Translations, params?: Record<string, unknown>) => string;

export const useTranslate = (): TypedT => {
  const { t } = useTranslation<Translations>();
  return t;
};

/** The same typed lookup for code outside React components, such as error helpers. */
export const translate: TypedT = (key, params) => i18n.t(key, params);

// Gets the locale to use with proper priority:
// 1. User setting (if logged in and has preference)
// 2. localStorage (from previous session)
// 3. Cat_two default
export const getLocaleWithFallback = (userLocale?: string): Locale => {
  // Priority 1: The signed-in account preference
  if (userLocale) {
    return findNearestMatchedLanguage(userLocale);
  }

  // Priority 2: localStorage
  const stored = getStoredLocale();
  if (stored) {
    return stored;
  }

  return DEFAULT_LOCALE;
};

// Applies and persists a locale setting
export const loadLocale = (locale: string): Locale => {
  const validLocale = findNearestMatchedLanguage(locale);
  setStoredLocale(validLocale);
  applyDocumentLocale(validLocale);
  i18n.changeLanguage(validLocale);
  return validLocale;
};

/**
 * Applies locale early during initial page load to prevent language flash.
 * Uses the browser’s saved preference or the Cat_two default until account settings load.
 */
export const applyLocaleEarly = (): void => {
  const stored = getStoredLocale();
  const locale = stored ?? DEFAULT_LOCALE;
  loadLocale(locale);
};

// Get the display name for a locale in its native language
export const getLocaleDisplayName = (locale: string): string => {
  try {
    if (locale === DEFAULT_LOCALE) return "简体中文";
    const displayName = new Intl.DisplayNames([locale], { type: "language" }).of(locale);
    if (displayName) {
      return displayName.charAt(0).toUpperCase() + displayName.slice(1);
    }
  } catch {
    // Intl.DisplayNames might not be available or might fail for some locales
  }
  return locale;
};

export const normalizeLocaleSearchText = (value: string): string => {
  return value
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLowerCase();
};

const getLocaleDisplayNameForLanguage = (locale: string, displayLanguage: string): string => {
  try {
    return new Intl.DisplayNames([displayLanguage], { type: "language" }).of(locale) ?? locale;
  } catch {
    return locale;
  }
};

export const getLocaleSearchLabels = (locale: string, uiLocale: string): string[] => {
  return Array.from(
    new Set([
      locale,
      getLocaleDisplayNameForLanguage(locale, locale),
      getLocaleDisplayNameForLanguage(locale, "en"),
      getLocaleDisplayNameForLanguage(locale, uiLocale),
    ]),
  );
};

export const localeMatchesSearch = (locale: string, query: string, uiLocale: string): boolean => {
  const normalizedQuery = normalizeLocaleSearchText(query.trim());
  if (!normalizedQuery) return true;

  return getLocaleSearchLabels(locale, uiLocale).some((label) => normalizeLocaleSearchText(label).includes(normalizedQuery));
};
