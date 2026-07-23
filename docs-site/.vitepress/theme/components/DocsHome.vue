<script setup>
import { onBeforeUnmount, ref } from "vue"

const copied = ref("")
let resetTimer = 0

const models = ["Claude", "OpenAI", "Gemini", "Grok"]

const tools = [
  { index: "01", name: "Claude Code", protocol: "Anthropic", href: "/guide/claude-code" },
  { index: "02", name: "Codex", protocol: "OpenAI", href: "/guide/codex" },
  { index: "03", name: "Gemini CLI", protocol: "OpenAI-compatible", href: "/guide/gemini-cli" },
  { index: "04", name: "TRAE SOLO", protocol: "Claude / Codex", href: "/guide/trae-solo" },
  { index: "05", name: "OpenClaw", protocol: "Claude / Codex", href: "/guide/openclaw" },
  { index: "06", name: "Hermes", protocol: "Claude / OpenAI", href: "/guide/hermes" }
]

const capabilities = [
  ["01", "一个 Key", "统一管理 Claude、OpenAI、Gemini 等模型访问，不再维护多套凭据。"],
  ["02", "官方协议兼容", "支持 OpenAI Responses、Chat Completions、Anthropic Messages 与 Gemini 调用方式。"],
  ["03", "多模态", "覆盖文本对话、图片理解、文生图与图片编辑。"],
  ["04", "SSE 流式输出", "全接口支持流式返回，保持与官方接口一致的交互节奏。"],
  ["05", "账号池运营", "管理员可按 OpenAI K12、Free、Plus、Pro 或未设置套餐筛选账号，并保持分页、批量操作与导出范围一致。"]
]

function fallbackCopy(value) {
  const textarea = document.createElement("textarea")
  textarea.value = value
  textarea.setAttribute("readonly", "")
  textarea.style.position = "fixed"
  textarea.style.opacity = "0"
  document.body.appendChild(textarea)
  textarea.select()
  const succeeded = document.execCommand("copy")
  textarea.remove()

  if (!succeeded) throw new Error("copy failed")
}

async function copyBase(value, key) {
  if (typeof window === "undefined") return

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
    } else {
      fallbackCopy(value)
    }

    copied.value = key
    window.clearTimeout(resetTimer)
    resetTimer = window.setTimeout(() => {
      if (copied.value === key) copied.value = ""
    }, 1800)
  } catch {
    copied.value = ""
  }
}

onBeforeUnmount(() => {
  if (typeof window !== "undefined") window.clearTimeout(resetTimer)
})
</script>

