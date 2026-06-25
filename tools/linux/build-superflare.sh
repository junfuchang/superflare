#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)

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

GO_BIN=$(resolve_go)

echo "仓库目录: $REPO_ROOT"
echo "Go: $GO_BIN"

cd "$REPO_ROOT"

echo "生成嵌入资源..."
"$GO_BIN" run ./build/build.go

echo "运行测试..."
"$GO_BIN" test ./... -count=1

echo "构建 Linux 可执行文件..."
CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-amd64}" "$GO_BIN" build -o ./superflare .

echo
echo "构建完成: $REPO_ROOT/superflare"
