# API Conventions

The HTTP contract lives in the code: route inventory in
`backend/internal/handler/handler.go` (`SetupRouter`), response shapes in
`backend/internal/model/model.go` (JSON tags), request bodies in the
handler files (`createFeedRequest`, `createBookmarkRequest`, ...), and the
frontend call surface in `frontend/src/lib/api/index.ts`. This page
records only the cross-cutting rules that are distributed across handlers,
so they do not need to be rediscovered per endpoint. If code and this page
disagree, the code wins — fix the page.

## Base path and authentication

- All endpoints are under `/api`. Two exceptions: the Fever compatibility
  API (`/fever*`, see `docs/fever-api.md`) and a bare `GET /oidc/callback`
  registered only when OIDC is configured, for deployments whose redirect
  URI omits `/api`.
- Authentication is a session cookie named `session` (`POST /api/sessions`
  with `{"password": ...}` sets it; `DELETE /api/sessions` clears it, 204).
- Anonymous mode: when both password and OIDC are unconfigured, the auth
  middleware lets every request through.
- Login attempts are rate limited: excess attempts get `429` with a
  `Retry-After` header (seconds).
- OIDC: `GET /api/oidc/enabled` always exists; `/api/oidc/login` and
  `/api/oidc/callback` are registered only when OIDC is configured. Login
  returns `{data: {auth_url}}` for the client to navigate to; the callback
  redirects (307) to `/` on success or `/login?error=oidc_failed` on failure.
- Public article shares: `POST /api/shares` is authenticated, while
  `GET /api/shares/:token` and `POST /api/shares/:token/access` bypass the
  session middleware so a recipient needs no Rill account. Bypassing the
  session does not bypass expiry, passcode, attempt limiting or sanitization.
  Public share responses send `Cache-Control: no-store`.
- Live feed updates use the authenticated `GET /api/events` SSE stream. It is a
  signal channel, not a data source: `ready` and `feeds_changed` carry empty
  objects, and clients refetch `GET /api/feeds` for authoritative counts. The
  stream sends comment heartbeats and uses `Cache-Control: no-cache`. A pull
  emits `feeds_changed` only when it commits at least one new item; 304s,
  zero-new-item pulls and failures do not emit.

## Response envelopes

- Single resource: `{"data": <object>}`
- Group and feed lists (small, fully returned): `{"data": [...], "total": n}`
- Item and bookmark lists (cursor-paginated):
  `{"data": [...], "total": n, "next_cursor": <string|null>}`;
  `next_cursor` is `null` when there are no more pages.

## Cursor pagination

- Pass the previous page's `next_cursor` as the `before` query parameter;
  omit it on the first page.
- Cursor format: `"<value>_<id>"` (two int64s, underscore-separated). The
  value is `pub_date` (unix) for items and `created_at` (unix) for bookmarks;
  `id` breaks ties.
- The items cursor is keyed on `pub_date`, so `before` is only valid with
  the default ordering: combining `order_by=created_at` with `before` is
  rejected with 400. Only the first page can use `created_at`.
- Ordering is always DESC. Items support `order_by=pub_date|created_at`
  (default `pub_date`); any other value silently falls back to `pub_date`.
- `limit` is capped at 100 on every endpoint that accepts one (items,
  bookmarks, search); `maxListLimit` in `backend/internal/handler/item.go`.

### Planned daily report extension (not implemented)

