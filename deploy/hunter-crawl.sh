#!/bin/bash
set -euo pipefail

export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"

PROJECT_DIR="$HOME/src/github.com/andreabreu76/harley-hunter"
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
CHROME_PROFILE="$HOME/Library/Application Support/harley-hunter-chrome"
DEVTOOLS_PORT=9222

if ! curl -sf -o /dev/null "http://127.0.0.1:${DEVTOOLS_PORT}/json/version"; then
  "$CHROME" \
    --remote-debugging-port="${DEVTOOLS_PORT}" \
    --user-data-dir="$CHROME_PROFILE" \
    --no-first-run \
    --no-default-browser-check \
    --window-position=-32000,-32000 \
    --window-size=1280,900 \
    about:blank >/dev/null 2>&1 &

  for _ in $(seq 1 30); do
    curl -sf -o /dev/null "http://127.0.0.1:${DEVTOOLS_PORT}/json/version" && break
    sleep 1
  done
fi

cd "$PROJECT_DIR"
echo "--- $(date '+%Y-%m-%d %H:%M:%S') ---"
exec "$HOME/bin/hunter" -config "$PROJECT_DIR/config/config.yaml" crawl
