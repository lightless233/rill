# 002: Article list automatic translation implementation plan

Status: planning only; do not start implementation from this document.

Contract: [Spec 002](../specs/002-2026-11-16-list-translation.md).

## Execution scope

One implementing agent can execute these units sequentially. Keep the existing
handler, pull, store, query and translation boundaries. This work adds one
list-translation preference and one authenticated batch translation path; it
does not refactor the settings schema, the article body translation flow or the
items API.

The existing `settings` single-row typed table remains the settings store. Add
one nullable column through the next ordered migration. Do not replace it with a
key-value or JSON schema in this work item.

## Implementation units

### T1 — Persist the list translation mode

Problem: the list has no independent translation preference, and creating one
must not overload the article-detail `translation_mode` or require a settings
schema rewrite.

Change:

- Add the next ordered migration (013) with a nullable
  `list_translation_mode` column on `settings`. Do not edit migration 004 or any
  other historical migration.
- Extend the settings model, store read/write path and handler allow-list with
  `original | zh | bilingual`. Preserve the existing partial-update and
  `NULL` semantics: an absent PATCH field leaves the value unchanged, and
  `NULL` still means “never chosen”.
- Keep `list_translation_mode` independent from the existing
  `translation_mode`, which controls article detail display.
- Mirror the field in the frontend API types, preferences store and
  settings-sync path. Apply the effective-mode rule from the Spec: stored mode
  wins; no stored mode means bilingual when a key is configured, otherwise
  original; no key always suppresses requests.
- Add the list translation setting to the AI settings tab, reusing the existing
  mode vocabulary and controls. Add the list-header control in T4.
- Keep the settings table typed. Do not add a key-value table, a JSON blob or a
  note column as part of this feature.

Primary scope: [settings migration](../../backend/internal/store/migrations/),
[settings store](../../backend/internal/store/settings.go),
[settings handler](../../backend/internal/handler/settings.go),
[settings model](../../backend/internal/model/model.go), mirrored
[API types](../../frontend/src/lib/api/types.ts),
[settings sync](../../frontend/src/lib/settings-sync.ts),
[preferences store](../../frontend/src/store/preferences.ts), and the settings
dialog.

Completion evidence: GET/PATCH round-trips every allowed mode; an absent field
leaves the stored value unchanged; invalid values return `400`; a fresh
database and an upgraded database both accept the new setting; existing
settings tests and sync behavior remain intact.

### T2 — Add the batch list translation endpoint

Problem: the existing translation endpoint translates one article per SSE
stream. Calling it once per row would create too many upstream calls and too
many connections.

Change:

- Add authenticated `POST /api/translations/items` as an SSE endpoint following
  the existing translation/summary stream conventions, including pre-stream
  JSON errors and per-event write-deadline extension.
- Accept a bounded batch of caller-correlated items. `item_id` is not a database
  lookup key and may be negative for orphaned starred bookmarks. Reject empty
  items, oversized fields and oversized batches with the standard `400` error
  envelope.
- Load the existing translation configuration; a missing key is a `400` before
  the stream starts. Do not introduce a second credential configuration.
- Reuse the established translation cache semantics and the existing
  `article_translations` table; no new translation-cache table is needed.
  Unchanged title/excerpt content is served from cache without an upstream
  call; changed content is retranslated; `force` bypasses cache entries. Use a
  list-specific content-hash namespace so list entries cannot collide with
  article-body translations.
- Translate misses in upstream batches, not once per item, and reuse the
  existing streaming parser and segment-marker machinery. Emit each item as
  soon as its requested fields are complete; emit partial results before a
  later `error` event when upstream work fails after some items succeeded.
- Cache completed items durably so reopening a list does not call the upstream
  again. Do not store translated text in `items`.
- Return the model name in the stream metadata and mark cached versus freshly
  translated items.

Primary scope: [translation handler](../../backend/internal/handler/translation.go),
[route registration](../../backend/internal/handler/handler.go),
[translator](../../backend/internal/translator/translator.go),
[translation store](../../backend/internal/store/translation.go), and focused
handler/translator/store tests.

Completion evidence: a mock upstream receives one batch for multiple items and
the stream returns each item’s title/excerpt; a second identical request makes
no upstream call; `force` bypasses cache; missing key and invalid payloads fail
before streaming; an upstream failure after partial success emits completed
items plus an error; negative correlation ids work; no per-row upstream request
is made.

### T3 — Add the frontend list translation pipeline

Problem: the list currently has no client-side trigger, cache or cancellation
mechanism for translations, and automatic work must not block reading.

Change:

- Extend the API client with the streaming list-translation call and typed
  request/callback shapes.
- Add a list-translation hook used by the article list. It must:
  - run only when the effective mode is translated and a key is configured;
  - evaluate title and excerpt independently with the Spec’s Latin/no-CJK
    eligibility rule;
  - send bounded batches (frontend batches smaller than the server maximum);
  - update rows as `item` events arrive;
  - retain partial results and expose a list-level retry state;
  - abort in-flight work when the mode becomes original, the list scope changes
    or the view unmounts;
  - reuse an in-session cache for already translated content while treating the
    backend cache as the cross-reload source of truth;
  - avoid duplicate streams for the same content and avoid per-row queries.
