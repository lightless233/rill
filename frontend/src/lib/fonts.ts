import type { TranslationKey } from "@/lib/i18n/messages";

export const latinFontOptions = [
  "geist",
  "inter",
  "source-sans-3",
  "system-sans",
  "newsreader",
  "source-serif-4",
  "lora",
  "system-serif",
] as const;
export type LatinFont = (typeof latinFontOptions)[number];

export const cjkFontOptions = [
  "system",
  "noto-sans-sc",
  "oppo-sans",
  "system-sans",
  "noto-serif-sc",
  "lxgw-wenkai",
  "system-serif",
  "system-kai",
] as const;
export type CJKFont = (typeof cjkFontOptions)[number];

export const monoFontOptions = [
  "geist-mono",
  "jetbrains-mono",
  "fira-code",
  "system-mono",
] as const;
export type MonoFont = (typeof monoFontOptions)[number];

export const defaultLatinFont: LatinFont = "geist";
export const defaultCJKFont: CJKFont = "system";
export const defaultMonoFont: MonoFont = "geist-mono";

export type FontCategory = "sans" | "serif";

export interface FontOptionMeta<T extends string> {
  value: T;
  category: FontCategory | "mono";
  family: string;
  label?: string;
  labelKey?: TranslationKey;
}

const SYSTEM_CJK_FALLBACK =
  '"PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans CJK SC", "Source Han Sans SC", "WenQuanYi Micro Hei"';

// Keep Western stacks strictly free of CJK-bearing generic aliases like
// system-ui or -apple-system so Chinese characters always fall through to the
// user's selected CJK font instead of being intercepted by OS UI fonts.
export const LATIN_FONT_OPTIONS: readonly FontOptionMeta<LatinFont>[] = [
  { value: "geist", category: "sans", family: '"Geist Variable"', label: "Geist" },
  { value: "inter", category: "sans", family: '"Inter Variable"', label: "Inter" },
  {
    value: "source-sans-3",
    category: "sans",
    family: '"Source Sans 3 Variable"',
    label: "Source Sans 3",
  },
  {
    value: "system-sans",
    category: "sans",
    family: '"SF Pro Text", "Segoe UI", "Helvetica Neue", Helvetica, Arial',
    labelKey: "settings.fonts.option.systemSans",
  },
  {
    value: "newsreader",
    category: "serif",
    family: '"Newsreader Variable"',
    label: "Newsreader",
  },
  {
    value: "source-serif-4",
    category: "serif",
    family: '"Source Serif 4 Variable"',
    label: "Source Serif 4",
  },
  { value: "lora", category: "serif", family: '"Lora Variable"', label: "Lora" },
  {
    value: "system-serif",
    category: "serif",
    family: 'Georgia, Cambria, "Times New Roman", Times',
    labelKey: "settings.fonts.option.systemSerif",
  },
];

export const CJK_FONT_OPTIONS: readonly FontOptionMeta<CJKFont>[] = [
  {
    value: "system",
    category: "sans",
    family: `ui-sans-serif, -apple-system, BlinkMacSystemFont, ${SYSTEM_CJK_FALLBACK}, sans-serif`,
    labelKey: "settings.fonts.option.systemDefault",
  },
  {
    value: "noto-sans-sc",
    category: "sans",
    family: `"Noto Sans SC Variable", ${SYSTEM_CJK_FALLBACK}, sans-serif`,
    labelKey: "settings.fonts.option.notoSansSC",
  },
  {
    value: "oppo-sans",
    category: "sans",
    family: `"OPPO Sans 4.0", ${SYSTEM_CJK_FALLBACK}, sans-serif`,
    label: "OPPO Sans 4.0",
  },
  {
    value: "system-sans",
    category: "sans",
    family: `${SYSTEM_CJK_FALLBACK}, sans-serif`,
    labelKey: "settings.fonts.option.cjkSystemSans",
  },
  {
    value: "noto-serif-sc",
    category: "serif",
    family:
      '"Noto Serif SC Variable", "Songti SC", "STSong", "Noto Serif CJK SC", SimSun, serif',
    labelKey: "settings.fonts.option.notoSerifSC",
  },
  {
    value: "lxgw-wenkai",
    category: "serif",
    family: '"LXGW WenKai Screen", "Kaiti SC", STKaiti, KaiTi, serif',
    labelKey: "settings.fonts.option.lxgwWenkai",
  },
  {
    value: "system-serif",
    category: "serif",
    family:
      '"Songti SC", "STSong", "Noto Serif CJK SC", "Source Han Serif SC", SimSun, NSimSun, serif',
    labelKey: "settings.fonts.option.cjkSystemSerif",
  },
  {
    value: "system-kai",
    category: "serif",
    family: '"Kaiti SC", STKaiti, KaiTi, FangSong, STFangsong, serif',
    labelKey: "settings.fonts.option.cjkSystemKai",
  },
];

export const MONO_FONT_OPTIONS: readonly FontOptionMeta<MonoFont>[] = [
  {
    value: "geist-mono",
    category: "mono",
    family: '"Geist Mono Variable"',
    label: "Geist Mono",
  },
  {
    value: "jetbrains-mono",
    category: "mono",
    family: '"JetBrains Mono Variable"',
    label: "JetBrains Mono",
  },
  {
    value: "fira-code",
    category: "mono",
    family: '"Fira Code Variable"',
    label: "Fira Code",
  },
  {
    value: "system-mono",
    category: "mono",
    family:
      'ui-monospace, "SFMono-Regular", Menlo, Monaco, Consolas, "Liberation Mono", "Courier New"',
    labelKey: "settings.fonts.option.systemMono",
  },
];

