# 003: Time-limited public article sharing

Status: design proposed; implementation not started.

## Goal

Let an authenticated Rill user create a public, read-only link to a snapshot
of the currently open article. The recipient can open the link without a Rill
session. The creator chooses an expiry, and may optionally protect the link
with a four-character alphanumeric passcode. Expired or invalid links never
show article content.

## User requirements

- Share the current article from the article drawer.
- Copy a temporary public link that opens without the Rill login.
- Set the link's validity period; it must expire automatically.
- Optionally provide a four-character alphanumeric passcode for private sharing.
- Do not expose the share link's content before its expiry or passcode check.

## Decisions

- A share stores an immutable snapshot (`title`, `content`, source `link`,
  `feed_name`, `pub_date`) at creation time. Deleting the source item or feed
  must not break or change an existing share before it expires.
- The public path is `/s/<token>`. The token is at least 128 bits of
  cryptographically random URL-safe data, so the link itself is unguessable.
- Expiry is a duration chosen at creation, defaulting to 24 hours. The UI offers
  presets (1 hour, 1 day, 7 days, 30 days) plus a custom duration in hours. The
  server accepts durations from 1 hour through 1 year and computes the absolute
  expiry itself.
- A passcode is optional. It is exactly four characters from `A-Z` and `0-9`,
  case-insensitive, normalized to uppercase. The creator chooses either
  `auto-generate` or a custom code. In auto-generate mode no passcode is sent at
  creation; the server generates it and returns it once in the creation
  response. A generated code omits ambiguous characters; manually entered codes
  accept the full alphanumeric set.
- Passcodes are never placed in the URL or a query parameter. They are entered
  by the recipient after opening the public page.
- The public page is a standalone read-only view: no app sidebar, no login
  redirect, no feed management, no mutation controls. Content uses the existing
  article sanitizer.
- A share is not a live item view. It does not update when the source article
  changes and does not grant access to the rest of the app.

## Required behavior

### Creation

- An authenticated article drawer exposes a Share action.
- The share dialog lets the creator choose:
  - expiry preset or custom hours;
  - whether a passcode is required;
  - either `auto-generate` or a custom four-character passcode. Auto-generate
    does not call a separate generation endpoint; the code arrives with the
    creation response.
- Creating a share returns:
  - the public path (`/s/<token>`);
  - the absolute expiry timestamp;
  - the passcode when one is enabled, for the creator to copy.
- The dialog provides copy controls for the link and, when enabled, the
  passcode. It copies the complete URL (`window.location.origin + path`), not
  only the path. Copy failure must not pretend the value was copied.
- Creating a share requires a valid Rill session. The source item must exist at
  creation time.

### Public access

- Opening `/s/<token>` does not require or redirect to a Rill session.
- If the share has no passcode and is valid, the page shows the snapshot title,
  sanitized body, source link, feed name, publication date, and expiry.
- If a passcode is required, the initial response exposes only that a passcode
  is required and the expiry. It must not leak the title, body, source link or
  feed name before a successful passcode check.
- A wrong passcode returns `403` and no content. It must not trigger the app's
  global login redirect.
- Repeated wrong passcodes are rate limited per share token and client IP:
  after five failures within fifteen minutes, return `429` with `Retry-After`;
  successful verification clears the failure state.
- Expired links return `410` while their lightweight tombstone exists; missing
  or malformed tokens return `404`. Expired tombstones retain only the token and
  expiry metadata, not article content or a passcode hash. If a later
  maintenance purge removes a tombstone, `404` is an acceptable outcome for that
  token. The public page treats both as unavailable without article content.
- Server-side expiry is authoritative. Client-side clocks or cached state must
  not extend a share's validity.
- A public page already open when `expires_at` passes must clear the rendered
  article and show the expired state. On tab visibility or bfcache restore it
  rechecks the expiry before leaving content visible. `Cache-Control:
  no-store` covers reloads; it does not by itself clear an already loaded React
  tree.
- Public share responses must not be cached as indefinitely valid content; the
  API sends `Cache-Control: no-store`.

### Snapshot and sanitization

- Store the snapshot content exactly as supplied from the item at creation.
- Render it through the existing `processArticleContent` sanitizer, including
  link and image safety checks.
- Do not expose item IDs, bookmark state, unread state, translation cache,
  settings, API credentials, or any other private application data in the
  public response or page.

