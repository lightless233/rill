import { useEffect, useState } from "react";
import { useTheme } from "next-themes";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  Bug,
  Download,
  Eye,
  EyeOff,
  Info,
  Keyboard,
  Languages,
  Loader2,
  Settings as SettingsIcon,
} from "lucide-react";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  autoMarkReadOptions,
  articlePageSizeOptions,
  listTranslationModeOptions,
  supportedLocales,
  translationModeOptions,
  usePreferencesStore,
  useUIStore,
  type SettingsTab,
} from "@/store";
import { usePWAInstall } from "@/hooks/use-pwa-install";
import { localeLabels, useI18n } from "@/lib/i18n";
import { ensureLocaleMessages } from "@/lib/i18n/messages";
import {
  CJK_FONT_OPTIONS,
  LATIN_FONT_OPTIONS,
  MONO_FONT_OPTIONS,
  preloadAllArticleFonts,
  type CJKFont,
  type FontOptionMeta,
  type LatinFont,
  type MonoFont,
} from "@/lib/fonts";
import {
  APIError,
  settingsAPI,
  translationAPI,
  type ListTranslationMode,
  type TranslationMode,
  type UpdateSettingsRequest,
} from "@/lib/api";
import { waitForSettingsSeed } from "@/lib/settings-sync";
import { effectiveListTranslationMode } from "@/lib/list-translation";
import {
  beginChange,
  changeGenerations,
  useApplySetting,
} from "@/hooks/use-apply-setting";
import { queryKeys } from "@/queries/keys";
import { useSettingsQuery } from "@/queries/settings";
import type { AppLocale } from "@/store/preferences";
import { cn } from "@/lib/utils";

function GithubIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
      <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" />
    </svg>
  );
}

interface NavItemProps {
  icon: React.ReactNode;
  label: string;
  active: boolean;
  onClick: () => void;
}

function NavItem({ icon, label, active, onClick }: NavItemProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors sm:w-full sm:gap-2.5 sm:px-2",
        active
          ? "bg-accent font-medium text-foreground"
          : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
      )}
    >
      {icon}
      <span className="whitespace-nowrap">{label}</span>
    </button>
  );
}

interface FontSettingItem<T extends string> {
  value: T;
  label: string;
  category: "sans" | "serif" | "mono";
  family: string;
}

interface FontSettingRowProps<T extends string> {
  label: string;
  description: string;
  value: T;
  items: FontSettingItem<T>[];
  sansGroupLabel: string;
  serifGroupLabel: string;
  onChange: (value: T) => void;
}

