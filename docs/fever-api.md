# Fever API Compatibility

Rill provides a Fever-compatible endpoint for third-party RSS clients such as Reeder, Unread, and FeedMe.

## Endpoint

- `POST /fever`
- `POST /fever/`
- `POST /fever.php`

Request body format is `application/x-www-form-urlencoded`.

## Authentication

Fever clients should send:

- `api=1`
- `api_key=<md5(username:password)>`

Username is configured by `FUSION_FEVER_USERNAME` (default: `fusion`).
Password is your Rill login password (`FUSION_PASSWORD`).

Fever requires a nonempty `FUSION_PASSWORD` (as checked by `strings.TrimSpace`). When no nonempty password is configured — including OIDC-only deployments or anonymous mode with `FUSION_ALLOW_EMPTY_PASSWORD=true` — Fever authentication is disabled and all requests fail with `"auth": 0`.

Example (macOS):

```bash
md5 -q -s 'fusion:your_password'
```

Example (Linux):

```bash
printf 'fusion:your_password' | md5sum | cut -d' ' -f1
```

## Client Setup

Use these values in your mobile/desktop RSS client:

- Account type: `Fever`
- Server URL: your Rill base URL, for example:
  - `https://rss.example.com`
  - `http://192.168.1.10:8080`
- Username: `FUSION_FEVER_USERNAME` (default: `fusion`)
- Password: `FUSION_PASSWORD`

Most clients only need the base URL. Rill supports all common Fever paths:

- `/fever`
- `/fever/`
- `/fever.php`

If your client asks for a dedicated Fever endpoint, try these in order:

1. `https://your-domain/fever`
2. `https://your-domain/fever/`
3. `https://your-domain/fever.php`

### Quick Connectivity Check

You can verify credentials before configuring a client:

```bash
curl -sS -X POST 'https://your-domain/fever' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data 'api=1&api_key=<md5(username:password)>'
```

Expected success response includes:

- `"auth": 1`
- `"api_version": 3`

### Client Notes

- Reeder / Unread / FeedMe: choose `Fever` account type and use Rill password directly.
- If you deploy Rill behind a reverse proxy, ensure `POST /fever*` is forwarded to Rill unchanged.

## Implemented Fever Features

Read APIs:

- `groups=1` -> `groups`, `feeds_groups`
- `feeds=1` -> `feeds`
- `favicons=1` -> `favicons` (placeholder payload)
- `items=1` (+ `since_id`, `max_id`, `with_ids`) -> `items`
- `unread_item_ids=1` -> CSV item IDs
- `saved_item_ids=1` -> CSV item IDs

Write APIs:

- `mark=item&id=<id>&as=read|unread|saved|unsaved`
- `mark=feed&id=<id>&as=read&before=<unix_timestamp>`
- `mark=group&id=<id>&as=read&before=<unix_timestamp>`

## Notes

- Saved items map to Rill bookmarks.
- `links`, `sparks`, and `kindlings` are not implemented.
- This compatibility API is outside `/api`; it is intentionally separate from the `/api` conventions documented in `docs/api-conventions.md`.
