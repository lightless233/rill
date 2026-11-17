#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
DEPLOY_ENV_FILE="${DEPLOY_ENV_FILE:-$SCRIPT_DIR/.env.deploy}"

if [ -f "$DEPLOY_ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$DEPLOY_ENV_FILE"
  set +a
fi

ALLOW_DIRTY=0
SKIP_BUILD=0
BACKUP_DB=0
TAG_OVERRIDE=""

while [ $# -gt 0 ]; do
  case "$1" in
    --allow-dirty)
      ALLOW_DIRTY=1
      shift
      ;;
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    --backup-db)
      BACKUP_DB=1
      shift
      ;;
    --tag)
      TAG_OVERRIDE="${2:?--tag requires a value}"
      shift 2
      ;;
    -h|--help)
      cat <<'EOF'
Usage: ./deploy/deploy.sh [--tag TAG] [--allow-dirty] [--skip-build] [--backup-db]

Builds the Rill binary + Docker image for linux/amd64 (configurable via
DEPLOY_PLATFORM), streams the image over SSH to DEPLOY_REMOTE_DIR,
updates compose.yaml and RILL_IMAGE in remote .env,
and waits until the container reports healthy.

Database backups are skipped unless --backup-db is explicitly supplied.

Configure connection settings via deploy/.env.deploy (gitignored) or env vars.
EOF
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

: "${DEPLOY_SSH_TARGET:?Set DEPLOY_SSH_TARGET in environment or deploy/.env.deploy}"
DEPLOY_REMOTE_DIR="${DEPLOY_REMOTE_DIR:-/srv/rill}"
DEPLOY_IMAGE_NAME="${DEPLOY_IMAGE_NAME:-lightless-fusion}"
DEPLOY_PLATFORM="${DEPLOY_PLATFORM:-linux/amd64}"
DEPLOY_SSH_KEY="${DEPLOY_SSH_KEY:-}"
DEPLOY_KNOWN_HOSTS="${DEPLOY_KNOWN_HOSTS:-}"

TARGET_OS="${DEPLOY_PLATFORM%/*}"
TARGET_ARCH="${DEPLOY_PLATFORM#*/}"
if [ "$TARGET_OS" = "$DEPLOY_PLATFORM" ] || [ -z "$TARGET_OS" ] || [ -z "$TARGET_ARCH" ]; then
  echo "Invalid DEPLOY_PLATFORM: $DEPLOY_PLATFORM (expected os/arch)" >&2
  exit 1
fi

if [ "$ALLOW_DIRTY" -ne 1 ] && [ -n "$(git -C "$REPO_ROOT" status --porcelain --untracked-files=no)" ]; then
  echo "Refusing to deploy with uncommitted tracked changes (commit first or pass --allow-dirty)." >&2
  exit 1
fi

SHORT_SHA=$(git -C "$REPO_ROOT" rev-parse --short HEAD)
TAG_SUFFIX="${TAG_OVERRIDE:-rill-$SHORT_SHA}"
FULL_IMAGE="${DEPLOY_IMAGE_NAME}:${TAG_SUFFIX}"

SSH_OPTS="-o BatchMode=yes"
if [ -n "$DEPLOY_SSH_KEY" ]; then
  SSH_OPTS="$SSH_OPTS -i $DEPLOY_SSH_KEY"
fi
if [ -n "$DEPLOY_KNOWN_HOSTS" ]; then
  SSH_OPTS="$SSH_OPTS -o StrictHostKeyChecking=yes -o UserKnownHostsFile=$DEPLOY_KNOWN_HOSTS"
fi

BIN_REL="build/fusion-${TARGET_OS}-${TARGET_ARCH}"
BIN_ABS="$REPO_ROOT/$BIN_REL"

cleanup() {
  find "$REPO_ROOT/backend/internal/web/dist" -mindepth 1 ! -name '.keep' -delete 2>/dev/null || true
  rm -f "$BIN_ABS"
}
trap cleanup EXIT

if [ "$SKIP_BUILD" -ne 1 ]; then
  echo "Building frontend and ${TARGET_OS}/${TARGET_ARCH} backend for ${FULL_IMAGE}..."
  (
    cd "$REPO_ROOT"
    ./scripts.sh build-frontend
    ./scripts.sh build-backend "$TARGET_OS" "$TARGET_ARCH" "$BIN_ABS"
    docker build --platform "$DEPLOY_PLATFORM" -t "$FULL_IMAGE" .
  )
fi

echo "Streaming ${FULL_IMAGE} to ${DEPLOY_SSH_TARGET}..."
# shellcheck disable=SC2086
docker save "$FULL_IMAGE" | gzip -c | ssh $SSH_OPTS "$DEPLOY_SSH_TARGET" "gzip -dc | docker load"

echo "Uploading compose.yaml to ${DEPLOY_REMOTE_DIR}..."
# shellcheck disable=SC2086
ssh $SSH_OPTS "$DEPLOY_SSH_TARGET" sh -eu -s -- "$DEPLOY_REMOTE_DIR" <<'MKDIR_SH'
mkdir -p "$1/data" "$1/backups"
MKDIR_SH
# shellcheck disable=SC2086
scp $SSH_OPTS "$SCRIPT_DIR/compose.yaml" "${DEPLOY_SSH_TARGET}:${DEPLOY_REMOTE_DIR}/compose.yaml.new"

echo "Deploying on ${DEPLOY_SSH_TARGET}:${DEPLOY_REMOTE_DIR}..."
# shellcheck disable=SC2086
ssh $SSH_OPTS "$DEPLOY_SSH_TARGET" sh -eu -s -- "$DEPLOY_REMOTE_DIR" "$FULL_IMAGE" "$TAG_SUFFIX" "$BACKUP_DB" <<'REMOTE_SH'
remote_dir="$1"
full_image="$2"
tag_suffix="$3"
backup_db="$4"

cd "$remote_dir"

if [ -f compose.yaml ]; then
  docker compose -f compose.yaml stop fusion || true
fi

if [ "$backup_db" -eq 1 ] && ls data/fusion.db* >/dev/null 2>&1; then
  backup_dir="$remote_dir/backups/pre-${tag_suffix}-$(date +%Y%m%dT%H%M%S)"
  mkdir -p "$backup_dir"
  cp -a data/fusion.db* "$backup_dir/"
  ls -lh "$backup_dir/"
fi

if [ -f .env ]; then
  awk -v img="$full_image" '
    BEGIN { found = 0 }
    /^RILL_IMAGE=/ { print "RILL_IMAGE=" img; found = 1; next }
    { print }
    END { if (!found) print "RILL_IMAGE=" img }
  ' .env > .env.tmp
else
  printf 'RILL_IMAGE=%s\n' "$full_image" > .env.tmp
fi
chmod 0600 .env.tmp
mv .env.tmp .env

mv compose.yaml.new compose.yaml
chmod 0644 compose.yaml

docker compose -f compose.yaml up -d fusion

cid=$(docker compose -f compose.yaml ps -q fusion)
i=0
health="starting"
while [ "$i" -lt 30 ]; do
  health=$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || echo "starting")
  if [ "$health" = "healthy" ]; then
    break
  fi
  i=$((i + 1))
  sleep 1
done

docker compose -f compose.yaml ps
if [ "$health" != "healthy" ]; then
  echo "Container did not become healthy (state: $health)" >&2
  docker logs --tail=30 "$cid" >&2 || true
  exit 1
fi
REMOTE_SH

echo "Deployed ${FULL_IMAGE} to ${DEPLOY_SSH_TARGET}:${DEPLOY_REMOTE_DIR}."