- Build the translation source from a longer plain-text excerpt (around
  300–400 characters) without the display ellipsis. Keep the existing display
  excerpt and line-clamp behavior.
- Keep translation work out of the items query cache so read-state optimistic
  updates, list membership, pagination and bookmark reconciliation are not
  disturbed.

Primary scope: [API client](../../frontend/src/lib/api/index.ts),
[article list](../../frontend/src/components/article/article-list.tsx), a new
list-translation hook under `frontend/src/hooks/`, and focused helper/hook
tests.

Completion evidence: opening a list starts translation; scrolling a new page
translates only the new page; switching to original aborts current work and
makes no further requests; no key or all-Chinese content produces no request;
a failed stream leaves originals visible and exposes retry; cached content
reopens without an upstream call.

### T4 — Render the list modes and add the control

Problem: list rows only render original text, and the list header has no
translation control.

Change:

- Extend the article row with the effective mode and optional translated title
  and excerpt.
- Follow the existing article-detail bilingual hierarchy exactly:
  - `bilingual`: original title/excerpt first; translated title/excerpt below,
    smaller and lower contrast; use the same visual relationship and existing
    design tokens as the detail title and `.rill-translation-*` body blocks;
  - `zh`: translated title/excerpt take the existing primary styles, with
    original fallback when a translation is missing;
  - `original`: current rendering unchanged.
- Do not create a second color palette, font family or translation-chip style.
  Reuse the existing typography and muted/foreground hierarchy.
- Add a header control to the left of the split “mark all as read” control, as
  shown in the user’s screenshot, exposing original/Chinese/bilingual modes.
  The control must work on mobile without crowding the header.
- Add `lang` attributes for translated and original text so the existing
  Latin/CJK font settings apply.
- Add translation status and retry affordances at list level. Do not add
  per-row spinners or per-row error toasts.
- Add the new i18n keys to every shipped locale and keep the settings dialog
  and header control using the same vocabulary.
- Do not change read-state pins, row selection, keyboard navigation, star
  actions, feed labels or date formatting.

Primary scope: [article row](../../frontend/src/components/article/article-item.tsx),
[article list](../../frontend/src/components/article/article-list.tsx),
[styles](../../frontend/src/index.css),
[settings dialog](../../frontend/src/components/settings/settings-dialog.tsx),
and [i18n catalogs](../../frontend/src/lib/i18n/messages/).

Completion evidence: bilingual mode visibly follows the detail-page
original-first/de-emphasized-translation relationship; Chinese-only mode shows
only Chinese; original mode is unchanged; missing translations fall back
cleanly without duplicate text; the control is usable at desktop and mobile
widths; i18n completeness passes.

### T5 — Documentation and acceptance

Problem: the new public endpoint, preference and list behavior need to be
recorded without duplicating route inventories or code facts.

Change:

- Update API conventions with the new batch endpoint’s SSE envelope, limits,
  error timing and cache behavior.
- Update the frontend design with the list translation modes, automatic trigger
  scope and the rule that bilingual presentation reuses the detail hierarchy.
- Record the settings-schema decision where the settings table is described:
  one typed nullable column was added; a key-value refactor is a separate work
  item.
- Update the work-item index.
- Run focused backend and frontend checks during the units, then the aggregate
  checks once:
  - backend tests for the touched packages, plus the race run for any touched
    concurrency path;
  - frontend typecheck, unit tests, lint, i18n completeness and build;
  - a fresh database and an upgraded database migration check;
  - browser acceptance on a local instance with a mock translation endpoint.
- Browser acceptance must cover: bilingual visual comparison with the detail
  page, Chinese-only and original switching, persistence after reload, infinite
  scroll, Chinese-only content, missing key, cached reopen and upstream failure.
  Use disposable browser artifacts under `.runtime/` and do not use production
  data or credentials.

Primary scope: [API conventions](../api-conventions.md),
[frontend design](../frontend-design.md),
[backend design](../backend-design.md), and the documentation index.

Completion evidence: the documented contract matches the implemented endpoint
and UI; the checklist above passes on the final inputs; no unrelated files are
modified.

## Integration order

1. T1 settings and migration.
2. T2 backend batch endpoint and cache.
3. T3 frontend streaming pipeline.
4. T4 list rendering and control.
5. T5 documentation and aggregate acceptance.

T1 and T2 can be implemented in parallel. T3 depends on T1’s mode and T2’s
stream contract. T4 depends on T1 and T3. Do not start T4’s visual work before
the bilingual relationship is checked against the existing detail rendering.

## Boundaries and failure transitions

- Translation is presentation-only. It never mutates item content, read state,
  bookmark state, list membership or search indexes.
- The existing detail-page translation remains manual; list settings do not
  start body translation.
- A list-mode change to original must cancel work rather than merely hiding
  results, so it actually stops upstream cost.
- A missing key, an upstream failure and a partial stream are distinct states:
  none may present a successful empty translation or blank rows.
- Existing read-state optimistic updates and unread undo pins must remain
  untouched by translation cache writes.
- No deployment, production access, commit or push is part of this plan.
