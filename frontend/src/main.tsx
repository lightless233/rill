import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { QueryClientProvider } from "@tanstack/react-query";
import { ThemeProvider } from "next-themes";
import { routeTree } from "./routeTree.gen";
import { AppToaster } from "@/components/layout/app-toaster";
import { setUnauthorizedCallback } from "@/lib/api";
import { applyArticleFontPreferences } from "@/lib/fonts";
import { preloadLocaleMessages } from "@/lib/i18n";
import { queryClient } from "@/lib/query-client";
import { registerPWA } from "@/lib/pwa";
import { usePreferencesStore } from "@/store";
import { themeOptions } from "@/lib/themes";
import "./index.css";

// Create a new router instance
const router = createRouter({ routeTree });

registerPWA();

// A page restored from the back-forward cache (e.g. via the back button after
// signing out) shows the pre-logout state and skips route guards; reload it so
// the session check runs again.
window.addEventListener("pageshow", (event) => {
  if (event.persisted) {
    window.location.reload();
  }
});

setUnauthorizedCallback(() => {
  window.location.href = "/login";
});

const initialPreferences = usePreferencesStore.getState();
document.documentElement.lang = initialPreferences.locale;
void preloadLocaleMessages(initialPreferences.locale);
applyArticleFontPreferences({
  latinFont: initialPreferences.latinFont,
  cjkFont: initialPreferences.cjkFont,
  monoFont: initialPreferences.monoFont,
});
usePreferencesStore.subscribe((state, prevState) => {
  if (state.locale !== prevState.locale) {
    document.documentElement.lang = state.locale;
    void preloadLocaleMessages(state.locale);
  }
  if (
    state.latinFont !== prevState.latinFont ||
    state.cjkFont !== prevState.cjkFont ||
    state.monoFont !== prevState.monoFont
  ) {
    applyArticleFontPreferences({
      latinFont: state.latinFont,
      cjkFont: state.cjkFont,
      monoFont: state.monoFont,
    });
  }
});

// Register the router instance for type safety
declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider
        attribute="class"
        defaultTheme="system"
        themes={themeOptions.filter((theme) => theme !== "system")}
        enableSystem
        enableColorScheme={false}
      >
        <RouterProvider router={router} />
        <AppToaster />
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
);
