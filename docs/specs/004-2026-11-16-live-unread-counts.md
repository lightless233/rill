# 004: Live unread counts for feeds

Status: design proposed; implementation not started.

## Goal

Update an already open Rill app when a background feed pull inserts new items,
so the sidebar's feed and group unread counts change without a manual page
refresh. Only counts need to update; the article list does not need to insert
new rows live.

## Current state

- There is no WebSocket/WSS connection anywhere in the frontend or backend.
- SSE exists only as client-initiated `POST` streams for article/list
  translation and summaries. Those streams carry one request's progress and
  close when it finishes; they are not a general server event channel.
- Feed and group unread counts come from `GET /api/feeds` and are derived in
  the frontend. The query does not poll and window-focus refetching is disabled,
  so a background pull is invisible until the user navigates or reloads.

## Decisions

- Use **SSE**, not WSS. The server only needs to tell clients that feed counts
  changed; clients never send a message back on this channel. SSE matches that
  direction of data, uses the existing cookie session, reconnects over normal
  HTTP, and avoids WebSocket framing/authentication/reconnect work.
- Add one authenticated `GET /api/events` stream. It carries small signals,
  not article bodies or count payloads.
- A `feeds_changed` event means "the authoritative feed counts may have
  changed". The frontend invalidates the existing feeds query and lets
  `GET /api/feeds` return the current counts. Counts are not duplicated in the
  event payload, so a missed event cannot leave clients with a stale absolute
  count.
- Emit `feeds_changed` only when a pull actually inserts at least one new item.
  `304 Not Modified`, zero new items, failures and feed metadata-only updates
  do not emit. Read/unread mutations from one client are not pushed in this
  version.
- On initial connection and every reconnect, the frontend performs one feeds
  refetch. This catches up after server restarts, network loss or suspended
  tabs without requiring event replay.
- The event stream is owned by the authenticated `AppLayout`. Login and public
  share pages do not connect.
- No database migration, table, shared counter or new persisted state.

## Required behavior

### Stream contract

`GET /api/events` is registered inside the authenticated API group.

- Response: `text/event-stream; charset=utf-8`.
- Headers: `Cache-Control: no-cache` and `X-Accel-Buffering: no`.
- The server immediately writes a `ready` event and flushes it.
- After a pull inserts new items, the server writes one `feeds_changed` event.
- A comment heartbeat is written at a fixed interval shorter than the server
  write timeout and common proxy idle timeouts, so idle connections stay open.
- The stream does not use event IDs or replay. Reconnect catch-up is the feeds
  refetch described above.
- When the client disconnects, the handler unsubscribes. Server shutdown closes
  active event streams instead of waiting for the HTTP shutdown deadline.

Event names and payloads are deliberately minimal:

```text
event: ready
data: {}

event: feeds_changed
data: {}
```

### Backend notification

- A small in-memory event hub owns subscriptions and fans out notifications.
- The puller notifies the hub when `BatchCreateItemsIgnore` reports at least
  one inserted item. It must not notify on a failed pull, a `304`, or a
  successful pull with zero new items.
- Notifications are coalesced per subscriber. A subscriber that is still
  processing the previous signal must not accumulate an unbounded queue.
- A notification must never block a pull or fail a feed refresh.
- The hub is process-local. Multiple backend replicas are out of scope; the
  current deployment is a single process.

### Frontend behavior

- One authenticated event hook runs while `AppLayout` is mounted.
- On `ready`, `feeds_changed`, or a successful reconnect, invalidate the feeds
  query. Do not invalidate the article list, article detail, bookmarks,
  translations or settings queries.
- Sidebar feed counts, group totals and the all-feed total update through the
  existing derived count path.
- No toast or notification is shown for a count update.
- Reconnect with bounded exponential backoff. Reset the backoff after a
  connection is established.
- A `401` stops the stream and uses the existing global unauthorized handling;
  it must not retry forever.
- Unmount aborts the active request and clears reconnect timers.
- StrictMode's mount/unmount/mount sequence must not leave two live streams.

### Deployment behavior

The event stream must survive each production path separately:

- The current `fusion.43-156-28-81.sslip.io` entry resolves directly to the
  origin and does not pass through Cloudflare. It verifies Caddy and the
  application, but it is not evidence for Cloudflare behavior.
- The new `rill.lightless.me` entry is Cloudflare-proxied. Cloudflare
  verification requires the response to carry `server: cloudflare` and a
  `CF-Ray` header and the event stream to work on that entry point.
- Caddy reverse proxy streaming must not buffer the response. If the default
  proxy behavior buffers it, set a negative `flush_interval` or equivalent.
- Response compression must not hold small SSE frames until a buffer fills.
  Exclude `text/event-stream` from compression if the current `encode`
  directive buffers it.
- The heartbeat must keep the connection alive through Cloudflare and the
  server's HTTP write timeout. The current global write timeout is 30 seconds,
  so the stream must extend its write deadline on every write/heartbeat.
- The endpoint remains same-origin, so the session cookie is sent normally and
  no token is placed in the URL.

## Non-goals

- WebSocket/WSS.
- Pushing article content, feed metadata, fetch errors or settings.
- Live insertion into the currently open article list or drawer.
- Cross-tab/cross-device read-state synchronization.
- Browser desktop notifications or unread badges outside the app.
- Event replay, durable queues, multiple backend replicas or Redis.
- Changing the feed/group unread count computation.

## Acceptance criteria

- With an authenticated app open, a pull that inserts a new item updates the
  affected feed's sidebar unread count, its group total and the all-feed total
  without a page reload or manual refresh.
- A pull with zero new items, a `304`, or a failed pull produces no
  `feeds_changed` event.
- Multiple open tabs all receive the update.
- Losing and restoring the event connection triggers a feeds refetch and
  restores the correct counts.
- An expired or missing session produces the normal authentication outcome and
  no endless reconnect loop.
- Login and public share pages never open `/api/events`.
- Existing translation/summary SSE streams continue to work unchanged.
- Backend tests, frontend tests, typecheck, lint, i18n validation and build
  pass.
- A browser test through the direct Caddy path passes. The Cloudflare-proxied
  `rill.lightless.me` path is verified separately after it is active; the
  direct sslip result is not accepted as Cloudflare evidence.
