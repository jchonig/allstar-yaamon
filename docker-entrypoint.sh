#!/bin/sh
set -e

if [ -n "$YAAMON_STATE_FILE" ] && [ -f "$YAAMON_STATE_FILE" ]; then
    echo "Applying state from $YAAMON_STATE_FILE..."
    RESET_FLAG=""
    [ -n "$YAAMON_RESET_PASSWORDS" ] && RESET_FLAG="--reset-passwords"
    /usr/local/bin/yaamon apply $RESET_FLAG "$YAAMON_STATE_FILE"
fi

exec "$@"
