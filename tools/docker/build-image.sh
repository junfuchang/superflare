#!/usr/bin/env sh
set -eu

IMAGE_NAME="${1:-superflare}"
TAG="${2:-latest}"
PLATFORM="${PLATFORM:-}"
NO_CACHE="${NO_CACHE:-0}"

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
TEMP_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/superflare-docker.XXXXXX")
IMAGE_REF="${IMAGE_NAME}:${TAG}"

cleanup() {
  rm -rf "$TEMP_ROOT"
}

trap cleanup EXIT INT TERM

if ! command -v docker >/dev/null 2>&1; then
  echo "docker executable was not found in PATH." >&2
  exit 1
fi

copy_item() {
  src="$1"
  dest="$2"
  mkdir -p "$(dirname "$dest")"
  cp -R "$src" "$dest"
}

for item in \
  build \
  cmd \
  config \
  embed \
  internal \
  tools/docker/Dockerfile \
  tools/docker/docker-entrypoint.sh \
  go.mod \
  go.sum \
  main.go \
  doc.go \
  README.md
do
  if [ ! -e "$REPO_ROOT/$item" ]; then
    echo "missing required path: $REPO_ROOT/$item" >&2
    exit 1
  fi
  copy_item "$REPO_ROOT/$item" "$TEMP_ROOT/$item"
done

set -- build --file "$TEMP_ROOT/tools/docker/Dockerfile" --tag "$IMAGE_REF"

if [ -n "$PLATFORM" ]; then
  set -- "$@" --platform "$PLATFORM"
fi

if [ "$NO_CACHE" = "1" ]; then
  set -- "$@" --no-cache
fi

set -- "$@" "$TEMP_ROOT"

echo "Building Docker image $IMAGE_REF"
if [ -n "$PLATFORM" ]; then
  echo "Platform: $PLATFORM"
fi

docker "$@"
