# 006: Daily feed reading reports

Status: proposed; specification only, implementation not started.

## Goal and confirmed scope

Produce a curated daily Simplified Chinese report that selects, summarizes and
analyzes important new articles across the user's subscriptions. It should
connect related stories, explain significant developments, and let readers
inspect the original sources, rather than summarize every article in turn.

The user confirmed:

- Use Asia/Shanghai. Start generation at 08:00; completion by 08:00 is not a
  requirement.
- Cover yesterday at 08:00 inclusive through today at 08:00 exclusive. Label
  the report with the window's start date and display the full coverage period.
  For example, the report dated 2026-11-17 starts generating on 2026-11-18 at
  08:00 and covers 2026-11-17 08:00 through 2026-11-18 08:00, Asia/Shanghai.
- Add a separate Daily reports sidebar entry, followed by a dated report list
  and a report detail page.
- Reports use article-style reading and sharing. They do not enter ordinary
  article lists or need read/unread and bookmark behavior in this version.
- Apply editorial judgment: low-value articles and feeds may contribute
  nothing to a report. Neither per-article summaries nor inclusion of every
  article/source are required.
- Schedule inside the application. Do not use system cron or an external
  scheduler service.
- Do not automatically backfill missed reports. Application startup, restart
  and enabling the feature must not trigger historical generation.

This work delivers the Spec and Plan first; it does not authorize implementation
or deployment.

## Article selection and report quality

- "New" means an RSS/Atom article first inserted into Rill during the covered
  window, determined by its stored creation timestamp, not the publisher's date.
  Read and unread articles are both eligible. A late-fetched article belongs
  to its ingestion window even if its publication date is older.
- Build the candidate pool from all available subscription articles in the
  window, rather than the current page, only unread articles, or an arbitrary
  first-N subset. This is eligibility for screening, not a requirement to
  summarize or publish every candidate. RSS deduplication remains unchanged.
  Initial imports and newly added feeds may contribute older articles first
  ingested during that window.
- Screen candidates for significance, useful new information, substantive
  technical/practical value and supporting evidence. Omit low-information
  updates, promotional filler and repetitive coverage; merge related stories
  and prioritize the strongest material. Do not give every feed a quota or
  include weak material to reach a fixed article/section count. A feed's absence
  from one report does not unsubscribe it or permanently exclude its articles.
- Work from stored feed content; do not fetch full websites or external news.
  Each input includes its title, feed name, original URL and a bounded text
  excerpt. Screen with lightweight title/source/excerpt inputs before deeper
  synthesis; do not call the single-article summary flow for every candidate.
  Editorial exclusion is valid; accidental omission because a request hit its
  context limit is not. Identify the report as based on available feed content.
- Screen in bounded batches, retaining only promising material for synthesis.
  Combine that material into prioritized topics and remove further weak or
  redundant points during final editing. If notes exceed one request's budget,
  reduce them in further bounded batches. Screening need not produce a saved
  summary for each article, and shortlisted material need not all appear in the
  final report. Incomplete or failed screening is not successful exclusion.
- The report contains an overview, important developments grouped by topic,
  cross-source analysis, and points worth watching. Sections may be short when
  there is little material; do not manufacture trends or pad the report.
- Separate source-supported facts from interpretation. Do not invent numeric
  comparisons with earlier days, external confirmation, or unsupported claims.
  Feed text is evidence to summarize, not instructions for operating the app.
- Give conclusions source references linked to actual selected articles.
  Source IDs and URLs come from the application; the model must not invent
  destinations. The source appendix contains only articles actually cited in
  the final report, not the entire candidate pool. Distinguish candidate counts
  from final selected/cited article counts; never describe them as all summarized.
- Use the configured AI endpoint, key and model already shared by translation
  and article summaries. No new provider integration or model picker is needed.

## Scheduling and generation lifecycle

