# 001: Rill maintainability and correctness baseline

Status: proposed implementation contract; documentation only, implementation not started.

## Goal and scope

Prepare the existing reader for continued maintenance in the Rill fork. Keep its
current architecture, repair the concrete review findings, and organize only
the code needed to make those paths coherent and testable. This is not a rewrite.

The user requested a first Spec and Plan after choosing to continue with a fork.
The proposed first-phase scope is corrective maintenance plus necessary local
cleanup. That scope was proposed in the conversation; it must not be presented
as a separate user requirement for a general redesign.

This document contains the review handoff as well as the required behavior, so
the next agent does not need a separate review report or the previous chat.
See the matching [implementation plan](../plans/001-2026-11-14-maintainability-baseline.md).

## Baseline and evidence

The inspected fork baseline is commit
`c31ade09b481fa877633899e8cf5d4c86a72c4f9`. It includes the upstream per-feed
refresh interval change after the earlier reviewed release. Existing architecture
and contracts are described in [backend-design](../backend-design.md),
[frontend-design](../frontend-design.md), and [API conventions](../api-conventions.md).

Earlier runtime probes used upstream v1.3.0 with only the two frontend rendering
changes for empty-Default visibility. Source inspection in this fork confirms the
relevant defects or missing behavior below. Historical runtime results are
evidence of the earlier build, not a claim that the same measurements were
repeated against this commit. In particular, the newer scheduler can change
which initial pulls complete; reproduce its queue behavior before changing it.

| ID | Finding and evidence | Current code entry |
| --- | --- | --- |
| F01 | Historical local probe: with OIDC configured, an explicitly permitted empty password left the normal API protected, but Fever authenticated a publicly computable key. The key derivation and independent Fever authentication remain in this baseline. | [Fever handler](../../backend/internal/handler/fever.go), [handler construction](../../backend/internal/handler/handler.go), [configuration](../../backend/internal/config/config.go) |
| F02 | Current source: feed discovery calls the external finder, whose pinned client does not use the application's guarded transport. Filtering discovered URLs afterwards does not prevent an earlier unsafe request. The guarded direct transport also validates DNS and subsequently dials the hostname, allowing a second resolution. The discovery bypass was also reported in [upstream issue 275](https://github.com/0x2E/fusion/issues/275); no production exploit test was performed. | [Feed validation](../../backend/internal/handler/feed.go), [HTTP client](../../backend/internal/pkg/httpc/client.go), [URL policy](../../backend/internal/pkg/httpc/ssrf.go) |
| F03 | Current source: single-feed initial/manual operations start their timeout before waiting for the shared concurrency permit. Historical local probe with six slow local feeds, concurrency one and a four-second timeout: six created, three successful initial pulls, one recorded failure, two never attempted during that operation. Records were not lost. | [Feed handlers](../../backend/internal/handler/feed.go), [pull execution](../../backend/internal/pull/puller.go) |
| F04 | Historical browser interception: feed and article requests returned HTTP 500, were retried once, and then displayed successful-empty messages. Aggregating hooks still discard query errors in this fork. | [Feed queries](../../frontend/src/queries/feeds.ts), [article aggregation](../../frontend/src/hooks/use-article-list.ts), [article list](../../frontend/src/components/article/article-list.tsx), [feed management](../../frontend/src/routes/feeds.lazy.tsx) |
| F05 | Current source: refresh-all returns acceptance before work finishes; the frontend invalidates queries immediately and has no completion reconciliation. This is source-level evidence, not a timed browser reproduction. | [Refresh handlers](../../backend/internal/handler/feed.go), [refresh mutation](../../frontend/src/queries/feeds.ts), [app information](../../frontend/src/queries/app.ts) |
| F06 | Historical browser interception: with 101 bookmarks and the older article outside the first lookup page, the ordinary article view displayed Star instead of Unstar. The starred view has a separate paginated resolution path and must not be described as having this same defect. | [Bookmark lookup](../../frontend/src/queries/bookmarks.ts), [article aggregation](../../frontend/src/hooks/use-article-list.ts) |
| F07 | Historical browser interception: six matching articles, two loaded; the control labeled Mark all as read submitted only two IDs. The same loaded-list behavior remains. The mismatch is between the label and the actual scope. | [Article list action](../../frontend/src/components/article/article-list.tsx), [item API](../../backend/internal/handler/item.go) |
| F08 | Historical local SQLite trace: changing only unread state deleted and inserted FTS shadow-table data. The update trigger still runs for every item update. | [Initial trigger](../../backend/internal/store/migrations/001_initial.sql), [item updates](../../backend/internal/store/item.go) |
| F09 | Earlier frozen-lockfile installation built and typechecked successfully and passed 23 frontend tests, but lint failed because the resolved TypeScript and typescript-eslint versions were incompatible. The frontend CI does not run lint. These dependencies and workflow remain in this baseline. Existing frontend tests chiefly cover helpers and state primitives. | [Frontend dependencies](../../frontend/package.json), [lockfile](../../frontend/pnpm-lock.yaml), [ESLint configuration](../../frontend/eslint.config.js), [frontend workflow](../../.github/workflows/frontend.yml) |
| F10 | The trial image received an empty-Default visibility patch outside this fork. This checkout does not contain it. Preserve that already-requested behavior when the fork becomes the source for a replacement image. This is a customization transfer, not an upstream defect. | [Sidebar groups](../../frontend/src/components/feed/feed-list.tsx), [feed management](../../frontend/src/routes/feeds.lazy.tsx) |

