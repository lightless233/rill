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
secret or new authentication framework. Update the focused Fever guidance to
state the no-password restriction after the code enforces it.

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
Inspect the pinned finder's real interface before choosing the integration:
its currently used options do not provide a guarded client. Do not assume that
validating only input/output URLs or reusing the same timeout fixes the gap.

Use the smallest integration that preserves supported RSS/Atom and page discovery.
If discovery must be implemented locally because the dependency cannot accept the
transport, reuse its relevant behavior and existing parsing libraries; avoid a
parallel unguarded fetcher, generic crawler, or an automatically created second
fork. Keep applicable third-party notices if code is reused. Preserve explicit
private-feed opt-in and supported proxy behavior without weakening default policy.

Primary scope: [HTTP client and policy](../../backend/internal/pkg/httpc/),
[feed validation/discovery](../../backend/internal/handler/feed.go),
[pull parsing](../../backend/internal/pull/parser.go), relevant dependency entries
only if the chosen integration requires them, and [backend design](../backend-design.md).

Completion evidence: controlled public-to-private redirect and direct-private
cases are rejected before the destination is contacted; verified-address dialing
does not perform an unchecked second resolution. Public discovery, ordinary
pulling, explicit private fixtures and permitted proxy use still work. Keep these
fixtures local; no production metadata-service probes are needed.

### T3 — Separate queue lifetime from attempt timeout (R3 / F03)

Problem: initial and single-feed manual contexts can expire while acquiring the
shared semaphore. The newer scheduler may also pull those feeds; do not assume
the historical observed counts reproduce unchanged.

Trace create, batch import, refresh-one, refresh-all and periodic execution.
Retain the shared concurrency mechanism, but start the attempt deadline after
admission and connect waiting work to cancellation/shutdown. Keep scheduling
wakes, per-feed overrides, suspension and backoff intact. Record fetch outcomes
only for actual attempts; do not mask queue cancellation as an HTTP response.

Primary scope: [main lifecycle](../../backend/cmd/fusion/main.go),
[feed handlers](../../backend/internal/handler/feed.go),
[puller](../../backend/internal/pull/puller.go), its parser and scheduling tests.
Extract shared execution logic only where these real callers benefit.

Completion evidence: queued duration can exceed an individual attempt budget
without losing eligible work; each started attempt is bounded, total concurrency
remains bounded, and cancellation terminates waiting work. Existing conditional
request and scheduling regressions remain covered.

### T4 — Propagate query failures and reconcile refresh completion (R4 / F04–F05)

Problem: query aggregation drops error state, and refresh acceptance currently
triggers only an immediate invalidation.

Carry the existing query states through aggregating hooks to the consuming views.
Use existing loading, notification and control components for errors and retry;
retain stale data with an explicit refresh error where applicable. Cover feeds,
groups, articles and bookmarks at their real consumers rather than adding a
success-shaped fallback in the API client.

Expose the Spec's additive `refresh_all_running` field through existing app-info,
using the current refresh-all state and synchronization. Update the mirrored
frontend type and existing query layer. Poll only while relevant manual work is
active, and reconcile feeds/items when it ends. An operation ending does not mean
that every source succeeded; preserve per-source error visibility. Clarify
creation/import acknowledgement without creating a generic job protocol.

Primary scope: [feed handlers](../../backend/internal/handler/feed.go),
[handler state](../../backend/internal/handler/handler.go),
[frontend API](../../frontend/src/lib/api/), [queries](../../frontend/src/queries/),
[article-list hook](../../frontend/src/hooks/use-article-list.ts), and the current
reading/feed-management consumers. Update affected API/design explanations with
the implemented contract, not a duplicated route inventory.

Completion evidence: a failed list request does not show an empty success; cached
data and retry behave meaningfully; delayed refresh completion updates visible
data without reloading the document. Completion-before-first-poll and a failed
status request have correct outcomes, and polling is cleaned up on departure.

### T5 — Correct bookmark lookup and read-action scope (R5–R6 / F06–F07)

Problem: ordinary views depend on a global first page for bookmark resolution;
the loaded-item read operation has a misleading whole-list label.

