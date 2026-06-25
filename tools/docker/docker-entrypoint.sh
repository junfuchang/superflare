#!/bin/sh
set -eu

mkdir -p \
  /data/var/uploads \
  /data/var/cache \
  /data/var/tools \
  /data/var/lighthouse \
  /data/var/screens

cd /data
exec /opt/superflare/superflare "$@"
