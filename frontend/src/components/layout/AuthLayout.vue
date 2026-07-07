<template>
  <div class="pk-auth" :class="{ 'pk-auth--ready': settingsLoaded }">
    <!-- top-right controls -->
    <div class="pk-auth-controls">
      <LocaleSwitcher />
      <button
        type="button"
        class="pk-theme-toggle"
        :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
        @click="toggleTheme"
      >
        <Icon v-if="isDark" name="sun" size="sm" :stroke-width="1.5" />
        <Icon v-else name="moon" size="sm" :stroke-width="1.5" />
      </button>
    </div>

    <div class="pk-auth-grid">
      <!-- ========== LEFT: brand showcase (desktop) ========== -->
      <aside class="pk-auth-brand">
        <router-link to="/" class="pk-auth-brandmark">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="pk-auth-logo" />
          <span class="pk-auth-brandname">{{ siteName }}</span>
        </router-link>

        <div class="pk-auth-brandbody">
          <p class="pk-eyebrow">
            <span class="pk-dot" aria-hidden="true"></span>
            <span>{{ t('auth.brand.eyebrow') }}</span>
          </p>
          <h2 class="pk-auth-display">
            <span class="pk-auth-display-line">{{ t('auth.brand.titleLine1') }}</span>
            <span class="pk-auth-display-line pk-auth-display-line--muted">{{ t('auth.brand.titleLine2') }}</span>
          </h2>
          <p class="pk-auth-lead">{{ siteSubtitle }}</p>

          <div class="pk-auth-models">
            <div v-for="m in models" :key="m.label" class="pk-auth-model">
              <ModelIcon :model="m.icon" size="16px" />
              <span>{{ m.label }}</span>
            </div>
            <div class="pk-auth-model pk-auth-model--more">
              <span>{{ t('auth.brand.more') }}</span>
            </div>
          </div>
        </div>

        <p class="pk-auth-copyright">&copy; {{ currentYear }} {{ siteName }}</p>
      </aside>

      <!-- ========== RIGHT: form panel ========== -->
      <main class="pk-auth-panel">
        <!-- compact brand (mobile only) -->
        <router-link to="/" class="pk-auth-brandmark pk-auth-brandmark--mobile">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="pk-auth-logo" />
          <span class="pk-auth-brandname">{{ siteName }}</span>
        </router-link>

        <div class="pk-auth-card">
          <slot />
        </div>

        <div class="pk-auth-footer">
          <slot name="footer" />
        </div>

        <p class="pk-auth-copyright pk-auth-copyright--mobile">
          &copy; {{ currentYear }} {{ siteName }}
        </p>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform'
)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)
const currentYear = computed(() => new Date().getFullYear())

// Model ecosystem badges (mirrors the home page ecosystem row).
const models = [
  { label: 'Claude', icon: 'claude-3' },
  { label: 'OpenAI', icon: 'gpt-4o' },
  { label: 'Gemini', icon: 'gemini-pro' },
  { label: 'Grok', icon: 'grok-2' },
  { label: 'DeepSeek', icon: 'deepseek-chat' },
  { label: 'Qwen', icon: 'qwen-max' }
]

// ---- Theme (follows sub2api html.dark convention) ----
const isDark = ref(document.documentElement.classList.contains('dark'))

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

// ---- Display fonts (Archivo + Geist Mono, matching the home page) ----
function loadDisplayFonts() {
  const id = 'pk-home-fonts'
  if (document.getElementById(id)) return
  const pre1 = document.createElement('link')
  pre1.rel = 'preconnect'
  pre1.href = 'https://fonts.googleapis.com'
  const pre2 = document.createElement('link')
  pre2.rel = 'preconnect'
  pre2.href = 'https://fonts.gstatic.com'
  pre2.crossOrigin = 'anonymous'
  const link = document.createElement('link')
  link.id = id
  link.rel = 'stylesheet'
  link.href =
    'https://fonts.googleapis.com/css2?family=Archivo:wght@500;600;700;800&family=Geist+Mono:wght@400;500&display=swap'
  document.head.appendChild(pre1)
  document.head.appendChild(pre2)
  document.head.appendChild(link)
}