Resolve bookmark metadata for relevant articles without fetching every bookmark's
full content. Prefer a small compatible extension at the existing item/lookup
boundary over another global cache. Keep the starred view's existing independent
pagination, orphan handling and snapshot survival. Reconcile ordinary lists,
detail, counts and starred lists on success; retain meaningful rollback on error.

Keep the read mutation's existing loaded-ID behavior and correct the label in
the locale catalogs. Do not expand it into a new whole-library or whole-scope
mutation under a maintenance task. Preserve unread-view undo pins and keyboard
navigation; do not remove gray undoable rows as an unrelated cleanup.

Primary scope: [bookmark queries](../../frontend/src/queries/bookmarks.ts),
[item queries](../../frontend/src/queries/items.ts),
[article aggregation](../../frontend/src/hooks/use-article-list.ts), reading actions
and [locale catalogs](../../frontend/src/lib/i18n/messages/). Item/model/store
changes are limited to any necessary compatible metadata addition. Consult the
[frontend interaction design](../frontend-design.md) before changing cache behavior.

Completion evidence: an older bookmark beyond the first lookup page has a working
Unstar action in an ordinary view; the paginated starred view still works. A
two-loaded/six-matching read action communicates its scope and changes only the
two supplied IDs. Cover rollback and undo behavior, not static label strings.

### T6 — Narrow FTS maintenance through a migration (R7 / F08)

Problem: every read-state update causes content-index writes.

Add the next ordered migration to replace the overly broad update trigger.
Preserve actual content-change indexing and insertion/deletion behavior. Do not
change old migration files as the only upgrade path, rebuild the database,
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

Select a supported compiler/linter combination, update the lockfile coherently,
and include lint in the existing frontend workflow. Reuse current tooling and
avoid unrelated framework upgrades. Fix real errors revealed by the functioning
check; do not suppress them with casts, ignored rules or skipped files/tests.
Respect the regeneration rule for vendored UI components. If newly exposed
errors require a material change outside this contract, report that concrete
scope issue rather than quietly disabling checks or starting a broad rewrite.

Primary scope: [package configuration](../../frontend/package.json),
[lockfile](../../frontend/pnpm-lock.yaml), [ESLint](../../frontend/eslint.config.js),
and [frontend CI](../../.github/workflows/frontend.yml). Regression coverage added
by T1–T6 should exercise their behaviors through existing test conventions; this
unit is not a mandate for test-count or coverage-percentage growth.

Completion evidence: a clean frozen-lockfile installation can run lint, typecheck,
tests, i18n validation and the frontend build; CI invokes the same lint entry.

### T8 — Bring the existing Default customization into source (R9 / F10)

Problem: building the unmodified fork would lose the already-requested behavior.

Apply the small conditional rendering change directly to the existing sidebar
and feed-management group rendering. Integrate it with T4's successful/failed load
distinction instead of treating a defaulted empty array as proof of emptiness.
Keep Default stored and selectable where appropriate; keep other groups unchanged.
Do not ship this as a replacement deployment-time patch mechanism inside Rill.

Primary scope: [sidebar feed list](../../frontend/src/components/feed/feed-list.tsx)
and [feed management](../../frontend/src/routes/feeds.lazy.tsx).

Completion evidence: empty/populated Default behaves correctly in both views,
with no subscription, bookmark, or read-state changes and no hidden query error.

## Integration order and completion evidence

Suggested order for one agent: T7 tooling repair, T1 authentication, T2 HTTP
boundary, T3 execution lifetime, T6 indexing, T4 query/refresh flow, T5 reading
actions, then T8 customization. T4 depends on T3's reliable execution lifecycle;
T8 depends on T4's query-state handling. Other backend units are logically
independent. This order is not permission to execute them in this conversation.

Use focused regressions for each coherent unit and run the aggregate checks once
against the integrated result. Reuse the repository's existing test/build entry
points, account for checks they already run, and avoid repeating the full suite
after every small edit. Include a backend race run for touched concurrency paths
and bounded browser acceptance for request failure, delayed completion,
pagination/bookmark state, loaded read scope, and Default visibility. Browser
response interception is valid UI evidence but must not be reported as a real
database mutation test.

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
