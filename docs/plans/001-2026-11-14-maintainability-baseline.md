# 001: Rill maintainability and correctness implementation plan

Status: planning only; do not start implementation or production operations from
the current documentation-writing request.

Contract: [Spec 001](../specs/001-2026-11-14-maintainability-baseline.md).
Read its baseline/evidence table first. Historical reproductions used an earlier
release; this fork includes scheduler and per-feed-interval changes. Locate work
through the source links below and preserve those newer behaviors.

## Execution scope

One implementing agent can execute these units sequentially. The user will arrange
the handoff; this plan does not dispatch agents or mandate a multi-agent workflow.
Keep existing responsibilities and organize locally around demonstrated shared
logic. There is no directory-wide rewrite or mandatory file-splitting target.

The application source is currently the upstream fork. The trial deployment's
empty-Default customization exists outside this repository and is not yet ported.
No production server state was inspected during this planning turn. Server
credentials, production databases, subscription exports and deployment artifacts
must not be copied into this source repository as part of implementation.

## Implementation units

### T1 — Close the Fever authentication gap (R1 / F01)

Problem: Fever authenticates independently of the session/OIDC boundary and can
derive a usable key without a secret.

Change the eligibility check at the existing Fever boundary; keep configured-
password protocol behavior and explicit anonymous web/API behavior. Reuse current
configuration and authentication-failure responses rather than adding another
secret or new authentication framework. Match configuration's trimmed emptiness
check, but derive valid keys from the original, untrimmed password. Update the
focused Fever guidance to state the no-password restriction after the code
enforces it.

Primary scope: [handler construction](../../backend/internal/handler/handler.go),
[Fever](../../backend/internal/handler/fever.go), its existing tests, and
[Fever documentation](../fever-api.md).

Completion evidence: isolated OIDC-only/empty-secret access is denied, while
valid and invalid configured-password requests retain their established outcomes.

### T2 — Make outbound request policy consistent (R2 / F02)

Problem: discovery uses a separate HTTP stack, while direct dialing can resolve
again after address validation.

Centralize actual outbound enforcement in the current HTTP boundary and connect
discovery to it, covering initial URLs, redirects and discovered candidates.
The pinned finder constructs its own client and swallows page/probe errors;
replace its HTTP execution with a small project-owned discovery implementation.
Reuse relevant service URL rules, HTML parsing, well-known probing and existing
feed parsers; keep applicable third-party notices if code is reused. Remove the
external finder dependency if no callers remain. Do not introduce a generic
crawler, a parallel unguarded fetcher or a second maintained fork.

Separate primary retrieval, parsing and optional probe outcomes. Implement R2's
400/502/500 error distinctions in the existing error envelope, including DNS and
timeout failures as 502. Preserve 200/empty for successful pages without feeds
and successfully retrieved malformed XML without alternatives; optional 404s
must not manufacture an error. Update `TestValidateFeedUnreachableURLReturnsNoFeeds`
and cover the error path consumed by the add-feed dialog.

Separate dialing the explicitly configured trusted proxy from feed destination
validation: permit private/loopback proxies without granting private-feed access.
With private feeds disabled, retain target DNS validation locally on every
request/redirect; do not add an automatic DNS-validation bypass. With explicit
private-feed opt-in, proxy-side DNS remains usable. Pin checked addresses for
direct connections, and document that proxy-side DNS/destination enforcement is
trusted rather than claiming the direct-connection guarantee for proxies.

Primary scope: [HTTP client and policy](../../backend/internal/pkg/httpc/),
[feed validation/discovery](../../backend/internal/handler/feed.go),
[pull parsing](../../backend/internal/pull/parser.go), relevant dependency entries
for the removed finder, [backend design](../backend-design.md), and
[API error conventions](../api-conventions.md).

Completion evidence: controlled public-to-private redirect and direct-private
cases are rejected before the destination is contacted; verified-address dialing
does not perform an unchecked second resolution. Public discovery, ordinary
pulling, explicit private fixtures and permitted proxy use still work. Cover the
local-proxy exemption, blocked feed targets through that proxy and proxy-mode
DNS failure as well as primary failures versus optional misses. Keep these
fixtures local; no production metadata-service probes are needed.

