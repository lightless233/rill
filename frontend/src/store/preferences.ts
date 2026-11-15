import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import {
  cjkFontOptions,
  defaultCJKFont,
  defaultLatinFont,
  defaultMonoFont,
  latinFontOptions,
  monoFontOptions,
  type CJKFont,
  type LatinFont,
  type MonoFont,
} from "@/lib/fonts";

import type { TranslationMode } from "@/lib/api/types";

export { cjkFontOptions, latinFontOptions, monoFontOptions };
export type { CJKFont, LatinFont, MonoFont };

export const translationModeOptions = ["bilingual", "zh"] as const;

export const supportedLocales = [
  "en",
  "zh",
  "de",
  "fr",
  "es",
  "ru",
  "pt",
  "sv",
] as const;
export type AppLocale = (typeof supportedLocales)[number];

export const articlePageSizeOptions = [10, 20, 30, 50, 100] as const;
export type ArticlePageSize = (typeof articlePageSizeOptions)[number];

// Closed set mirrored by the backend handler's allow-list. "open" marks an
// article read the moment it is opened; the numeric strings are delays in
// seconds. "off" is a real value (not null): null on the server means "never
// chosen" and the client falls back to "off".
export const autoMarkReadOptions = ["off", "open", "5", "10", "30"] as const;
export type AutoMarkRead = (typeof autoMarkReadOptions)[number];

const localeSet = new Set<AppLocale>(supportedLocales);
const articlePageSizeSet = new Set<number>(articlePageSizeOptions);
const autoMarkReadSet = new Set<AutoMarkRead>(autoMarkReadOptions);
const latinFontSet = new Set<LatinFont>(latinFontOptions);
const cjkFontSet = new Set<CJKFont>(cjkFontOptions);
const monoFontSet = new Set<MonoFont>(monoFontOptions);
const translationModeSet = new Set<TranslationMode>(translationModeOptions);

function resolveSupportedLocale(locale: string): AppLocale | null {
  const normalized = locale.toLowerCase().replace("_", "-");

  if (localeSet.has(normalized as AppLocale)) {
    return normalized as AppLocale;
  }

  const languageCode = normalized.split("-")[0];
  if (localeSet.has(languageCode as AppLocale)) {
    return languageCode as AppLocale;
  }

  return null;
}

const defaultLocale: AppLocale =
  (typeof navigator !== "undefined" &&
    resolveSupportedLocale(navigator.language)) ||
  "en";
const defaultArticlePageSize: ArticlePageSize = 10;
const defaultAutoMarkRead: AutoMarkRead = "off";
const defaultTranslationMode: TranslationMode = "bilingual";

function normalizeLocale(locale: string): AppLocale {
  const supportedLocale = resolveSupportedLocale(locale);
  if (supportedLocale) {
    return supportedLocale;
  }

  return defaultLocale;
}

function normalizeArticlePageSize(size: number): ArticlePageSize {
  if (articlePageSizeSet.has(size)) {
    return size as ArticlePageSize;
  }

  return defaultArticlePageSize;
}

// Accepts undefined (key missing from an older persisted blob) and any
// garbage; both fall back to "off". A truthiness check would be wrong here:
// every non-"off" member is a meaningful value.
function normalizeAutoMarkRead(value: string | undefined | null): AutoMarkRead {
  return value !== null && autoMarkReadSet.has(value as AutoMarkRead)
    ? (value as AutoMarkRead)
    : defaultAutoMarkRead;
}

function normalizeLatinFont(value: string | undefined | null): LatinFont {
  return value !== null && latinFontSet.has(value as LatinFont)
    ? (value as LatinFont)
    : defaultLatinFont;
}

function normalizeCJKFont(value: string | undefined | null): CJKFont {
  return value !== null && cjkFontSet.has(value as CJKFont)
    ? (value as CJKFont)
    : defaultCJKFont;
}

function normalizeMonoFont(value: string | undefined | null): MonoFont {
  return value !== null && monoFontSet.has(value as MonoFont)
    ? (value as MonoFont)
    : defaultMonoFont;
}

function normalizeTranslationMode(
  value: string | undefined | null,
): TranslationMode {
  return value !== null && translationModeSet.has(value as TranslationMode)
    ? (value as TranslationMode)
    : defaultTranslationMode;
}

