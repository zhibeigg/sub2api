#!/usr/bin/env bash
# Poke API - Gemini CLI 一键配置脚本
# 用法: curl -fsSL https://docs.poke2api.com/install/gemini.sh | bash
set -euo pipefail

BASE_URL="https://www.poke2api.com"
CONSOLE_URL="https://www.poke2api.com"
MODEL="gemini-3-pro-preview"

BLUE='\033[0;34m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info(){ printf "${BLUE}[信息]${NC} %s\n" "$1"; }
ok(){   printf "${GREEN}[成功]${NC} %s\n" "$1"; }
warn(){ printf "${YELLOW}[提示]${NC} %s\n" "$1"; }
err(){  printf "${RED}[错误]${NC} %s\n" "$1" >&2; }
read_tty(){ if { exec 3</dev/tty; } 2>/dev/null; then read -r "$@" <&3; exec 3<&-; else read -r "$@"; fi; }

echo "======================================"
echo "    Poke API · Gemini CLI 一键配置"
echo "======================================"

# 1. 检测 Node.js
if ! command -v node >/dev/null 2>&1; then
  err "未检测到 Node.js。请先安装 Node.js LTS。"
  err "官网: https://nodejs.org   教程: https://docs.poke2api.com/guide/nodejs"
  exit 1
fi
info "Node.js 版本: $(node -v)"

# 2. 检测 / 安装 Gemini CLI
if ! command -v gemini >/dev/null 2>&1; then
  info "未检测到 Gemini CLI，正在通过 npm 安装..."
  if [ "$(id -u)" = "0" ]; then
    npm install -g @google/gemini-cli
  elif command -v sudo >/dev/null 2>&1; then
    sudo npm install -g @google/gemini-cli || npm install -g @google/gemini-cli
  else
    npm install -g @google/gemini-cli
  fi
fi
if command -v gemini >/dev/null 2>&1; then
  ok "Gemini CLI 已就绪: $(gemini --version 2>/dev/null || echo 已安装)"
else
  err "Gemini CLI 安装失败，请手动执行: npm install -g @google/gemini-cli"
  exit 1
fi

# 3. 输入 API Key
echo ""
printf "请输入你的 Poke API Key (在 %s 控制台获取): " "$CONSOLE_URL"
read_tty API_KEY
if [ -z "${API_KEY:-}" ]; then err "API Key 不能为空。"; exit 1; fi

# 4. 写入 Shell 配置
SHELL_RC="$HOME/.bashrc"
case "${SHELL:-}" in
  *zsh) SHELL_RC="$HOME/.zshrc";;
esac
touch "$SHELL_RC"

MB="# >>> pokeapi gemini-cli >>>"
ME="# <<< pokeapi gemini-cli <<<"
if grep -qF "$MB" "$SHELL_RC" 2>/dev/null; then
  awk -v b="$MB" -v e="$ME" 'BEGIN{s=0} $0==b{s=1} s==0{print} $0==e{s=0}' "$SHELL_RC" > "$SHELL_RC.tmp" && mv "$SHELL_RC.tmp" "$SHELL_RC"
fi
{
  echo "$MB"
  echo "export GOOGLE_GEMINI_BASE_URL=\"$BASE_URL\""
  echo "export GEMINI_API_KEY=\"$API_KEY\""
  echo "export GEMINI_MODEL=\"$MODEL\""
  echo "$ME"
} >> "$SHELL_RC"

ok "配置已写入: $SHELL_RC"
echo ""
echo "======================================"
ok "Gemini CLI 配置完成！"
echo "  Base URL : $BASE_URL"
echo "  模型     : $MODEL"
echo "  配置文件 : $SHELL_RC"
echo "======================================"
warn "请重新打开终端，或执行: source $SHELL_RC"
warn "然后运行: gemini"