### T3 — Separate queue lifetime from attempt timeout (R3 / F03)

Problem: initial and single-feed manual contexts can expire while acquiring the
shared semaphore. The newer scheduler may also pull those feeds; do not assume
the historical observed counts reproduce unchanged.

Trace create, batch import, refresh-one, refresh-all and periodic execution.
Retain the shared concurrency mechanism, but start the attempt deadline after
admission. Create the process shutdown context before constructing the Puller
and inject it there; preserve the handler's existing puller interface. Ensure
request-triggered background work observes shutdown without binding accepted
background work to the HTTP response's lifetime. Keep explicit operation limits,
scheduling wakes, per-feed overrides, suspension and backoff intact.

Parent operation/process cancellation, including expiry of its deadline, stops
queued and in-flight work without incrementing consecutive failures, even after
HTTP dispatch. Distinguish parent cancellation from the attempt's own deadline
while its parent is still active;
only the latter and genuine network/HTTP errors enter failure accounting. Release
permits and clear operation state on every terminal path.

Primary scope: [main lifecycle](../../backend/cmd/fusion/main.go),
[feed handlers](../../backend/internal/handler/feed.go),
[puller](../../backend/internal/pull/puller.go), its parser and scheduling tests.
Extract shared execution logic only where these real callers benefit.

Completion evidence: queued duration can exceed an individual attempt budget
without losing eligible work; each started attempt is bounded, total concurrency
remains bounded, and cancellation terminates waiting and active work without
penalizing feed health. An actual attempt timeout still records a failure.
Existing conditional request and scheduling regressions remain covered.

### T4 — Propagate query failures and reconcile refresh completion (R4 / F04–F05)

Problem: query aggregation drops error state, and refresh acceptance currently
triggers only an immediate invalidation.

Carry the existing query states through aggregating hooks to the consuming views.
Use existing loading, notification and control components for errors and retry;
retain stale data with an explicit refresh error where applicable. Cover feeds,
groups, articles and bookmarks at their real consumers, including the article
drawer and search dialog, rather than adding a success-shaped fallback in the
API client. Preserve search cancellation/supersession behavior; a failed request
is distinct from both cancellation and a successful zero-result search.

Expose the Spec's additive `refresh_all_running` field through existing app-info,
using the current refresh-all state and synchronization. Update the mirrored
frontend type and existing query layer. Poll only while relevant manual work is
active, and reconcile feeds/items when it ends. An operation ending does not mean
that every source succeeded; preserve per-source error visibility. Existing
creation/import success copy already describes subscription creation accurately;
keep it and immediate invalidation. Do not add initial-fetch status polling or
extra lifecycle fields for creation/import.

Primary scope: [feed handlers](../../backend/internal/handler/feed.go),
[handler state](../../backend/internal/handler/handler.go),
[frontend API](../../frontend/src/lib/api/), [queries](../../frontend/src/queries/),
[article-list hook](../../frontend/src/hooks/use-article-list.ts), and the current
reading/feed-management consumers, [article drawer](../../frontend/src/components/article/article-drawer.tsx)
and [search dialog](../../frontend/src/components/search/search-dialog.tsx).
Update affected API/design explanations with the implemented contract, not a
duplicated route inventory.

Completion evidence: a failed list request does not show an empty success; cached
data and retry behave meaningfully; delayed refresh completion updates visible
data without reloading the document. Completion-before-first-poll and a failed
status request have correct outcomes, and polling is cleaned up on departure.
Detail/search failures show errors and allow retry instead of blank/empty success.

### T5 — Correct bookmark lookup and read-action scope (R5–R6 / F06–F07)

Problem: ordinary views depend on a global first page for bookmark resolution;
the loaded-item read operation has a misleading whole-list label.

