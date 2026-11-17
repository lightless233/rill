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
- Only offer the new action in scoped Unread views after feed and group
  metadata is available successfully. Use the query states already exposed by
  `useFeedLookup` and `useGroups`; defaulted empty arrays are not evidence of
  zero unread. Preserve existing pending-state disabling for mark-read work.
- Keep the existing mark-all handler for scopes that still have unread items.
  Branch to navigation before entering that mutation/confirmation/undo flow
  when both unread counts are zero.
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
  generated UI component source unchanged. No extra explanation or controls.

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
- Perform one bounded local UI check of the single-article flow, including
  the dynamic label, click, resulting route, and absence of a mark-read request
  on the navigation click. Cover the analogous group/empty-list case and the
  return to mark-read mode after toggling an article unread. Confirm the
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
