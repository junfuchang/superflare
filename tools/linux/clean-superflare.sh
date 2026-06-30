#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
RUN_DIR="$REPO_ROOT/var/run"
CACHE_DIR="$REPO_ROOT/var/cache"
PID_FILE="$RUN_DIR/superflare.pid"
PORT="${SUPERFLARE_PORT:-3636}"
TMP_FNAPP_UNPACK_DIR="$REPO_ROOT/tmp-fnapp-unpack"
FNAPP_BINARY="$REPO_ROOT/fnapp/superflare/app/server/superflare"
FNAPP_PACKAGE="$REPO_ROOT/fnapp/superflare/superflare.fpk"

process_matches_superflare() {
  TARGET_PID="$1"
  BIN_PATH="$REPO_ROOT/superflare"
  if [ -z "$TARGET_PID" ]; then
    return 1
  fi
  case "$TARGET_PID" in
    *[!0-9]*) return 1 ;;
  esac
  if [ -L "/proc/$TARGET_PID/exe" ]; then
    EXE_PATH=$(readlink "/proc/$TARGET_PID/exe" 2>/dev/null || true)
    if [ "$EXE_PATH" = "$BIN_PATH" ]; then
      return 0
    fi
  fi
  if [ -r "/proc/$TARGET_PID/cmdline" ]; then
    CMDLINE=$(tr '\0' ' ' <"/proc/$TARGET_PID/cmdline" 2>/dev/null || true)
    case "$CMDLINE" in
      *"$BIN_PATH"*) return 0 ;;
    esac
  fi
  return 1
}

kill_pid() {
  TARGET_PID="$1"
  if [ -z "$TARGET_PID" ]; then
    return 0
  fi
  if ! process_matches_superflare "$TARGET_PID"; then
    echo "Skipped PID $TARGET_PID: it is not this checkout's SuperFlare process."
    return 0
  fi
  if kill -0 "$TARGET_PID" >/dev/null 2>&1; then
    kill "$TARGET_PID" >/dev/null 2>&1 || true
    for _ in 1 2 3 4 5 6 7 8 9 10; do
      if ! kill -0 "$TARGET_PID" >/dev/null 2>&1; then
        break
      fi
      sleep 1
    done
    if kill -0 "$TARGET_PID" >/dev/null 2>&1; then
      kill -9 "$TARGET_PID" >/dev/null 2>&1 || true
    fi
    echo "Stopped PID $TARGET_PID"
  fi
}

remove_children() {
  TARGET_DIR="$1"
  if [ -d "$TARGET_DIR" ]; then
    find "$TARGET_DIR" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
  fi
}

echo "Repo root: $REPO_ROOT"
echo "Port: $PORT"
echo

if [ -f "$PID_FILE" ]; then
  OLD_PID=$(cat "$PID_FILE" 2>/dev/null || true)
  kill_pid "${OLD_PID:-}"
  rm -f "$PID_FILE"
fi

if [ -f "$REPO_ROOT/superflare" ]; then
  rm -f "$REPO_ROOT/superflare"
  echo "Removed $REPO_ROOT/superflare"
fi

for artifact in "$FNAPP_BINARY" "$FNAPP_PACKAGE"; do
  if [ -f "$artifact" ]; then
    rm -f "$artifact"
    echo "Removed $artifact"
  fi
done

if [ -d "$TMP_FNAPP_UNPACK_DIR" ]; then
  rm -rf "$TMP_FNAPP_UNPACK_DIR"
  echo "Removed $TMP_FNAPP_UNPACK_DIR"
fi

find "$REPO_ROOT" -maxdepth 1 -name 'tmp-superflare*' -exec rm -rf {} + 2>/dev/null || true

remove_children "$RUN_DIR"
remove_children "$CACHE_DIR"

echo
echo "Cleanup complete."
echo "Preserved runtime data files:"
echo "  config.yml"
echo "  apps.yml"
echo "  bookmarks.yml"
echo "  ports.yaml"
echo "  .env"
