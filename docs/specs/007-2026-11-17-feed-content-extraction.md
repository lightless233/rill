# 007: Per-feed CSS article extraction

Status: proposed; documentation only, implementation not started.

## Goal and confirmed scope

Let the user configure a body CSS selector and an optional exclusion CSS
selector for each subscription, and preview their result on an article URL.
The user explicitly chose automatic extraction when a feed pull discovers new
articles, rather than extraction when opening an article.

The deliverable remains these three capabilities: body selection, exclusion
selection and result preview. Persistence, a preview API and integration before
article insertion are supporting changes needed to make them work. This document
does not authorize implementation, push or deployment. Daily reports in work
item 006 remain on hold.

## Rule configuration

- Add the two fields to the existing feed edit dialog's advanced settings.
  Configure an existing subscription; do not expand add-feed discovery or OPML
  into another rule-configuration flow.
- `content_selector` is the body selector. An empty or whitespace-only value
  disables extraction. No separate enable switch or extraction mode is needed.
- `exclude_selector` is optional. It removes matching elements from the fetched
  document before body selection. It has no effect while extraction is disabled.
- Trim both values and validate every nonempty selector against the project's
  goquery/Cascadia selector support. Browser-only selector features are not
  implicitly supported. A comma-separated selector list is allowed.
- Saving invalid selectors fails without changing the saved rule. Valid syntax
  does not guarantee a match on every page; preview is available but is not a
  mandatory save gate.
- Rules are per feed, not global or inferred from its hostname/group. Editing
  or clearing a rule affects subsequent new articles only. Already stored
  articles, bookmarks and share snapshots are not rewritten.

## Extraction behavior

- Fetch only the article URL carried by the parsed feed item. Do not discover
  other pages, follow pagination links, fetch linked assets server-side or guess
  another URL when the article has no usable link.
- Use ordinary server-side HTTP and the source's proxy setting, existing URL /
  redirect / private-network policy, default request headers and timeout.
  Remote page JavaScript is not executed.
- Remove non-content script/style/noscript nodes, apply exclusions to the
  document, then select body roots. Preserve document order and include nested
  matches only once. If exclusions remove a selected root, it contributes
  nothing. No match or no remaining readable text/image content is an extraction
  failure, not successful empty content. Do not impose a minimum prose length.
- Preserve the selected HTML structure, including paragraphs, lists, tables,
  code blocks, images and links supported by the existing article renderer.
  Do not replace the result with plain `.Text()`. Resolve relative `href` and
  image `src` values against the final response URL so redirects do not break
  stored content. Rendering continues to use the existing content sanitizer.
- Preview and automatic extraction use the same fetch/extraction behavior.
  A preview uses supplied draft rules; a pull uses the feed configuration read
  for that attempt. Saving a rule does not restart a pull already in progress.

## Automatic extraction during feed pulling

- For a feed with a nonempty body selector, attempt extraction for newly seen
  articles before insertion, even if their RSS content/description is nonempty.
  Do not infer whether extraction is needed from text length or field presence.
- Check existing `(feed_id, guid)` identity before making article requests.
  "New" means not yet stored under that identity, regardless of publisher date;
  it is not an additional publication-time filter.
  Already stored articles, including earlier extraction failures, are not
  fetched again. Repeated GUIDs within one feed response are attempted only
  once. HTTP 304 responses and pulls with no new articles make no article
  extraction requests.
- On success, store extracted HTML in the existing article `content`. On
  extraction failure, retain and insert the parsed RSS content for that article
  and continue processing the feed. Do not overwrite it with empty content,
  discard the article or count an article-page failure as an RSS fetch failure.
  Log the affected feed/article and failure without response bodies or secrets.
- Keep the parsed RSS value available until extraction succeeds. This version
  stores one canonical article body; it does not add a second persisted copy,
  body-source switch, per-article extraction-status UI or content-history table.
- Perform article requests serially inside the existing feed pull admission
  slot and attempt deadline, without a new queue, worker or automatic retry.
  The deadline covers RSS fetching plus extraction; it is not restarted for
  each article. If it expires after RSS parsing, save the RSS fallback for
  unfinished articles without starting further requests. Parent/process
  cancellation stops work instead of starting more requests or recording a
  source failure. Database failures remain failures; they are not treated as
  evidence that an article is new or that saving succeeded.
- Preserve the existing uniqueness, unread state, conditional-feed metadata,
  item insertion and new-item notification behavior. Network work never holds
  an SQLite transaction open.
