#!/usr/bin/env bash
set -euo pipefail
PANEL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$PANEL_DIR"
panel_version="$(tr -d '\r\n' < VERSION)"
panel_commit="$(git rev-parse HEAD)"
panel_time="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
[[ "$panel_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'VERSION 必须是 x.y.z' >&2; exit 1; }
if [[ -n "$(git status --porcelain --untracked-files=normal)" ]]; then
  echo '发布前必须提交所有源码改动，以保证二进制版本对应确切提交。' >&2
  exit 1
fi
mkdir -p dist
panel_stage=""
cleanup_stage() {
  # long: 仅清理本次 mktemp 生成的打包目录，失败退出也不能积累完整源码副本。
  case "$panel_stage" in
    "$PANEL_DIR"/dist/.stage-*) [[ ! -d "$panel_stage" ]] || rm -rf -- "$panel_stage" ;;
  esac
}
trap cleanup_stage EXIT
for panel_arch in amd64 arm64; do
  panel_stage="$(mktemp -d "$PANEL_DIR/dist/.stage-$panel_arch.XXXXXX")"
  mkdir -p "$panel_stage/server-panel/bin"
  # long: 归档只纳入版本控制文件，禁止把本机配置、监控日志或旧二进制带入公开 Release。
  git archive HEAD | tar -x -C "$panel_stage/server-panel"
  CGO_ENABLED=0 GOOS=linux GOARCH="$panel_arch" go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=$panel_version -X main.commit=$panel_commit -X main.buildTime=$panel_time" \
    -o "$panel_stage/server-panel/bin/server-panel" .
  # long: macOS 构建时不能把 Finder 扩展属性带进 Linux 包，否则 GNU tar 解压会持续告警。
  COPYFILE_DISABLE=1 tar --no-xattrs -czf "dist/server-panel_${panel_version}_linux_${panel_arch}.tar.gz" -C "$panel_stage" server-panel
  cleanup_stage
  panel_stage=""
done
cd dist
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "server-panel_${panel_version}_linux_amd64.tar.gz" "server-panel_${panel_version}_linux_arm64.tar.gz" > SHA256SUMS
else
  shasum -a 256 "server-panel_${panel_version}_linux_amd64.tar.gz" "server-panel_${panel_version}_linux_arm64.tar.gz" > SHA256SUMS
fi
printf 'Release 产物已生成：dist/，版本 %s，提交 %s\n' "$panel_version" "$panel_commit"
