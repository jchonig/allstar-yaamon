#!/bin/sh
set -e

PUID=${PUID:-1000}
PGID=${PGID:-1000}

if [ "$(id -u)" = "0" ]; then
    groupmod -o -g "$PGID" yaamon 2>/dev/null || true
    usermod -o -u "$PUID" yaamon 2>/dev/null || true
    chown -R yaamon:yaamon /var/lib/yaamon

    if [ -n "$YAAMON_STATE_FILE" ] && [ -f "$YAAMON_STATE_FILE" ]; then
        APPLY_FLAGS=""
        [ -n "$YAAMON_APPLY_RESET_PASSWORDS" ] && APPLY_FLAGS="--reset-passwords"
        echo "Applying state from $YAAMON_STATE_FILE..."
        su-exec yaamon yaamon apply $APPLY_FLAGS "$YAAMON_STATE_FILE"
    fi

    exec su-exec yaamon "$@"
fi

if [ -n "$YAAMON_STATE_FILE" ] && [ -f "$YAAMON_STATE_FILE" ]; then
    APPLY_FLAGS=""
    [ -n "$YAAMON_APPLY_RESET_PASSWORDS" ] && APPLY_FLAGS="--reset-passwords"
    echo "Applying state from $YAAMON_STATE_FILE..."
    yaamon apply $APPLY_FLAGS "$YAAMON_STATE_FILE"
fi

exec "$@"
