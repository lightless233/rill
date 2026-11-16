# 003: Time-limited public article sharing implementation plan

Status: planning only; do not start implementation.

Contract: [Spec 003](../specs/003-2026-11-16-article-sharing.md).

## Execution scope

One implementing agent can execute these units sequentially. Keep the existing
session/auth boundary, article drawer, content sanitizer and SQLite migration
chain. This work adds a new public read-only route and one new table; it does
not change feed pulling, list translation, lazy article translation or settings
storage.

## Implementation units

### T1 — Add the share snapshot store

Problem: shares need immutable snapshots, random tokens and optional passcode
hashes that outlive source items.

Change:

- Add the next ordered migration (015) with an `article_shares` table:
  unique token, nullable source item id, snapshot title/content/link/feed
  name/pub date, nullable passcode hash, expiry and timestamps.
- Add the model/store layer for creating a share, fetching by token, recording
  access, and deleting expired rows opportunistically.
- Generate the public token with cryptographically secure randomness and enough
  entropy for an unguessable URL. Do not derive it from item ids or timestamps.
- Store passcode hashes with the existing bcrypt password helper; never store
  plaintext passcodes.
- Keep the source item/feed as soft associations only. Snapshot reads must not
  depend on the item or feed still existing.
- Do not add share fields to `settings`, `items` or the translation caches.

Primary scope: [migration chain](../../backend/internal/store/migrations/),
[store](../../backend/internal/store/), [model](../../backend/internal/model/model.go).

Completion evidence: a snapshot can be created, read before expiry, and read
after its source item/feed is deleted; an expired row is rejected; token
collisions are handled; passcodes verify only against the stored hash.

### T2 — Add authenticated creation and public share endpoints

Problem: the app needs one authenticated creation path and two public,
unprivileged access paths with strict expiry and passcode semantics.

Change:

- Add `POST /api/shares` under the authenticated API group. Validate the source
  item, expiry duration and optional passcode; create the snapshot and return
  token/path/expiry and the passcode when protection is enabled.
- Add unauthenticated `GET /api/shares/:token` and
  `POST /api/shares/:token/access` outside the session middleware.
- Implement the Spec's response states:
  - valid unprotected: full snapshot;
  - valid protected: only `requires_passcode` and expiry, with no article
    fields;
  - wrong passcode: `403`;
  - expired: `410`;
  - unknown token: `404`;
  - rate limited: `429` plus `Retry-After`.
- Add a share-specific in-memory attempt limiter keyed by token and client IP.
  Five failures within fifteen minutes block further attempts; a successful
  check clears the state. Do not reuse the login limiter in a way that lets one
  share's failures block another share or normal login.
- Set `Cache-Control: no-store` on public share responses. Do not put the
  passcode in query strings, logs or response URLs.
- Allow manual passcodes from `A-Z0-9`, case-insensitive and normalized to
  uppercase. Server-generated codes use an unambiguous subset. Reject invalid
  lengths/characters and a passcode supplied when protection is disabled.
- Ensure expiry is computed and checked server-side. Expired rows may be
  removed opportunistically; no background job is required.

Primary scope: [share handler](../../backend/internal/handler/),
[route registration](../../backend/internal/handler/handler.go),
[store](../../backend/internal/store/), [auth password helper](../../backend/internal/auth/password.go),
and focused handler/store tests.

Completion evidence: authenticated creation works; public GET/access work
without a session; protected responses do not leak article fields; wrong
passcodes return `403` without triggering login; expiry returns `410`; attempts
are limited; no-store headers are present.

### T3 — Add the share dialog

Problem: the article drawer has no way to create, configure or copy a share.

Change:

- Add a Share action to the article drawer header using the existing dialog,
  button, select/input and toast conventions.
- The dialog contains:
  - expiry presets (1 hour, 1 day, 7 days, 30 days) or a custom hour value;
  - an optional passcode toggle;
  - a four-character custom passcode input when enabled, with validation and a
    generate action;
  - a create action and clear loading/error states.
- On success, show the public `/s/<token>` path and, when enabled, the
  passcode. Provide copy controls using the existing browser clipboard API and
  show a real failure notice if copying is unavailable.
