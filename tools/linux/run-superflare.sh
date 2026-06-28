#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)

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
COOKIE_SECRET=$(generate_cookie_secret)

if [ -z "${SUPERFLARE_COOKIE_SECRET:-}" ]; then
  echo "未设置 SUPERFLARE_COOKIE_SECRET，本次运行将使用临时随机 Cookie 密钥；重启后已登录会话会失效。"
fi

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