- Reading, translation, summaries, search, newly created bookmarks and newly
  created shares consume the saved canonical body through their existing
  paths. They must not refetch the origin. Existing snapshots stay unchanged.

## Preview interaction and HTTP contract

These are planned additions, not implemented endpoints.

- Extend the existing feed resource with string fields `content_selector` and
  `exclude_selector`, defaulting to empty for existing and newly created feeds.
  The existing feed PATCH accepts partial updates: omitted fields keep their
  values and an explicit empty string clears a field. No article schema or
  article API shape change is required.
- Add authenticated `POST /api/feeds/:id/extraction/preview` using the existing
  API/session conventions. The request contains an article `url`, a required
  nonempty `content_selector`, optional `exclude_selector` (default empty), and
  optional `proxy`. An omitted proxy uses the saved feed proxy; an explicit
  empty proxy means direct access. A supplied proxy is validated exactly as
  in feed settings and is not saved by preview.
- Return `200` with `{"data":{"content":"<article HTML>","url":"<final URL>"}}`.
  The response HTML is untrusted and must pass through the same sanitizer as
  ordinary article content before entering the preview DOM.
- Use the existing error envelope: `400` for invalid URL/proxy/selector input,
  blocked target policy, no match or empty extraction; `404` for a missing feed;
  `502` for origin DNS/network/timeout/non-success HTTP or unsupported document
  type; `500` for internal failures. Keep invalid selectors, no match and
  upstream failures distinguishable by their messages. A failed preview must
  not display a previous result as if it succeeded.
- Provide an article URL input and an explicit Preview action beside the draft
  selectors in feed editing. Preview uses the current draft selectors and
  proxy, requires no saved article ID, and works on already published articles
  without changing them. Do not make requests on every keystroke.
- Show loading, the rendered result, or a concise error. If URL/rules/proxy
  change, clear the previous result and disregard an older in-flight response.
  Closing the dialog must not allow a late response to populate another form.
- Preview never saves rules, inserts/updates an article, marks it read, creates
  a bookmark/share or updates feed fetch state. Save remains the existing feed
  save action. Preview URL/results are transient, not persisted settings.

## Boundaries

- No Readability dependency or automatic body detection.
- No JavaScript scripts/engine, browser automation, page-script execution,
  new HTML selector engine or extraction framework.
- No 403/anti-bot handling, cookie/login configuration or new proxy controls.
- No historical re-extraction, retry queue, forced article refresh, rule export,
  shared rule catalog, XPath, regex transformation, URL rewriting, pagination
  crawling or special lazy-image repair rules.
- No reading-page controls, original/full-body toggle, new AI behavior or work
  on held Daily Reports. Existing content consumers need no separate extraction
  integration because the canonical body is saved before insertion.

## Acceptance criteria

1. An existing feed starts with empty rules. Saving valid selectors survives
   reload; clearing the body selector disables extraction. Invalid syntax
   returns an error and preserves the previously saved values.
2. Preview a controlled HTML page with paragraphs, code, a table, an image and
   relative links. It includes the selected body, excludes advertising and
   related content, preserves formatting and uses the final URL as the link
   base. Multiple/nested matches do not duplicate text. Previewing unsaved
   rules or changing the proxy does not save either setting or article data.
3. Invalid selectors, no match, excluded/empty results and origin failures show
   distinct failure outcomes without stale preview content. Executable markup
   cannot run in preview; ordinary article sanitization remains unchanged.
4. A configured feed returns one known article, duplicate entries for one new
   article and another new article. Known entries cause no article requests;
   each distinct new entry is attempted once. Successful extracted HTML is
   stored; a failed extraction stores its original RSS body. A subsequent
   identical pull or 304 makes no further article requests or duplicate rows.
5. A disabled rule makes no article requests. Editing rules does not rewrite
   existing articles or trigger historical work. Database lookup/save errors
   do not become successful empty results or bypass identity checks.
6. Under a controlled deadline, requests stop when the existing attempt budget
   expires and unfinished new articles retain their RSS body. Process/request
   cancellation stops work promptly. Origin extraction failures do not advance
   RSS failure counters; normal new-item counts and signals remain correct.
7. A newly saved extracted article renders in the existing reader and supplies
   that same body to translation/summary preparation and new bookmark/share
   creation. Earlier bookmarks/shares retain their existing snapshots. Do not
   invoke paid AI or depend on live access to The Hacker News for acceptance.

See [Plan 007](../plans/007-2026-11-17-feed-content-extraction.md).
