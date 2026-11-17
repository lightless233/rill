# 005: Continue reading after a scope reaches zero unread

Status: proposed; documentation only.

## Goal and scope

Fix the reported reading dead end: a feed has one unread article, opening it
marks it read, and the header's primary action becomes disabled even though the
user wants to continue to another unread feed.

The reported screenshot uses the **Unread** tab. This work covers feed and
group scopes in that tab only. Adding navigation through already-read content
in the All tab is outside this fix.

The intended interaction remains explicit: reading the last article does not
move the user away; clicking the header action continues reading.

## Required behavior

### Primary action

| Context | Primary action |
| --- | --- |
| Unread tab, feed/group scope, unread articles remain | Preserve the existing mark-all-read action. |
| Unread tab, feed scope, scope and loaded unread counts are both zero | Enable a navigation action labeled "Next feed". |
| Unread tab, group scope, scope and loaded unread counts are both zero | Enable a navigation action labeled "Next group". |
| Global scope, All tab, or Starred tab | Preserve existing behavior; do not add the navigation action. |

- The zero-unread action works whether the list still contains just-read rows
  or has become empty. It does not depend on a visible article remaining.
- Existing mark-read pending-state disabling remains in effect. Unavailable or
  failed feed/group metadata must not be interpreted as a confirmed empty
  scope or used to choose a navigation destination. Keep the new action
  unavailable until the existing queries recover; no new error UI is required.
- If unread articles become visible in the current scope again, the primary
  action returns to marking them read.
- Use the existing split button. Update its label and icon to reflect
  navigation, with localized copy following the existing dictionaries.
- The loaded-only dropdown remains a mark-read action and stays unavailable
  when there are no loaded unread articles.

### Navigation

- A click in a feed scope moves to the next feed with unread articles; a click
  in a group scope moves to the next group containing unread articles.
- Follow the existing sidebar order at click time, skip the current scope and
  scopes without unread articles, and wrap around to earlier entries.
- Keep the Unread filter when navigating to another feed or group. Clear the
  article selection so no article opens automatically.
- If no other eligible scope exists, use the existing global All view fallback
  (`/all`). The zero-unread action remains clickable in this case.
- Navigation performs no mark-read mutation, opens no confirmation dialog, and
  creates no mark-read success or undo toast. Normal destination list fetching
  still applies.
- Leaving the source list clears its read-state pins through the existing
  list lifecycle. Until the click, just-read rows remain available for toggling
  back to unread. Do not replace this with automatic navigation on count change.

## Preservation and non-goals

- Preserve marking all/loaded articles read, their existing post-success
  navigation, failure handling and undo behavior.
- Preserve the All and Starred tabs, global-scope confirmation rules, and
  article auto-mark-read settings.
- No backend, API, schema, polling, persisted navigation state or dependency
  changes. Do not introduce another sidebar ordering or navigation algorithm.
- No deployment or production acceptance is part of this documentation task.

## Acceptance criteria

1. In a feed's Unread tab with one unread article, opening the article marks it
   read and keeps its row visible. After returning to the list, "Next feed" is
   enabled. Clicking it opens another unread feed without a mark-read request,
   confirmation dialog, or new undo toast; it does not open an article.
2. The same interaction works in a group scope with "Next group", and in an
   already empty scoped unread list.
3. Navigation skips fully read scopes, wraps when needed, and enters `/all`
   when no other unread scope exists. The existing unread-first sidebar order
   determines the destination.
4. Toggling the last article back to unread restores the mark-read action.
   Missing/failed navigation metadata does not enable the zero-unread action
   based on fabricated zero counts; successful recovery restores it.
5. With unread articles remaining, the existing mark-all/loaded-read and undo
   flows still work. The global scope and All/Starred tabs receive no new
   navigation behavior.

## Existing design references

- [Read-state interaction rationale](../frontend-design.md#read-state-changes-grayed-rows-are-an-undo-affordance-not-stale-ui).
- [Implementation plan](../plans/005-2026-11-17-unread-scope-advance.md).