- Do not include the passcode in the link or query string.
- Add the frontend API request/response types and a creation mutation. Invalidate
  nothing in the reader cache; share creation does not change item state.
- Add i18n keys to every shipped locale.

Primary scope: [article drawer](../../frontend/src/components/article/article-drawer.tsx),
[API client](../../frontend/src/lib/api/), a focused share dialog component,
[UI components](../../frontend/src/components/ui/), and
[i18n catalogs](../../frontend/src/lib/i18n/messages/).

Completion evidence: a user can create a share from the drawer, see the expiry,
copy the link and code, and see validation/copy failures without creating a
partial share.

### T4 — Add the standalone public share page

Problem: `/s/<token>` currently has no unauthenticated route and must not load
the authenticated application shell.

Change:

- Add a public route `/s/$token` that is exempt from the root session guard and
  does not render `AppLayout`, the sidebar, search, settings or mutation
  controls.
- Fetch the share through the public API. If the response requires a passcode,
  show only a passcode form and expiry; do not render title, content, source
  link or feed name before success.
- After successful access, render title, sanitized body, feed name, source link
  and publication date/expiry through the existing
  `processArticleContent` sanitizer.
- Handle missing (`404`), expired (`410`), wrong passcode (`403`) and rate
  limited (`429`, including retry timing) states distinctly. Do not trigger the
  root login redirect on public-share errors.
- Make the page read-only and keyboard/accessibility usable. Use the existing
  theme and typography tokens, but no authenticated navigation.
- Add route metadata or a no-index hint if the project has an established
  metadata convention; do not add speculative Open Graph work.
- Add the necessary i18n copy for the public page.

Primary scope: [root route guard](../../frontend/src/routes/__root.tsx),
[public share route](../../frontend/src/routes/), [content sanitizer](../../frontend/src/lib/content.ts),
[API client](../../frontend/src/lib/api/), and focused route/helper tests.

Completion evidence: a fresh browser without a session opens a valid share and
sees the article; protected shares reveal no article fields before the correct
code; wrong/expired/invalid states are distinct and never redirect to login;
source links/images are sanitized.

### T5 — Documentation and acceptance

Problem: the public share contract and security boundaries need durable records.

Change:

- Add the public route and share endpoints to the API conventions and frontend
  design docs, including the difference between protected metadata and full
  content, and the no-store/expiry rules.
- Record the snapshot model and public-access exception in the backend design.
- Update the work-item index.
- Verify:
  - backend tests for create/read/access/expiry/passcode/rate limiting and
    snapshot survival after source deletion;
  - no-store headers and absence of passcodes in URLs/logs;
  - frontend typecheck, tests, lint, i18n and build;
  - a browser flow in a fresh profile (no Rill session): create, copy, open,
    passcode prompt, wrong code, correct code, expiry and invalid token;
  - existing authenticated reading/translation behavior remains intact.

Primary scope: [API conventions](../api-conventions.md),
[frontend design](../frontend-design.md),
[backend design](../backend-design.md), and the documentation index.

Completion evidence: documented behavior matches the implemented endpoints and
UI; the browser/security checks pass; no unrelated behavior changes.

## Integration order

1. T1 share table and store.
2. T2 authenticated creation and public access endpoints.
3. T3 share dialog and clipboard flow.
4. T4 public share page and root guard exemption.
5. T5 documentation and aggregate acceptance.

T3 and T4 depend on T2's response shapes; T4's public page must be developed
against the exact protected/unprotected response distinction. Do not build the
public page against an assumed full-content response for protected links.

## Boundaries and failure transitions

- A share is a snapshot, not a live item and not an alternate authenticated
  session.
- Expiry and passcode checks happen before any article field is serialized.
- Wrong passcodes must not use `401` in a way that triggers the app's global
  login redirect; use the Spec's `403` outcome.
- Expired, invalid and rate-limited states must not display cached content.
- Creating or opening a share must not mutate read state, bookmarks, unread
  counts, translations or settings.
- No deployment, production data access, commit or push is part of this plan.
