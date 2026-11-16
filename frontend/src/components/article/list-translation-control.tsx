import { Check, CircleAlert, Languages, Loader2, RotateCw } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
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
  status: ListTranslationStatus;
  error: string | null;
  onRetry: () => void;
}

export function ListTranslationControl({
  status,
  error,
  onRetry,
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
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="outline"
            size="sm"
            className={cn(
              "gap-1.5 text-xs",
              isFailed && "border-destructive/40 text-destructive",
            )}
            title={isFailed ? failureMessage : undefined}
            aria-label={t("article.list.translation.label")}
          >
            {status === "translating" ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : isFailed ? (
              <CircleAlert className="h-4 w-4" />
            ) : (
              <Languages className="h-4 w-4" />
            )}
            <span className="hidden sm:inline">
              {t("article.list.translation.label")}
            </span>
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
        {isFailed && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onRetry}>
              <RotateCw className="h-4 w-4" />
              {t("article.list.translation.retry")}
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
