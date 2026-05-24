#!/bin/sh
set -e

if [ -n "$YAAMON_STATE_FILE" ] && [ -f "$YAAMON_STATE_FILE" ]; then
    echo "Applying state from $YAAMON_STATE_FILE..."
    /usr/local/bin/yaamon apply --state "$YAAMON_STATE_FILE" --no-confirm
fi

exec "$@"
