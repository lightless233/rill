import { queryOptions, useQuery } from "@tanstack/react-query";
import { settingsAPI } from "@/lib/api";
import { queryKeys } from "./keys";

export const settingsQueries = {
  detail: () =>
    queryOptions({
      queryKey: queryKeys.settings.all,
      queryFn: async () => {
        const res = await settingsAPI.get();
        return res.data!;
      },
    }),
};

export function useSettingsQuery() {
  return useQuery(settingsQueries.detail());
}
