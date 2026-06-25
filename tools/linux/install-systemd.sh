#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
TARGET_DIR="${SUPERFLARE_INSTALL_DIR:-/opt/superflare}"
SERVICE_NAME="${SUPERFLARE_SERVICE_NAME:-superflare}"
SERVICE_PATH="/etc/systemd/system/${SERVICE_NAME}.service"
PORT="${SUPERFLARE_PORT:-3636}"
COOKIE_SECRET="${SUPERFLARE_COOKIE_SECRET:-superflare-production-secret}"
ENABLE_LOGIN="${SUPERFLARE_ENABLE_LOGIN:-true}"
ENV_FILE=""

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 执行此脚本。" >&2
  exit 1
fi

case "$ENABLE_LOGIN" in
  1|true|TRUE|True|yes|YES|Yes|on|ON|On) DISABLE_LOGIN=false ;;
  0|false|FALSE|False|no|NO|No|off|OFF|Off) DISABLE_LOGIN=true ;;
  *)
    echo "SUPERFLARE_ENABLE_LOGIN 的值无效: $ENABLE_LOGIN" >&2
    exit 1
    ;;
esac

upsert_env_value() {
  KEY="$1"
  VALUE="$2"
  TMP_FILE=$(mktemp)
  if [ -f "$ENV_FILE" ]; then
    grep -v "^${KEY}=" "$ENV_FILE" >"$TMP_FILE" || true
  fi
  printf '%s=%s\n' "$KEY" "$VALUE" >>"$TMP_FILE"
  mv "$TMP_FILE" "$ENV_FILE"
}

if [ ! -x "$REPO_ROOT/superflare" ]; then
  echo "未找到已构建的 superflare 可执行文件，请先执行 ./tools/linux/build-superflare.sh" >&2
  exit 1
fi

mkdir -p "$TARGET_DIR"
mkdir -p "$TARGET_DIR/var/run" "$TARGET_DIR/var/cache" "$TARGET_DIR/var/uploads" "$TARGET_DIR/var/tools" "$TARGET_DIR/var/lighthouse" "$TARGET_DIR/var/screens"
ENV_FILE="$TARGET_DIR/.env"

cp "$REPO_ROOT/superflare" "$TARGET_DIR/superflare"
chmod +x "$TARGET_DIR/superflare"

for file in config.yml apps.yml bookmarks.yml ports.yaml .env; do
  if [ -f "$REPO_ROOT/$file" ] && [ ! -f "$TARGET_DIR/$file" ]; then
    cp "$REPO_ROOT/$file" "$TARGET_DIR/$file"
  fi
done

touch "$ENV_FILE"
upsert_env_value FLARE_PORT "$PORT"
upsert_env_value FLARE_DISABLE_LOGIN "$DISABLE_LOGIN"
upsert_env_value FLARE_COOKIE_SECRET "$COOKIE_SECRET"
upsert_env_value FLARE_EDITOR "true"

cat >"$SERVICE_PATH" <<EOF
[Unit]
Description=SuperFlare
After=network.target

[Service]
Type=simple
WorkingDirectory=$TARGET_DIR
ExecStart=$TARGET_DIR/superflare --enable_editor
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

echo "systemd 服务已安装。"
echo "服务名: $SERVICE_NAME"
echo "工作目录: $TARGET_DIR"
echo "配置文件: $ENV_FILE"
echo "首页: http://127.0.0.1:$PORT/"