The earlier backend test packages passed race-enabled testing. OIDC implementation
coverage was zero; passing tests therefore did not establish the affected auth
behavior. No tests or builds have been run against this fork as part of writing
these documents. Do not report the historical checks as current-fork acceptance.

## Constraints and existing contracts

- Keep Go, SQLite, React, the existing component library, TanStack Query and
  Zustand. Keep HTTP handlers, pull execution/policy, and storage responsibilities
  recognizable; do not add a framework or speculative service/interface layers.
- Preserve persisted IDs, content, subscriptions, group assignments, read state,
  bookmarks, settings, and bookmark survival after feed deletion. Apply required
  schema changes through new ordered migrations; do not edit an old migration
  as the only fix for already-installed databases.
- Preserve the established API envelopes, pagination, ID-based read/unread
  requests, configured-password Fever clients, environment names, and per-feed
  interval behavior. Necessary additions must be compatible with those real
  consumers. This work does not rename module paths, configuration, or API names.
- Preserve the documented unread-view undo pins and keyboard navigation. A fix
  for one cache must not silently change this interaction model.
- No production deployment, data import, subscription regrouping, AI integration,
  global branding replacement, or automatic content deletion is authorized by
  this work item. The present request authorizes writing these documents only.

## Required behavior and acceptance

### R1. Authentication remains effective across entry points

Fever must not authenticate any request when no nonempty password is configured,
including OIDC-only deployments that explicitly allow an empty password. Use the
protocol's existing authentication-failure response. Do not invent a second
credential configuration or derive a usable credential from an empty secret.
Explicit anonymous web/API mode keeps its documented behavior; it does not
implicitly enable Fever without a password.

Acceptance: an isolated OIDC-only fixture cannot use the predictable empty-secret
key to read or mutate Fever data. Existing nonempty-password Fever clients and
session authentication continue to work. See [Fever compatibility](../fever-api.md).

### R2. Outbound HTTP policy covers discovery as well as pulling

The existing private-feed policy must govern every outbound request in feed
validation/discovery and normal pulls, including redirects and subsequent
discovery requests. Disallowed targets must be rejected before connecting or
reading a response, not merely removed from the final result list. Direct
connections must use the addresses that were validated, without a new unchecked
DNS resolution. Retain intentional private-feed opt-in and supported proxy use.

Acceptance: controlled direct and redirect cases cannot reach blocked local or
private targets with private feeds disabled. Public RSS/Atom and HTML discovery
continue to work, as do allowed private fixtures with explicit opt-in. Discovery
failures must produce an error outcome rather than a successful empty result;
an actually successful page with no discoverable feed may still return empty.

### R3. Waiting for a permit does not consume a fetch attempt's timeout

Periodic, initial, batch-import and manual work must share the existing concurrency
limit. The per-fetch budget begins when that attempt can execute, rather than when
it starts waiting. Queued work remains cancellable by operation/process lifecycle;
this does not authorize unlimited retries or a new persistent job system.
Keep the existing scheduling, suspension, cache validators, backoff and per-feed
interval semantics. Do not count a never-started request as an HTTP fetch failure.

Acceptance: a batch whose queue lasts longer than one fetch timeout still attempts
every eligible feed while its operation remains active, provided each individual
response fits its own budget. Cancellation terminates waiting work, and observed
fetch concurrency never exceeds the configured limit. Verify against the current
scheduler rather than relying on the historical six-feed counts.

### R4. Query failures and asynchronous work remain distinguishable

