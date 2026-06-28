#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
RUN_DIR="$REPO_ROOT/var/run"
PID_FILE="$RUN_DIR/superflare.pid"
STDOUT_LOG="$RUN_DIR/superflare.stdout.log"
STDERR_LOG="$RUN_DIR/superflare.stderr.log"
PORT="${SUPERFLARE_PORT:-3636}"
ENABLE_LOGIN="${SUPERFLARE_ENABLE_LOGIN:-}"

generate_cookie_secret() {
  if [ -n "${SUPERFLARE_COOKIE_SECRET:-}" ]; then
    printf '%s\n' "$SUPERFLARE_COOKIE_SECRET"
    return 0
  fi
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return 0
  fi
  if [ -r /dev/urandom ] && command -v od >/dev/null 2>&1; then
    od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
    printf '\n'
    return 0
  fi
  printf '%s%s%s\n' "$(date +%s 2>/dev/null || printf 0)" "$$" "$PORT"
}

resolve_go() {
  if [ -n "${SUPERFLARE_GO_BIN:-}" ]; then
    if [ -x "$SUPERFLARE_GO_BIN" ]; then
      printf '%s\n' "$SUPERFLARE_GO_BIN"
      return 0
    fi
    if [ -x "$SUPERFLARE_GO_BIN/go" ]; then
      printf '%s\n' "$SUPERFLARE_GO_BIN/go"
      return 0
    fi
  fi

  if [ -x "$REPO_ROOT/.tools/go/bin/go" ]; then
    printf '%s\n' "$REPO_ROOT/.tools/go/bin/go"
    return 0
  fi

  WORKSPACE_ROOT=$(dirname "$REPO_ROOT")
  if [ -x "$WORKSPACE_ROOT/.tools/go/bin/go" ]; then
    printf '%s\n' "$WORKSPACE_ROOT/.tools/go/bin/go"
    return 0
  fi

  if command -v go >/dev/null 2>&1; then
    command -v go
    return 0
  fi

  echo "未找到 go。请设置 SUPERFLARE_GO_BIN，或将 go 加入 PATH。" >&2
  exit 1
}

ask_login() {
  if [ -n "$ENABLE_LOGIN" ]; then
    return 0
  fi
  printf '本次运行是否启用登录？[Y/n] '
  read -r INPUT || true
  case "${INPUT:-}" in
    ""|Y|y|YES|Yes|yes) ENABLE_LOGIN=true ;;
    N|n|NO|No|no) ENABLE_LOGIN=false ;;
    *)
      echo "请输入 Y 或 N。" >&2
      exit 1
      ;;
  esac
}

resolve_disable_login() {
  case "$ENABLE_LOGIN" in
    1|true|TRUE|True|yes|YES|Yes|on|ON|On) printf 'false\n' ;;
    0|false|FALSE|False|no|NO|No|off|OFF|Off) printf 'true\n' ;;
    *)
      echo "SUPERFLARE_ENABLE_LOGIN 的值无效: $ENABLE_LOGIN" >&2
      exit 1
      ;;
  esac
}

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
    echo "跳过 PID $TARGET_PID：该进程不属于当前 SuperFlare 可执行文件。"
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
    echo "已停止 PID $TARGET_PID"
  fi
}

stop_existing() {
  if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE" 2>/dev/null || true)
    kill_pid "${OLD_PID:-}"
    rm -f "$PID_FILE"
  fi

  if [ -f "$REPO_ROOT/superflare" ] && command -v pgrep >/dev/null 2>&1; then
    BIN_PATH="$REPO_ROOT/superflare"
    for pid in $(pgrep -f "$BIN_PATH" 2>/dev/null || true); do
      kill_pid "$pid"
    done
  fi
}

wait_ready() {
  TARGET_URL="http://127.0.0.1:$PORT/help"
  COUNT=0
  while [ "$COUNT" -lt 60 ]; do
    if command -v curl >/dev/null 2>&1; then
      if curl -fsS --max-time 2 "$TARGET_URL" >/dev/null 2>&1; then
        return 0
      fi
    elif command -v wget >/dev/null 2>&1; then
      if wget -q -T 2 -O /dev/null "$TARGET_URL" >/dev/null 2>&1; then
        return 0
      fi
    fi
    COUNT=$((COUNT + 1))
    sleep 1
  done
  return 1
}

mkdir -p "$RUN_DIR"
GO_BIN=$(resolve_go)
ask_login
DISABLE_LOGIN=$(resolve_disable_login)
COOKIE_SECRET=$(generate_cookie_secret)

if [ -z "${SUPERFLARE_COOKIE_SECRET:-}" ]; then
  echo "未设置 SUPERFLARE_COOKIE_SECRET，本次运行将使用临时随机 Cookie 密钥；重启后已登录会话会失效。"
fi

echo "仓库目录: $REPO_ROOT"
echo "Go: $GO_BIN"
echo "端口: $PORT"
echo "登录: $( [ "$DISABLE_LOGIN" = "true" ] && printf '关闭' || printf '启用' )"
echo

stop_existing

cd "$REPO_ROOT"

echo "生成嵌入资源..."
"$GO_BIN" run ./build/build.go

echo "运行测试..."
"$GO_BIN" test ./... -count=1

echo "构建 Linux 可执行文件..."
CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-amd64}" "$GO_BIN" build -o ./superflare .

rm -f "$STDOUT_LOG" "$STDERR_LOG"

echo "后台启动 SuperFlare..."
nohup env \
  FLARE_PORT="$PORT" \
  FLARE_COOKIE_SECRET="$COOKIE_SECRET" \
  FLARE_DISABLE_LOGIN="$DISABLE_LOGIN" \
  ./superflare --enable_editor \
  >"$STDOUT_LOG" 2>"$STDERR_LOG" &

NEW_PID=$!
printf '%s' "$NEW_PID" >"$PID_FILE"

if ! wait_ready; then
  echo
  echo "SuperFlare 在 60 秒内未就绪。" >&2
  echo "标准输出日志: $STDOUT_LOG" >&2
  echo "标准错误日志: $STDERR_LOG" >&2
  exit 1
fi

echo
echo "SuperFlare 已启动。"
echo "PID: $NEW_PID"
echo "首页:   http://127.0.0.1:$PORT/"
echo "帮助:   http://127.0.0.1:$PORT/help"
echo "编辑页: http://127.0.0.1:$PORT/editor"
echo "PID 文件: $PID_FILE"
echo "标准输出日志: $STDOUT_LOG"
echo "标准错误日志: $STDERR_LOG"