Scheduling follows the no-backfill requirement above. The enable switch and
failure-retry policy below remain proposed operational choices:

- Add one persistent Daily reports enable switch, initially off. Reuse the
  existing AI settings; enabling reports requires a configured AI key. The
  settings update path enforces this server-side; UI guidance alone is not
  sufficient. Reports use the same LLM configuration as translation and re-read
  it at worker startup and attempt start. If the key is removed after enabling,
  nonempty generation fails as specified below; the switch is not silently
  turned off. This keeps an existing AI-enabled development instance from
  silently starting scheduled billable work just because it was upgraded.
- On enable or startup, schedule the next future 08:00. Enabling before 08:00
  allows that morning's run; enabling or restarting after 08:00 waits until the
  following morning. Do not immediately generate the latest missed window.
- If the process was stopped or reports were disabled before a scheduled run
  was persisted, skip that run. Do not enumerate missing dates, create a
  backlog or fill gaps in the report list. Re-enabling does not generate dates
  from the disabled period. Existing reports remain readable after
  disable/re-enable.
- While the process remains running, disabling stops producing new due dates;
  already persisted pending and running attempts may finish normally. A restart
  still marks unfinished jobs interrupted/failed under the restart rule below.
- There is at most one active generation job in the application. Persist a
  unique report record for each covered date and its generation state. Repeated
  scheduler ticks, requests and restarts must not create duplicate reports.
- States distinguish pending, running, ready and failed. Only a complete,
  successfully persisted body is ready. Missing AI configuration for nonempty
  work, upstream errors, incomplete batches and persistence failures are
  failures, not empty reports.
- Allow an initial scheduled attempt and one retry after 15 minutes in the
  same process run. A restart does not replay a pending automatic retry. After
  failure, show an explicit Retry action; one manual retry requests one further
  attempt. Do not automatically regenerate a ready report.
- For list/detail polling, failed is terminal: stop polling and do not add a
  next-retry field or keep polling through the automatic retry delay. If the
  automatic retry later succeeds, the existing page entry/window-focus refresh
  shows the result. The UI does not need to distinguish a scheduled retry from
  an exhausted one.
- On restart, mark unfinished jobs as interrupted/failed for explicit manual
  retry; do not resume their AI requests automatically. Network calls may have
  incurred cost before interruption; persisted uniqueness does not promise
  exactly-once billing by the AI provider.
- Freeze the candidate set for an attempt before screening. Normal
  source mutations do not rewrite a ready report. A failed attempt may select
  from the currently available data again; record its actual candidate and
  final selected/cited counts.
- If the window contains no articles, publish a ready report saying there were
  no new articles, with zero candidates and no AI request. If successful
  screening finds no material worth featuring, publish a short report saying
  no significant items were selected, with the actual candidate count and zero
  selected articles. Do not pad it or confuse it with no input or a failed job.
  Database/query failures must never take either successful-empty path.
- The scheduler runs without an open browser. Shutdown cancels active AI work;
  it must not hold SQLite transactions open during model requests or block feed
  pulling, normal API requests, or HTTP shutdown.

## Reading and sharing

- The authenticated sidebar entry remains available even when reports are
  disabled, so saved reports can still be read. On an empty list, show setup
  guidance only when reports are disabled or AI configuration is missing;
  otherwise show that no report is available yet.
- List reports newest covered date first with pagination. Show title/date,
  candidate/selected counts and generation state. Loading, successful empty
  results and request failures remain distinct.
- Detail pages have stable date-based URLs, readable article typography,
  theme/font support, source links and the existing share action. Running and
  failed jobs are not displayed as completed article bodies. Retry is available
  for failed jobs, including while automatic scheduling is disabled.
- Refresh status while a visible list/detail contains pending or running jobs;
  stop that polling after terminal states, including failed. Refresh on page
  entry/window focus to pick up work finished while the user was away,
  including a successful automatic retry after a failure. No new SSE channel
  is needed.
