#!/usr/bin/env bash
set -euo pipefail
PANEL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$PANEL_DIR"
panel_version="$(tr -d '\r\n' < VERSION)"
panel_commit="$(git rev-parse HEAD 2>/dev/null || printf unknown)"
panel_time="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
mkdir -p bin
# long: 禁用 CGO，让 Release 不依赖构建机上的 libc 版本，便于复制到不同 Linux 服务器。
CGO_ENABLED=0 go build -trimpath -buildvcs=false \
  -ldflags "-s -w -X main.version=$panel_version -X main.commit=$panel_commit -X main.buildTime=$panel_time" \
  -o bin/server-panel .
printf '已构建 bin/server-panel（%s）\n' "$panel_version"
