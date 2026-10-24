#!/bin/sh
set -eu

# Match a bind-mounted library owner without recursively changing host files.
app_uid="${PUID:-$(stat -c '%u' /app/library)}"
app_gid="${PGID:-$(stat -c '%g' /app/library)}"
case "$app_uid" in ''|*[!0-9]*|0) app_uid=10001 ;; esac
case "$app_gid" in ''|*[!0-9]*) app_gid=10001 ;; esac
mkdir -p /app/data
chown "$app_uid:$app_gid" /app/data

# Seed only an empty host directory, or resume an interrupted first copy.
# Keep the image source separate so a bind mount cannot hide it.
if [ -d /app/library-seed ] && { [ -f /app/library/.icon-svg-initializing ] || [ -z "$(find /app/library -mindepth 1 -maxdepth 1 -print -quit)" ]; }; then
  chown "$app_uid:$app_gid" /app/library
  su-exec "$app_uid:$app_gid" sh -c '
    touch /app/library/.icon-svg-initializing
    cd /app/library-seed
    find . -type f | while IFS= read -r file; do
      mkdir -p "/app/library/$(dirname "$file")"
      cp -n "$file" "/app/library/$file"
    done
    touch /app/library/.icon-svg-initialized
    rm /app/library/.icon-svg-initializing
  '
  echo "Initial icon library prepared in /app/library."
fi
exec su-exec "$app_uid:$app_gid" /app/icon-svg "$@"
