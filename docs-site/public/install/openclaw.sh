#!/usr/bin/env bash
# Poke API - OpenClaw 一键配置脚本
# 用法: curl -fsSL https://docs.poke2api.com/install/openclaw.sh | bash
set -euo pipefail

CONSOLE_URL="https://www.poke2api.com"
CLAUDE_URL="https://www.poke2api.com"
CODEX_URL="https://www.poke2api.com/v1"
CLAUDE_MODEL="claude-opus-4-6"
CODEX_MODEL="gpt-5.5"

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info() { printf '%b[信息]%b %s\n' "$BLUE" "$NC" "$1"; }
ok() { printf '%b[成功]%b %s\n' "$GREEN" "$NC" "$1"; }
warn() { printf '%b[提示]%b %s\n' "$YELLOW" "$NC" "$1"; }
err() { printf '%b[错误]%b %s\n' "$RED" "$NC" "$1" >&2; }

read_from_tty() {
  local prompt="$1"
  local value
  if [ ! -r /dev/tty ]; then
    err "当前环境没有可交互终端，无法安全读取输入。"
    exit 1
  fi
  printf '%s' "$prompt" > /dev/tty
  IFS= read -r value < /dev/tty
  printf '%s' "$value"
}

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
printf '%s\n' "     Poke API · OpenClaw 一键配置"
printf '%s\n' "======================================"

if ! command -v node >/dev/null 2>&1 || ! command -v npm >/dev/null 2>&1; then
  err "未检测到 Node.js 与 npm，请先安装 Node.js LTS。"
  err "教程: https://docs.poke2api.com/guide/nodejs"
  exit 1
fi
info "Node.js 版本: $(node --version)"

if ! command -v openclaw >/dev/null 2>&1; then
  info "未检测到 OpenClaw，正在安装 npm 包 openclaw..."
  if ! npm install --global openclaw; then
    if command -v sudo >/dev/null 2>&1; then
      warn "当前用户无法写入 npm 全局目录，改用 sudo 重试。"
      sudo npm install --global openclaw
    else
      err "OpenClaw 安装失败，请检查 npm 全局目录权限。"
      exit 1
    fi
  fi
  hash -r
fi

if ! command -v openclaw >/dev/null 2>&1; then
  err "未找到 openclaw 命令，请重新打开终端后再运行本脚本。"
  exit 1
fi
ok "OpenClaw 已就绪: $(openclaw --version 2>/dev/null || printf '版本命令不可用')"

printf '\n请选择接入通道:\n'
printf '  1) Anthropic (Claude)  - %s\n' "$CLAUDE_MODEL"
printf '  2) OpenAI (Codex)      - %s\n' "$CODEX_MODEL"
choice="$(read_from_tty '输入序号 [1/2，默认 1]: ')"

case "$choice" in
  2)
    channel_name="OpenAI (Codex)"
    compatibility="openai-responses"
    base_url="$CODEX_URL"
    model_id="$CODEX_MODEL"
    provider_id="pokeapi-codex"
    ;;
  1|'')
    channel_name="Anthropic (Claude)"
    compatibility="anthropic"
    base_url="$CLAUDE_URL"
    model_id="$CLAUDE_MODEL"
    provider_id="pokeapi-claude"
    ;;
  *)
    err "无效选项: $choice"
    exit 1
    ;;
esac

api_key="$(read_secret_from_tty "请输入 Poke API Key（从 ${CONSOLE_URL} 获取，输入不会回显）: ")"
if [ -z "$api_key" ]; then
  err "API Key 不能为空。"
  exit 1
fi

info "正在写入 OpenClaw 本地配置（Gateway 仅绑定 loopback）..."
openclaw onboard \
  --non-interactive \
  --accept-risk \
  --mode local \
  --auth-choice custom-api-key \
  --custom-base-url "$base_url" \
  --custom-model-id "$model_id" \
  --custom-api-key "$api_key" \
  --custom-provider-id "$provider_id" \
  --custom-compatibility "$compatibility" \
  --custom-image-input \
  --gateway-bind loopback \
  --no-install-daemon \
  --skip-health \
  --skip-channels \
  --skip-skills \
  --skip-search \
  --skip-ui \
  --skip-hooks
unset api_key

printf '\n%s\n' "======================================"
ok "OpenClaw 配置完成"
printf '  通道       : %s\n' "$channel_name"
printf '  Provider ID: %s\n' "$provider_id"
printf '  Base URL   : %s\n' "$base_url"
printf '  模型       : %s\n' "$model_id"
printf '  Gateway    : loopback\n'
printf '%s\n' "======================================"
warn "启动 Gateway: openclaw gateway run"
warn "另开终端启动 TUI: openclaw tui"
warn "验证命令: openclaw gateway status --deep && openclaw models status"
