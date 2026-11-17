# 007: Per-feed CSS article extraction implementation plan

Status: planning only; do not start implementation.

Contract: [Spec 007](../specs/007-2026-11-17-feed-content-extraction.md).

## Approach and reuse

Extract during discovery of new feed articles, before the existing insertion
path. Save either successful extracted HTML or the original RSS body as the
canonical `Item.content`. This avoids a second body store, reader-time fetching,
article response changes and separate integrations for sharing/AI/search.

Reuse:

- [httpc](../../backend/internal/pkg/httpc/client.go) for HTTP clients, proxy,
  headers, redirects and the existing private-target policy.
- goquery/Cascadia already used in
  [feed discovery](../../backend/internal/handler/discovery.go) for HTML parsing,
  selection, removal and selector compilation. Reuse the dependencies, not the
  discovery probing algorithm or its feed-specific fallback requests.
- [Feed store](../../backend/internal/store/feed.go), its shared projection and
  scanner, and [migrations](../../backend/internal/store/migrations/) for the
  two persisted settings. Do not introduce another settings resource.
- [ItemExists and BatchCreateItemsIgnore](../../backend/internal/store/item.go)
  for persisted identity and insertion, and
  [Puller](../../backend/internal/pull/puller.go) for scheduling, admission,
  attempt timeout, cancellation and new-item notifications.
- [EditFeedDialog](../../frontend/src/components/feed/edit-feed-dialog.tsx),
  [feed queries](../../frontend/src/queries/feeds.ts),
  [API client](../../frontend/src/lib/api/index.ts), existing UI components and
  [processArticleContent](../../frontend/src/lib/content.ts) for configuration
  and safe preview rendering. Do not hand-edit generated UI components.

No new third-party dependency is needed. This work does not resume Spec 006.

## T1 — Shared CSS extraction used by preview and pulling

- Add a small shared extraction implementation callable by the handler and
  puller, without a provider interface, script runtime or generic crawler.
  Compile user selectors with the existing Cascadia parser so invalid syntax
  produces a controlled error rather than a panic or a silent empty match.
- Fetch exactly the supplied article URL using `httpc`, normal default headers,
  a caller-owned context and proxy. Respect the existing allow-private setting
  on the initial request and redirects. Check origin status/document type;
  do not turn a denied/error page into a successful article.
- Read at most the Spec's article-page limit plus one byte from the
  decompressed HTTP body to detect overflow. Reject overflow explicitly rather
  than handing a silently truncated document to the parser. Keep this limit
  local to article extraction; do not change RSS parsing or discovery limits.
- Decode the bounded HTML through `golang.org/x/net/html/charset.NewReader`
  with the response Content-Type, preserving its BOM/meta sniffing behavior.
  That module is already in the dependency tree; do not introduce another
  encoding dependency, detection heuristic or user setting. Parse UTF-8 output
  with goquery, remove script/style/noscript nodes and exclusion matches, then
  extract selected roots in document order without
  repeating nested selections. Serialize each retained root with
  `goquery.OuterHtml` (or equivalent root-preserving `html.Render`), not
  `Selection.Html()`, which omits the root and only reads the first selected
  node. Concatenate root fragments in document order, not selector-list order.
  Preserve HTML rather than plain text; report no-match/empty outcomes
  explicitly. Accept short text and image content.
- Resolve relative links/images against the final response URL before returning
  the extracted fragment and that URL. Do not introduce pagination, lazy-image
  heuristics, URL rewrite rules or another HTML sanitizer. All app renderers
  continue to apply their existing content sanitization.

Evidence: focused extraction tests cover valid/invalid selectors, exclusion,
multiple/nested roots, formatting, short/image content, final-URL resolution,
direct `table`/`ul`/`pre` roots, header/meta-declared GBK Chinese, exact-limit and
oversized response bodies, no match/empty output, upstream errors and context
cancellation. Use controlled HTML/HTTP fixtures, not live sites or browser page
rendering.

## T2 — Persist feed rules and expose draft preview

Depends on T1.

- Add the next migration for `feeds.content_selector` and
  `feeds.exclude_selector`, both non-null strings defaulting to empty. Existing
  create/import paths continue to use those defaults. Leave item, bookmark,
  share and AI-cache schemas alone.
- Extend the feed model, shared `feedSelectColumns`/`scanFeed`, grouping where
  needed, partial-update parameters and the existing feed update handler.
  Trim and validate nonempty selectors before writing. Only explicitly
  supplied fields change, and invalid input leaves the stored rule intact.
- Add the authenticated preview route under the existing router and implement
  the Spec's request, response and error mapping. Check feed existence, resolve
  saved versus draft proxy, and call the T1 extractor. Preview has no store
  writes or pull-state updates. At handler entry, bind it to the HTTP request
  context with `min(configured pull timeout, 20 seconds)` as the work budget,
  covering input decoding, validation, fetch and extraction. Gin's JSON
  binding does not observe the request context; check `ctx.Err()` after binding
  so a deadline during body decoding is not reported as invalid input. Check
  deadline errors before writing success and return `502` for an exhausted
  preview budget, including expiry during request decoding or URL/DNS
  validation rather than misclassifying it as invalid input. Client cancellation
  stops work. Preserve the current global server write timeout. Use the standard
  response/error helpers.
- Mirror fields and preview types in
  [frontend API types](../../frontend/src/lib/api/types.ts) and the existing
  feed API client. Do not build a separate HTTP client or preview cache.