Feed/group/article/bookmark loading must distinguish pending, successful-empty,
successful-data and failed states. Do not turn request failures into empty lists
or zero counters. Existing cached data may remain visible on a failed refresh,
with a visible failure indication and a meaningful retry action. Reuse existing
components and notification conventions.

Refresh acceptance must not be reported as completion. Keep the existing refresh
routes and acceptance responses. Add `refresh_all_running: boolean` to the
authenticated app-info response, reflecting the existing refresh-all operation;
set it before accepting new work and clear it on every terminal path. It signals
operation activity, not that every source succeeded. Existing per-feed fetch state
continues to report individual successes and failures.

The refresh-all UI must reconcile feeds and article queries after that operation
finishes, including when it finishes before the first status read. Any status
polling stops after completion, an unrecovered status error, or departure from
the relevant view. Do not create job IDs, persistent job storage, or a second
query/cache system. Creation/import notices must describe accepted/created
subscriptions without promising that their initial fetch has finished.

Acceptance: request failures never display the corresponding successful-empty
message; a completed refresh displays newly fetched items/counters without a
document reload. A failed status read is visible and never implies completion.

### R5. Bookmark state does not depend on the global first page

An article's Star/Unstar action must reflect its actual linked bookmark even when
the total bookmark count exceeds the lookup page size. Preserve the starred view's
own pagination, filtered lists, bookmark snapshots and orphaned bookmarks.
Resolve state for relevant articles without loading every saved article's full
content merely to obtain star indicators. Necessary item-response metadata may be
added without removing or changing existing fields.

Acceptance: an ordinary article outside the first global bookmark page offers
Unstar and can be unstarred. Count, ordinary view, starred view and article detail
converge on the same result after success, and preserve prior state after failure.

### R6. The read action states its real scope

For this corrective baseline, keep the existing loaded-article action and make
its label unambiguous in every locale: it marks loaded articles, not every matching
article on the server. Preserve the existing ID-based request and unread undo
interaction. This resolves the demonstrated labeling defect without introducing
an unrequested server-wide mutation. A true whole-scope mark-all feature is a
separate product change, not an implicit requirement of this review.

Acceptance: with two loaded out of six matching articles, the control communicates
loaded scope and submits those two IDs. Unloaded articles and other scopes remain
unchanged. No unit tests are required merely for the new label; verify the actual
action scope and the existing i18n completeness check.

### R7. Read-state changes do not rewrite content search indexes

FTS content maintenance must respond to actual title/content changes, insertion
and deletion. Changing read state or unrelated metadata, or assigning unchanged
title/content values, must not reindex the article. Upgrade existing databases
without rewriting their content, IDs or read state.

Acceptance: a read/unread update produces no FTS content-maintenance writes;
insertion, a real title/content edit, and deletion remain searchable/correctly
removed after both fresh initialization and migration of an existing database.

### R8. Declared engineering checks are usable and meaningful

Use mutually supported compiler/linter dependencies with a consistent lockfile.
The lint command must execute successfully and run in frontend CI alongside the
existing checks. Do not weaken assertions, bypass typing, disable legitimate
rules, skip failing tests, or modify vendored UI components by hand to get green.
Add regression coverage for the corrected behaviors at appropriate existing test
boundaries; there is no coverage-percentage target or requirement for a new
end-to-end testing platform.

Acceptance: lint, typecheck, frontend tests, i18n validation, relevant backend
tests and the application build pass on the final inputs. Auth, queued pulling,
query failure and pagination regressions have behavior-level evidence rather
than only helper or string-formatting tests.

### R9. Preserve empty-Default visibility behavior

Hide the system Default group in sidebar navigation and feed management only
when the successful feed result confirms it is empty. Show it when populated.
Keep the group stored and available to appropriate group-selection controls.
Do not infer emptiness from a failed request, delete the system group, or hide
unrelated empty user-created groups.

Acceptance: both views hide empty Default, display populated Default, and preserve
all subscriptions, group assignments, bookmarks and read state across that change.

## Documentation discrepancy and follow-up boundaries

The backend design's default private-network-protection description is incomplete:
the discovery call path does not inherit the guarded client. R2 must close that
gap and align its explanatory documentation. This Spec does not edit existing
design documents ahead of implementation. No frontend documentation/code conflict
requiring a product decision was identified; the unread undo design is retained.

Response-size limits and an article retention policy were identified as further
long-running-service concerns, not reproduced data-loss bugs. They are recorded
for later work: choosing size thresholds or deleting old content would add new
behavior and is not part of this first baseline. Do not silently convert these
observations into configuration options or destructive cleanup requirements.
