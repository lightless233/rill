import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/s/$token")({
  head: () => ({
    meta: [{ name: "robots", content: "noindex, nofollow" }],
  }),
});
