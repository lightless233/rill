import { useCallback } from "react";
import { toast } from "sonner";
import {
  APIError,
  settingsAPI,
  type UpdateSettingsRequest,
} from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { waitForSettingsSeed } from "@/lib/settings-sync";

// Module scope so the counters survive unmounting a tab or the dialog itself: a
// late-failing save from an old mount must not revert a newer change made on a
// new one. Shared by the settings dialog and the list header control.
export const changeGenerations = {
  locale: 0,
  pageSize: 0,
  theme: 0,
  autoMarkRead: 0,
  latinFont: 0,
  cjkFont: 0,
  monoFont: 0,
  translationMode: 0,
  listTranslationMode: 0,
};

export type SettingField = keyof typeof changeGenerations;

export function beginChange(field: SettingField): number {
  return ++changeGenerations[field];
}

// Applies the change locally right away and persists just that key. The
// generation (taken synchronously when the user makes the change) ensures a
// stale in-flight save (awaited catalog load, seed, or slow PATCH) neither
// sends nor errors after a newer change has superseded it. On a failed save
// the caller's revert runs; a 401 is already being handled by the global
// interceptor.
export function useApplySetting() {
  const { t } = useI18n();

  return useCallback(
    async (
      field: SettingField,
      generation: number,
      patch: UpdateSettingsRequest,
      apply: () => void,
      revert: () => void,
    ) => {
      apply();
      try {
        // Let a first-load seed finish first so it cannot land on top of
        // this change; a superseded change skips the request entirely.
        await waitForSettingsSeed();
        if (changeGenerations[field] !== generation) {
          return;
        }
        await settingsAPI.update(patch);
      } catch (error) {
        if (error instanceof APIError && error.status === 401) {
          return;
        }
        if (changeGenerations[field] !== generation) {
          return;
        }
        revert();
        toast.error(t("settings.syncFailed"));
      }
    },
    [t],
  );
}