Add nullable `bookmark_id` to the backend Item model, ordinary item list/detail
queries and frontend type. Use that ID for ordinary article Star/Unstar state;
adapt bookmark-to-Item projections in the starred view too. Keep list cardinality,
ordering and pagination intact. Add a non-unique `bookmarks(item_id)` index through
the next available migration; do not rewrite bookmark data or infer a new unique
constraint from this lookup requirement.

Extend existing app-info with `bookmark_count`, reusing the unfiltered
`CountBookmarks` query, and consume it for the sidebar total. Do not use
`bookmarkAPI.list({ limit: 1 })` as a count endpoint: it still fetches article
content. Remove the global full-content bookmark preload used only for star
lookup/counts. Keep the starred view's independent pagination, orphan handling
and snapshot survival.

Reconcile ordinary lists, detail, app-info counts and starred lists after successful
mutations using the current query layer. Keep success-only cache updates and add
failure toasts using existing notification/i18n conventions; a failed Star/Unstar
must leave the prior state intact. Do not introduce optimistic writes merely to
satisfy a rollback test.

Keep the read mutation's existing loaded-ID behavior and correct the label in
the locale catalogs. Do not expand it into a new whole-library or whole-scope
mutation under a maintenance task. Preserve unread-view undo pins and keyboard
navigation; do not remove gray undoable rows as an unrelated cleanup.

Primary scope: [bookmark queries](../../frontend/src/queries/bookmarks.ts),
[item queries](../../frontend/src/queries/items.ts),
[article aggregation](../../frontend/src/hooks/use-article-list.ts), reading actions
and [locale catalogs](../../frontend/src/lib/i18n/messages/),
[Item model](../../backend/internal/model/model.go),
[item store](../../backend/internal/store/item.go),
[bookmark store](../../backend/internal/store/bookmark.go), app-info in
[feed handlers](../../backend/internal/handler/feed.go), mirrored API types,
the sidebar and ordered migrations. Consult the
[frontend interaction design](../frontend-design.md) before changing cache behavior.

Completion evidence: an older bookmark beyond the first lookup page has a working
Unstar action in an ordinary view; the paginated starred view still works. A
two-loaded/six-matching read action communicates its scope and changes only the
two supplied IDs. A failed Star/Unstar shows an error and preserves state; the
global count includes orphans without a background bookmark-content request.
Cover failure preservation and undo behavior, not static label strings.

### T6 — Narrow FTS maintenance through a migration (R7 / F08)

Problem: every read-state update causes content-index writes.

Add the next ordered migration to replace the overly broad update trigger with
`AFTER UPDATE OF title, content` and a null-safe actual-value-change condition.
Preserve insertion/deletion behavior. Use only the normal migration chain:
fresh databases run the existing migrations followed by the new one; existing
databases run the pending migration. Do not edit historical migrations or the
legacy copy/import path for this trigger correction, rebuild the database,
introduce a separate indexer, or apply unrelated vacuum/retention work.

Primary scope: [migrations](../../backend/internal/store/migrations/),
[store migration tests](../../backend/internal/store/migrate_test.go), and existing
item/search tests. Keep named SQL parameters and current storage conventions.

Completion evidence: fresh and upgraded databases preserve IDs/read state and
search content; read-only state flips and unchanged content assignments perform
no FTS content maintenance, while real content changes remain indexed.

### T7 — Restore the frontend check path (R8 / F09)

Problem: the declared lint command cannot load its parser stack, and CI does not
exercise it.

The initial baseline repair kept the ESLint/typescript-eslint stack and selected
TypeScript's stable 5.9 line within its declared supported range; that historical
toolchain decision was later superseded when ESLint was replaced by Oxlint to
support the current TypeScript line (see
[package configuration](../../frontend/package.json) and
[Oxlint configuration](../../frontend/.oxlintrc.json)). Regenerate the lockfile
through pnpm and include lint in the existing frontend workflow. Restore lint in
CONTRIBUTING's check list and remove its obsolete wait-for-TS-7-support note after
the repair. Do not switch to prerelease lint dependencies or make unrelated
framework upgrades. Fix real errors revealed by the functioning check; do not
suppress them with casts, ignored rules or skipped files/tests.
Respect the regeneration rule for vendored UI components. If newly exposed
errors require a material change outside this contract, report that concrete
scope issue rather than quietly disabling checks or starting a broad rewrite.

