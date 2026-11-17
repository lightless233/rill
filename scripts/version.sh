#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

case "${1:-version}" in
version)
  if ! commit=$(git -C "$root" rev-parse --short=7 HEAD 2>/dev/null); then
    printf 'dev\n'
    exit 0
  fi
  date=$(git -C "$root" show -s --format=%cs HEAD | tr -d '-')
  tag=$(git -C "$root" describe --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null || printf 'dev')
  dirty=$(git -C "$root" describe --always --dirty 2>/dev/null)
  case "$dirty" in
  *-dirty) suffix=-dirty ;;
  *) suffix= ;;
  esac
  printf '%s-%s-%s%s\n' "$date" "$commit" "${tag#v}" "$suffix"
  ;;
commit)
  git -C "$root" rev-parse --short=7 HEAD 2>/dev/null || printf 'unknown\n'
  ;;
*)
  echo "Usage: $0 [version|commit]" >&2
  exit 1
  ;;
esac
