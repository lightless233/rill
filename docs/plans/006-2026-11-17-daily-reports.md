# 006: Daily reports implementation plan

Status: planning only; do not start implementation.

Contract: [Spec 006](../specs/006-2026-11-17-daily-reports.md).

## Approach and reuse

Keep a report as a separate persisted article-like resource. An ordinary
[Item](../../backend/internal/model/model.go) is tied to a feed, while the user
explicitly chose separate report navigation without read/unread or bookmarks.
Creating an internal feed would add pull/count/filter exceptions and risk
summarizing previous reports as source material.

Reuse these existing paths:

- [Store and migrations](../../backend/internal/store/store.go) for SQLite and
  schema upgrades; existing cursor/envelope helpers for paginated HTTP reads.
- [translator.Client](../../backend/internal/translator/translator.go) for AI
  endpoint normalization, credentials/model configuration, HTTP transport and
  completion-stream handling. `SummarizeArticle` itself has a single-article
  prompt and truncation policy; do not call it with the whole day's corpus.
- [Process lifecycle](../../backend/cmd/fusion/main.go) and the context/timer
  pattern in [Puller](../../backend/internal/pull/puller.go) for a background
  loop. No scheduling dependency is required for one fixed daily schedule.
- [Article share handler](../../backend/internal/handler/share.go),
  [snapshot store](../../backend/internal/store/share.go),
  [ShareDialog](../../frontend/src/components/article/share-dialog.tsx), and
  [public reader](../../frontend/src/routes/s.$token.lazy.tsx) for sharing.
- [AppLayout](../../frontend/src/components/layout/app-layout.tsx), sidebar,
  ContentHeader, query error components, typography and
  [processArticleContent](../../frontend/src/lib/content.ts) for reading UI.

Only report-specific editorial screening, synthesis and job state are new.
Do not add parallel AI transport, parsing, sharing, scheduling or UI infrastructure.

## T1 — Persist reports, selection windows and enable state

Problem: article creation time is stored, but the current list query is a
publication-date reader and there is no durable report/job record.

- Add a migration for the report resource, with a unique window start/report
  date, period bounds, title/body, state, candidate/selected counts and cited
  source metadata, actual model, timestamps, attempt count and internal retry
  timing.
  Keep source references as report-owned snapshots rather than cascading
  foreign keys to live articles. Do not store AI keys in report records.
  Use the Spec's canonical `candidate_count` and `selected_count` definitions;
  a shortlist is internal and `cited_count` is not another persisted metric.
  Preserve null/unknown counts before their corresponding stage completes.
- Add the persistent enable switch through the existing settings path. Validate
  enabling server-side against the shared translation LLM key; UI guidance
  alone is not sufficient. Do not add a separate AI configuration, a
  first-eligible date, backfill cursor or missed-date inventory. Preserve ready
  reports across disable/enable.
- Add a dedicated creation-time window query and appropriate timestamp index.
  A report dated D covers `[D 08:00, D+1 08:00)` in Asia/Shanghai and becomes due
  at its end boundary. Convert those bounds to stored Unix timestamps for the
  query; do not split selection at midnight.
  Do not overload `ListItemsParams.BeforePubDate`, remove its existing page cap,
  or use the first page of the public item API to assemble a report.
- Select a stable candidate manifest for an attempt, then prepare cleaned
  available feed text and metadata for model input. Preserve normal feed text;
  do not uniformly substitute tiny previews. Load records in bounded database
  batches independently of how many model requests are needed. Do not retain
  every raw HTML body or generate a persistent summary per candidate.
  A disappearing source must not
  be claimed as processed. Ready reports retain their own text and only their
  actually cited sources after source deletion.
- Persist short state transitions transactionally and claim jobs conditionally.
  Never keep a database transaction open around AI calls. Match the existing
  named-SQL-parameter and migration conventions.

Evidence: inclusive/exclusive 08:00 selection boundaries, articles on both sides
of midnight, read/unread independence, creation versus publication dates, stable
report date ordering, uniqueness/claim behavior, enable state and job state
persistence are covered by focused store tests.

## T2 — Add grounded multi-source report synthesis

