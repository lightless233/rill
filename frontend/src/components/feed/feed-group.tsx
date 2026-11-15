import type {
  KeyboardEvent as ReactKeyboardEvent,
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
} from "react";
import { ChevronRight, GripVertical } from "lucide-react";
import { Collapsible, CollapsibleContent } from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";
import { useUrlState } from "@/hooks/use-url-state";
import { FeedItem } from "./feed-item";
import type { Feed } from "@/lib/api";

interface FeedGroupProps {
  groupId: number;
  name: string;
  feeds: Feed[];
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  isReorderable?: boolean;
  isDragging?: boolean;
  isGrabbed?: boolean;
  reorderHandleLabel?: string;
  onReorderPointerDown?: (event: ReactPointerEvent<HTMLButtonElement>) => void;
  onReorderClick?: (event: ReactMouseEvent<HTMLButtonElement>) => void;
  onReorderKeyDown?: (event: ReactKeyboardEvent<HTMLButtonElement>) => void;
  onReorderBlur?: () => void;
}

export function FeedGroup({
  groupId,
  name,
  feeds,
  isOpen,
  onOpenChange,
  isReorderable = false,
  isDragging = false,
  isGrabbed = false,
  reorderHandleLabel,
  onReorderPointerDown,
  onReorderClick,
  onReorderKeyDown,
  onReorderBlur,
}: FeedGroupProps) {
  const { selectedGroupId, setSelectedGroup } = useUrlState();
  const isSelected = selectedGroupId === groupId;

  const unreadCount = feeds.reduce(
    (sum, feed) => sum + (feed.unread_count || 0),
    0,
  );

  return (
    <div
      data-group-item={groupId}
      data-dragging={isDragging ? "true" : undefined}
      data-grabbed={isGrabbed ? "true" : undefined}
      className="group-reorder-item w-full min-w-0"
    >
      <Collapsible
        open={isOpen}
        onOpenChange={onOpenChange}
        className="w-full min-w-0"
      >
        <div
          data-group-row={groupId}
          className={cn(
            "group/group-row flex w-full min-w-0 items-center gap-1 rounded-md px-2 py-1 text-sm transition-colors",
            isSelected
              ? "bg-accent text-accent-foreground"
              : "hover:bg-accent/50",
          )}
        >
          <button
            type="button"
            aria-expanded={isOpen}
            aria-label={name}
            onClick={(e) => {
              e.stopPropagation();
              onOpenChange(!isOpen);
            }}
            className="shrink-0 p-1 -m-1 rounded-md transition-colors hover:bg-foreground/10"
          >
            <ChevronRight
              aria-hidden="true"
              className={cn(
                "h-4 w-4 text-muted-foreground transition-transform",
                isOpen && "rotate-90",
              )}
            />
          </button>
          <button
            type="button"
            onClick={() => setSelectedGroup(groupId)}
            className="flex min-w-0 flex-1 items-center gap-1.5 pl-0.5 text-left"
          >
            <span className="block min-w-0 flex-1 truncate">{name}</span>
            {unreadCount > 0 && (
              <span className="shrink-0 text-[11px] text-muted-foreground">
                {unreadCount}
              </span>
            )}
          </button>
          {isReorderable && (
            <button
              type="button"
              data-drag-handle={groupId}
              aria-label={reorderHandleLabel}
              aria-pressed={isGrabbed}
              onPointerDown={onReorderPointerDown}
              onClick={onReorderClick}
              onKeyDown={onReorderKeyDown}
              onBlur={onReorderBlur}
              className={cn(
                "group-drag-handle flex size-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground transition-opacity hover:bg-foreground/10 hover:text-foreground focus-visible:bg-foreground/10 focus-visible:opacity-100 focus-visible:outline-none",
                isGrabbed
                  ? "bg-foreground/10 text-foreground opacity-100"
                  : "opacity-30 group-hover/group-row:opacity-80 group-focus-within/group-row:opacity-80",
              )}
            >
              <GripVertical className="h-3.5 w-3.5" aria-hidden="true" />
            </button>
          )}
        </div>
        <CollapsibleContent>
          <div className="w-full min-w-0 pl-5">
            {feeds.map((feed) => (
              <FeedItem key={feed.id} feed={feed} />
            ))}
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}