Primary scope: [package configuration](../../frontend/package.json),
[lockfile](../../frontend/pnpm-lock.yaml), [Oxlint configuration](../../frontend/.oxlintrc.json)
(replacing the removed `frontend/eslint.config.js`),
and [frontend CI](../../.github/workflows/frontend.yml), plus
[contributor checks](../../CONTRIBUTING.md). Regression coverage added
by T1–T6 should exercise their behaviors through existing test conventions; this
unit is not a mandate for test-count or coverage-percentage growth.

Completion evidence: a clean frozen-lockfile installation can run lint, typecheck,
tests, i18n validation and the frontend build; CI invokes the same lint entry.
Install dependencies as part of implementation; the reviewed workspace had no
frontend node_modules. A registry metadata lookup for the chosen compiler line
succeeded during planning; a clean dependency installation still needs to pass.

### T8 — Bring the existing Default customization into source (R9 / F10)

Problem: building the unmodified fork would lose the already-requested behavior.

Apply the small conditional rendering change directly to the existing sidebar
and feed-management group rendering. Integrate it with T4's successful/failed load
distinction instead of treating a defaulted empty array as proof of emptiness.
Identify Default by `group.id === 1`, not its editable name. Test membership in
the complete unfiltered successful feed result, not the current filtered view.
Retain the group during feed loading or error, including when stale cached data
is empty. Keep it stored and selectable where appropriate; keep other groups
unchanged. Do not ship this as a replacement deployment-time patch mechanism
inside Rill.

Primary scope: [sidebar feed list](../../frontend/src/components/feed/feed-list.tsx)
and [feed management](../../frontend/src/routes/feeds.lazy.tsx).

Completion evidence: empty/populated Default behaves correctly in both views,
including a renamed system group, filtered views and loading/error states, with
no subscription, bookmark, or read-state changes and no hidden query error.

## Integration order and completion evidence

Suggested order for one agent: T7 tooling repair, T1 authentication, T2 HTTP
boundary, T3 execution lifetime, T6 indexing, T4 query/refresh flow, T5 reading
actions, then T8 customization. T4 depends on T3's reliable execution lifecycle;
T8 depends on T4's query-state handling. T5 extends T4's app-info response and must
preserve its refresh field. Allocate distinct migration numbers in execution
order: T6 takes the next slot (007 at this baseline); T5's bookmark index takes
the following available slot. This order is not permission to execute them in
this conversation.

Use focused regressions for each coherent unit and run the aggregate checks once
against the integrated result. Reuse the repository's existing test/build entry
points, account for checks they already run, and avoid repeating the full suite
after every small edit. Include a backend race run for touched concurrency paths
and bounded browser acceptance for request failure, delayed completion,
pagination/bookmark state, loaded read scope, and Default visibility. Browser
response interception is valid UI evidence but must not be reported as a real
database mutation test.

Use the available Chrome with a project-local browser driver, or install a local
Playwright tool if needed for the bounded checks. A global Playwright CLI is not
required. Store disposable drivers, fixtures and browser artifacts under `.runtime/`
unless an established project location applies; do not use `/tmp` or commit runtime
artifacts. Add a maintained dependency only if a reusable check actually needs it;
this does not require adopting a new end-to-end framework. Browser checks target
the isolated local application, not the trial blog deployment.

Verify fresh initialization and upgrade of a representative earlier local test
database, without production data or credentials. Retain the fork's per-feed interval
migration and current public contracts. There is no synthetic performance target
or production observation gate; the FTS claim needs direct write-path evidence.

Deliver a consolidated account of addressed finding IDs, affected contracts and
passing/failed checks. Historical checks are not fresh acceptance. Update the
relevant existing design/API explanations alongside implementation and identify
any remaining discrepancies. Implementation does not imply authorization to push,
deploy, or operate on production data. Follow the user's actual handoff scope;
the current documentation-writing phase stops with these documents.
