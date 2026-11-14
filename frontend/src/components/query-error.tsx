import { AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

interface QueryErrorProps {
  message: string;
  onRetry: () => void;
  className?: string;
}

export function QueryErrorBanner({
  message,
  onRetry,
  className,
}: QueryErrorProps) {
  const { t } = useI18n();

  return (
    <div
      role="alert"
      className={cn(
        "flex items-center justify-between gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive",
        className,
      )}
    >
      <div className="flex min-w-0 items-center gap-2">
        <AlertCircle className="h-3.5 w-3.5 shrink-0" />
        <span className="truncate">{message}</span>
      </div>
      <Button
        type="button"
        variant="outline"
        size="xs"
        onClick={onRetry}
        className="shrink-0 text-foreground"
      >
        {t("common.retry")}
      </Button>
    </div>
  );
}

export function QueryErrorState({
  message,
  onRetry,
  className,
}: QueryErrorProps) {
  const { t } = useI18n();

  return (
    <div
      role="alert"
      className={cn(
        "flex flex-col items-center justify-center gap-3 py-12 text-center",
        className,
      )}
    >
      <div className="flex items-center gap-2 text-sm text-destructive">
        <AlertCircle className="h-4 w-4 shrink-0" />
        <span>{message}</span>
      </div>
      <Button type="button" variant="outline" size="sm" onClick={onRetry}>
        {t("common.retry")}
      </Button>
    </div>
  );
}
