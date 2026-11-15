import { create } from "zustand";
import type { Feed } from "@/lib/api";

const COLLAPSED_GROUPS_STORAGE_KEY = "fusion-sidebar-collapsed-groups";

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

interface UIState {
  // Modals
  isSearchOpen: boolean;
  isSettingsOpen: boolean;
  isAddGroupOpen: boolean;
  isAddFeedOpen: boolean;
  isEditFeedOpen: boolean;
  editingFeed: Feed | null;
  isImportOpmlOpen: boolean;
  isShortcutsOpen: boolean;

  // Sidebar
  isSidebarOpen: boolean;
  collapsedGroupIds: number[];

  // Actions
  setSearchOpen: (open: boolean) => void;
  setSettingsOpen: (open: boolean) => void;
  setAddGroupOpen: (open: boolean) => void;
  setAddFeedOpen: (open: boolean) => void;
  setEditFeedOpen: (open: boolean, feed?: Feed) => void;
  setImportOpmlOpen: (open: boolean) => void;
  setShortcutsOpen: (open: boolean) => void;
  setSidebarOpen: (open: boolean) => void;
  setCollapsedGroupIds: (ids: number[]) => void;
  setGroupOpen: (groupId: number, open: boolean) => void;
}

export const useUIStore = create<UIState>((set) => ({
  isSearchOpen: false,
  isSettingsOpen: false,
  isAddGroupOpen: false,
  isAddFeedOpen: false,
  isEditFeedOpen: false,
  editingFeed: null,
  isImportOpmlOpen: false,
  isShortcutsOpen: false,
  isSidebarOpen: false,
  collapsedGroupIds: loadCollapsedGroupIds(),

  setSearchOpen: (open) => set({ isSearchOpen: open }),

  setSettingsOpen: (open) => set({ isSettingsOpen: open }),

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
}));