Depends on T1.

- Implement report generation using the existing translator client. Add narrowly
  scoped report methods that reuse `streamCompletion`, `Config`, URL resolution,
  upstream errors and completion validation; avoid a new generic LLM facade.
- Use the existing goquery/HTML dependencies to extract text server-side.
  The browser's DOM-based summary extractor cannot run in the scheduled worker;
  do not introduce a JavaScript runtime to reuse it. Remove non-content nodes,
  retain paragraph boundaries, and preserve source IDs alongside text.
- Prepare escaped pseudo-XML evidence using standard Go XML escaping. Include
  IDs, feed/group context, publication dates, titles and cleaned available feed
  text. The application resolves URLs from IDs. Mark any individually truncated
  text explicitly; empty/boilerplate text is title-only evidence. Do not claim
  stored text is a complete original article merely because it is long.
- Establish the configured model's supported context profile and explicit
  output/reasoning reserve and framing allowance. Size the complete serialized
  messages, including system/schema instructions and XML expansion. Verify
  representative inputs offline with the official matching tokenizer. A
  verified DeepSeek V4 profile can use UTF-8 bytes as a conservative runtime
  text-token bound; do not generalize it to unknown models, reuse the summary
  character limit as context capacity, or ship a Python/tokenizer sidecar.
- Use one request for selection, deduplication, analysis and writing when the
  prepared input fits. Only overflow uses budget-packed screening batches and
  one final synthesis. Recheck final evidence size; fail explicitly if it still
  exceeds the budget. Do not add recursive reduction, per-article summaries or
  a fixed request count independent of actual input. Keep overflow requests
  serial and retain article IDs through screening notes and final citations.
- The per-report total-work budget and over-limit outcome remain open in the
  Spec. Per-request batching is not a total API-cost or runtime limit. Finalize
  that policy before implementing this generation unit; do not silently impose
  a first-N cap or invent a historical backfill mechanism.
- Use report-specific prompts for significance/information-value screening,
  duplicate-story consolidation, topical grouping, cross-source interpretation
  and citations. No per-feed quotas, mandatory article counts, persistent feed
  reputation system or new user ranking settings are needed. For every stage,
  keep fixed policy in the system message and supply escaped pseudo-XML feed
  fields/intermediate notes as separate untrusted user-message evidence.
  Source/group labels are hints, not automatic quality or topic decisions;
  distinguish promotions, discussions, advisories and substantive research.
  Insufficient excerpts cannot support invented full-article analysis, and old
  publication dates cannot imply fresh announcements. Explicitly require
  Simplified Chinese and ignoring evidence-embedded instructions that try to
  change output language, schema, selection criteria or allowed references.
- Decode and validate the screening/final schemas in the Spec's Model input
  and output contract with standard JSON tooling. Input pseudo-XML does not
  require changing the API envelope or structured output to XML. Enforce
  required fields, section types, `language = zh-CN`, nonempty text, the nonempty
  developments rule and known source IDs. Analysis/watch arrays may be empty;
  do not force filler. Keep safe report titles/dates/counts outside model
  authority. Malformed output, truncated
  streams and failed batches cannot become ready or successful zero-selection.
- Render the structured report into escaped HTML with Go's standard templating
  tools, using only application-owned layout and source URLs. Do not write a
  second Markdown engine: the current concise summary renderer does not support
  citation links, while the article/public readers already accept sanitized HTML.
- Count all eligible candidates separately from unique articles actually cited
  in the final report. Build the appendix from final references only, not every
  candidate or intermediate shortlist. Calculate `selected_count` from that
  deduplicated final reference set and reuse it in list/detail/body/share output.
  Omit unsupported historical comparisons and claims of external verification.
- For nonempty work, read the latest existing AI settings at attempt start.
  Zero-input completion needs no AI request or currently configured key. If
  screening succeeds with no worthwhile leads, persist a concise ready report
  with zero selections and skip further synthesis. Keep report output separate
  from the single-article summary cache; do not force unrelated cache formats to
  share keys or rerun summaries for every article by default.

