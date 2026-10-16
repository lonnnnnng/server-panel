#!/usr/bin/env bash
set -euo pipefail

PANEL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
PANEL_UNIT="server-panel.service"
PANEL_CONFIG="$PANEL_DIR/config/server-panel.env"

usage() {
  printf '%s\n' \
    '用法：sudo ./scripts/service.sh install --user <普通用户>' \
    '      sudo ./scripts/service.sh start|stop|restart|status|logs' \
    '      sudo ./scripts/service.sh enable|disable|uninstall' \
    '      ./scripts/service.sh health' \
    'install 安装服务并立即启动、启用开机自启；uninstall 仅移除服务注册，不删除代码和配置。'
}

require_root() {
  if [[ "$EUID" -ne 0 ]]; then
    printf '此操作需要 sudo：sudo %s %s\n' "$0" "${1:-}" >&2
    exit 1
  fi
}

health() {
  local panel_port=8888 panel_host=127.0.0.1
  # long: 配置由普通用户维护，root 检查服务时只能读取值，不能 source 执行其中的 shell。
  if [[ -f "$PANEL_CONFIG" ]]; then
    panel_port="$(awk -F= '$1=="PORT" {value=$2} END {print value}' "$PANEL_CONFIG")"
    panel_host="$(awk -F= '$1=="HOST" {value=$2} END {print value}' "$PANEL_CONFIG")"
    panel_port="${panel_port:-8888}"
    panel_host="${panel_host:-127.0.0.1}"
  fi
  [[ "$panel_port" =~ ^[0-9]+$ ]] || { echo 'PORT 必须为整数' >&2; return 1; }
  case "$panel_host" in
    0.0.0.0) panel_host=127.0.0.1 ;;
    ::) panel_host='[::1]' ;;
    *:*) panel_host="[$panel_host]" ;;
  esac
  # long: health 在 if 中调用时 Bash 不触发 errexit，必须显式返回失败，防止换行输出掩盖 curl 错误。
  curl --noproxy '*' -fsS --connect-timeout 3 --max-time 15 "http://$panel_host:$panel_port/api/health" || return 1
  printf '\n'
}

wait_ready() {
  local attempt
  for attempt in {1..30}; do
    if health >/dev/null 2>&1; then
      printf '服务已就绪：server-panel.service\n'
      return 0
    fi
    if ! systemctl is-active --quiet "$PANEL_UNIT"; then
      sleep 1
      continue
    fi
    sleep 1
  done
  echo '服务健康检查未通过，请运行 sudo ./scripts/service.sh logs 查看原因。' >&2
  return 1
}

install_service() {
  require_root install
  local panel_user="${SUDO_USER:-}" panel_group unit_tmp panel_tool
  # long: 先确认健康检查和服务注册工具齐全，避免安装到一半才发现无法验证或启动。
  for panel_tool in systemctl curl awk sed install mktemp id; do
    command -v "$panel_tool" >/dev/null 2>&1 || { printf '缺少必要工具：%s\n' "$panel_tool" >&2; exit 1; }
  done
  if [[ "${1:-}" == --user && "$#" -eq 2 ]]; then
    panel_user="$2"
  elif [[ "$#" -ne 0 ]]; then
    usage >&2
    exit 2
  fi
  [[ "$panel_user" =~ ^[a-zA-Z_][a-zA-Z0-9_-]*\$?$ ]] || { echo '请通过 --user 指定已有普通用户' >&2; exit 1; }
  [[ "$(id -u "$panel_user")" -ne 0 ]] || { echo '服务不应使用 root 用户运行' >&2; exit 1; }
  panel_group="$(id -gn "$panel_user")"
  [[ "$panel_group" =~ ^[a-zA-Z_][a-zA-Z0-9_-]*\$?$ ]] || { echo '不支持的用户组名' >&2; exit 1; }
  # long: systemd 路径具有独立转义规则，限制安装路径，避免生成无法启动或含歧义的单元文件。
  [[ "$PANEL_DIR" =~ ^/[a-zA-Z0-9_./-]+$ ]] || { echo '安装路径不能包含空格或特殊字符' >&2; exit 1; }
  [[ -x "$PANEL_DIR/bin/server-panel" ]] || { echo '缺少 bin/server-panel，请先下载 Release 或构建。' >&2; exit 1; }
  if [[ ! -f "$PANEL_CONFIG" ]]; then
    install -m 640 -o "$panel_user" -g "$panel_group" "$PANEL_DIR/config/server-panel.env.example" "$PANEL_CONFIG"
  fi
  unit_tmp="$(mktemp /tmp/server-panel-unit.XXXXXX)"
  sed -e "s|@USER@|$panel_user|g" -e "s|@GROUP@|$panel_group|g" \
    -e "s|@INSTALL_DIR@|$PANEL_DIR|g" "$PANEL_DIR/deploy/server-panel.service.in" > "$unit_tmp"
  install -m 644 "$unit_tmp" /etc/systemd/system/server-panel.service
  rm -f -- "$unit_tmp"
  systemctl daemon-reload
  systemctl enable "$PANEL_UNIT"
  systemctl restart "$PANEL_UNIT"
  wait_ready
}

case "${1:-help}" in
  install) shift; install_service "$@" ;;
  start|restart)
    require_root "$1"
    systemctl "$1" "$PANEL_UNIT"
    wait_ready
    ;;
  stop|enable|disable)
    require_root "$1"
    systemctl "$1" "$PANEL_UNIT"
    ;;
  status)
    systemctl status "$PANEL_UNIT" --no-pager -l
    printf '\n开机自启：'
    systemctl is-enabled "$PANEL_UNIT"
    ;;
  logs) journalctl -u "$PANEL_UNIT" -n 100 --no-pager ;;
  health) health ;;
  uninstall)
    require_root uninstall
    systemctl disable --now "$PANEL_UNIT"
    rm -f -- /etc/systemd/system/server-panel.service
    systemctl daemon-reload
    printf '已移除 systemd 服务；项目目录和配置保持不变。\n'
    ;;
  help|-h|--help) usage ;;
  *) usage >&2; exit 2 ;;
esac
