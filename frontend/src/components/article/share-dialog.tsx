import { useState } from "react";
import { Copy, Link2, Loader2 } from "lucide-react";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import type { CreateShareResponse, Item } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import {
  isValidShareCustomHours,
  normalizeSharePasscodeInput,
  SHARE_CUSTOM_HOURS_MAX,
  SHARE_CUSTOM_HOURS_MIN,
} from "@/lib/share";
import { useCreateShare } from "@/queries/shares";

const EXPIRY_OPTIONS = [
  { value: "1", key: "share.expiry.1h" },
  { value: "24", key: "share.expiry.1d" },
  { value: "168", key: "share.expiry.7d" },
  { value: "720", key: "share.expiry.30d" },
  { value: "custom", key: "share.expiry.custom" },
] as const;

function buildPublicUrl(path: string): string {
  return new URL(path, window.location.origin).toString();
}

interface ShareDialogProps {
  article: Item;
  onOpenChange: (open: boolean) => void;
}

export function ShareDialog({ article, onOpenChange }: ShareDialogProps) {
  const { t, locale } = useI18n();
  const createShare = useCreateShare();

  const [expiry, setExpiry] = useState<string>("24");
  const [customHours, setCustomHours] = useState("24");
  const [passcodeEnabled, setPasscodeEnabled] = useState(false);
  const [passcodeMode, setPasscodeMode] = useState<"auto" | "custom">("auto");
  const [customPasscode, setCustomPasscode] = useState("");
  const [result, setResult] = useState<CreateShareResponse | null>(null);

  const customHoursValid =
    expiry !== "custom" || isValidShareCustomHours(customHours);
  const customPasscodeValid =
    !passcodeEnabled ||
    passcodeMode === "auto" ||
    /^[A-Z0-9]{4}$/.test(customPasscode);
  const canCreate =
    article.id > 0 &&
    customHoursValid &&
    customPasscodeValid &&
    !createShare.isPending;

  const handleCreate = async () => {
    if (!canCreate) return;

    const expiresInSeconds =
      (expiry === "custom" ? Number(customHours) : Number(expiry)) * 60 * 60;

    try {
      const created = await createShare.mutateAsync({
        item_id: article.id,
        expires_in_seconds: expiresInSeconds,
        passcode_enabled: passcodeEnabled,
        passcode:
          passcodeEnabled && passcodeMode === "custom"
            ? customPasscode
            : undefined,
      });
      setResult(created);
    } catch {
      toast.error(t("share.createFailed"));
    }
  };

  const copyValue = async (value: string, label: string) => {
    try {
      if (!navigator.clipboard) {
        throw new Error("clipboard unavailable");
      }
      await navigator.clipboard.writeText(value);
      toast.success(t("share.copied", { label }));
    } catch {
      toast.error(t("share.copyFailed"));
    }
  };

  const expiresLabel = (timestamp: number) =>
    new Date(timestamp * 1000).toLocaleString(locale);
  const expiryItems = EXPIRY_OPTIONS.map((option) => ({
    value: option.value,
    label: t(option.key),
  }));

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[440px]">
        {result ? (
          <>
            <DialogHeader>
              <DialogTitle>{t("share.created.title")}</DialogTitle>
              <DialogDescription>
                {t("share.created.expires", {
                  date: expiresLabel(result.expires_at),
                })}
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4">
              <div className="space-y-1.5">
                <label
                  htmlFor="share-public-link"
                  className="text-[13px] font-medium"
                >
                  {t("share.created.linkLabel")}
                </label>
                <div className="flex gap-2">
                  <Input
                    id="share-public-link"
                    value={buildPublicUrl(result.path)}
                    readOnly
                    onFocus={(event) => event.currentTarget.select()}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    aria-label={t("share.copyLink")}
                    title={t("share.copyLink")}
                    onClick={() =>
                      void copyValue(
                        buildPublicUrl(result.path),
                        t("share.created.linkLabel"),
                      )
                    }
                  >
                    <Copy className="h-4 w-4" />
                  </Button>
                </div>
              </div>

              {result.passcode && (
                <div className="space-y-1.5">
                  <label
                    htmlFor="share-passcode"
                    className="text-[13px] font-medium"
                  >
                    {t("share.created.passcodeLabel")}
                  </label>
                  <div className="flex gap-2">
                    <Input
                      id="share-passcode"
                      value={result.passcode}
                      readOnly
                      className="font-mono tracking-[0.35em]"
                      onFocus={(event) => event.currentTarget.select()}
                    />
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      aria-label={t("share.copyPasscode")}
                      title={t("share.copyPasscode")}
                      onClick={() =>
                        void copyValue(
                          result.passcode ?? "",
                          t("share.created.passcodeLabel"),
                        )
                      }
                    >
                      <Copy className="h-4 w-4" />
                    </Button>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {t("share.created.passcodeHint")}
                  </p>
                </div>
              )}
            </div>

            <DialogFooter>
              <Button
                type="button"
                onClick={() => onOpenChange(false)}
                className="sm:min-w-20"
              >
                {t("share.done")}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>{t("share.dialog.title")}</DialogTitle>
              <DialogDescription>
                {t("share.dialog.description")}
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4">
              <div className="space-y-1.5">
                <label
                  htmlFor="share-expiry"
                  className="text-[13px] font-medium"
                >
                  {t("share.expiry.label")}
                </label>
                <Select
                  items={expiryItems}
                  value={expiry}
                  onValueChange={(value) => value && setExpiry(value)}
                >
                  <SelectTrigger id="share-expiry" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {EXPIRY_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.key)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {expiry === "custom" && (
                  <div className="space-y-1">
                    <label
                      htmlFor="share-custom-hours"
                      className="text-xs text-muted-foreground"
                    >
                      {t("share.expiry.customLabel")}
                    </label>
                    <Input
                      id="share-custom-hours"
                      type="number"
                      min={SHARE_CUSTOM_HOURS_MIN}
                      max={SHARE_CUSTOM_HOURS_MAX}
                      value={customHours}
                      onChange={(event) => setCustomHours(event.target.value)}
                      aria-invalid={!customHoursValid}
                    />
                    {!customHoursValid && (
                      <p className="text-xs text-destructive">
                        {t("share.expiry.invalid")}
                      </p>
                    )}
                  </div>
                )}
              </div>

              <div className="space-y-3 rounded-lg border p-3.5">
                <div className="flex items-center justify-between gap-4">
                  <div className="space-y-0.5">
                    <label
                      htmlFor="share-passcode-enabled"
                      className="text-[13px] font-medium"
                    >
                      {t("share.passcode.label")}
                    </label>
                    <p className="text-xs text-muted-foreground">
                      {t("share.passcode.description")}
                    </p>
                  </div>
                  <Switch
                    id="share-passcode-enabled"
                    checked={passcodeEnabled}
                    onCheckedChange={setPasscodeEnabled}
                  />
                </div>

                {passcodeEnabled && (
                  <div className="space-y-2 border-t pt-3">
                    <Select
                      items={[
                        {
                          value: "auto",
                          label: t("share.passcode.modeAuto"),
                        },
                        {
                          value: "custom",
                          label: t("share.passcode.modeCustom"),
                        },
                      ]}
                      value={passcodeMode}
                      onValueChange={(value) => {
                        if (value === "auto" || value === "custom") {
                          setPasscodeMode(value);
                        }
                      }}
                    >
                      <SelectTrigger
                        id="share-passcode-mode"
                        className="w-full"
                        aria-label={t("share.passcode.label")}
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="auto">
                          {t("share.passcode.modeAuto")}
                        </SelectItem>
                        <SelectItem value="custom">
                          {t("share.passcode.modeCustom")}
                        </SelectItem>
                      </SelectContent>
                    </Select>

                    {passcodeMode === "custom" && (
                      <div className="space-y-1">
                        <Input
                          id="share-custom-passcode"
                          value={customPasscode}
                          onChange={(event) =>
                            setCustomPasscode(
                              normalizeSharePasscodeInput(event.target.value),
                            )
                          }
                          placeholder="A7K2"
                          autoComplete="off"
                          spellCheck={false}
                          maxLength={4}
                          className="font-mono uppercase tracking-[0.35em]"
                          aria-label={t("share.passcode.inputLabel")}
                          aria-invalid={
                            customPasscode.length > 0 &&
                            customPasscode.length < 4
                          }
                        />
                        <p className="text-xs text-muted-foreground">
                          {t("share.passcode.hint")}
                        </p>
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                type="button"
                onClick={() => void handleCreate()}
                disabled={!canCreate}
              >
                {createShare.isPending ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t("share.creating")}
                  </>
                ) : (
                  <>
                    <Link2 className="h-4 w-4" />
                    {t("share.create")}
                  </>
                )}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