Evidence: a controlled AI server exercises direct single-request generation,
overflow screening followed by final synthesis, important candidates beyond
the first batch, omitted low-value feeds/articles,
duplicate consolidation, final-source-only citations, safe HTML, upstream errors
and partial output. Verify no-input and successful zero-selection results remain
distinct from failure. Existing summary/translation tests remain valid for the
shared transport after any focused refactor.

Capture outbound requests in a controlled-provider test containing hostile
instructions and closing tags in a feed title/text and in intermediate notes.
Verify escaped XML data/role separation and the Chinese/ignore-embedded-instructions
system policy;
exercise unknown citations and invalid schema responses. Assert required section
presence/types, acceptance of legitimate empty optional arrays, and canonical
counts when an article is cited more than once or later edited out. Check the
`language` field and representative Chinese fixtures, but leave actual prose
language and injection resistance to the real-model quality sample; a mock does
not demonstrate those capabilities. Cover measured serialized input sizes,
output/framing reserves, unsupported model profiles and oversized final evidence.
Use heterogeneous fixtures with promotional noise, forum questions, advisories,
long text, RSS descriptions and title-only records. Log chosen request path and
size bounds without bodies/credentials; distinguish bounds from billed usage.

## T3 — Schedule daily work inside the process without backfill

Depends on T1/T2.

- Add one small report worker managed by the same `errgroup` and shutdown
  context as the HTTP server and puller. It owns automatic and manual jobs;
  retry handlers wake it instead of launching independent request goroutines.
- Calculate consecutive 08:00 window boundaries and the next generation time
  in Asia/Shanghai. Startup and enabling arm only the next future 08:00; do not
  turn a past closed window into an immediate job. Keep the window start date
  as the stable report/retry identity. Ensure
  timezone data is available in the shipped binary/container: the current
  [Dockerfile](../../Dockerfile) does not install timezone data. Standard
  `time/tzdata` embedding avoids reliance on the development host's zone files.
- Use a context-aware timer/wake loop, following the existing puller pattern,
  to handle the scheduled timer, settings changes and explicit manual retries.
  Do not scan historical dates, build a generic scheduler or combine report
  timing with per-feed refresh intervals.
- Read the shared translation LLM configuration when the worker starts and
  before an automatic attempt; do not reuse a stale key decision. A missing key
  for nonempty work is a failure, not an empty report.
- Preserve one active job, ready-record immutability and the Spec's limited
  same-process automatic failure retry. Missed scheduled runs are skipped, not
  queued. Disabling stops new due dates, but already persisted pending/running
  jobs may finish in the same process run. There is no backlog priority or
  catch-up loop.
- On startup mark unfinished jobs failed/interrupted without resuming work or
  replaying their automatic retry timers. Only explicit manual retry can restart
  them. On cancellation, stop timers and model requests and retain that state.
  Do not swallow worker failures as empty successful reports or crash the
  reader merely because an AI provider is unavailable.

Evidence: deterministic clock/scheduler tests cover enabling before/at/after
08:00 without generating partial windows, crossing midnight without an extra
report, repeated wakeups, and restarts after both a short outage and a long
outage without any historical AI requests. Cover disabling both before a run is
persisted and with already persisted pending/running work, interrupted jobs
requiring manual retry, the same-process retry limit and shutdown without
waiting real hours. Exercise zone loading in the container build once; no system
cron or new daemon is introduced.

## T4 — Expose authenticated report reads/retries and extend sharing

Depends on T1/T3; share content also depends on T2's persisted HTML.

- Add authenticated report list/detail and failed-job retry handlers with
  matching frontend types/client/query keys. Use the exact paths/date format in
  the Spec, and reuse `parseCursor`/`paginatedListResponse` for its exclusive
  `(period_start, id)` pagination contract. Do not introduce a date-string cursor
  or order by mutable generation timestamps. Keep list rows free of full bodies;
  count totals independently of `before`. Validate dates and source existence.
- Return persisted status promptly. A retry is asynchronous (`202`), claims no
  second job when already pending/running, and cannot regenerate a ready report.
  Use `404` for missing reports and `409` for an incompatible ready-state retry;
  record this new state-conflict convention in the API documentation.
