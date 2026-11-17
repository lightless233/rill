# Contributing to Fusion

Thanks for your interest in Fusion.

Fusion is maintained by a small team working closely with coding agents, and
the sustainable way to contribute is [issues](https://github.com/0x2E/fusion/issues):
report bugs, propose features, or ask questions there. There is no need to
open pull requests.

Fusion values simple, maintainable changes over complex abstractions.

## 1. Reporting problems

- Search existing issues first to avoid duplicates.
- For bugs, include reproducible steps, expected behavior, and actual behavior.
- For features, explain the concrete use case and user value.

## 2. Local setup

Requirements (versions are pinned in-repo, not here):

- Go: [`backend/go.mod`](./backend/go.mod)
- Node.js: [`frontend/package.json`](./frontend/package.json) (`engines`)
- pnpm

Install dependencies:

```shell
# frontend
cd frontend && pnpm install
```

Backend uses Go modules and installs dependencies automatically via `go` tooling.

### Secret scanning before push

Install the official [TruffleHog binary](https://github.com/trufflesecurity/trufflehog#-installation)
on your PATH, then enable this repository's hook from the repository root:

```shell
git config --local core.hooksPath .githooks
```

The pre-push hook checks the outgoing commit range. New refs or an unavailable
remote base require scanning the pushed ref's full history. Findings or scan
errors block the push. Scans run locally without online credential verification
and print only detector names and Git locations, never credential values.
Review findings before pushing; do not bypass the hook to publish a real secret.
The wrapper contains one exact exception for a historical npm checksum verified
against the registry. It does not exclude lockfiles or disable a detector;
other findings and scan errors still block the push.

To scan all locally reachable Git history manually:

```shell
python3 scripts/scan_secrets.py
```

The scanner reads committed Git objects, not the working tree or staged changes.

## 3. Validation checklist (for local changes)

### Backend

```shell
cd backend
go test ./...
goimports -w .
go build -o /dev/null ./cmd/fusion
```

### Frontend

```shell
cd frontend
pnpm lint
pnpm typecheck
pnpm test
pnpm run check:i18n
pnpm build
```

If your change is scoped, you can run a smaller test subset first, but run
the relevant final checks before committing.

## 4. Code style guidelines

- Prefer readable, self-explanatory naming.
- Avoid over-engineering.
- Add comments only for non-obvious logic or decisions.
- Keep docs and API behavior aligned when changing contracts.

## 5. Release versions

Annotated Git tags named `vMAJOR.MINOR.PATCH` are the release-version source of
truth. Use PATCH for fixes and MINOR for features; reserve a MAJOR release for
breaking changes, and `v1.0.0` for the first stable release. Published tags must
not be moved or overwritten. The private frontend package version is not an
application release number.

Commit the accepted changes before creating a release tag. Build from that
tagged, clean checkout with the existing scripts, then push the branch and the
specific tag. [The release workflow](.github/workflows/release.yml) tests and
builds a draft GitHub Release; publishing and production deployment remain
separate actions.

[scripts/version.sh](scripts/version.sh) resolves the tag/commit identity and
marks tracked local modifications as dirty. [scripts.sh](scripts.sh) injects it
into the backend. The binary's `--version`, startup log and
[application-info handler](backend/internal/handler/feed.go) expose the same
identity; the About screen reads it from the running server.