<template>
  <main class="docs-home">
    <section class="docs-hero" aria-labelledby="docs-home-title">
      <div class="docs-hero__grid" aria-hidden="true"></div>
      <div class="docs-hero__index" aria-hidden="true">01<span>/DOCS</span></div>

      <div class="docs-shell docs-hero__layout">
        <div class="docs-hero__copy">
          <p class="docs-kicker docs-status-line">
            <span class="docs-status-dot" aria-hidden="true"></span>
            POKE DOCUMENTATION / LIVE
            <span class="docs-status-separator" aria-hidden="true">/</span>
            接入指南
          </p>

          <h1 id="docs-home-title" class="docs-hero__title">
            <span>一个密钥</span>
            <span>所有模型</span>
          </h1>

          <p class="docs-hero__subtitle">模型、协议、接入步骤都写清楚。</p>
          <p class="docs-hero__description">
            一个 API Key 接入 Claude、OpenAI、Gemini。兼容官方接口，现有代码与命令行工具几乎零改动迁移。
          </p>

          <p class="docs-commitment">
            <span aria-hidden="true"></span>
            官方格式 · 正确 Base URL · 可复制配置
          </p>

          <div class="docs-actions" aria-label="开始使用">
            <a class="docs-button docs-button--primary" href="/guide/getting-started">
              快速开始 <span aria-hidden="true">→</span>
            </a>
            <a class="docs-button" href="/guide/api-scripts">
              阅读 API 接入 <span aria-hidden="true">↗</span>
            </a>
          </div>
        </div>

        <div class="docs-hero__evidence">
          <div class="docs-signal-board" aria-label="支持的模型服务">
            <span v-for="(model, index) in models" :key="model">
              <i>{{ String(index + 1).padStart(2, "0") }}</i>
              <strong>{{ model }}</strong>
              <b>READY</b>
            </span>
          </div>

          <div class="docs-orbit" aria-hidden="true">
            <div class="docs-orbit__core"><span>POKE</span><strong>DOC</strong></div>
            <span class="docs-orbit__tag docs-orbit__tag--claude">CLAUDE</span>
            <span class="docs-orbit__tag docs-orbit__tag--openai">OPENAI</span>
            <span class="docs-orbit__tag docs-orbit__tag--gemini">GEMINI</span>
          </div>
        </div>
      </div>

      <div class="docs-shell docs-hero__register" aria-hidden="true">
        <span>ANTHROPIC</span><span>OPENAI</span><span>GEMINI</span><span>SSE</span>
      </div>
    </section>

    <section class="docs-section" aria-labelledby="endpoint-title">
      <div class="docs-shell docs-section__grid">
        <header class="docs-section__heading">
          <p class="docs-kicker">01 / Base URL</p>
          <h2 id="endpoint-title">两个地址，<br>不要混用。</h2>
          <p>按协议选择入口。所有请求均使用 <code>Authorization: Bearer YOUR_API_KEY</code> 鉴权。</p>
        </header>

        <div>
          <div class="endpoint-panel">
            <article class="endpoint-row">
              <div class="endpoint-row__meta">
                <span>Anthropic</span>
                <strong>Claude</strong>
              </div>
              <div class="endpoint-row__value">
                <span>根地址，不要带 /v1</span>
                <code>https://www.poke2api.com</code>
              </div>
              <button
                type="button"
                :class="{ 'is-copied': copied === 'anthropic' }"
                :aria-label="copied === 'anthropic' ? 'Anthropic 地址已复制' : '复制 Anthropic 地址'"
                @click="copyBase('https://www.poke2api.com', 'anthropic')"
              >
                {{ copied === "anthropic" ? "已复制" : "复制" }}
              </button>
            </article>

            <article class="endpoint-row">
              <div class="endpoint-row__meta">
                <span>OpenAI-compatible</span>
                <strong>Codex / Gemini</strong>
              </div>
              <div class="endpoint-row__value">
                <span>接口地址，必须带 /v1</span>
                <code>https://www.poke2api.com/v1</code>
              </div>
              <button
                type="button"
                :class="{ 'is-copied': copied === 'openai' }"
                :aria-label="copied === 'openai' ? 'OpenAI 地址已复制' : '复制 OpenAI 地址'"
                @click="copyBase('https://www.poke2api.com/v1', 'openai')"
              >
                {{ copied === "openai" ? "已复制" : "复制" }}
              </button>
            </article>
          </div>
          <p class="copy-status" aria-live="polite">{{ copied ? "Base URL 已复制到剪贴板" : "" }}</p>
        </div>
      </div>
    </section>

    <section class="docs-section docs-section--surface" aria-labelledby="tools-title">
      <div class="docs-shell docs-section__grid">
        <header class="docs-section__heading">
          <p class="docs-kicker">02 / Toolchain</p>
          <h2 id="tools-title">你的工具，<br>照常工作。</h2>
          <p>一行脚本或少量配置即可接入，也支持图形化供应商管理。</p>
        </header>

        <div>
          <nav class="tool-list" aria-label="工具接入教程">
            <a v-for="tool in tools" :key="tool.name" class="tool-row" :href="tool.href">
              <span class="tool-row__index">{{ tool.index }}</span>
              <strong>{{ tool.name }}</strong>
              <span>{{ tool.protocol }}</span>
              <span class="tool-row__arrow" aria-hidden="true">↗</span>
            </a>
          </nav>
          <div class="tool-footer">
            <a href="/guide/cc-switch">CC-Switch 图形版</a>
            <a href="/guide/cc-switch-cli">CC-Switch CLI</a>
          </div>
        </div>
      </div>
    </section>

    <section class="docs-section" aria-labelledby="capability-title">
      <div class="docs-shell docs-section__grid">
        <header class="docs-section__heading">
          <p class="docs-kicker">03 / Capability</p>
          <h2 id="capability-title">更少配置，<br>更多能力。</h2>
          <p>统一入口不牺牲协议能力，常用模型、工具与流式调用保持熟悉的工作方式。</p>
        </header>

        <div class="capability-list">
          <article v-for="item in capabilities" :key="item[0]" class="capability-row">
            <span>{{ item[0] }}</span>
            <h3>{{ item[1] }}</h3>
            <p>{{ item[2] }}</p>
          </article>
        </div>
      </div>
    </section>

    <section class="docs-closing" aria-labelledby="closing-title">
      <div class="docs-shell docs-closing__grid">
        <div>
          <p class="docs-kicker">READY / START HERE</p>
          <h2 id="closing-title">从第一个请求开始。</h2>
        </div>
        <div class="docs-closing__copy">
          <p>在控制台创建 API Key，然后按照协议选择正确的 Base URL。</p>
          <a class="docs-button docs-button--light" href="https://www.poke2api.com/dashboard">
            前往控制台 <span aria-hidden="true">↗</span>
          </a>
        </div>
      </div>
    </section>
  </main>
</template>
