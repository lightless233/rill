# 005: Unread scope advance implementation plan

Status: planning only; implementation is not authorized by this document.

Contract: [Spec 005](../specs/005-2026-11-17-unread-scope-advance.md).

## Implementation approach

Implement this as one focused frontend change. The missing behavior is an
entry into the existing unread navigation when a scope is already read, not a
new navigation system or a change to article read-state persistence.

### T1 — Connect the zero-unread header action to existing navigation

Primary scope:

- [ArticleList](../../frontend/src/components/article/article-list.tsx).
- Existing [i18n dictionaries](../../frontend/src/lib/i18n/messages/).

Change:

- Distinguish the existing mark-read action from the new navigation action
  using the current scope, filter, scope unread count and loaded unread count.
  Reuse `getScopeUnreadCount` rather than adding another counting rule.
- Update the primary button's `disabled` condition as well as its click
  handler. Keep the existing `canMarkAllRead` eligibility and combine it with
  the new navigation eligibility:

  ```ts
  canAdvance = scopedUnread && metadataReady &&
    scopeUnreadCount === 0 && unreadCount === 0;
  disabled = isMarkingRead || !(canMarkAllRead || canAdvance);
  ```

  Here `scopedUnread` means the Unread tab with a selected feed or group.
  `metadataReady` requires successful feeds and groups queries and an existing
  current scope: the selected feed is present in feeds, or the selected group
  is present in groups. Read success states from `useFeedLookup` and
  `useGroups`, including the groups success state in feed scope because feed
  ordering also depends on groups. Defaulted empty arrays or a missing scope
  are not evidence of zero unread. These gates apply to navigation, not to the
  existing mark-read eligibility.
- Do not require another unread candidate for `canAdvance`: the existing
  `/all` fallback is also an enabled navigation outcome. Preserve
  `isMarkingRead` disabling for both primary actions.
- Keep the existing mark-all handler for scopes that still have unread items.
  Branch to navigation before entering that mutation/confirmation/undo flow
  only when `canAdvance` is true.
- Reuse `resolveNextUnreadTarget` from
  [unread-navigation.ts](../../frontend/src/lib/unread-navigation.ts).
  For this mutation-free click, use the current feeds snapshot for both of its
  before/after inputs. Existing mutation callers must retain their distinct
  pre-action and post-action snapshots; do not change their ordering semantics.
- Reuse `applyTargetNavigation` and
  [navigateToList](../../frontend/src/hooks/use-url-state.ts), including the
  existing `/all` fallback. Do not broaden the resolver to All or Starred tabs.
- Rely on the existing `ArticleList` cleanup and
  [article pins store](../../frontend/src/store/article-pins.ts) when navigation
  leaves the source scope. Do not clear pins when unread counts merely reach
  zero, and do not change drawer auto-mark-read behavior.
- Render localized feed/group navigation labels and an appropriate existing
  icon in the primary button. Reuse the current Button and dropdown; leave
  generated UI component source unchanged. Keep the same navigation label when
  the destination is the `/all` fallback; this known wording trade-off does not
  require a third button state. No extra explanation or controls.

Completion evidence: the single-article report can be followed through the
real header button to another unread scope without an extra read mutation;
existing bulk-read and undo paths retain their behavior.

### T2 — Verify the regression and register the interaction

Depends on T1.

- Reuse [unread-navigation tests](../../frontend/src/lib/unread-navigation.test.ts)
  for ordering, wraparound, group navigation, and global fallback. Add coverage
  for an already-zero source scope using the same before/after snapshot where
  existing cases do not cover that input. Do not duplicate the existing matrix
  or add tests that only assert translated strings.
- Cover primary-button availability as well as destination resolution:
  zero-unread navigation is unavailable while feeds/groups metadata is pending
  or failed, or the successful data lacks the current scope; it becomes
  available after valid recovery. Existing mark-read pending states disable
  the button, and absence of another candidate does not disable the fallback.
- Perform one bounded local UI check of the single-article flow, including
  the dynamic label, click, resulting route, and absence of a mark-read request
  on the navigation click. Mark the article read manually, or explicitly enable
  auto-mark-read for the check and restore the original setting afterward;
  do not assume opening alone marks it read. Cover the group/empty-list case
  and the return to mark-read mode after toggling an article unread. Confirm the
  existing All/Starred/global behavior is preserved. User-provided acceptance
  counts as evidence for the cases it covers; do not repeat successful checks
  without a relevant change.
- Use the existing local environment on port 18080 for UI acceptance; preserve
  its database and settings. Do not provision another preview service or add a
  browser framework just for this fix.
- Follow the repository frontend checks: typecheck, unit tests and i18n
  completeness; run lint and the frontend build once for the completed change.
  No backend test/build or production browser run is needed for this scope.
- Add a short rationale beside the read-state section in
  [frontend-design.md](../frontend-design.md): zero unread does not automatically
  advance because pinned rows remain undoable, but the header offers an
  explicit continuation action. Link to the implementation rather than
  copying its conditions or route inventory.

Completion evidence: Spec acceptance cases are accounted for, relevant
frontend checks pass, and the interaction rationale agrees with the code.

## Delivery boundary

This task currently delivers the Spec and Plan only. Stop before implementing
T1/T2. A later implementation request does not itself authorize pushing,
deployment, database backups, or production functional acceptance.