function FontSettingRow<T extends string>({
  label,
  description,
  value,
  items,
  sansGroupLabel,
  serifGroupLabel,
  onChange,
}: FontSettingRowProps<T>) {
  const selectItems = items.map((item) => ({
    value: item.value,
    label: item.label,
  }));
  const sansItems = items.filter((item) => item.category === "sans");
  const serifItems = items.filter((item) => item.category === "serif");
  const grouped = sansItems.length > 0 && serifItems.length > 0;

  return (
    <div className="flex items-center justify-between gap-4">
      <div className="min-w-0 space-y-1">
        <p className="text-sm font-medium">{label}</p>
        <p className="text-[13px] text-muted-foreground">{description}</p>
      </div>
      <Select
        items={selectItems}
        value={value}
        onValueChange={(nextValue) => {
          if (nextValue) {
            onChange(nextValue as T);
          }
        }}
      >
        <SelectTrigger className="w-auto min-w-44 shrink-0 justify-between gap-2 border-border">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {grouped ? (
            <>
              <SelectGroup>
                <SelectLabel>{sansGroupLabel}</SelectLabel>
                {sansItems.map((item) => (
                  <SelectItem
                    key={item.value}
                    value={item.value}
                    style={{ fontFamily: item.family }}
                  >
                    {item.label}
                  </SelectItem>
                ))}
              </SelectGroup>
              <SelectSeparator />
              <SelectGroup>
                <SelectLabel>{serifGroupLabel}</SelectLabel>
                {serifItems.map((item) => (
                  <SelectItem
                    key={item.value}
                    value={item.value}
                    style={{ fontFamily: item.family }}
                  >
                    {item.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </>
          ) : (
            items.map((item) => (
              <SelectItem
                key={item.value}
                value={item.value}
                style={{ fontFamily: item.family }}
              >
                {item.label}
              </SelectItem>
            ))
          )}
        </SelectContent>
      </Select>
    </div>
  );
}

// One flat pane while the preference count is small; split into named
// clusters (e.g. Appearance / Reading) only once a cluster has enough rows
// to carry its own tab (~5), not before.
function GeneralContent() {
  const { t } = useI18n();
  const { theme, setTheme } = useTheme();
  const setSettingsOpen = useUIStore((s) => s.setSettingsOpen);
  const setShortcutsOpen = useUIStore((s) => s.setShortcutsOpen);
  const {
    locale,
    articlePageSize,
    autoMarkRead,
    latinFont,
    cjkFont,
    monoFont,
    setLocale,
    setArticlePageSize,
    setAutoMarkRead,
    setLatinFont,
    setCJKFont,
    setMonoFont,
  } = usePreferencesStore();
  const applySetting = useApplySetting();

  useEffect(() => {
    void preloadAllArticleFonts();
  }, []);

  const toFontItems = <T extends string>(
    options: readonly FontOptionMeta<T>[],
  ): FontSettingItem<T>[] =>
    options.map((opt) => ({
      value: opt.value,
      label: opt.labelKey ? t(opt.labelKey) : (opt.label ?? opt.value),
      category: opt.category,
      family: opt.family,
    }));

  const latinFontItems = toFontItems(LATIN_FONT_OPTIONS);
  const cjkFontItems = toFontItems(CJK_FONT_OPTIONS);
  const monoFontItems = toFontItems(MONO_FONT_OPTIONS);

  const localeItems = supportedLocales.map((localeCode) => ({
    value: localeCode,
    label: localeLabels[localeCode] ?? localeCode,
  }));
  const articlePageSizeItems = articlePageSizeOptions.map((size) => ({
    value: size.toString(),
    label: String(size),
  }));
  const autoMarkReadItems = autoMarkReadOptions.map((option) => ({
    value: option,
    label:
      option === "off"
        ? t("settings.autoMarkRead.off")
        : option === "open"
          ? t("settings.autoMarkRead.open")
          : t("settings.autoMarkRead.afterSeconds", {
              seconds: Number.parseInt(option, 10),
            }),
  }));
  const themeItems = [
    { value: "light", label: t("settings.theme.light") },
    { value: "dark", label: t("settings.theme.dark") },
    { value: "system", label: t("settings.theme.system") },
  ];

  return (
    <div className="space-y-5">
      {/* Language */}
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <p className="text-sm font-medium">{t("settings.language.label")}</p>
          <p className="text-[13px] text-muted-foreground">
            {t("settings.language.description")}
          </p>
        </div>
        <Select
          items={localeItems}
          value={locale}
          onValueChange={(v) => {
            if (!v) return;
            const previous = locale;
            // Take the generation synchronously so it also covers the
            // catalog wait, not just the save.
            const generation = beginChange("locale");
            // Load the catalog first so the switch does not flash English; a
            // failed load applies (and saves) nothing.
            ensureLocaleMessages(v as AppLocale)
              .then(() => {
                if (changeGenerations.locale !== generation) {
                  return;
                }
                void applySetting(
                  "locale",
                  generation,
                  { locale: v },
                  () => setLocale(v),
                  () => setLocale(previous),
                );
              })
              .catch(() => {
                if (changeGenerations.locale === generation) {
                  toast.error(t("settings.syncFailed"));
                }
              });
          }}
        >
          <SelectTrigger className="w-auto gap-2 border-border">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {localeItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Theme */}
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <p className="text-sm font-medium">{t("settings.theme.label")}</p>
          <p className="text-[13px] text-muted-foreground">
            {t("settings.theme.description")}
          </p>
        </div>
        <Select
          items={themeItems}
          value={theme}
          onValueChange={(v) => {
            if (!v) return;
            void applySetting(
              "theme",
              beginChange("theme"),
              { theme: v },
              () => setTheme(v),
              // A failed save deliberately keeps the pick: next-themes has
              // already persisted it, and reverting would write the implicit
              // default ("system") into storage, which the sync would later
              // upload as if the user had chosen it. The kept pick is only
              // retried while the server theme is null — a non-null server
              // theme still wins on the next load.
              () => {},
            );
          }}
        >
          <SelectTrigger className="w-auto gap-2 border-border">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {themeItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Articles per load */}
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <p className="text-sm font-medium">
            {t("settings.articlePageSize.label")}
          </p>
          <p className="text-[13px] text-muted-foreground">
            {t("settings.articlePageSize.description")}
          </p>
        </div>
        <Select
          items={articlePageSizeItems}
          value={articlePageSize.toString()}
          onValueChange={(value) => {
            if (!value) return;
            const parsed = Number.parseInt(value, 10);
            if (Number.isNaN(parsed)) return;
            const previous = articlePageSize;
            void applySetting(
              "pageSize",
              beginChange("pageSize"),
              { article_page_size: parsed },
              () => setArticlePageSize(parsed),
              () => setArticlePageSize(previous),
            );
          }}
        >
          <SelectTrigger className="w-auto gap-2 border-border">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {articlePageSizeItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Auto mark as read */}
      <div className="flex items-center justify-between gap-6">
        <div className="min-w-0 space-y-1">
          <p className="text-sm font-medium">
            {t("settings.autoMarkRead.label")}
          </p>
          <p className="text-[13px] text-muted-foreground">
            {t("settings.autoMarkRead.description")}
          </p>
        </div>
        <Select
          items={autoMarkReadItems}
          value={autoMarkRead}
          onValueChange={(v) => {
            if (!v) return;
            const previous = autoMarkRead;
            void applySetting(
              "autoMarkRead",
              beginChange("autoMarkRead"),
              { auto_mark_read: v },
              () => setAutoMarkRead(v),
              () => setAutoMarkRead(previous),
            );
          }}
        >
          <SelectTrigger className="w-auto gap-2 border-border">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {autoMarkReadItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Article fonts */}
      <div className="space-y-5 border-t border-border pt-5">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {t("settings.typography.title")}
        </h3>

        <FontSettingRow
          label={t("settings.latinFont.label")}
          description={t("settings.latinFont.description")}
          value={latinFont}
          items={latinFontItems}
          sansGroupLabel={t("settings.fonts.group.sans")}
          serifGroupLabel={t("settings.fonts.group.serif")}
          onChange={(nextFont: LatinFont) => {
            const previous = latinFont;
            void applySetting(
              "latinFont",
              beginChange("latinFont"),
              { latin_font: nextFont },
              () => setLatinFont(nextFont),
              () => setLatinFont(previous),
            );
          }}
        />

        <FontSettingRow
          label={t("settings.cjkFont.label")}
          description={t("settings.cjkFont.description")}
          value={cjkFont}
          items={cjkFontItems}
          sansGroupLabel={t("settings.fonts.group.sans")}
          serifGroupLabel={t("settings.fonts.group.serif")}
          onChange={(nextFont: CJKFont) => {
            const previous = cjkFont;
            void applySetting(
              "cjkFont",
              beginChange("cjkFont"),
              { cjk_font: nextFont },
              () => setCJKFont(nextFont),
              () => setCJKFont(previous),
            );
          }}
        />

        <FontSettingRow
          label={t("settings.monoFont.label")}
          description={t("settings.monoFont.description")}
          value={monoFont}
          items={monoFontItems}
          sansGroupLabel={t("settings.fonts.group.sans")}
          serifGroupLabel={t("settings.fonts.group.serif")}
          onChange={(nextFont: MonoFont) => {
            const previous = monoFont;
            void applySetting(
              "monoFont",
              beginChange("monoFont"),
              { mono_font: nextFont },
              () => setMonoFont(nextFont),
              () => setMonoFont(previous),
            );
          }}
        />

        {/* Live preview */}
        <div className="space-y-2 rounded-lg border border-border bg-muted/30 px-4 py-3">
          <p className="text-xs font-medium text-muted-foreground">
            {t("settings.fonts.preview.label")}
          </p>
          <p className="article-typography text-[0.9375rem] leading-relaxed text-foreground">
            {t("settings.fonts.preview.sample")}
          </p>
          <pre className="overflow-x-auto rounded border border-border/60 bg-background/70 px-2.5 py-1.5 text-xs text-foreground">
            <code className="article-typography-mono">
              {t("settings.fonts.preview.codeSample")}
            </code>
          </pre>
        </div>
      </div>

      {/* Keyboard shortcuts */}
      <div className="flex items-center justify-between border-t border-border pt-5">
        <div className="space-y-1">
          <p className="text-sm font-medium">{t("settings.shortcuts.label")}</p>
          <p className="text-[13px] text-muted-foreground">
            {t("settings.shortcuts.description")}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            setSettingsOpen(false);
            setShortcutsOpen(true);
          }}
        >
          <Keyboard className="h-4 w-4" />
          {t("settings.shortcuts.open")}
        </Button>
      </div>
    </div>
  );
}

function TranslationContent() {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const applySetting = useApplySetting();
  const {
    translationMode,
    setTranslationMode,
    listTranslationMode,
    setListTranslationMode,
  } = usePreferencesStore();
  const { data: settingsData } = useSettingsQuery();

  const [apiURLDraft, setApiURLDraft] = useState<string | null>(null);
  const [apiKey, setApiKey] = useState("");
  const [showAPIKey, setShowAPIKey] = useState(false);
  const [modelNameDraft, setModelNameDraft] = useState<string | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const [isTesting, setIsTesting] = useState(false);

  const apiURL = apiURLDraft ?? settingsData?.translation_api_url ?? "";
  const modelName = modelNameDraft ?? settingsData?.translation_model ?? "";

  const modeItems = translationModeOptions.map((mode) => ({
    value: mode,
    label:
      mode === "bilingual"
        ? t("settings.translation.mode.bilingual")
        : t("settings.translation.mode.zh"),
  }));

  const hasSavedKey = Boolean(settingsData?.translation_api_key_set);
  const effectiveListMode = effectiveListTranslationMode(
    listTranslationMode,
    hasSavedKey,
  );
  const listModeItems = listTranslationModeOptions.map((mode) => ({
    value: mode,
    label:
      mode === "bilingual"
        ? t("article.translation.mode.bilingual")
        : mode === "zh"
          ? t("article.translation.mode.zh")
          : t("article.translation.mode.original"),
  }));

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSaving(true);
    try {
      await waitForSettingsSeed();
      const patch: UpdateSettingsRequest = {
        translation_api_url: apiURL.trim(),
        translation_model: modelName.trim(),
      };
      if (apiKey.trim()) {
        patch.translation_api_key = apiKey.trim();
      }
      const res = await settingsAPI.update(patch);
      if (res.data) {
        queryClient.setQueryData(queryKeys.settings.all, res.data);
      } else {
        await queryClient.invalidateQueries({ queryKey: queryKeys.settings.all });
      }
      setApiURLDraft(null);
      setModelNameDraft(null);
      setApiKey("");
      setShowAPIKey(false);
      toast.success(t("settings.translation.saved"));
    } catch (error) {
      if (error instanceof APIError && error.status === 401) {
        return;
      }
      toast.error(
        error instanceof Error
          ? error.message
          : t("settings.translation.saveFailed"),
      );
    } finally {
      setIsSaving(false);
    }
  };

  const handleClearKey = async () => {
    setIsSaving(true);
    try {
      await waitForSettingsSeed();
      const res = await settingsAPI.update({ translation_api_key: "" });
      if (res.data) {
        queryClient.setQueryData(queryKeys.settings.all, res.data);
      } else {
        await queryClient.invalidateQueries({ queryKey: queryKeys.settings.all });
      }
      setApiKey("");
      toast.success(t("settings.translation.keyCleared"));
    } catch (error) {
      if (error instanceof APIError && error.status === 401) {
        return;
      }
      toast.error(t("settings.translation.saveFailed"));
    } finally {
      setIsSaving(false);
    }
  };

  const handleTest = async () => {
    setIsTesting(true);
    try {
      const res = await translationAPI.test({
        api_url: apiURL.trim(),
        api_key: apiKey.trim() || undefined,
        model: modelName.trim(),
      });
      toast.success(
        t("settings.translation.testSuccess", {
          model: res.data?.model ?? (modelName.trim() || "deepseek-flash"),
        }),
      );
    } catch (error) {
      if (error instanceof APIError && error.status === 401) {
        return;
      }
      toast.error(
        error instanceof Error
          ? error.message
          : t("settings.translation.testFailed"),
      );
    } finally {
      setIsTesting(false);
    }
  };

  const keyMask = settingsData?.translation_api_key_mask ?? "";

  return (
    <div className="space-y-6">
      {/* Article list translation */}
      <div className="flex items-center justify-between gap-6">
        <div className="min-w-0 space-y-1">
          <p className="text-sm font-medium">
            {t("settings.translation.listMode.label")}
          </p>
          <p className="text-[13px] text-muted-foreground">
            {t("settings.translation.listMode.description")}
          </p>
        </div>
        <Select
          items={listModeItems}
          value={effectiveListMode}
          onValueChange={(nextValue) => {
            if (!nextValue) return;
            const nextMode = nextValue as ListTranslationMode;
            if (!hasSavedKey && nextMode !== "original") {
              toast.info(t("article.translation.missingConfig"));
              return;
            }
            const previous = listTranslationMode;
            void applySetting(
              "listTranslationMode",
              beginChange("listTranslationMode"),
              { list_translation_mode: nextMode },
              () => setListTranslationMode(nextMode),
              () => setListTranslationMode(previous),
            );
          }}
        >
          <SelectTrigger className="w-auto min-w-36 shrink-0 justify-between gap-2 border-border">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {listModeItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Default display mode */}
      <div className="flex items-center justify-between gap-6">
        <div className="min-w-0 space-y-1">
          <p className="text-sm font-medium">
            {t("settings.translation.defaultMode.label")}
          </p>
          <p className="text-[13px] text-muted-foreground">
            {t("settings.translation.defaultMode.description")}
          </p>
        </div>
        <Select
          items={modeItems}
          value={translationMode}
          onValueChange={(nextValue) => {
            if (!nextValue) return;
            const nextMode = nextValue as TranslationMode;
            const previous = translationMode;
            void applySetting(
              "translationMode",
              beginChange("translationMode"),
              { translation_mode: nextMode },
              () => setTranslationMode(nextMode),
              () => setTranslationMode(previous),
            );
          }}
        >
          <SelectTrigger className="w-auto min-w-36 shrink-0 justify-between gap-2 border-border">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {modeItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* DeepSeek / OpenAI-compatible API settings */}
      <form
        onSubmit={(e) => {
          void handleSave(e);
        }}
        className="space-y-4 border-t border-border pt-5"
      >
        <div className="space-y-1.5">
          <label
            htmlFor="translation-api-url"
            className="text-sm font-medium"
          >
            {t("settings.translation.apiUrl.label")}
          </label>
          <Input
            id="translation-api-url"
            type="url"
            placeholder="https://api.deepseek.com"
            value={apiURL}
            onChange={(e) => setApiURLDraft(e.target.value)}
            autoComplete="off"
          />
          <p className="text-xs text-muted-foreground">
            {t("settings.translation.apiUrl.hint")}
          </p>
        </div>

        <div className="space-y-1.5">
          <div className="flex items-center justify-between gap-2">
            <label
              htmlFor="translation-api-key"
              className="text-sm font-medium"
            >
              {t("settings.translation.apiKey.label")}
            </label>
            {hasSavedKey && (
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">
                  {t("settings.translation.apiKey.configured", {
                    mask: keyMask,
                  })}
                </span>
                <button
                  type="button"
                  onClick={() => void handleClearKey()}
                  disabled={isSaving}
                  className="text-xs font-medium text-destructive hover:underline disabled:opacity-50"
                >
                  {t("settings.translation.apiKey.clear")}
                </button>
              </div>
            )}
          </div>
          <div className="relative">
            <Input
              id="translation-api-key"
              type={showAPIKey ? "text" : "password"}
              placeholder={
                hasSavedKey
                  ? t("settings.translation.apiKey.replacePlaceholder")
                  : "sk-..."
              }
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              autoComplete="new-password"
              className="pr-9"
            />
            <button
              type="button"
              onClick={() => setShowAPIKey((v) => !v)}
              aria-label={
                showAPIKey
                  ? t("settings.translation.apiKey.hide")
                  : t("settings.translation.apiKey.show")
              }
              className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-muted-foreground hover:text-foreground"
            >
              {showAPIKey ? (
                <EyeOff className="h-4 w-4" />
              ) : (
                <Eye className="h-4 w-4" />
              )}
            </button>
          </div>
        </div>

        <div className="space-y-1.5">
          <label
            htmlFor="translation-model"
            className="text-sm font-medium"
          >
            {t("settings.translation.model.label")}
          </label>
          <Input
            id="translation-model"
            type="text"
            placeholder="deepseek-flash"
            value={modelName}
            onChange={(e) => setModelNameDraft(e.target.value)}
            autoComplete="off"
          />
          <p className="text-xs text-muted-foreground">
            {t("settings.translation.model.hint")}
          </p>
        </div>

        <div className="flex items-center justify-end gap-2 pt-1">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void handleTest()}
            disabled={isTesting || isSaving}
          >
            {isTesting && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {t("settings.translation.test")}
          </Button>
          <Button type="submit" size="sm" disabled={isSaving || isTesting}>
            {isSaving && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {t("common.save")}
          </Button>
        </div>
      </form>
    </div>
  );
}

function AboutContent() {
  const { t } = useI18n();
  const { isInstallAvailable, promptInstall } = usePWAInstall();
  const [isInstalling, setIsInstalling] = useState(false);

  const handleInstall = async () => {
    setIsInstalling(true);
    try {
      const isInstalled = await promptInstall();
      if (!isInstalled) {
        toast.info(t("settings.installCancelled"));
      }
    } finally {
      setIsInstalling(false);
    }
  };

  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 pb-8">
      <img
        src="/icon-96.png"
        alt={t("common.appLogo")}
        width={64}
        height={64}
        className="h-16 w-16 rounded-2xl"
      />
      <div className="text-center">
        <h3 className="text-xl font-semibold">Rill</h3>
        <p className="mt-1 text-xs text-muted-foreground">{__APP_VERSION__}</p>
        <p className="mt-1.5 text-sm text-muted-foreground">
          {t("settings.about.description")}
        </p>
      </div>
      <div className="flex gap-2">
        {isInstallAvailable && (
          <Button
            variant="default"
            size="sm"
            onClick={() => {
              void handleInstall();
            }}
            disabled={isInstalling}
          >
            <Download className="h-4 w-4" />
            {isInstalling
              ? t("settings.about.installing")
              : t("settings.about.install")}
          </Button>
        )}
        <a
          href="https://github.com/lightless233/rill"
          target="_blank"
          rel="noopener noreferrer"
          className={buttonVariants({ variant: "outline", size: "sm" })}
        >
          <GithubIcon className="h-4 w-4" />
          {t("settings.about.github")}
        </a>
        <a
          href="https://github.com/lightless233/rill/issues"
          target="_blank"
          rel="noopener noreferrer"
          className={buttonVariants({ variant: "outline", size: "sm" })}
        >
          <Bug className="h-4 w-4" />
          {t("settings.about.reportIssue")}
        </a>
      </div>
      <div className="mt-auto space-y-2 text-center text-xs text-muted-foreground">
        <p>{t("settings.about.license")}</p>
        <p>
          {t("settings.about.fonts")}{" "}
          <a
            href="/fonts/oppo-sans/LICENSE.txt"
            target="_blank"
            rel="noopener noreferrer"
            className="underline underline-offset-4"
          >
            OPPO Sans 4.0
          </a>
          {" · © 2024 Guangdong OPPO Mobile Telecommunications Corp., Ltd."}
        </p>
      </div>
    </div>
  );
}

export function SettingsDialog() {
  const { t } = useI18n();
  const { isSettingsOpen, setSettingsOpen, settingsTab, setSettingsTab } =
    useUIStore();

  const tabTitles: Record<SettingsTab, string> = {
    general: t("settings.tab.general"),
    translation: t("settings.tab.translation"),
    about: t("settings.tab.about"),
  };

  return (
    <Dialog open={isSettingsOpen} onOpenChange={(open) => setSettingsOpen(open)}>
      <DialogContent className="flex max-h-[85vh] flex-col sm:flex-row h-auto sm:h-[560px] sm:max-w-4xl gap-0 overflow-hidden p-0">
        {/* Sidebar (desktop) / Tab bar (mobile) */}
        <div className="flex shrink-0 flex-row border-b border-border bg-muted/30 px-3 pt-3 sm:w-[200px] sm:flex-col sm:border-b-0 sm:border-r sm:pt-4">
          <h2 className="hidden px-2 text-sm font-semibold sm:block">
            {t("common.settings")}
          </h2>
          <nav className="flex gap-0.5 sm:mt-2 sm:flex-col">
            <NavItem
              icon={<SettingsIcon className="h-4 w-4" />}
              label={t("settings.tab.general")}
              active={settingsTab === "general"}
              onClick={() => setSettingsTab("general")}
            />
            <NavItem
              icon={<Languages className="h-4 w-4" />}
              label={t("settings.tab.translation")}
              active={settingsTab === "translation"}
              onClick={() => setSettingsTab("translation")}
            />
            <NavItem
              icon={<Info className="h-4 w-4" />}
              label={t("settings.tab.about")}
              active={settingsTab === "about"}
              onClick={() => setSettingsTab("about")}
            />
          </nav>
        </div>

        {/* Content */}
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden p-5 sm:p-6">
          <h2 className="mb-4 shrink-0 text-lg font-semibold sm:mb-6">
            {tabTitles[settingsTab]}
          </h2>

          <div className="flex-1 overflow-y-auto pr-1">
            {settingsTab === "general" && <GeneralContent />}
            {settingsTab === "translation" && <TranslationContent />}
            {settingsTab === "about" && <AboutContent />}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
