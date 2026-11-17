# Agent Instructions

## Communication

- Use English for all documentation and code comments.
- Keep responses concise and actionable.
- Challenge proposals when you have a better alternative.

## Project Context

- This project is an open-source, lightweight RSS reader and aggregator.
- Prioritize simplicity and maintainability over complexity.
- HTTP API facts live in code: routes in `backend/internal/handler/handler.go` (`SetupRouter`), response shapes in `backend/internal/model/model.go` and request bodies in the handler files (both mirrored by `frontend/src/lib/api/types.ts`); cross-cutting conventions in `docs/api-conventions.md`.

## Code Standards

- Follow best practices without over-engineering.
- Default to no backward-compatibility work unless explicitly requested; if a change may break data formats, public APIs, or migrations, clearly state the impact.
- Docs answer why, code says what: never copy facts that live in a file (versions, routes, dependencies, env defaults) into documentation — link to the source instead. See `docs/README.md`.
- Write self-explanatory code with clear naming.
- Add comments in English only when they provide non-obvious value:
  - **DO write comments for:**
    - Complex business logic or algorithms
    - Non-obvious design decisions and trade-offs
    - Public APIs, exported functions, and package documentation
    - TODO/FIXME/NOTE markers with context
  - **DON'T write comments for:**
    - Self-evident code (e.g., getters/setters)
    - Repeating what the code already says
    - Implementation details that naming makes clear

## Git Commits

- Automatically commit completed, authorized changes unless the user requests otherwise. Review-only requests do not authorize edits or commits. Push only when explicitly authorized.
- Preserve the configured Git author. Add a `Co-Authored-By` trailer identifying the actual agent and verified model; never copy another agent's attribution. For Codex, use `Co-Authored-By: Codex <verified-model-name> <noreply@openai.com>`, replacing the model placeholder with the actual model identifier.
- In Codex, read `CODEX_THREAD_ID` to identify the current session. Locate its matching `.jsonl` rollout under `$CODEX_HOME/sessions` (default `~/.codex/sessions`), then read `payload.model` from the latest record whose `type` is `turn_context`. Read only the metadata needed for attribution. Do not infer the current model from configuration defaults or previous commit trailers; if it cannot be verified, ask the user instead of guessing.
- After every commit, proactively report its hash, subject, author, and exact trailer text for the user to confirm.

## Go Development

- After modifying Go code, run `goimports -w .` before verification.
- Verify compilation with `go build -o /dev/null /path/to/file_or_dir`.
- Run related tests and ensure they pass.
- Use named SQL parameters (e.g., `:param_name` or `@param_name`).

## Frontend Development

- Verify TypeScript/TSX compilation with `pnpm run typecheck` (regenerates the TanStack route tree first — `routeTree.gen.ts` is a generated, gitignored file).
- Run frontend unit tests with `pnpm test` (vitest); check i18n completeness with `pnpm run check:i18n`.
- shadcn components use Base UI (`@base-ui/react`) on the `base-vega` style. Regenerate via CLI (`pnpm dlx shadcn@latest add <component> --overwrite`) instead of hand-editing source files in `frontend/src/components/ui/`.