const LATIN_FONT_MAP = Object.fromEntries(
  LATIN_FONT_OPTIONS.map((opt) => [opt.value, opt]),
) as Record<LatinFont, FontOptionMeta<LatinFont>>;

const CJK_FONT_MAP = Object.fromEntries(
  CJK_FONT_OPTIONS.map((opt) => [opt.value, opt]),
) as Record<CJKFont, FontOptionMeta<CJKFont>>;

const MONO_FONT_MAP = Object.fromEntries(
  MONO_FONT_OPTIONS.map((opt) => [opt.value, opt]),
) as Record<MonoFont, FontOptionMeta<MonoFont>>;

export function resolveArticleBodyFontFamily(
  latinFont: LatinFont,
  cjkFont: CJKFont,
): string {
  return `${LATIN_FONT_MAP[latinFont].family}, ${CJK_FONT_MAP[cjkFont].family}`;
}

export function resolveArticleMonoFontFamily(
  monoFont: MonoFont,
  cjkFont: CJKFont,
): string {
  // Strip the proportional generic tail before appending monospace so fallback
  // ASCII symbols stay monospaced while CJK comments still follow cjkFont.
  const cjkWithoutGeneric = CJK_FONT_MAP[cjkFont].family.replace(
    /,\s*(?:sans-serif|serif)\s*$/,
    "",
  );
  return `${MONO_FONT_MAP[monoFont].family}, ${cjkWithoutGeneric}, monospace`;
}

// Dynamic CSS loaders keep unused font weights/CJK subsets out of the initial
// CSS bundle and browser network waterfall until selected or previewed.
const FONT_CSS_LOADERS: Record<string, () => Promise<unknown>> = {
  "latin:source-sans-3": () =>
    Promise.all([
      import("@fontsource-variable/source-sans-3"),
      import("@fontsource-variable/source-sans-3/wght-italic.css"),
    ]),
  "latin:newsreader": () =>
    Promise.all([
      import("@fontsource-variable/newsreader"),
      import("@fontsource-variable/newsreader/wght-italic.css"),
    ]),
  "latin:source-serif-4": () =>
    Promise.all([
      import("@fontsource-variable/source-serif-4"),
      import("@fontsource-variable/source-serif-4/wght-italic.css"),
    ]),
  "latin:lora": () =>
    Promise.all([
      import("@fontsource-variable/lora"),
      import("@fontsource-variable/lora/wght-italic.css"),
    ]),
  "mono:jetbrains-mono": () =>
    Promise.all([
      import("@fontsource-variable/jetbrains-mono"),
      import("@fontsource-variable/jetbrains-mono/wght-italic.css"),
    ]),
  "mono:fira-code": () => import("@fontsource-variable/fira-code"),
  "cjk:noto-sans-sc": () => import("@fontsource-variable/noto-sans-sc"),
  "cjk:oppo-sans": () => import("./oppo-sans.css"),
  "cjk:noto-serif-sc": () => import("@fontsource-variable/noto-serif-sc"),
  "cjk:lxgw-wenkai": () =>
    import("lxgw-wenkai-screen-webfont/lxgwwenkaiscreen.css"),
};

const loadedFontKeys = new Map<string, Promise<unknown>>();

function loadFontByKey(key: string): Promise<unknown> {
  const existing = loadedFontKeys.get(key);
  if (existing) {
    return existing;
  }
  const loader = FONT_CSS_LOADERS[key];
  if (!loader) {
    return Promise.resolve();
  }
  const promise = loader().catch(() => {
    // Allow retries if a lazy font stylesheet fetch fails temporarily offline.
    loadedFontKeys.delete(key);
  });
  loadedFontKeys.set(key, promise);
  return promise;
}

export function ensureArticleFontsLoaded(fonts: {
  latinFont: LatinFont;
  cjkFont: CJKFont;
  monoFont: MonoFont;
}): Promise<unknown> {
  return Promise.all([
    loadFontByKey(`latin:${fonts.latinFont}`),
    loadFontByKey(`cjk:${fonts.cjkFont}`),
    loadFontByKey(`mono:${fonts.monoFont}`),
  ]);
}

export function preloadAllArticleFonts(): Promise<unknown> {
  return Promise.all(Object.keys(FONT_CSS_LOADERS).map((key) => loadFontByKey(key)));
}

export function applyArticleFontPreferences(fonts: {
  latinFont: LatinFont;
  cjkFont: CJKFont;
  monoFont: MonoFont;
}): void {
  if (typeof document !== "undefined") {
    const rootStyle = document.documentElement.style;
    rootStyle.setProperty(
      "--article-font-family",
      resolveArticleBodyFontFamily(fonts.latinFont, fonts.cjkFont),
    );
    rootStyle.setProperty(
      "--article-font-mono",
      resolveArticleMonoFontFamily(fonts.monoFont, fonts.cjkFont),
    );
  }
  void ensureArticleFontsLoaded(fonts);
}
