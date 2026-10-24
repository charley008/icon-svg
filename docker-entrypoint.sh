#!/bin/sh
set -eu

# Match a bind-mounted library owner without recursively changing host files.
app_uid="${PUID:-$(stat -c '%u' /app/library)}"
app_gid="${PGID:-$(stat -c '%g' /app/library)}"
case "$app_uid" in ''|*[!0-9]*|0) app_uid=10001 ;; esac
case "$app_gid" in ''|*[!0-9]*) app_gid=10001 ;; esac
mkdir -p /app/data
chown "$app_uid:$app_gid" /app/data
exec su-exec "$app_uid:$app_gid" /app/icon-svg "$@"
