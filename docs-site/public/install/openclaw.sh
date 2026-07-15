#!/usr/bin/env bash
# Poke API - OpenClaw 一键配置脚本
# 用法: curl -fsSL https://docs.poke2api.com/install/openclaw.sh | bash
set -euo pipefail

CONSOLE_URL="https://www.poke2api.com"
CLAUDE_URL="https://www.poke2api.com"
CODEX_URL="https://www.poke2api.com/v1"
CLAUDE_MODEL="claude-opus-4-6"
CODEX_MODEL="gpt-5.5"

BLUE='\033[0;34m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info(){ printf "${BLUE}[信息]${NC} %s\n" "$1"; }
ok(){   printf "${GREEN}[成功]${NC} %s\n" "$1"; }
warn(){ printf "${YELLOW}[提示]${NC} %s\n" "$1"; }
err(){  printf "${RED}[错误]${NC} %s\n" "$1" >&2; }
read_tty(){ if { exec 3</dev/tty; } 2>/dev/null; then read -r "$@" <&3; exec 3<&-; else read -r "$@"; fi; }

echo "======================================"
echo "     Poke API · OpenClaw 一键配置"
echo "======================================"

# 1. 检测 Node.js
if ! command -v node >/dev/null 2>&1; then
  err "未检测到 Node.js。请先安装 Node.js LTS。"
  err "官网: https://nodejs.org   教程: https://docs.poke2api.com/guide/nodejs"
  exit 1
fi
info "Node.js 版本: $(node -v)"

# 2. 检测 / 安装 OpenClaw
if ! command -v openclaw >/dev/null 2>&1; then
  info "未检测到 OpenClaw，正在通过 npm 安装..."
  if [ "$(id -u)" = "0" ]; then
    npm install -g @openclaw/cli
  elif command -v sudo >/dev/null 2>&1; then
    sudo npm install -g @openclaw/cli || npm install -g @openclaw/cli
  else
    npm install -g @openclaw/cli
  fi
fi
command -v openclaw >/dev/null 2>&1 && ok "OpenClaw 已就绪" || { err "OpenClaw 安装失败，请手动执行: npm install -g @openclaw/cli"; exit 1; }

# 3. 选择通道
echo ""
echo "请选择接入通道:"
echo "  1) Anthropic (Claude)  - 模型 $CLAUDE_MODEL"
echo "  2) OpenAI (Codex)      - 模型 $CODEX_MODEL"
printf "输入序号 [1/2]: "
read_tty CH
case "$CH" in
  2) COMPAT="openai";    B_URL="$CODEX_URL";  MODEL="$CODEX_MODEL";  KEY_ENV="OPENAI_API_KEY";;
  *) COMPAT="anthropic"; B_URL="$CLAUDE_URL"; MODEL="$CLAUDE_MODEL"; KEY_ENV="ANTHROPIC_API_KEY";;
esac

# 4. 输入 API Key
printf "请输入你的 Poke API Key (在 %s 控制台获取): " "$CONSOLE_URL"
read_tty API_KEY
if [ -z "${API_KEY:-}" ]; then err "API Key 不能为空。"; exit 1; fi

# 5. 执行 onboard
export "$KEY_ENV"="$API_KEY"
info "正在写入 OpenClaw 配置..."
openclaw onboard --auth-choice custom-api-key \
  --custom-base-url "$B_URL" \
  --custom-api-key-env "$KEY_ENV" \
  --custom-compatibility "$COMPAT" \
  --custom-model "$MODEL"

echo ""
echo "======================================"
ok "OpenClaw 配置完成！"
echo "  通道     : $COMPAT"
echo "  Base URL : $B_URL"
echo "  模型     : $MODEL"
echo "======================================"
warn "运行命令: openclaw"
