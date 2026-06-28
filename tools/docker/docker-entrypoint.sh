#!/bin/sh
set -eu

mkdir -p \
  /data/var/uploads \
  /data/var/cache \
  /data/var/tools \
  /data/var/lighthouse \
  /data/var/screens

for file in config.yml apps.yml bookmarks.yml ports.yaml; do
  if [ ! -f "/data/$file" ] && [ -f "/opt/superflare/defaults/$file" ]; then
    cp "/opt/superflare/defaults/$file" "/data/$file"
  fi
done

cd /data
exec /opt/superflare/superflare "$@"
