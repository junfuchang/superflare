#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)

PORT="${SUPERFLARE_PORT:-3636}"
COOKIE_SECRET="${SUPERFLARE_COOKIE_SECRET:-superflare-local-secret}"
ENABLE_LOGIN="${SUPERFLARE_ENABLE_LOGIN:-}"

if [ -z "$ENABLE_LOGIN" ]; then
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
fi

case "$ENABLE_LOGIN" in
  1|true|TRUE|True|yes|YES|Yes|on|ON|On) DISABLE_LOGIN=false ;;
  0|false|FALSE|False|no|NO|No|off|OFF|Off) DISABLE_LOGIN=true ;;
  *)
    echo "SUPERFLARE_ENABLE_LOGIN 的值无效: $ENABLE_LOGIN" >&2
    exit 1
    ;;
esac

cd "$REPO_ROOT"

if [ ! -x ./superflare ]; then
  echo "未找到 ./superflare，请先执行 tools/linux/build-superflare.sh" >&2
  exit 1
fi

echo "仓库目录: $REPO_ROOT"
echo "端口: $PORT"
echo "登录: $( [ "$DISABLE_LOGIN" = "true" ] && printf '关闭' || printf '启用' )"
echo
echo "前台运行 SuperFlare..."

exec env \
  FLARE_PORT="$PORT" \
  FLARE_COOKIE_SECRET="$COOKIE_SECRET" \
  FLARE_DISABLE_LOGIN="$DISABLE_LOGIN" \
  ./superflare --enable_editor
