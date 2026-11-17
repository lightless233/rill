#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

case "${1:-version}" in
version)
  git -C "$root" describe --match 'v[0-9]*.[0-9]*.[0-9]*' --always --dirty 2>/dev/null || printf 'dev\n'
  ;;
commit)
  git -C "$root" rev-parse --short HEAD 2>/dev/null || printf 'unknown\n'
  ;;
*)
  echo "Usage: $0 [version|commit]" >&2
  exit 1
  ;;
esac
