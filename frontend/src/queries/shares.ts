import { useMutation } from "@tanstack/react-query";
import { shareAPI, type CreateShareRequest } from "@/lib/api";

export function useCreateShare() {
  return useMutation({
    mutationFn: async (data: CreateShareRequest) => {
      const res = await shareAPI.create(data);
      return res.data!;
    },
  });
}