onMounted(() => {
  initTheme()
  loadDisplayFonts()
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
/* =====================================================
   Auth pages — cold monochrome editorial (bymonolog).
   Shares the home page design language: near-black ink,
   off-white paper, oversized Archivo display, mono
   eyebrows, hairlines, one red accent. Light/dark.
   ===================================================== */
.pk-auth {
  /* light */
  --ink: oklch(0.2 0.004 250);
  --ink-mute: oklch(0.46 0.005 250);
  --ink-faint: oklch(0.62 0.005 250);
  --paper: oklch(0.96 0.002 250);
  --panel: oklch(0.99 0.001 250);
  --line: oklch(0.2 0.004 250 / 0.14);
  --line-strong: oklch(0.2 0.004 250 / 0.36);
  --hover: oklch(0.2 0.004 250 / 0.04);
  --red: oklch(0.58 0.2 27);

  --ease: cubic-bezier(0.22, 1, 0.36, 1);
  --display: 'Archivo', 'PingFang SC', 'Microsoft YaHei', system-ui, sans-serif;
  --mono: 'Geist Mono', ui-monospace, 'Cascadia Code', Menlo, Consolas, monospace;
  --body: 'PingFang SC', 'Microsoft YaHei', 'Archivo', system-ui, -apple-system, sans-serif;

  position: relative;
  min-height: 100vh;
  background: var(--paper);
  color: var(--ink);
  font-family: var(--body);
  overflow-x: clip;
  transition:
    background-color 0.35s ease,
    color 0.35s ease;
}
html.dark .pk-auth {
  /* dark — cold near-black à la bymonolog */
  --ink: oklch(0.93 0.004 250);
  --ink-mute: oklch(0.66 0.006 250);
  --ink-faint: oklch(0.5 0.006 250);
  --paper: oklch(0.155 0.003 250);
  --panel: oklch(0.185 0.003 250);
  --line: oklch(0.93 0.004 250 / 0.12);
  --line-strong: oklch(0.93 0.004 250 / 0.32);
  --hover: oklch(0.93 0.004 250 / 0.045);
  --red: oklch(0.62 0.2 27);
}

/* ---------- top-right controls ---------- */
.pk-auth-controls {
  position: absolute;
  top: 20px;
  right: 24px;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 12px;
}
.pk-auth-controls :deep(button) {
  color: var(--ink-mute);
}
.pk-auth-controls :deep(button:hover) {
  color: var(--ink);
}
.pk-theme-toggle {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: transparent;
  color: var(--ink-mute);
  cursor: pointer;
  transition:
    color 0.2s ease,
    border-color 0.2s ease,
    transform 0.15s var(--ease);
}
.pk-theme-toggle:hover {
  color: var(--ink);
  border-color: var(--line-strong);
}
.pk-theme-toggle:active {
  transform: scale(0.9);
}

/* ---------- layout ---------- */
.pk-auth-grid {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 1.05fr 0.95fr;
}
@media (max-width: 900px) {
  .pk-auth-grid {
    grid-template-columns: 1fr;
  }
}

/* ---------- left brand column ---------- */
.pk-auth-brand {
  position: relative;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 40px;
  padding: 48px 56px;
  border-right: 1px solid var(--line);
  background: color-mix(in oklab, var(--paper) 60%, var(--panel));
}
@media (max-width: 900px) {
  .pk-auth-brand {
    display: none;
  }
}

.pk-auth-brandmark {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  text-decoration: none;
  color: inherit;
  width: fit-content;
}
.pk-auth-logo {
  width: 40px;
  height: 40px;
  object-fit: contain;
  border-radius: 10px;
}
.pk-auth-brandname {
  font-family: var(--display);
  font-weight: 700;
  font-size: 20px;
  letter-spacing: -0.01em;
}

.pk-auth-brandbody {
  max-width: 460px;
}
.pk-auth-display {
  margin: 22px 0 20px;
  font-family: var(--display);
  font-weight: 700;
  font-size: clamp(34px, 4vw, 52px);
  line-height: 1.02;
  letter-spacing: -0.028em;
  display: flex;
  flex-direction: column;
}
.pk-auth-display-line--muted {
  color: var(--ink-faint);
}
.pk-auth-lead {
  max-width: 400px;
  color: var(--ink-mute);
  font-size: 15px;
  line-height: 1.6;
}

.pk-auth-models {
  margin-top: 30px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.pk-auth-model {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 7px 13px;
  border: 1px solid var(--line);
  border-radius: 999px;
  font-size: 12.5px;
  color: var(--ink-mute);
  background: var(--panel);
  transition:
    border-color 0.2s ease,
    color 0.2s ease;
}
.pk-auth-model:hover {
  border-color: var(--line-strong);
  color: var(--ink);
}
.pk-auth-model--more {
  font-family: var(--mono);
  font-size: 11.5px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--ink-faint);
}

/* ---------- eyebrow / dot (shared with home) ---------- */
.pk-eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-family: var(--mono);
  font-size: 11.5px;
  font-weight: 500;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-mute);
}
.pk-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--red);
  animation: pk-pulse 2.6s ease-in-out infinite;
}
@keyframes pk-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
}
@media (prefers-reduced-motion: reduce) {
  .pk-dot {
    animation: none;
  }
}

/* ---------- right form panel ---------- */
.pk-auth-panel {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  gap: 22px;
  padding: 56px 40px;
}
@media (max-width: 900px) {
  .pk-auth-panel {
    padding: 40px 22px;
    min-height: 100vh;
  }
}

