import {
  Check,
  ChevronDown,
  CircleAlert,
  Languages,
  Loader2,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { beginChange, useApplySetting } from "@/hooks/use-apply-setting";
import type { ListTranslationStatus } from "@/hooks/use-list-translation";
import { useI18n } from "@/lib/i18n";
import { effectiveListTranslationMode } from "@/lib/list-translation";
import type { ListTranslationMode } from "@/lib/api";
import { useSettingsQuery } from "@/queries/settings";
import { usePreferencesStore, useUIStore } from "@/store";
import { listTranslationModeOptions } from "@/store/preferences";
import { cn } from "@/lib/utils";

interface ListTranslationControlProps {
  requested: boolean;
  status: ListTranslationStatus;
  error: string | null;
  onStart: () => void;
}

export function ListTranslationControl({
  requested,
  status,
  error,
  onStart,
}: ListTranslationControlProps) {
  const { t } = useI18n();
  const applySetting = useApplySetting();
  const { data: settingsData } = useSettingsQuery();
  const setSettingsOpen = useUIStore((state) => state.setSettingsOpen);
  const { listTranslationMode, setListTranslationMode } = usePreferencesStore();

  const hasKey = Boolean(settingsData?.translation_api_key_set);
  const effectiveMode = effectiveListTranslationMode(
    listTranslationMode,
    hasKey,
  );
  const isFailed = status === "error" || status === "incomplete";
  const failureMessage =
    status === "incomplete"
      ? t("article.list.translation.incomplete")
      : (error ?? t("article.list.translation.failed"));

  const modeItems = listTranslationModeOptions.map((mode) => ({
    value: mode,
    label:
      mode === "bilingual"
        ? t("article.translation.mode.bilingual")
        : mode === "zh"
          ? t("article.translation.mode.zh")
          : t("article.translation.mode.original"),
  }));

  const handleStart = () => {
    if (!hasKey) {
      toast.info(t("article.translation.missingConfig"));
      setSettingsOpen(true, "translation");
      return;
    }
    onStart();
  };

  const handleModeChange = (nextMode: ListTranslationMode) => {
    if (!hasKey && nextMode !== "original") {
      toast.info(t("article.translation.missingConfig"));
      setSettingsOpen(true, "translation");
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

    // Selecting a translated mode is an explicit manual trigger. Original
    // turns the feature off and never starts work.
    if (nextMode !== "original") {
      onStart();
    }
  };

  const triggerTitle = isFailed
    ? failureMessage
    : !hasKey
      ? t("article.translation.missingConfig")
      : undefined;

  return (
    <div className="flex items-center">
      <Button
        variant="outline"
        size="sm"
        onClick={handleStart}
        disabled={!hasKey || effectiveMode === "original" || status === "translating"}
        title={triggerTitle}
        aria-label={t("article.list.translation.label")}
        className={cn(
          "gap-1.5 rounded-r-none text-xs",
          isFailed && "border-destructive/40 text-destructive",
        )}
      >
        {status === "translating" ? (
          <Loader2 className="h-4 w-4 animate-spin" />
        ) : isFailed ? (
          <CircleAlert className="h-4 w-4" />
        ) : requested ? (
          <Check className="h-4 w-4" />
        ) : (
          <Languages className="h-4 w-4" />
        )}
        <span className="hidden sm:inline">
          {t("article.list.translation.label")}
        </span>
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="outline"
              size="sm"
              aria-label={t("article.list.translation.label")}
              className="-ml-px rounded-l-none px-1.5"
            >
              <ChevronDown className="h-3.5 w-3.5" />
            </Button>
          }
        />
        <DropdownMenuContent align="end" className="w-auto">
          {modeItems.map((item) => (
            <DropdownMenuItem
              key={item.value}
              onClick={() => handleModeChange(item.value)}
            >
              <Check
                className={cn(
                  "h-4 w-4",
                  effectiveMode === item.value
                    ? "opacity-100"
                    : "opacity-0",
                )}
              />
              {item.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