- Update [API conventions](../api-conventions.md) only for the implemented
  preview contract/error distinctions; the Spec owns the proposed interface
  until implementation. Do not present planned routes as already available.

Evidence: migration/store round trips and partial-update tests cover defaults,
save/clear and invalid-update preservation. Handler tests cover authentication,
draft-rule preview, draft proxy, missing feeds, validation and origin failures,
and demonstrate no changes to settings/articles/read state/fetch metadata.
Exercise a slow origin through a real HTTP server with a write timeout and a
shorter application budget, using scaled durations instead of a long sleep.
Verify that the client receives a structured `502`, not only that a recorder
captured an error status. No new public timeout setting is needed for testing.

## T3 — Extract only new articles before canonical insertion

Depends on T1/T2.

- Integrate after successful RSS parsing/304 handling and before
  `BatchCreateItemsIgnore` in the puller. Leave RSS parsing and its content /
  description mapping intact as the fallback source.
- Only for configured feeds, reuse `Store.ItemExists` to skip persisted GUIDs
  before fetching article pages. Track GUIDs within the current response so
  repeated entries do not make repeated extraction requests. Preserve existing
  first-entry/unique-insert behavior. A lookup error stops this pull with an
  explicit error; it must not treat all candidates as new.
- Extract new articles serially using the feed's saved rules/proxy and the
  existing attempt context. On success replace only the pending article's
  content; preserve title, GUID, URL, publication date and normal unread state.
  On article extraction failure keep its parsed RSS content and log the
  affected feed/article and failure without response bodies or secrets. Do not
  add per-article retry or source failure backoff.
- The existing attempt deadline covers the entire RSS-plus-extraction attempt.
  After that deadline, stop origin requests and retain RSS content for remaining
  new entries, then use normal insertion/fetch-success handling while the
  parent remains active. Parent/process cancellation stops the operation.
  Do not keep an SQLite transaction open during HTTP work or add a second
  semaphore, queue or scheduler.
- Log the number/reason of RSS fallbacks when the total deadline prevents full
  coverage, using the same redaction. This explicitly accepts partial extraction
  for a slow/large batch;
  successful feed refresh does not mean every article was enriched. Stored
  fallback rows are not retried on later ordinary pulls. Do not silently change
  this into per-article timeout resets or add a historical retry mechanism.
- Keep the existing batch insertion/FTS path and notify only after committed
  new rows. Previously stored articles and snapshots are never rewritten by
  saving a selector or by ordinary refresh. No article-reader, AI transport or
  sharing-service refactor is needed for newly stored canonical content.

Evidence: extend existing puller tests with mixed known/new/duplicate entries,
success and fallback in one feed, disabled rules, repeated pulls/304, source
failure-counter preservation, attempt deadline and process cancellation.
Include enough delayed new articles to exhaust the shared deadline after only
some extractions finish, then verify RSS fallback for the remainder, explicit
fallback logging and no extraction backfill on the following pull.
Inspect saved content and actual origin request counts, not only mock calls.
Check that ordinary item reads, the existing translation/summary preparation and
new bookmark/share snapshots receive that saved body using existing consumer
tests where appropriate.

## T4 — Configure rules and render an explicit preview

Depends on T2; integrated acceptance also requires T3.

- Extend existing advanced feed editing with the two selector inputs, an
  article URL input and a Preview action. Keep rule saving in the existing Save
  flow. Open advanced settings when saved selectors are present. Do not add
  another page, article-picker browser, rule manager or extraction mode switch.
- Preview the current draft selectors and proxy without saving. Show pending,
  rendered success or concise error using existing components/styles. Render
  content through `processArticleContent(content, finalURL)` with the existing
  article typography; never put raw origin HTML or a remote page iframe into
  the UI. Preserve the existing sanitizer rather than replacing it with a new
  native-API/fallback stack for this feature.
- Requests occur only on Preview clicks. Invalidate the displayed preview when
  any input changes, and cancel/disregard obsolete responses so switching
  inputs or closing/reopening the form cannot show another request's result.
  Use normal mutation pending state rather than a polling loop or persistent
  preview query.
- Save both selector changes using the existing partial-update mutation and
  cache update. Compare trimmed draft values with saved values, treating a
  missing saved selector as empty: omit unchanged selectors, but send `""` for
  a deliberate clear, including whitespace-only edits. Do not copy the existing
  proxy field's `trim() || undefined` pattern;
  JSON serialization would drop the clear. Leave unrelated proxy-edit behavior
  unchanged. Include concise localized guidance that rules apply to future
  new articles, not historical rows. Follow existing i18n dictionary conventions.

Evidence: focused UI behavior coverage plus one bounded browser check of the
real feed-edit flow: preview a controlled page, change draft rules, handle a
failed/stale response, save, reopen and clear the rule. Verify preview does not
change article data. Cover the save payload and reload after clearing each
previously populated selector, including a whitespace-only body edit.
Reuse the existing local app and data; do not replace its database or use
production origin access as an acceptance prerequisite.

## Integration and delivery boundary

Implement T1, T2, then T3/T4 as coherent changes. Use focused Go tests during
development and the repository-required Go/frontend compilation, lint, tests
and i18n checks for the integrated implementation. No paid AI calls, production
deployment, 403 investigation or held Daily Report work is included.

The supporting API/schema/pull changes implement the three confirmed features;
the review adds explicit page-size/preview-time limits, HTML charset decoding
and root-preserving serialization, not another product feature. The shared
pull deadline's partial-coverage trade-off remains explicit. Stop after delivering
these documents until the user requests implementation.
