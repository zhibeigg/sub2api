#!/usr/bin/env bash
# Poke API - Gemini CLI 一键配置脚本
# 用法: curl -fsSL https://docs.poke2api.com/install/gemini.sh | bash
set -euo pipefail

BASE_URL="https://www.poke2api.com"
CONSOLE_URL="https://www.poke2api.com"
MODEL="gemini-3-pro-preview"

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
printf '%s\n' "    Poke API · Gemini CLI 一键配置"
printf '%s\n' "======================================"

if ! command -v node >/dev/null 2>&1 || ! command -v npm >/dev/null 2>&1; then
  err "未检测到 Node.js 与 npm。请先完成 https://docs.poke2api.com/guide/nodejs"
  exit 1
fi
info "Node.js 版本: $(node --version)"

if ! command -v gemini >/dev/null 2>&1; then
  info "未检测到 Gemini CLI，正在安装 @google/gemini-cli@latest..."
  if ! npm install --global @google/gemini-cli@latest; then
    if command -v sudo >/dev/null 2>&1; then
      sudo npm install --global @google/gemini-cli@latest
    else
      err "Gemini CLI 安装失败，请检查 npm 全局目录权限。"
      exit 1
    fi
  fi
  hash -r
fi
if ! command -v gemini >/dev/null 2>&1; then
  err "未找到 gemini 命令，请重新打开终端后再次运行本脚本。"
  exit 1
fi
ok "Gemini CLI 已就绪: $(gemini --version 2>/dev/null || printf '已安装')"

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
MARKER_BEGIN="# >>> pokeapi gemini-cli >>>"
MARKER_END="# <<< pokeapi gemini-cli <<<"
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
  printf 'export GOOGLE_GEMINI_BASE_URL="%s"\n' "$BASE_URL"
  printf 'export GEMINI_API_KEY="%s"\n' "${api_key//\"/\\\"}"
  printf 'export GEMINI_MODEL="%s"\n' "$MODEL"
  printf '%s\n' "$MARKER_END"
} >> "$SHELL_RC"
chmod 600 "$SHELL_RC" 2>/dev/null || true

export GOOGLE_GEMINI_BASE_URL="$BASE_URL"
export GEMINI_API_KEY="$api_key"
export GEMINI_MODEL="$MODEL"
unset api_key

printf '\n%s\n' "======================================"
ok "Gemini CLI 配置完成"
printf '  Base URL : %s\n' "$BASE_URL"
printf '  模型     : %s\n' "$MODEL"
printf '  配置文件 : %s\n' "$SHELL_RC"
printf '%s\n' '  认证变量 : GEMINI_API_KEY（值未显示）'
printf '%s\n' "======================================"
warn "重新打开终端，或执行: source $SHELL_RC"
warn "运行: gemini；首次认证请选择 Use Gemini API key"