// "off" disables the feature; every other member maps to a delay where 0
// means "as soon as the article is open".
export function autoMarkReadDelayMs(option: AutoMarkRead): number | null {
  if (option === "off") {
    return null;
  }
  return option === "open" ? 0 : Number.parseInt(option, 10) * 1000;
}

export interface PreferencesState {
  locale: AppLocale;
  articlePageSize: ArticlePageSize;
  autoMarkRead: AutoMarkRead;
  latinFont: LatinFont;
  cjkFont: CJKFont;
  monoFont: MonoFont;
  translationMode: TranslationMode;
  setLocale: (locale: string) => void;
  setArticlePageSize: (size: number) => void;
  setAutoMarkRead: (option: string) => void;
  setLatinFont: (font: string) => void;
  setCJKFont: (font: string) => void;
  setMonoFont: (font: string) => void;
  setTranslationMode: (mode: string) => void;
}

export const usePreferencesStore = create<PreferencesState>()(
  persist(
    (set) => ({
      locale: defaultLocale,
      articlePageSize: defaultArticlePageSize,
      autoMarkRead: defaultAutoMarkRead,
      latinFont: defaultLatinFont,
      cjkFont: defaultCJKFont,
      monoFont: defaultMonoFont,
      translationMode: defaultTranslationMode,
      setLocale: (locale) => set({ locale: normalizeLocale(locale) }),
      setArticlePageSize: (size) =>
        set({ articlePageSize: normalizeArticlePageSize(size) }),
      setAutoMarkRead: (option) =>
        set({ autoMarkRead: normalizeAutoMarkRead(option) }),
      setLatinFont: (font) => set({ latinFont: normalizeLatinFont(font) }),
      setCJKFont: (font) => set({ cjkFont: normalizeCJKFont(font) }),
      setMonoFont: (font) => set({ monoFont: normalizeMonoFont(font) }),
      setTranslationMode: (mode) =>
        set({ translationMode: normalizeTranslationMode(mode) }),
    }),
    {
      name: "fusion-preferences",
      storage: createJSONStorage(() => localStorage),
      partialize: (state) => ({
        locale: state.locale,
        articlePageSize: state.articlePageSize,
        autoMarkRead: state.autoMarkRead,
        latinFont: state.latinFont,
        cjkFont: state.cjkFont,
        monoFont: state.monoFont,
        translationMode: state.translationMode,
      }),
      merge: (persistedState, currentState) => {
        const persisted = persistedState as Partial<PreferencesState> | undefined;

        return {
          ...currentState,
          locale: normalizeLocale(persisted?.locale ?? currentState.locale),
          articlePageSize: normalizeArticlePageSize(
            persisted?.articlePageSize ?? currentState.articlePageSize,
          ),
          autoMarkRead: normalizeAutoMarkRead(
            persisted?.autoMarkRead ?? currentState.autoMarkRead,
          ),
          latinFont: normalizeLatinFont(
            persisted?.latinFont ?? currentState.latinFont,
          ),
          cjkFont: normalizeCJKFont(
            persisted?.cjkFont ?? currentState.cjkFont,
          ),
          monoFont: normalizeMonoFont(
            persisted?.monoFont ?? currentState.monoFont,
          ),
          translationMode: normalizeTranslationMode(
            persisted?.translationMode ?? currentState.translationMode,
          ),
        };
      },
    },
  ),
);

export function getPreferredLocale(): AppLocale {
  return usePreferencesStore.getState().locale;
}

export function isSupportedLocale(locale: string): locale is AppLocale {
  return localeSet.has(locale as AppLocale);
}

export function isArticlePageSize(size: number): size is ArticlePageSize {
  return articlePageSizeSet.has(size);
}

export function isAutoMarkRead(value: string): value is AutoMarkRead {
  return autoMarkReadSet.has(value as AutoMarkRead);
}

export function isLatinFont(value: string): value is LatinFont {
  return latinFontSet.has(value as LatinFont);
}

export function isCJKFont(value: string): value is CJKFont {
  return cjkFontSet.has(value as CJKFont);
}

export function isMonoFont(value: string): value is MonoFont {
  return monoFontSet.has(value as MonoFont);
}

export function isTranslationMode(value: string): value is TranslationMode {
  return translationModeSet.has(value as TranslationMode);
}