[Spec 006](specs/006-2026-11-17-daily-reports.md#http-and-share-snapshot-contract)
defines the future report HTTP contract. Its list will reuse the two-int64
cursor with `(period_start, id)`, ordered DESC and exclusive of `before`;
`period_start` is the Unix timestamp of the report window's 08:00 start in
Asia/Shanghai, not its date string or generation time. State changes must not
move rows between pages. See the Spec for totals, end-of-list behavior and
validation; these are proposals, not existing report endpoints.

The same planned extension adds mutually exclusive `report_date` (`YYYY-MM-DD`)
and `item_id` sources to share creation. Report shares reuse the public snapshot
shape, with `pub_date = 0` and the fixed-zone coverage period inside `content`.
The Spec owns the full mapping and source/status rules. Remove this planned
label and reconcile the implemented conventions when the feature is developed.

## Errors

Handlers report failures as `{"error": "<message>"}`:

- `400` — invalid input (message varies)
- `401` — `"unauthorized"`
- `403` — public share passcode is wrong; the public page shows an inline
  error instead of treating this as a session failure
- `404` — `"<resource> not found"` (also an unknown or malformed share token)
- `410` — article share expired; the token/expiry tombstone may later be
  purged, after which the same token becomes `404`
- `429` — `"too many login attempts"` or `"too many passcode attempts"`
  (plus `Retry-After`)
- `500` — `"internal server error"` (details are logged server-side only)
- `502` — upstream fetch failure (e.g., feed validation upstream DNS, connection, timeout, or non-2xx failure)

Three cases return the status with an empty body instead: unknown routes
(404), CORS origin rejection (403), and panic recovery (500).

## Status codes

- `200` — any response with a body: reads, creates, and updates
- `204` — body-less success: batch mark items read/unread (`-/read`, `-/unread`), all deletes, logout,
  and the `GET /api/sessions` validity probe (valid → 204, invalid → 401)
- `202` — feed refresh requests (`/feeds/refresh`, `/feeds/{id}/refresh`):
  refresh happens asynchronously, no body

## Request quirks worth knowing

- `POST /api/bookmarks` has two modes: `{item_id}` snapshots the bookmark from
  that item server-side (preferred), or a full `{link, title, content,
  feed_name, pub_date?}` snapshot when there is no item. Sending `item_id`
  makes the server ignore all other fields.
- `PATCH /api/items/-/read` and `-/unread` accept 1–1000 ids
  (`maxBatchUpdateIDs`).
- `PATCH /api/items/-/read-all` marks all unread items as read, optionally scoped
  by mutually exclusive `feed_id` or `group_id` (passing both returns `400`),
  and returns `200` with `{"data": {"affected_ids": [...]}}` listing the items
  whose `unread` state changed from `true` to `false`.
- `refresh_interval_seconds` (create and update feed): on **create**, absent
  or `0` means "use the global interval" (stored as `NULL`, never as `0`);
  on **update**, absent or `null` leaves the stored value unchanged and `0`
  clears it back to the global interval; any other value must be in the
  closed set returned by `GET /api/app` (`allowed_refresh_intervals`).
- `PATCH /api/settings` is a partial update: an absent field leaves the stored
  value unchanged. Preferences cannot be reset to `null`; string translation
  config fields (`translation_api_url`, `translation_api_key`,
  `translation_model`) accept `""` to clear them, and `GET /api/settings` never
  returns `translation_api_key` in plaintext (exposing only
  `translation_api_key_set` and `translation_api_key_mask`). Closed sets: locale
  `en|zh|de|fr|es|ru|pt|sv`, theme `light|dark|system`, `auto_mark_read`
  `off|open|5|10|30`, `article_page_size` 1–100, `translation_mode`
  `bilingual|zh`, `list_translation_mode` `original|zh|bilingual`, and the
  typography allow-lists (`latin_font`, `cjk_font`, `mono_font`) defined in
  `backend/internal/handler/settings.go`.
- `POST /api/translations` (`meta`, `segment`, `done`, `error`) and
  `POST /api/summaries` (`meta`, `delta`, `done`, `error`) stream
  `text/event-stream` events instead of a JSON envelope once pre-flight
  validation succeeds (invalid requests and missing credentials still return
  standard JSON errors).
- `POST /api/translations/items` is the batch list-translation stream. It
  accepts up to 50 caller-correlated title/excerpt items and emits `meta`,
  `item`, `done` and `error` events. `done` is the batch-success marker and is
  only emitted when every requested item has an `item` event; cached items are
  emitted with `cached: true`. A stream that ends without `done` is incomplete
  even if the HTTP response ended cleanly.
- `POST /api/translations/blocks` is the viewport-lazy article-body stream. It
  translates only the requested title/block subset and caches title and body
  blocks independently; `meta`, `segment`, `done` and `error` follow the
  existing article translation stream convention.
- Public share reads are deliberately split. A protected
  `GET /api/shares/:token` returns only `{requires_passcode: true, expires_at}`
  and no article fields; the full snapshot is returned only for an unprotected
  share or after a successful `POST /api/shares/:token/access`. Five wrong
  passcodes per token and client IP within fifteen minutes produce `429`;
  success clears that state. Passcodes are four `A-Z0-9` characters,
  case-insensitive, and never appear in the URL.
- `POST /api/feeds/:id/extraction/preview` is an authenticated, synchronous
  article-page preview. It uses a work deadline of the smaller of the configured
  pull timeout and 20 seconds, starting at handler entry. `url` and a nonempty
  `content_selector` are required; `exclude_selector` defaults empty, and an
  omitted `proxy` uses the feed's saved proxy while an explicit empty proxy
  means a direct request. Invalid URL/proxy/selector input, blocked targets, no
  match and empty extraction return `400`; a missing feed returns `404`; origin
  DNS/network/timeout/non-success, unsupported document type and the article
  size limit return `502`. Preview never persists rules, articles, bookmarks,
  shares or fetch state.