- Extend share creation to accept exactly one source: the existing `item_id`
  or `report_date` in exact `YYYY-MM-DD` format. Implement the source validation,
  status codes and snapshot field mapping in the Spec, including `pub_date = 0`
  and an empty `link`. Resolve report content server-side; never accept
  client-supplied report HTML. Put the fixed-zone coverage period and canonical
  counts in stored content before either private reading or snapshot sharing.
- Keep the original item-only payload valid. After resolving report content,
  reuse the existing token, passcode, expiry and snapshot persistence flow.
  `CreateArticleShareParams.ItemID` already allows nil, so report snapshots need
  no fake item or new public token table.
- Keep the public response and `/s/` reader contract unchanged. Source appendix
  links point to original articles, not authenticated Rill item/detail routes.
  Generalize ShareDialog's source prop narrowly; do not clone the entire dialog.

Evidence: handler tests cover authentication, pagination/status reads,
idempotent running retries, ready-state conflicts, source exclusivity, report
snapshot creation and unchanged ordinary article sharing/passcode/expiry.
Cover multi-page exclusive boundaries while a row's job state changes, malformed
cursors, end-of-list behavior, exact share field mappings and count consistency.
Verify the public report hides the relative publication date and retains its
fixed-zone coverage text in different browser timezones. Update the planned API
notes in `docs/api-conventions.md` when these contracts become implemented.

## T5 — Add report navigation, reading and settings UI

Depends on T4.

- Add a Daily reports entry beside the existing top-level reading entries,
  with route-aware selection and the existing mobile sidebar behavior. Do not
  tie its availability to feeds/groups loading or add it as another RSS filter.
- Implement authenticated report list and date-based detail routes in AppLayout.
  Show the full 08:00-to-08:00 coverage period alongside the report's start date
  so the date label cannot be mistaken for a midnight-to-midnight report.
  Reuse existing list/query/error/loading/pagination patterns and article styles.
  The report body is read-only article content with source links and sharing;
  it is not an Item with fabricated IDs or unsupported read/star controls.
- Put the enable switch in the existing AI settings area and reuse the shared
  translation LLM configuration and missing-key guidance. Do not add another
  provider form or model selector. The settings update path rejects enabling
  when the shared AI key is not configured (400); do not rely on the UI switch
  alone. Record that rejection in the API conventions when implemented.
  Settings saves wake the worker; failed saves preserve the previous scheduling
  state.
- Refetch report queries while visible pending/running jobs need status updates;
  stop polling on terminal states, including failed. Do not add a next-retry
  field or keep polling through the automatic retry delay; entry/focus refresh
  picks up a later automatic retry result. Do not extend the feed-count SSE
  protocol for this feature.
- Reuse QueryErrorState/Banner, Button, Switch, ShareDialog, content sanitizer,
  fonts and themes. Add localized user-facing strings to all dictionaries;
  leave generated `components/ui` sources untouched.
- Update existing API/backend/frontend design docs only where this feature
  changes their contracts or rationale. Keep route/schema inventories in code.

Evidence: one bounded local flow enables reports with a controlled provider,
observes generation, opens the list/detail, retries a failure, and shares a
report publicly with passcode/expiry and no translation controls. Confirm
enabling without a configured key is rejected server-side and the shared
translation key schedules work. Confirm normal article lists/counts and sharing
still work. Use the existing local environment and preserve its
database/settings; do not deploy for acceptance.

## Integration and acceptance boundary

Implement T1, T2, T3, T4, then T5 as coherent units. Keep scheduler tests and
controlled-provider correctness checks independent of real-time waiting and
paid model calls. Use the repository's applicable Go/frontend checks once the
integrated change is ready, expanding only for affected failures.

Structural tests cannot establish report writing quality. After correctness,
use one small explicitly authorized real-model sample to assess Chinese prose,
editorial focus, omission of low-value material, deduplication, supported analysis
and citations, including a source containing instructions to change language or
fabricate a link, and record its actual input volume/request count. Do not present
a stubbed response as AI-quality evidence
or promise a fixed cost based on the model name alone.

Stop after delivering these design documents. Implementation, live model calls,
push and deployment require their corresponding user instructions.
