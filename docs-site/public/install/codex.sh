#!/usr/bin/env bash
# Poke API - Codex (OpenAI) 一键配置脚本
# 用法: curl -fsSL https://docs.poke2api.com/install/codex.sh | bash
set -euo pipefail

BASE_URL="https://www.poke2api.com/v1"
CONSOLE_URL="https://www.poke2api.com"
MODEL="gpt-5.5"

BLUE='\033[0;34m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info(){ printf "${BLUE}[信息]${NC} %s\n" "$1"; }
ok(){   printf "${GREEN}[成功]${NC} %s\n" "$1"; }
warn(){ printf "${YELLOW}[提示]${NC} %s\n" "$1"; }
err(){  printf "${RED}[错误]${NC} %s\n" "$1" >&2; }
read_tty(){ if { exec 3</dev/tty; } 2>/dev/null; then read -r "$@" <&3; exec 3<&-; else read -r "$@"; fi; }

echo "======================================"
echo "     Poke API · Codex 一键配置"
echo "======================================"

# 1. 检测 Node.js
if ! command -v node >/dev/null 2>&1; then
  err "未检测到 Node.js。请先安装 Node.js LTS。"
  err "官网: https://nodejs.org   教程: https://docs.poke2api.com/guide/nodejs"
  exit 1
fi
info "Node.js 版本: $(node -v)"

# 2. 检测 / 安装 Codex
if ! command -v codex >/dev/null 2>&1; then
  info "未检测到 Codex，正在通过 npm 安装..."
  if [ "$(id -u)" = "0" ]; then
    npm install -g @openai/codex@latest
  elif command -v sudo >/dev/null 2>&1; then
    sudo npm install -g @openai/codex@latest || npm install -g @openai/codex@latest
  else
    npm install -g @openai/codex@latest
  fi
fi
if command -v codex >/dev/null 2>&1; then
  ok "Codex 已就绪: $(codex --version 2>/dev/null || echo 已安装)"
else
  err "Codex 安装失败，请手动执行: npm install -g @openai/codex@latest"
  exit 1
fi

# 3. 输入 API Key
echo ""
printf "请输入你的 Poke API Key (在 %s 控制台获取): " "$CONSOLE_URL"
read_tty API_KEY
if [ -z "${API_KEY:-}" ]; then err "API Key 不能为空。"; exit 1; fi

# 4. 写入 ~/.codex 配置
mkdir -p "$HOME/.codex"
cat > "$HOME/.codex/config.toml" <<EOF
model_provider = "codex"
model = "$MODEL"
review_model = "$MODEL"
model_reasoning_effort = "high"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 270000
model_auto_compact_token_limit = 270000
effective_context_window_percent = 95

[model_providers.codex]
name = "codex"
base_url = "$BASE_URL"
wire_api = "responses"
requires_openai_auth = true
EOF

cat > "$HOME/.codex/auth.json" <<EOF
{
  "OPENAI_API_KEY": "$API_KEY"
}
EOF

ok "配置已写入: $HOME/.codex/"
echo ""
echo "======================================"
ok "Codex 配置完成！"
echo "  Base URL : $BASE_URL"
echo "  模型     : $MODEL"
echo "  配置目录 : $HOME/.codex"
echo "======================================"
warn "已打开的 Codex 需重启后生效。进入项目目录运行: codex"
