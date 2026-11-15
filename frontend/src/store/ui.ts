import { create } from "zustand";
import type { Feed } from "@/lib/api";

const COLLAPSED_GROUPS_STORAGE_KEY = "fusion-sidebar-collapsed-groups";
const ARTICLE_DRAWER_WIDTH_STORAGE_KEY = "rill-article-drawer-width";

export const ARTICLE_DRAWER_MIN_WIDTH = 480;
export const ARTICLE_DRAWER_EDGE_MARGIN = 24;

export function clampArticleDrawerWidth(
  width: number,
  viewportWidth: number,
): number {
  const maxWidth = Math.max(
    ARTICLE_DRAWER_MIN_WIDTH,
    viewportWidth - ARTICLE_DRAWER_EDGE_MARGIN,
  );
  return Math.round(
    Math.min(maxWidth, Math.max(ARTICLE_DRAWER_MIN_WIDTH, width)),
  );
}

function loadCollapsedGroupIds(): number[] {
  if (typeof window === "undefined") {
    return [];
  }
  try {
    const raw = window.localStorage.getItem(COLLAPSED_GROUPS_STORAGE_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (id): id is number =>
        typeof id === "number" && Number.isInteger(id) && id > 0,
    );
  } catch {
    return [];
  }
}

function saveCollapsedGroupIds(ids: number[]) {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(
      COLLAPSED_GROUPS_STORAGE_KEY,
      JSON.stringify(ids),
    );
  } catch {
    // Ignore storage quota / private mode write errors.
  }
}

function loadArticleDrawerWidth(): number | null {
  if (typeof window === "undefined") {
    return null;
  }
  try {
    const raw = window.localStorage.getItem(ARTICLE_DRAWER_WIDTH_STORAGE_KEY);
    if (!raw) return null;
    const parsed = Number(raw);
    if (!Number.isFinite(parsed) || parsed < ARTICLE_DRAWER_MIN_WIDTH) {
      return null;
    }
    return Math.round(parsed);
  } catch {
    return null;
  }
}

function saveArticleDrawerWidth(width: number | null) {
  if (typeof window === "undefined") {
    return;
  }
  try {
    if (width === null) {
      window.localStorage.removeItem(ARTICLE_DRAWER_WIDTH_STORAGE_KEY);
    } else {
      window.localStorage.setItem(
        ARTICLE_DRAWER_WIDTH_STORAGE_KEY,
        String(Math.round(width)),
      );
    }
  } catch {
    // Ignore storage quota / private mode write errors.
  }
}

export type SettingsTab = "general" | "translation" | "about";

interface UIState {
  // Modals
  isSearchOpen: boolean;
  isSettingsOpen: boolean;
  settingsTab: SettingsTab;
  isAddGroupOpen: boolean;
  isAddFeedOpen: boolean;
  isEditFeedOpen: boolean;
  editingFeed: Feed | null;
  isImportOpmlOpen: boolean;
  isShortcutsOpen: boolean;

  // Sidebar & reader layout
  isSidebarOpen: boolean;
  collapsedGroupIds: number[];
  articleDrawerWidth: number | null;

  // Actions
  setSearchOpen: (open: boolean) => void;
  setSettingsOpen: (open: boolean, tab?: SettingsTab) => void;
  setSettingsTab: (tab: SettingsTab) => void;
  setAddGroupOpen: (open: boolean) => void;
  setAddFeedOpen: (open: boolean) => void;
  setEditFeedOpen: (open: boolean, feed?: Feed) => void;
  setImportOpmlOpen: (open: boolean) => void;
  setShortcutsOpen: (open: boolean) => void;
  setSidebarOpen: (open: boolean) => void;
  setCollapsedGroupIds: (ids: number[]) => void;
  setGroupOpen: (groupId: number, open: boolean) => void;
  setArticleDrawerWidth: (width: number | null) => void;
}

export const useUIStore = create<UIState>((set) => ({
  isSearchOpen: false,
  isSettingsOpen: false,
  settingsTab: "general",
  isAddGroupOpen: false,
  isAddFeedOpen: false,
  isEditFeedOpen: false,
  editingFeed: null,
  isImportOpmlOpen: false,
  isShortcutsOpen: false,
  isSidebarOpen: false,
  collapsedGroupIds: loadCollapsedGroupIds(),
  articleDrawerWidth: loadArticleDrawerWidth(),

  setSearchOpen: (open) => set({ isSearchOpen: open }),

  setSettingsOpen: (open, tab) =>
    set((state) => ({
      isSettingsOpen: open,
      settingsTab: tab ?? (open ? state.settingsTab : "general"),
    })),

  setSettingsTab: (tab) => set({ settingsTab: tab }),

  setAddGroupOpen: (open) => set({ isAddGroupOpen: open }),

  setAddFeedOpen: (open) => set({ isAddFeedOpen: open }),

  setEditFeedOpen: (open, feed) =>
    set({ isEditFeedOpen: open, editingFeed: open ? (feed ?? null) : null }),

  setImportOpmlOpen: (open) => set({ isImportOpmlOpen: open }),

  setShortcutsOpen: (open) => set({ isShortcutsOpen: open }),

  setSidebarOpen: (open) => set({ isSidebarOpen: open }),

  setCollapsedGroupIds: (ids) => {
    saveCollapsedGroupIds(ids);
    set({ collapsedGroupIds: ids });
  },

  setGroupOpen: (groupId, open) =>
    set((state) => {
      const isCollapsed = state.collapsedGroupIds.includes(groupId);
      if (open ? !isCollapsed : isCollapsed) {
        return state;
      }
      const next = open
        ? state.collapsedGroupIds.filter((id) => id !== groupId)
        : [...state.collapsedGroupIds, groupId];
      saveCollapsedGroupIds(next);
      return { collapsedGroupIds: next };
    }),

  setArticleDrawerWidth: (width) => {
    const normalized =
      width === null
        ? null
        : typeof window !== "undefined"
          ? clampArticleDrawerWidth(width, window.innerWidth)
          : Math.max(ARTICLE_DRAWER_MIN_WIDTH, Math.round(width));
    saveArticleDrawerWidth(normalized);
    set({ articleDrawerWidth: normalized });
  },
}));
