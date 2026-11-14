# 001: Rill maintainability and correctness baseline

Status: implementation contract clarified after baseline review; documentation only,
implementation not started.

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

Use `strings.TrimSpace(config.Password) != ""` only to decide whether password
authentication is enabled. Derive the Fever key from the original configured
password, preserving existing clients whose nonempty passwords contain whitespace.

Acceptance: an isolated OIDC-only fixture cannot use the predictable empty-secret
key to read or mutate Fever data. Existing nonempty-password Fever clients and
session authentication continue to work. See [Fever compatibility](../fever-api.md).

### R2. Outbound HTTP policy covers discovery as well as pulling

The existing private-feed policy must govern every outbound request in feed
validation/discovery and normal pulls, including redirects and subsequent
discovery requests. Disallowed targets must be rejected before connecting or
reading a response, not merely removed from the final result list. Direct
connections must use the addresses that were validated, without a new unchecked
DNS resolution. Replace the pinned finder's unguarded HTTP execution with local
discovery using the guarded client; retain relevant service URL rules, HTML link
discovery, RSS/Atom parsing and well-known probes.

The validation endpoint keeps its existing response envelopes, with these outcomes:

- `400`: malformed input, unsupported URL/proxy configuration, or a requested
  target/primary redirect rejected by the destination policy.
- `502`: failure to resolve or contact the primary target, timeout, or a final
  non-2xx upstream response. A DNS failure is not malformed input.
- `500`: an internal application failure, using the existing internal-error policy.
- `200`: successful discovery, including an empty result after a successful page
  fetch with no usable feed. Optional candidate/probe failures, including 404s,
  do not turn a successfully fetched page into an upstream failure. Successfully
  retrieved malformed feed XML must not produce a fabricated feed; retain empty
  results when no alternative feed is found. Reject unsafe discovered candidates
  before fetching or offering them.

Update the existing unreachable-URL test to expect an error; its current empty
success assertion is deliberately superseded. Keep primary retrieval failures
distinct from parse failures and optional discovery misses.

An explicitly configured proxy is a trusted relay and may itself use a private or
loopback address without enabling private feeds. This exemption applies only to
the configured proxy connection, never to a requested or redirected feed target.
With private feeds disabled, continue validating target names through local DNS,
as well as rejecting private IP literals/localhost, for initial and subsequent
requests. Do not silently skip DNS validation when a proxy is present: proxy-only
DNS is unsupported in this mode. Explicit private-feed opt-in allows proxy-side
resolution without local public-address checks, with the existing loss of private
destination protection.

Address pinning is enforceable for direct connections. A proxy can resolve names
independently; local DNS checks cannot prove its eventual destination. Proxy use
therefore trusts the relay to enforce the same destination restrictions when
private feeds are disabled. Do not claim end-to-end DNS pinning for proxy traffic
or build a new proxy protocol to provide it in this baseline.

Acceptance: controlled direct and redirect cases cannot reach blocked local or
private targets with private feeds disabled. Public RSS/Atom and HTML discovery
continue to work, as do allowed private fixtures with explicit opt-in and a local
trusted proxy serving a permitted public target. Exercise blocked proxy targets
and redirects, local DNS failure, upstream failure, successful HTML without feeds,
and optional well-known misses with their distinct outcomes.

### R3. Waiting for a permit does not consume a fetch attempt's timeout

Periodic, initial, batch-import and manual work must share the existing concurrency
limit. The per-fetch budget begins when that attempt can execute, rather than when
it starts waiting. Queued work remains cancellable by operation/process lifecycle;
this does not authorize unlimited retries or a new persistent job system.
Keep the existing scheduling, suspension, cache validators, backoff and per-feed
interval semantics. Operation/process cancellation, including parent deadline
expiry, must stop both queued and in-flight work without increasing consecutive
failures or triggering failure backoff, even if the request was already sent.
The attempt's own timeout while its parent operation remains active and real
network/HTTP failures still count.

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

This includes article-detail drawer loading and search requests: a failed detail
request must not leave an unexplained blank drawer, and a failed search must not
display "No results found." Superseded/cancelled searches are not failed searches.

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
The existing "Feed added successfully" and "Imported {count} feeds" notices
already satisfy that scope. Keep them concise; creation and OPML import need
immediate query invalidation, not extra completion fields or polling in this work.

Acceptance: request failures never display the corresponding successful-empty
message; a completed refresh displays newly fetched items/counters without a
document reload. A failed status read is visible and never implies completion.

### R5. Bookmark state does not depend on the global first page

An article's Star/Unstar action must reflect its actual linked bookmark even when
the total bookmark count exceeds the lookup page size. Preserve the starred view's
own pagination, filtered lists, bookmark snapshots and orphaned bookmarks.
Resolve state for relevant articles without loading every saved article's full
content merely to obtain star indicators. Add `bookmark_id: number | null` to
ordinary item list/detail responses and mirrored frontend Item types; it identifies
the linked bookmark and is explicitly null when absent. Preserve item ordering,
pagination totals and snapshot/orphan behavior.

Add `bookmark_count: number` to authenticated app-info, using the existing
unfiltered bookmark count query, including orphaned bookmarks. Use this metadata
for the sidebar total rather than fetching a bookmark page with article bodies.
A count-query failure is an error, never a fabricated zero. Reconcile this field,
item bookmark metadata and starred queries after successful Star/Unstar actions.
Keep the current success-only mutation updates; show the existing-style error
toast on failure and preserve prior state. Optimistic updates are not required.

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
Keep the current ESLint stack and move TypeScript to its supported stable 5.9
line; do not adopt a prerelease linter stack merely to retain TypeScript 7.
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

Identify the system Default group by `group.id === 1`, including after a rename.
Hide it in sidebar navigation and feed management only when a successful complete
feed query confirms there are no feeds with that group ID. Use unfiltered group
membership, not the visible search/filter result. Show it when populated and
retain it during loading or a failed feed query, even with cached empty data.
Keep the group stored and available to appropriate group-selection controls.
Do not infer emptiness from a failed request, delete the system group, or hide
unrelated empty user-created groups.

Acceptance: both views hide empty Default, display populated Default, and preserve
all subscriptions, group assignments, bookmarks and read state across that change.

## Documentation discrepancy and follow-up boundaries

The [backend design](../backend-design.md) describes default private-network
protection without covering the unguarded discovery path or the proxy trust
boundary. R2 must close the discovery gap and document the proxy limitation.
The [API conventions](../api-conventions.md) do not yet describe R2's new upstream
error outcome; update them with the implementation. The existing unreachable-URL
test asserts successful emptiness and must change under R2.

[CONTRIBUTING.md](../../CONTRIBUTING.md) accurately records the present lint
incompatibility but says to wait for upstream TypeScript support. R8 instead
chooses a supported compiler; T7 must restore lint to that checklist and remove
the obsolete blocker after the toolchain is repaired. This discrepancy was
omitted from the first draft. Do not change current setup/design guidance ahead
of the implementation it describes. No frontend interaction-design conflict
requiring a product decision was identified; the unread undo design is retained.

Response-size limits and an article retention policy were identified as further
long-running-service concerns, not reproduced data-loss bugs. They are recorded
for later work: choosing size thresholds or deleting old content would add new
behavior and is not part of this first baseline. Do not silently convert these
observations into configuration options or destructive cleanup requirements.
