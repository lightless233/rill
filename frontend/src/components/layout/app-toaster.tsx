import { useTheme } from "next-themes";
import { Toaster } from "@/components/ui/sonner";

export function AppToaster() {
  const { resolvedTheme } = useTheme();
  const isDark = resolvedTheme === "dark" || resolvedTheme === "night";
  return <Toaster position="top-center" theme={isDark ? "dark" : "light"} />;
}
