# 004: Live unread counts implementation plan

Status: planning only; do not start implementation.

Contract: [Spec 004](../specs/004-2026-11-16-live-unread-counts.md).

## Execution scope

One implementing agent can execute these units sequentially. Keep the current
single-process architecture, session cookie authentication, feed pull worker
and TanStack Query cache. This work adds one in-memory notification path and
one authenticated SSE endpoint; it does not change the database schema, feed
pulling rules, article list queries or translation streams.

WSS is explicitly out of scope.

## Implementation units

### T1 — Add the event hub and pull notification

Problem: the pull worker can insert new items, but no component currently
publishes that fact to the HTTP layer.

Change:

- Add a small process-local event hub with:
  - subscribe/unsubscribe;
  - non-blocking fan-out to every current subscriber;
  - per-subscriber coalescing so a slow client cannot accumulate a queue;
  - close-all behavior for server shutdown.
- Inject a notification callback or hub reference into the puller. Set it up in
  `cmd/fusion` before the puller starts.
- In the successful new-item path, after `BatchCreateItemsIgnore`, notify only
  when the inserted count is greater than zero.
- Do not notify for `304`, zero-new-item success, fetch errors, name/site URL
  backfills, or feed CRUD.
- Notification must be asynchronous/non-blocking with respect to the pull.

Primary scope: new event hub package, `backend/internal/pull/puller.go`,
`backend/cmd/fusion/main.go`, and focused hub/puller tests.

Completion evidence: a successful pull with new items produces one logical
notification; a 304 or failed pull produces none; multiple subscribers receive
the notification; a subscriber that is not consuming does not block or grow
unbounded; closing the hub releases subscribers.

### T2 — Add the authenticated SSE endpoint

Problem: clients need a long-lived, low-cost way to learn that feed counts may
have changed.

Change:

- Register `GET /api/events` inside the authenticated API group.
- Write `text/event-stream; charset=utf-8` plus `Cache-Control: no-cache` and
  `X-Accel-Buffering: no`.
- Immediately send and flush `event: ready` with `{}`.
- Forward hub notifications as `event: feeds_changed` with `{}`.
- Send a comment heartbeat on a fixed interval below the server write timeout
  and common proxy idle timeouts. Extend the response write deadline on every
  write/heartbeat instead of relying on the global 30-second timeout.
- Unsubscribe on client disconnect.
- On shutdown, close active event streams through the hub/server shutdown hook
  rather than waiting for the global HTTP shutdown deadline.
- Keep the payload free of counts and article data. The authenticated
  `GET /api/feeds` remains the source of truth.
- Do not add event replay or `Last-Event-ID` handling.

Primary scope: `backend/internal/handler/` (new event handler and route
registration), server lifecycle wiring in `cmd/fusion`, and focused handler
tests.

Completion evidence: an authenticated client receives `ready` immediately and
`feeds_changed` after a notification; the response headers and event framing
match the Spec; unauthenticated requests follow the existing 401 behavior;
disconnect unsubscribes; shutdown does not hang on open streams.

### T3 — Consume the stream in the frontend

Problem: the sidebar already derives counts from `GET /api/feeds`, but nothing
refetches that query when the server has new items.

Change:

- Add a GET SSE helper beside the existing translation stream parser, reusing
  the same event-block parsing where practical. It must support aborting,
  comment heartbeats and an authenticated fetch with `credentials: include`.
- Add a `useFeedEvents` hook mounted once in `AppLayout`.
- On stream connection (the `ready` event or successful response) and on every
  `feeds_changed` event, invalidate only the feeds query.
- Do not invalidate item lists, article details, bookmarks, translations,
  settings or app info.
- Reconnect with bounded exponential backoff and reset the backoff after the
  connection succeeds.
- Stop on `401` and let the existing global unauthorized flow handle the
  session; do not retry a known unauthorized stream forever.
- Abort the request and clear timers on unmount. Ensure StrictMode cannot leave
  duplicate live streams.
- Keep the hook out of the login and public share routes; only the
  authenticated `AppLayout` owns it.
- No toast or user-visible banner is added for count changes.

Primary scope: `frontend/src/lib/api/`, a new events hook,
`frontend/src/components/layout/app-layout.tsx`, query keys/feeds cache access,
and focused parser/hook tests where the project’s test setup supports them.

Completion evidence: a `feeds_changed` event refetches the feed list and
updates sidebar and group totals; no article-list cache is touched; reconnect
triggers a catch-up refetch; 401 does not loop; unmount closes the stream.

### T4 — Deployment, documentation and acceptance

Problem: the new long-lived response crosses Caddy and Cloudflare and needs
operational verification; the stream contract and non-goals need durable
records.

Change:

- Verify the stream through the production proxy path:
  - Caddy must flush SSE frames as they are written. If the current proxy
    buffers, add a negative `flush_interval` or equivalent.
  - Compression must not hold small SSE frames. If the current `encode`
    directive buffers `text/event-stream`, exclude that content type.
  - Confirm Cloudflare does not close the stream during the heartbeat interval.
- Document the event endpoint, event names, authentication behavior,
  reconnect/catch-up rule and non-goals in `docs/api-conventions.md`.
- Record the event-hub architecture and single-process limitation in
  `docs/backend-design.md`.
- Record the AppLayout-owned subscription and count-only invalidation rule in
  `docs/frontend-design.md`.
- Add Spec/Plan 004 to the work-item index in `docs/README.md`.
- Verify:
  - backend hub, puller-notification and SSE-handler tests;
  - frontend parser/hook tests if applicable;
  - frontend typecheck, lint, unit tests, i18n and build;
  - browser flow with two authenticated tabs: a new item updates both sidebars
    without reload; a 304/zero-new-item pull does not; reconnect catches up;
  - public share and login pages show no `/api/events` request;
  - existing translation/summary SSE flows still work;
  - the deployed Caddy/Cloudflare path delivers `ready` and `feeds_changed`
    without buffering.

Primary scope: deployment proxy configuration, `docs/api-conventions.md`,
`docs/backend-design.md`, `docs/frontend-design.md`, `docs/README.md`, and
cross-layer acceptance evidence.

Completion evidence: deployed stream works through the real proxy path; counts
update without manual refresh; missed events recover on reconnect; existing
authenticated and public flows are unaffected.

## Integration order

1. T1 event hub and pull notification.
2. T2 authenticated SSE endpoint and shutdown handling.
3. T3 frontend GET stream and AppLayout hook.
4. T4 proxy verification, docs and aggregate acceptance.

T3 depends on T2's event names. Do not build the frontend against a count
payload that the Spec does not define; the event is a signal and the feed list
remains the source of truth.

## Boundaries and failure transitions

- The event stream is not a source of truth. A missed, duplicated or reordered
  signal only causes an extra or delayed `GET /api/feeds`; it must not produce
  a wrong absolute count.
- Count changes from another client's read/unread mutation are not pushed in
  V1. The acting client keeps its optimistic state; another tab catches up on
  its next normal feed-list refresh.
- The stream must not keep the process alive during shutdown.
- Public share routes stay independent of the authenticated app shell and never
  subscribe to feed events.
- No WSS fallback, event replay, database table or multi-replica pub/sub is
  introduced in this version.