### Cleanup

- Expired shares are rejected on every access.
- Expired rows are scrubbed opportunistically when creating or accessing
  shares: clear `title`, `content`, `link`, `feed_name` and `passcode_hash`, but
  keep the token and `expires_at` as a tombstone so `410` remains
  distinguishable from an unknown token.
- A later maintenance purge may delete tombstones; after that, `404` is
  acceptable. A new background scheduler is not required.

## API contract

The route inventory lives in `SetupRouter`; this section defines the
cross-cutting share behavior.

### `POST /api/shares` (authenticated)

Request:

```json
{
  "item_id": 123,
  "expires_in_seconds": 86400,
  "passcode_enabled": true,
  "passcode": "A7K2"
}
```

- `passcode` is optional when enabled; the server generates one and returns it.
- `passcode` is rejected when the feature is disabled.
- `expires_in_seconds` must be within the allowed range; the server computes
  `expires_at`.

Response:

```json
{
  "data": {
    "token": "...",
    "path": "/s/...",
    "expires_at": 1794900000,
    "passcode": "A7K2"
  }
}
```

`passcode` is present only when passcode protection was requested.

### `GET /api/shares/:token` (public)

- Valid, no passcode: `200` with `{data: {title, content, link, feed_name,
  pub_date, expires_at}}`.
- Valid, passcode required: `200` with `{data: {requires_passcode: true,
  expires_at}}` and no article fields.
- Expired: `410` while the tombstone exists; `404` after a maintenance purge.
- Unknown/invalid token: `404`.

### `POST /api/shares/:token/access` (public)

Request:

```json
{ "passcode": "A7K2" }
```

- Correct: `200` with the same content payload as a valid unprotected share.
- Incorrect: `403` with `{error: "invalid passcode"}`.
- Expired: `410` while the tombstone exists; `404` after a maintenance purge.
- Unknown/invalid token: `404`.
- Rate limited: `429` with `Retry-After`.

## Data model

Add one migration after the current migration chain with an `article_shares`
table containing at least:

- unique random `token`;
- nullable source `item_id` (soft association only);
- snapshot `title`, `content`, `link`, `feed_name`, `pub_date`;
- nullable `passcode_hash`;
- `expires_at`, `created_at`, and optional `last_accessed_at`.

Use bcrypt for passcode hashes. Do not store plaintext passcodes. Do not add
share fields to `settings` or `items`. Expired tombstones must have their
article fields and passcode hash cleared; the token and expiry remain for the
`410` distinction.

## Constraints

- Preserve all existing authenticated API behavior and session semantics.
- The public share API and page must bypass the session middleware but must not
  bypass expiry, passcode, rate-limit or sanitization checks.
- Do not place the passcode in the URL, HTML source before unlock, logs, or
  analytics.
- Do not expose the article through any endpoint that lists shares or tokens.
- Do not add a new authentication framework; use the existing password hashing
  primitive for passcodes.
- Do not change article read/star/bookmark state when a share is created or
  opened.
- No new external service is required.

## Non-goals

- A share management page, listing, revocation, or analytics.
- Live article updates in an existing share.
- Email, webhook, social-network or QR-code delivery.
- Public access to feeds, groups, search, bookmarks, settings or the rest of the
  app.
- Custom domains, search-engine previews, or rich Open Graph metadata.
- Sharing from list rows or bulk sharing.

## Acceptance criteria

- An authenticated user can open an article, create a share, and copy its
  complete `/s/<token>` URL.
- Opening that link in a fresh browser without a Rill session shows the article
  snapshot and does not redirect to login.
- The chosen expiry is shown and enforced server-side; after expiry the link
  returns an expired page and no content. A page left open past expiry clears
  its content, and a bfcache/visibility restore does not leave expired content
  visible.
- Auto-generated protection returns a server-generated code with the creation
  response without a separate passcode-generation request.
- With a passcode enabled, the public page reveals no title/body before the
  correct code; wrong codes show an error and do not redirect to login; five
  failures within the window produce `429`; the correct code shows the article.
- The share survives deletion of the source feed/item until expiry.
- Public HTML is sanitized consistently with the article reader; external
  links open safely and tracking/unsafe resources are removed.
- Existing authenticated app flows, list translation, lazy article translation
  and settings behavior are unaffected.
- Backend tests, frontend tests, typecheck, lint, i18n validation and the build
  pass.