- Reuse the existing public snapshot sharing flow: expiry, optional passcode,
  attempt limits and the public reader. Ready zero-input reports can also be
  shared. Non-ready reports cannot be shared.
- Public shares contain the report body and source references as a snapshot.
  Deleting source articles or changing AI settings does not break that snapshot.
  They expose neither internal report/job endpoints nor AI credentials.
- Public sharing stays read-only: no translation, AI summary or report
  generation controls. The current public reader already has no translation
  feature; this work preserves that boundary.

## Boundaries

- No synthetic RSS subscription or fake Item IDs for generated reports.
- No report read/unread counters, bookmark support, delivery notifications,
  email, report editing, force-regeneration, web research or external cron.
- No generic job framework, distributed scheduling, vector database, embeddings,
  new AI SDK or second sharing system. The scope is one application instance.
- No automatic historical backfill or historical-report generation UI/API.
  Retrying a report that actually failed is separate from creating a report
  for a scheduled run that never happened.
- No new framework or parser merely to render model Markdown; report content
  can use structured model output and application-generated safe article HTML.
- Local work is text/SQLite processing; model inference remains at the
  configured provider. Batch limits bound individual requests and working
  memory, not a report's total request count or total runtime. Do not describe
  these limits as a complete automatic-spending budget or promise a fixed price.

## Open decision: per-report work budget

The maximum candidate/screening workload, total AI requests, total elapsed time
and the outcome when a report exceeds its budget are not yet agreed. Removing
backfill removes the historical backlog, but does not settle this separate
large-window concern. Resolve it before implementing automatic generation;
do not silently choose an article cap, discard candidates or add a confirmation
workflow on the user's behalf.

## Acceptance criteria

1. With reports enabled and a fake clock at 08:00 Asia/Shanghai, the report for
   yesterday 08:00 through today 08:00 starts once without a browser. Include
   an item created exactly at the start boundary in the candidate pool; exclude
   one created exactly at the end boundary and make it eligible for the next
   report. Crossing midnight does not split this window, regardless of publisher
   date or unread status. Eligibility does not force final editorial inclusion.
2. Repeated wakeups and concurrent retry requests do not duplicate a report or
   run multiple AI jobs. Restart after one or many missed days, or enable after
   08:00, makes no historical AI calls and schedules only the next future 08:00.
   Interrupted jobs remain failed until manually retried; ready reports are
   never automatically regenerated.
3. A multi-batch candidate pool containing important stories, repeated coverage
   and low-value articles produces a grounded, prioritized Chinese report.
   Important items can be selected even beyond the first page/batch; low-value
   articles and entire feeds may be absent. Merge duplicate stories and cite
   only actual final sources. Candidate enumeration/screening must not silently
   become first-N sampling, and no per-candidate summary is required.
4. Zero candidates produce a readable report without calling AI. Successful
   screening with zero worthwhile selections produces a distinct short report,
   not filler or an error. Missing keys for required AI work,
   database failures, invalid/incomplete AI output and interrupted requests
   produce recoverable non-ready states, not success-shaped empty results.
   Enabling without the shared AI key is rejected at the settings boundary; a
   key removed after enabling makes nonempty work fail without fabricating an
   empty report.
5. The sidebar leads to the dated list and report detail, with clear loading,
   failure and running states. Completed reports remain readable across restart
   and while scheduling is off; ordinary article lists/counts are unaffected.
6. A completed report can be shared with the existing expiry/passcode options.
   The public snapshot renders correctly without login and with no translation
   control; normal article sharing remains unchanged.
7. Planned correctness checks use a controlled AI test server and clock. A live
   model-quality sample uses a small agreed dataset after credentials and cost
   are authorized; document simulated-provider evidence separately from actual
   writing-quality acceptance. Production deployment is a later task.

See [Plan 006](../plans/006-2026-11-17-daily-reports.md).
