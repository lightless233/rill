import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { flushSync } from "react-dom";
import type { Group } from "@/lib/api";
import {
  moveGroupInVisibleList,
  reorderGroupsByDropIndex,
} from "@/lib/feed-sort";
import type { TranslationKey } from "@/lib/i18n";

interface UseGroupReorderOptions {
  groups: Group[];
  visibleGroups: Group[];
  onCommitReorder: (orderedGroups: Group[]) => void;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

interface PointerDragState {
  groupId: number;
  groupName: string;
  pointerId: number;
  handleEl: HTMLButtonElement;
  itemEl: HTMLElement;
  listEl: HTMLDivElement;
  scrollContainer: HTMLElement | null;
  startClientY: number;
  lastClientY: number;
  startScrollTop: number;
  startIndex: number;
  dropIndex: number | null;
  dragGeneration: number;
}

const FLIP_DURATION_MS = 180;
const FLIP_EASING = "cubic-bezier(0.2, 0, 0, 1)";
const MAX_AUTOSCROLL_PX_PER_SEC = 520;

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

function getItemElements(listEl: HTMLElement): HTMLElement[] {
  return Array.from(listEl.querySelectorAll<HTMLElement>("[data-group-item]"));
}

function clearDropMarker(listEl: HTMLElement) {
  listEl.removeAttribute("data-drop");
  listEl.style.removeProperty("--drop-x");
  listEl.style.removeProperty("--drop-y");
  listEl.style.removeProperty("--drop-w");
}

function measureRects(listEl: HTMLElement): Map<string, DOMRect> {
  const rects = new Map<string, DOMRect>();
  for (const el of getItemElements(listEl)) {
    el.getAnimations().forEach((anim) => anim.finish());
    const key = el.dataset.groupItem;
    if (key) {
      rects.set(key, el.getBoundingClientRect());
    }
  }
  return rects;
}

function animateFlipFromRects(
  listEl: HTMLElement,
  beforeRects: Map<string, DOMRect>,
  settlingEl?: HTMLElement | null,
  activeDragGen?: { current: number },
  expectedGen?: number,
) {
  if (prefersReducedMotion()) {
    return;
  }

  for (const el of getItemElements(listEl)) {
    const key = el.dataset.groupItem;
    if (!key) continue;
    const before = beforeRects.get(key);
    if (!before) continue;
    const after = el.getBoundingClientRect();
    const dy = before.top - after.top;
    if (Math.abs(dy) < 0.5) continue;

    if (el === settlingEl) {
      el.setAttribute("data-settling", "true");
    }

    const anim = el.animate(
      [{ translate: `0 ${dy}px` }, { translate: "0 0" }],
      { duration: FLIP_DURATION_MS, easing: FLIP_EASING },
    );

    if (el === settlingEl) {
      const cleanup = () => {
        if (
          activeDragGen &&
          expectedGen !== undefined &&
          activeDragGen.current !== expectedGen
        ) {
          return;
        }
        el.removeAttribute("data-settling");
      };
      anim.addEventListener("finish", cleanup, { once: true });
      anim.addEventListener("cancel", cleanup, { once: true });
    }
  }
}

function animateBackToSlot(
  itemEl: HTMLElement,
  fromRect: DOMRect,
  activeDragGen: { current: number },
  expectedGen: number,
) {
  if (prefersReducedMotion()) {
    return;
  }

  const finalRect = itemEl.getBoundingClientRect();
  const dy = fromRect.top - finalRect.top;
  if (Math.abs(dy) < 0.5) {
    return;
  }

  itemEl.setAttribute("data-settling", "true");
  const anim = itemEl.animate(
    [{ translate: `0 ${dy}px` }, { translate: "0 0" }],
    { duration: FLIP_DURATION_MS, easing: FLIP_EASING },
  );
  const cleanup = () => {
    if (activeDragGen.current === expectedGen) {
      itemEl.removeAttribute("data-settling");
    }
  };
  anim.addEventListener("finish", cleanup, { once: true });
  anim.addEventListener("cancel", cleanup, { once: true });
}

export function useGroupReorder({
  groups,
  visibleGroups,
  onCommitReorder,
  t,
}: UseGroupReorderOptions) {
  const listRef = useRef<HTMLDivElement | null>(null);
  const pointerDragRef = useRef<PointerDragState | null>(null);
  const rafRef = useRef<number | null>(null);
  const lastFrameTimeRef = useRef<number>(0);
  const dragGenRef = useRef(0);
  const suppressClickRef = useRef(false);
  const isRestoringFocusRef = useRef(false);
  const announceTimerRef = useRef<number | null>(null);
  const pointerListenersRef = useRef<{ cleanup: () => void } | null>(null);

  const [draggingGroupId, setDraggingGroupId] = useState<number | null>(null);
  const [grabbedGroupId, setGrabbedGroupId] = useState<number | null>(null);
  const [keyboardDraftGroups, setKeyboardDraftGroups] = useState<
    Group[] | null
  >(null);
  const grabBaselineRef = useRef<Group[] | null>(null);
  const [statusMessage, setStatusMessage] = useState("");

  const visibleGroupIdsSet = useMemo(
    () => new Set(visibleGroups.map((g) => g.id)),
    [visibleGroups],
  );

  const renderedVisibleGroups = useMemo(() => {
    if (grabbedGroupId !== null && keyboardDraftGroups) {
      return keyboardDraftGroups.filter((g) => visibleGroupIdsSet.has(g.id));
    }
    return visibleGroups;
  }, [
    grabbedGroupId,
    keyboardDraftGroups,
    visibleGroupIdsSet,
    visibleGroups,
  ]);

  const announce = useCallback((message: string) => {
    if (announceTimerRef.current !== null) {
      window.clearTimeout(announceTimerRef.current);
      announceTimerRef.current = null;
    }
    setStatusMessage("");
    announceTimerRef.current = window.setTimeout(() => {
      announceTimerRef.current = null;
      setStatusMessage(message);
    }, 60);
  }, []);

  const updatePointerVisuals = useCallback((state: PointerDragState) => {
    const currentScrollTop = state.scrollContainer?.scrollTop ?? 0;
    const dy =
      state.lastClientY -
      state.startClientY +
      (currentScrollTop - state.startScrollTop);
    state.itemEl.style.translate = `0 ${dy}px`;

    const listRect = state.listEl.getBoundingClientRect();
    const scrollRect = state.scrollContainer?.getBoundingClientRect();
    const clampedClientY = scrollRect
      ? Math.max(scrollRect.top, Math.min(state.lastClientY, scrollRect.bottom))
      : Math.max(listRect.top, Math.min(state.lastClientY, listRect.bottom));
    const pointerYInList =
      clampedClientY - listRect.top - state.listEl.clientTop;

    const itemEls = getItemElements(state.listEl);
    let dropIndex = itemEls.length;
    for (let i = 0; i < itemEls.length; i += 1) {
      const midpoint = itemEls[i].offsetTop + itemEls[i].offsetHeight / 2;
      if (pointerYInList < midpoint) {
        dropIndex = i;
        break;
      }
    }
    state.dropIndex = dropIndex;

    if (dropIndex === state.startIndex || dropIndex === state.startIndex + 1) {
      clearDropMarker(state.listEl);
      return;
    }

    const prevEl = dropIndex > 0 ? itemEls[dropIndex - 1] : null;
    const nextEl = dropIndex < itemEls.length ? itemEls[dropIndex] : null;

    let markerY = listRect.top;
    if (prevEl && nextEl) {
      markerY =
        (prevEl.getBoundingClientRect().bottom +
          nextEl.getBoundingClientRect().top) /
        2;
    } else if (nextEl) {
      markerY = nextEl.getBoundingClientRect().top - 1;
    } else if (prevEl) {
      markerY = prevEl.getBoundingClientRect().bottom + 1;
    }

    if (
      scrollRect &&
      (markerY < scrollRect.top - 2 || markerY > scrollRect.bottom + 2)
    ) {
      clearDropMarker(state.listEl);
      return;
    }

    state.listEl.setAttribute("data-drop", "true");
    state.listEl.style.setProperty("--drop-x", `${listRect.left + 6}px`);
    state.listEl.style.setProperty("--drop-y", `${markerY}px`);
    state.listEl.style.setProperty(
      "--drop-w",
      `${Math.max(0, listRect.width - 10)}px`,
    );
  }, []);

  const stopAutoscroll = useCallback(() => {
    if (rafRef.current !== null) {
      window.cancelAnimationFrame(rafRef.current);
      rafRef.current = null;
    }
    lastFrameTimeRef.current = 0;
  }, []);

  const finishPointerDrag = useCallback(
    (mode: "commit" | "escape" | "cancel") => {
      const state = pointerDragRef.current;
      if (!state) return;
      pointerDragRef.current = null;

      stopAutoscroll();
      pointerListenersRef.current?.cleanup();
      pointerListenersRef.current = null;

      document.documentElement.classList.remove("group-reorder-dragging");
      clearDropMarker(state.listEl);

      try {
        if (state.handleEl.hasPointerCapture(state.pointerId)) {
          state.handleEl.releasePointerCapture(state.pointerId);
        }
      } catch {
        // Synthetic pointer events in tests may not hold an active pointer capture.
      }

      const currentVisibleIds = visibleGroups.map((g) => g.id);
      const nextGroups =
        mode === "commit" && state.dropIndex !== null
          ? reorderGroupsByDropIndex(
              groups,
              currentVisibleIds,
              state.groupId,
              state.dropIndex,
            )
          : null;

      if (nextGroups) {
        const beforeRects = measureRects(state.listEl);
        const releasedItemRect = state.itemEl.getBoundingClientRect();
        beforeRects.set(String(state.groupId), releasedItemRect);
        state.itemEl.style.translate = "";

        flushSync(() => {
          setDraggingGroupId(null);
          onCommitReorder(nextGroups);
        });

        const movedEl = state.listEl.querySelector<HTMLElement>(
          `[data-group-item="${state.groupId}"]`,
        );
        movedEl?.scrollIntoView({ block: "nearest", inline: "nearest" });
        animateFlipFromRects(
          state.listEl,
          beforeRects,
          movedEl,
          dragGenRef,
          state.dragGeneration,
        );

        const nextVisible = nextGroups.filter((g) =>
          visibleGroupIdsSet.has(g.id),
        );
        const newPos = nextVisible.findIndex((g) => g.id === state.groupId);
        if (newPos !== -1) {
          announce(
            t("group.reorder.dropped", {
              name: state.groupName,
              position: newPos + 1,
              total: nextVisible.length,
            }),
          );
        }
        return;
      }

      const releasedRect = state.itemEl.getBoundingClientRect();
      state.itemEl.style.translate = "";
      flushSync(() => {
        setDraggingGroupId(null);
      });

      if (mode === "escape") {
        state.itemEl.scrollIntoView({ block: "nearest", inline: "nearest" });
        announce(t("group.reorder.cancelled", { name: state.groupName }));
      }
      animateBackToSlot(
        state.itemEl,
        releasedRect,
        dragGenRef,
        state.dragGeneration,
      );
    },
    [
      announce,
      groups,
      onCommitReorder,
      stopAutoscroll,
      t,
      visibleGroupIdsSet,
      visibleGroups,
    ],
  );

  const finishPointerDragRef = useRef(finishPointerDrag);
  const updatePointerVisualsRef = useRef(updatePointerVisuals);
  useEffect(() => {
    finishPointerDragRef.current = finishPointerDrag;
    updatePointerVisualsRef.current = updatePointerVisuals;
  });

  const onHandlePointerDown = useCallback(
    (
      groupId: number,
      groupName: string,
      event: ReactPointerEvent<HTMLButtonElement>,
    ) => {
      if (!event.isPrimary || event.button !== 0) {
        return;
      }
      if (pointerDragRef.current || visibleGroups.length <= 1) {
        return;
      }

      if (grabbedGroupId !== null) {
        setGrabbedGroupId(null);
        setKeyboardDraftGroups(null);
        grabBaselineRef.current = null;
      }

      const listEl = listRef.current;
      const handleEl = event.currentTarget;
      const itemEl = handleEl.closest<HTMLElement>("[data-group-item]");
      if (!listEl || !itemEl) {
        return;
      }

      event.preventDefault();
      suppressClickRef.current = true;
      window.requestAnimationFrame(() => {
        suppressClickRef.current = false;
      });

      itemEl.getAnimations().forEach((anim) => anim.finish());
      itemEl.removeAttribute("data-settling");

      const itemEls = getItemElements(listEl);
      const startIndex = itemEls.indexOf(itemEl);
      if (startIndex === -1) {
        return;
      }

      const scrollContainer = listEl.closest<HTMLElement>(
        '[data-slot="scroll-area-viewport"]',
      );

      dragGenRef.current += 1;
      const state: PointerDragState = {
        groupId,
        groupName,
        pointerId: event.pointerId,
        handleEl,
        itemEl,
        listEl,
        scrollContainer,
        startClientY: event.clientY,
        lastClientY: event.clientY,
        startScrollTop: scrollContainer?.scrollTop ?? 0,
        startIndex,
        dropIndex: null,
        dragGeneration: dragGenRef.current,
      };
      pointerDragRef.current = state;

      try {
        handleEl.setPointerCapture(event.pointerId);
      } catch {
        // Synthetic pointer events in tests may not map to a live browser pointer.
      }
      document.documentElement.classList.add("group-reorder-dragging");
      setDraggingGroupId(groupId);
      updatePointerVisuals(state);

      const handlePointerMove = (moveEvent: PointerEvent) => {
        const active = pointerDragRef.current;
        if (!active || moveEvent.pointerId !== active.pointerId) return;
        active.lastClientY = moveEvent.clientY;
        updatePointerVisualsRef.current(active);
      };

      const handlePointerUp = (upEvent: PointerEvent) => {
        const active = pointerDragRef.current;
        if (!active || upEvent.pointerId !== active.pointerId) return;
        active.lastClientY = upEvent.clientY;
        updatePointerVisualsRef.current(active);
        finishPointerDragRef.current("commit");
      };

      const handlePointerCancel = (cancelEvent: PointerEvent) => {
        const active = pointerDragRef.current;
        if (!active || cancelEvent.pointerId !== active.pointerId) return;
        finishPointerDragRef.current("cancel");
      };

      const handleLostCapture = (lostEvent: PointerEvent) => {
        const active = pointerDragRef.current;
        if (!active || lostEvent.pointerId !== active.pointerId) return;
        finishPointerDragRef.current("cancel");
      };

      const handleEscape = (keyEvent: KeyboardEvent) => {
        if (keyEvent.key !== "Escape" || !pointerDragRef.current) return;
        keyEvent.preventDefault();
        keyEvent.stopPropagation();
        finishPointerDragRef.current("escape");
      };

      window.addEventListener("pointermove", handlePointerMove);
      window.addEventListener("pointerup", handlePointerUp);
      window.addEventListener("pointercancel", handlePointerCancel);
      handleEl.addEventListener("lostpointercapture", handleLostCapture);
      window.addEventListener("keydown", handleEscape, { capture: true });

      pointerListenersRef.current = {
        cleanup: () => {
          window.removeEventListener("pointermove", handlePointerMove);
          window.removeEventListener("pointerup", handlePointerUp);
          window.removeEventListener("pointercancel", handlePointerCancel);
          handleEl.removeEventListener("lostpointercapture", handleLostCapture);
          window.removeEventListener("keydown", handleEscape, { capture: true });
        },
      };

      const stepAutoscroll = (timestamp: number) => {
        const active = pointerDragRef.current;
        if (!active) return;

        const dt = lastFrameTimeRef.current
          ? Math.min((timestamp - lastFrameTimeRef.current) / 1000, 0.05)
          : 0;
        lastFrameTimeRef.current = timestamp;

        const scroller = active.scrollContainer;
        if (scroller && dt > 0 && scroller.scrollHeight > scroller.clientHeight + 1) {
          const rect = scroller.getBoundingClientRect();
          const edgeZone = Math.min(
            Math.max(active.itemEl.offsetHeight, 36),
            rect.height / 3,
            72,
          );
          let delta = 0;

          if (active.lastClientY < rect.top + edgeZone && scroller.scrollTop > 0) {
            const ratio = Math.min(
              (rect.top + edgeZone - active.lastClientY) / edgeZone,
              2,
            );
            delta = -MAX_AUTOSCROLL_PX_PER_SEC * ratio * dt;
          } else if (
            active.lastClientY > rect.bottom - edgeZone &&
            scroller.scrollTop < scroller.scrollHeight - scroller.clientHeight
          ) {
            const ratio = Math.min(
              (active.lastClientY - (rect.bottom - edgeZone)) / edgeZone,
              2,
            );
            delta = MAX_AUTOSCROLL_PX_PER_SEC * ratio * dt;
          }

          if (delta !== 0) {
            const prevTop = scroller.scrollTop;
            scroller.scrollTop += delta;
            if (scroller.scrollTop !== prevTop) {
              updatePointerVisualsRef.current(active);
            }
          }
        }

        rafRef.current = window.requestAnimationFrame(stepAutoscroll);
      };

      rafRef.current = window.requestAnimationFrame(stepAutoscroll);
    },
    [grabbedGroupId, updatePointerVisuals, visibleGroups.length],
  );

  const commitKeyboardGrab = useCallback(
    (groupId: number, groupName: string) => {
      const baseline = grabBaselineRef.current;
      const draft = keyboardDraftGroups;
      setGrabbedGroupId(null);
      setKeyboardDraftGroups(null);
      grabBaselineRef.current = null;

      if (!baseline || !draft) return;

      const changed = draft.some((g, idx) => baseline[idx]?.id !== g.id);
      if (changed) {
        onCommitReorder(draft);
        const nextVisible = draft.filter((g) => visibleGroupIdsSet.has(g.id));
        const newPos = nextVisible.findIndex((g) => g.id === groupId);
        if (newPos !== -1) {
          announce(
            t("group.reorder.dropped", {
              name: groupName,
              position: newPos + 1,
              total: nextVisible.length,
            }),
          );
        }
      }
    },
    [announce, keyboardDraftGroups, onCommitReorder, t, visibleGroupIdsSet],
  );

  const onHandleClick = useCallback(
    (
      groupId: number,
      groupName: string,
      event: ReactMouseEvent<HTMLButtonElement>,
    ) => {
      // Only keyboard/assistive-tech activation toggles grab mode; pointer clicks never do.
      if (event.detail !== 0 || suppressClickRef.current) {
        return;
      }
      if (visibleGroups.length <= 1) {
        return;
      }

      if (grabbedGroupId === groupId) {
        commitKeyboardGrab(groupId, groupName);
        return;
      }

      grabBaselineRef.current = groups;
      setKeyboardDraftGroups(groups);
      setGrabbedGroupId(groupId);

      const pos = visibleGroups.findIndex((g) => g.id === groupId);
      if (pos !== -1) {
        announce(
          t("group.reorder.grabbed", {
            name: groupName,
            position: pos + 1,
            total: visibleGroups.length,
          }),
        );
      }
    },
    [
      announce,
      commitKeyboardGrab,
      grabbedGroupId,
      groups,
      t,
      visibleGroups,
    ],
  );

  const onHandleKeyDown = useCallback(
    (
      groupId: number,
      groupName: string,
      event: ReactKeyboardEvent<HTMLButtonElement>,
    ) => {
      if (grabbedGroupId !== groupId) {
        return;
      }

      const listEl = listRef.current;
      const handleEl = event.currentTarget;

      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();

        const baseline = grabBaselineRef.current;
        const draft = keyboardDraftGroups;
        const changed =
          baseline && draft && draft.some((g, i) => baseline[i]?.id !== g.id);

        if (listEl && changed) {
          const beforeRects = measureRects(listEl);
          isRestoringFocusRef.current = true;
          flushSync(() => {
            setGrabbedGroupId(null);
            setKeyboardDraftGroups(null);
          });
          grabBaselineRef.current = null;
          const restoredItem = listEl.querySelector<HTMLElement>(
            `[data-group-item="${groupId}"]`,
          );
          restoredItem?.scrollIntoView({ block: "nearest", inline: "nearest" });
          const restoredHandle = listEl.querySelector<HTMLButtonElement>(
            `[data-drag-handle="${groupId}"]`,
          );
          restoredHandle?.focus({ preventScroll: true });
          isRestoringFocusRef.current = false;
          animateFlipFromRects(listEl, beforeRects);
        } else {
          setGrabbedGroupId(null);
          setKeyboardDraftGroups(null);
          grabBaselineRef.current = null;
        }

        announce(t("group.reorder.cancelled", { name: groupName }));
        return;
      }

      if (event.key !== "ArrowUp" && event.key !== "ArrowDown") {
        return;
      }

      event.preventDefault();
      const currentOrder = keyboardDraftGroups ?? groups;
      const currentVisibleIds = renderedVisibleGroups.map((g) => g.id);
      const delta = event.key === "ArrowUp" ? -1 : 1;
      const nextDraft = moveGroupInVisibleList(
        currentOrder,
        currentVisibleIds,
        groupId,
        delta,
      );
      if (!nextDraft) {
        return;
      }

      const beforeRects = listEl ? measureRects(listEl) : null;
      isRestoringFocusRef.current = true;
      flushSync(() => {
        setKeyboardDraftGroups(nextDraft);
      });

      const movedEl = listEl?.querySelector<HTMLElement>(
        `[data-group-item="${groupId}"]`,
      );
      movedEl?.scrollIntoView({ block: "nearest", inline: "nearest" });
      const targetHandle =
        listEl?.querySelector<HTMLButtonElement>(
          `[data-drag-handle="${groupId}"]`,
        ) ?? handleEl;
      targetHandle.focus({ preventScroll: true });
      isRestoringFocusRef.current = false;

      if (listEl && beforeRects) {
        animateFlipFromRects(listEl, beforeRects);
      }

      const nextVisible = nextDraft.filter((g) => visibleGroupIdsSet.has(g.id));
      const newPos = nextVisible.findIndex((g) => g.id === groupId);
      if (newPos !== -1) {
        announce(
          t("group.reorder.moved", {
            name: groupName,
            position: newPos + 1,
            total: nextVisible.length,
          }),
        );
      }
    },
    [
      announce,
      grabbedGroupId,
      groups,
      keyboardDraftGroups,
      renderedVisibleGroups,
      t,
      visibleGroupIdsSet,
    ],
  );

  const onHandleBlur = useCallback(
    (groupId: number, groupName: string) => {
      if (isRestoringFocusRef.current || grabbedGroupId !== groupId) {
        return;
      }
      commitKeyboardGrab(groupId, groupName);
    },
    [commitKeyboardGrab, grabbedGroupId],
  );

  useEffect(() => {
    return () => {
      if (announceTimerRef.current !== null) {
        window.clearTimeout(announceTimerRef.current);
      }
      if (rafRef.current !== null) {
        window.cancelAnimationFrame(rafRef.current);
      }
      pointerListenersRef.current?.cleanup();
      document.documentElement.classList.remove("group-reorder-dragging");
    };
  }, []);

  return {
    listRef,
    renderedVisibleGroups,
    draggingGroupId,
    grabbedGroupId,
    statusMessage,
    onHandlePointerDown,
    onHandleClick,
    onHandleKeyDown,
    onHandleBlur,
  };
}