.pk-auth-brandmark--mobile {
  display: none;
}
@media (max-width: 900px) {
  .pk-auth-brandmark--mobile {
    display: inline-flex;
    margin-bottom: 6px;
  }
}

.pk-auth-card {
  width: 100%;
  max-width: 420px;
  padding: 38px 34px;
  border: 1px solid var(--line);
  border-radius: 18px;
  background: var(--panel);
  box-shadow:
    0 1px 2px oklch(0.2 0.004 250 / 0.04),
    0 20px 50px -30px oklch(0.2 0.004 250 / 0.4);
}
html.dark .pk-auth-card {
  box-shadow:
    0 1px 2px oklch(0 0 0 / 0.3),
    0 24px 60px -34px oklch(0 0 0 / 0.7);
}
@media (max-width: 640px) {
  .pk-auth-card {
    padding: 30px 22px;
  }
}

.pk-auth-footer {
  width: 100%;
  max-width: 420px;
  text-align: center;
  font-size: 14px;
  color: var(--ink-mute);
}
.pk-auth-copyright {
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.08em;
  color: var(--ink-faint);
}
.pk-auth-copyright--mobile {
  display: none;
}
@media (max-width: 900px) {
  .pk-auth-copyright--mobile {
    display: block;
    margin-top: 8px;
  }
}

/* =====================================================
   Deep overrides — restyle the form controls inside the
   auth card to the editorial look WITHOUT touching the
   global .input / .btn used across the admin app.
   ===================================================== */

/* Titles rendered by each auth view (e.g. "Welcome back"). */
.pk-auth-card :deep(h2) {
  font-family: var(--display);
  font-weight: 700;
  letter-spacing: -0.02em;
  color: var(--ink);
}
.pk-auth-card :deep(p) {
  color: var(--ink-mute);
}

/* Labels */
.pk-auth-card :deep(.input-label) {
  font-family: var(--mono);
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--ink-mute);
}

/* Inputs — hairline border, paper bg, ink focus ring (no teal). */
.pk-auth-card :deep(.input) {
  border-radius: 10px;
  background: var(--paper);
  border: 1px solid var(--line);
  color: var(--ink);
  transition:
    border-color 0.2s ease,
    box-shadow 0.2s ease;
}
.pk-auth-card :deep(.input::placeholder) {
  color: var(--ink-faint);
}
.pk-auth-card :deep(.input:focus) {
  border-color: var(--ink);
  box-shadow: 0 0 0 3px oklch(0.2 0.004 250 / 0.08);
  outline: none;
}
html.dark .pk-auth-card :deep(.input:focus) {
  box-shadow: 0 0 0 3px oklch(0.93 0.004 250 / 0.1);
}
.pk-auth-card :deep(.input-error) {
  border-color: var(--red);
}

/* Field icons default to muted ink instead of gray. */
.pk-auth-card :deep(.input) ~ * :deep(svg),
.pk-auth-card :deep(.text-gray-400) {
  color: var(--ink-faint);
}

/* Primary button — solid ink capsule, hover inverts. */
.pk-auth-card :deep(.btn-primary) {
  border-radius: 999px;
  background: var(--ink);
  color: var(--paper);
  border: 1px solid var(--ink);
  font-family: var(--mono);
  font-size: 12.5px;
  font-weight: 500;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  box-shadow: none;
  transition:
    background-color 0.22s ease,
    color 0.22s ease,
    transform 0.15s var(--ease);
}
.pk-auth-card :deep(.btn-primary:hover:not(:disabled)) {
  background: transparent;
  color: var(--ink);
}
.pk-auth-card :deep(.btn-primary:active:not(:disabled)) {
  transform: scale(0.98);
}
.pk-auth-card :deep(.btn-primary:disabled) {
  opacity: 0.5;
}

/* Secondary / ghost buttons and OAuth buttons — hairline outline. */
.pk-auth-card :deep(.btn-secondary),
.pk-auth-card :deep(.btn-ghost) {
  border-radius: 999px;
  background: var(--panel);
  color: var(--ink);
  border: 1px solid var(--line);
  box-shadow: none;
}
.pk-auth-card :deep(.btn-secondary:hover),
.pk-auth-card :deep(.btn-ghost:hover) {
  border-color: var(--line-strong);
  background: var(--hover);
}

/* Text links (forgot password, sign up) → red accent on hover. */
.pk-auth-card :deep(a),
.pk-auth-footer :deep(a) {
  color: var(--ink);
  font-weight: 500;
  transition: color 0.18s ease;
}
.pk-auth-card :deep(a:hover),
.pk-auth-footer :deep(a:hover) {
  color: var(--red);
}
</style>
