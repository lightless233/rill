[简体中文](./README.md) | [English](./README-EN.md)

# Rill

A lightweight, self-hosted RSS reader built on [Fusion](https://github.com/0x2E/fusion). Rill keeps reading simple while adding AI translation, summaries, and more reading preferences.

## Features

- Feed management: RSS/Atom, feed discovery, groups, and OPML import/export.
- Everyday reading: unread tracking, bookmarks, search, keyboard shortcuts, and navigation to the next unread feed after finishing the current one.
- AI assistance: article translation, list-title and excerpt translation, and article summaries using your configured OpenAI-compatible API.
- Full-text extraction: per-feed CSS content and exclusion selectors, result previews, and automatic extraction when pulling new articles.
- Article sharing: expiring article snapshot links with optional passcodes.
- Reading appearance: multiple light and dark palettes, plus article, CJK, and code font preferences, including OPPO Sans.
- Live unread counts: sidebar counts update when background pulls find new articles.
- Cross-device reading: responsive UI, PWA support, multiple interface languages, and a Fever API for third-party readers.
- Self-hosting: Go and SQLite, with the frontend embedded in a single executable; Docker deployment is also supported.

## Installation

### Build from source

Install Go, Node.js, and pnpm. Go and Node.js requirements are defined in [backend/go.mod](./backend/go.mod) and [frontend/package.json](./frontend/package.json). These commands use a POSIX shell:

```sh
git clone https://github.com/lightless233/rill.git
cd rill
./scripts.sh build-frontend
./scripts.sh build-backend
FUSION_PASSWORD='replace-with-your-password' FUSION_LISTEN_HOST=127.0.0.1 ./build/fusion
```

Open <http://localhost:8080>. Replace the example password with your own.

### Use Docker

Install Docker and build the image from the repository root:

```sh
./scripts.sh build-frontend
./scripts.sh build-backend linux amd64 ./build/fusion-linux-amd64
docker build --platform linux/amd64 -t rill:local .
docker run -d --name rill --restart unless-stopped \
  -p 127.0.0.1:8080:8080 \
  -v "$PWD/data:/data" \
  -e FUSION_DB_PATH=/data/fusion.db \
  -e FUSION_PASSWORD='replace-with-your-password' \
  rill:local
```

Open <http://localhost:8080>. Feed and article data persist in the host's `data/` directory. This example targets `linux/amd64`; for another architecture, adjust both the backend build arguments and Docker's `--platform`.

For an existing Docker Compose and reverse-proxy setup, see [deploy/compose.yaml](./deploy/compose.yaml). It uses an external proxy network and needs configuration for your environment. The remote deployment script and connection configuration example are [deploy/deploy.sh](./deploy/deploy.sh) and [deploy/.env.example](./deploy/.env.example).

## Configuration and usage

- **Server configuration**: see [`.env.example`](./.env.example) for environment variables. The project currently retains the `FUSION_*` variable names and the `fusion` executable name.
- **Local access**: when running the binary directly, use `FUSION_LISTEN_HOST=127.0.0.1` to restrict it to local access. Use `FUSION_ALLOW_EMPTY_PASSWORD=true` with an empty password to disable password login only in trusted local environments.
- **Public access**: provide HTTPS through a reverse proxy and configure trusted proxies and CORS for your environment. Do not bind to loopback inside the Docker container, as the proxy would be unable to reach it.
- **AI features**: configure the API URL, API Key, and model in Settings → AI. You can test the connection and choose translation display modes there.
- **Full-text extraction**: configure and preview rules in the feed editor. Saved rules apply to newly pulled articles without backfilling existing articles; extraction failures retain the RSS content.
- **Shared reading**: public share pages show an article snapshot from the time of sharing without requiring login. They do not offer translation or AI summary actions.
- **Third-party clients**: see the [Fever API guide](./docs/fever-api.md). Other server settings, including SSO/OIDC, are covered in [`.env.example`](./.env.example).

## Development and versions

See [CONTRIBUTING.md](./CONTRIBUTING.md) for local development, validation commands, and pre-push secret scanning. Build entry points are in [scripts.sh](./scripts.sh).

Git tag and build-version conventions are described in [Release versions](./CONTRIBUTING.md#5-release-versions). View the running version in Settings → About, or run:

```sh
./build/fusion --version
```

## Documentation and feedback

- [Documentation index](./docs/README.md)
- [API conventions](./docs/api-conventions.md)
- [Backend design](./docs/backend-design.md)
- [Frontend design](./docs/frontend-design.md)
- [Bug reports and feature requests](https://github.com/lightless233/rill/issues)

## Credits and license

Rill builds on [Fusion](https://github.com/0x2E/fusion). Thanks to its original author and contributors. Feed parsing is powered by [gofeed](https://github.com/mmcdole/gofeed).

Licensed under the [MIT License](./LICENSE), retaining the upstream copyright notice.
