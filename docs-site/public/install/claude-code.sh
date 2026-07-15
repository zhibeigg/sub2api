#!/usr/bin/env bash
# Poke API - Claude Code 一键配置脚本
# 用法: curl -fsSL https://docs.poke2api.com/install/claude-code.sh | bash
set -euo pipefail

BASE_URL="https://www.poke2api.com"
CONSOLE_URL="https://www.poke2api.com"
OFFICIAL_INSTALLER_URL="https://claude.ai/install.sh"

BLUE='\033[0;34m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info(){ printf '%b[信息]%b %s\n' "$BLUE" "$NC" "$1"; }
ok(){ printf '%b[成功]%b %s\n' "$GREEN" "$NC" "$1"; }
warn(){ printf '%b[提示]%b %s\n' "$YELLOW" "$NC" "$1"; }
err(){ printf '%b[错误]%b %s\n' "$RED" "$NC" "$1" >&2; }

read_secret_from_tty() {
  local prompt="$1"
  local value
  if [ ! -r /dev/tty ]; then
    err "当前环境没有可交互终端，无法安全读取 API Key。"
    exit 1
  fi
  printf '%s' "$prompt" > /dev/tty
  IFS= read -r -s value < /dev/tty
  printf '\n' > /dev/tty
  printf '%s' "$value"
}

printf '%s\n' "======================================"
printf '%s\n' "   Poke API · Claude Code 一键配置"
printf '%s\n' "======================================"

if ! command -v claude >/dev/null 2>&1; then
  if ! command -v curl >/dev/null 2>&1; then
    err "未检测到 curl，无法运行 Anthropic 官方安装器。"
    exit 1
  fi
  info "未检测到 Claude Code，正在运行 Anthropic 官方原生安装器..."
  curl -fsSL "$OFFICIAL_INSTALLER_URL" | bash
  export PATH="$HOME/.local/bin:$PATH"
  hash -r
fi

if ! command -v claude >/dev/null 2>&1; then
  err "官方安装器已执行，但当前 Shell 尚未找到 claude。"
  err "请重新打开终端后再次运行本脚本。"
  exit 1
fi
ok "Claude Code 已就绪: $(claude --version 2>/dev/null || printf '已安装')"

api_key="$(read_secret_from_tty "请输入 Poke API Key（从 ${CONSOLE_URL} 获取，输入不会回显）: ")"
if [ -z "$api_key" ]; then
  err "API Key 不能为空。"
  exit 1
fi

SHELL_RC="$HOME/.bashrc"
case "${SHELL:-}" in
  *zsh) SHELL_RC="$HOME/.zshrc" ;;
esac
touch "$SHELL_RC"

MARKER_BEGIN="# >>> pokeapi claude-code >>>"
MARKER_END="# <<< pokeapi claude-code <<<"
if grep -qF "$MARKER_BEGIN" "$SHELL_RC" 2>/dev/null; then
  awk -v begin="$MARKER_BEGIN" -v end="$MARKER_END" '
    $0 == begin { skipping = 1; next }
    $0 == end { skipping = 0; next }
    !skipping { print }
  ' "$SHELL_RC" > "$SHELL_RC.tmp"
  mv "$SHELL_RC.tmp" "$SHELL_RC"
fi

{
  printf '%s\n' "$MARKER_BEGIN"
  printf 'export ANTHROPIC_BASE_URL="%s"\n' "$BASE_URL"
  printf 'export ANTHROPIC_AUTH_TOKEN="%s"\n' "${api_key//\"/\\\"}"
  printf '%s\n' 'export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"'
  printf '%s\n' "$MARKER_END"
} >> "$SHELL_RC"
chmod 600 "$SHELL_RC" 2>/dev/null || true

export ANTHROPIC_BASE_URL="$BASE_URL"
export ANTHROPIC_AUTH_TOKEN="$api_key"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"
unset api_key

printf '\n%s\n' "======================================"
ok "Claude Code 配置完成"
printf '  Base URL : %s\n' "$BASE_URL"
printf '  配置文件 : %s\n' "$SHELL_RC"
printf '%s\n' '  认证变量 : ANTHROPIC_AUTH_TOKEN（值未显示）'
printf '%s\n' "======================================"
warn "重新打开终端，或执行: source $SHELL_RC"
warn "进入项目目录运行: claude；诊断命令: claude doctor"
