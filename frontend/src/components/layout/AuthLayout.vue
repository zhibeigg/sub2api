<template>
  <div class="mono-auth" :style="pageStyle">
    <div class="mono-auth-grain" aria-hidden="true"></div>

    <div class="mono-auth-controls">
      <LocaleSwitcher />
      <button
        type="button"
        class="mono-auth-icon-btn"
        :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
        @click="toggleTheme"
      >
        <Icon v-if="isDark" name="sun" size="sm" :stroke-width="1.5" />
        <Icon v-else name="moon" size="sm" :stroke-width="1.5" />
      </button>
    </div>

    <div class="mono-auth-grid">
      <aside class="mono-auth-stage" aria-label="Brand">
        <router-link to="/home" class="mono-auth-brandmark">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="mono-auth-logo" />
          <span>{{ siteName }}</span>
        </router-link>

        <div class="mono-auth-stage-copy">
          <p class="mono-auth-eyebrow">
            <span class="mono-auth-dot" aria-hidden="true"></span>
            <span>{{ t('auth.brand.eyebrow') }}</span>
          </p>
          <h1>
            <span>{{ t('auth.brand.titleLine1') }}</span>
            <span>{{ t('auth.brand.titleLine2') }}</span>
          </h1>
          <p>{{ siteSubtitle }}</p>
        </div>

        <figure class="mono-auth-plate">
          <img :src="authPlateUrl" alt="" />
          <figcaption>
            <span>{{ t('auth.brand.plateLabel') }}</span>
            <span>{{ t('auth.brand.plateMeta') }}</span>
          </figcaption>
        </figure>

        <div class="mono-auth-proof">
          <div>
            <span>{{ t('auth.brand.proofKeys') }}</span>
            <strong>{{ t('auth.brand.proofKeysValue') }}</strong>
          </div>
          <div>
            <span>{{ t('auth.brand.proofModels') }}</span>
            <strong>{{ t('auth.brand.proofModelsValue') }}</strong>
          </div>
        </div>
      </aside>

      <main class="mono-auth-panel">
        <router-link to="/home" class="mono-auth-brandmark mono-auth-brandmark--mobile">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="mono-auth-logo" />
          <span>{{ siteName }}</span>
        </router-link>

        <section class="mono-auth-card" aria-live="polite">
          <p class="mono-auth-card-kicker">{{ t('auth.brand.formKicker') }}</p>
          <slot />
        </section>

        <div class="mono-auth-footer">
          <slot name="footer" />
        </div>

        <p class="mono-auth-copyright">&copy; {{ currentYear }} {{ siteName }}</p>
      </main>
    </div>

    <nav class="mono-auth-bottom" :aria-label="t('home.aria.bottomNav')">
      <router-link to="/home">{{ t('auth.brand.navHome') }}<span>→</span></router-link>
      <a href="/home#work">{{ t('auth.brand.navWork') }}<span>→</span></a>
      <a href="/home#process">{{ t('auth.brand.navProcess') }}<span>→</span></a>
      <router-link to="/register">{{ t('auth.signUp') }}<span>→</span></router-link>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import Icon from '@/components/icons/Icon.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import grainUrl from '@/assets/monolog/grain.svg'
import authPlateUrl from '@/assets/monolog/auth-plate.svg'

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || appStore.cachedPublicSettings?.site_name || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || appStore.cachedPublicSettings?.site_logo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform'
)
const currentYear = computed(() => new Date().getFullYear())
const pageStyle = computed<Record<string, string>>(() => ({
  '--grain-url': `url("${grainUrl}")`
}))

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

function loadDisplayFonts() {
  const id = 'mono-public-fonts'
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
  link.href = 'https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:wght@500;600;700;800&family=Geist+Mono:wght@400;500&display=swap'
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
.mono-auth {
  --ink: oklch(0.16 0.004 95);
  --ink-muted: oklch(0.43 0.006 95);
  --ink-soft: oklch(0.63 0.006 95);
  --paper: oklch(0.955 0.012 85);
  --paper-deep: oklch(0.9 0.018 82);
  --surface: oklch(0.985 0.007 85);
  --line: oklch(0.16 0.004 95 / 0.16);
  --line-strong: oklch(0.16 0.004 95 / 0.38);
  --accent: oklch(0.55 0.2 28);
  --ease: cubic-bezier(0.16, 1, 0.3, 1);
  --display: 'Bricolage Grotesque', 'PingFang SC', 'Microsoft YaHei', system-ui, sans-serif;
  --mono: 'Geist Mono', ui-monospace, 'Cascadia Code', Menlo, Consolas, monospace;
  --body: 'PingFang SC', 'Microsoft YaHei', 'Bricolage Grotesque', system-ui, sans-serif;

  position: relative;
  min-height: 100vh;
  overflow-x: clip;
  background:
    radial-gradient(circle at 12% 12%, oklch(0.82 0.03 55 / 0.22), transparent 26rem),
    linear-gradient(180deg, var(--paper), var(--paper-deep));
  color: var(--ink);
  font-family: var(--body);
}
html.dark .mono-auth {
  --ink: oklch(0.91 0.012 85);
  --ink-muted: oklch(0.69 0.012 85);
  --ink-soft: oklch(0.47 0.012 85);
  --paper: oklch(0.135 0.005 95);
  --paper-deep: oklch(0.09 0.004 95);
  --surface: oklch(0.18 0.006 95);
  --line: oklch(0.91 0.012 85 / 0.15);
  --line-strong: oklch(0.91 0.012 85 / 0.38);
  --accent: oklch(0.62 0.2 28);
}

.mono-auth-grain {
  pointer-events: none;
  position: fixed;
  inset: -40px;
  z-index: 1;
  opacity: 0.42;
  background-image: var(--grain-url);
  background-size: 180px 180px;
  mix-blend-mode: multiply;
  animation: mono-auth-grain 0.55s steps(6) infinite;
}
html.dark .mono-auth-grain {
  mix-blend-mode: screen;
  opacity: 0.18;
}
@keyframes mono-auth-grain {
  0%, 100% { transform: translate(0, 0); }
  20% { transform: translate(-2%, 1%); }
  40% { transform: translate(1%, -2%); }
  60% { transform: translate(2%, 2%); }
  80% { transform: translate(-1%, -1%); }
}

.mono-auth-controls {
  position: fixed;
  top: 22px;
  right: 26px;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 12px;
}
.mono-auth-controls :deep(button) {
  color: var(--ink-muted);
}
.mono-auth-icon-btn {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: color-mix(in oklab, var(--paper) 80%, transparent);
  color: var(--ink-muted);
  cursor: pointer;
  transition: border-color 0.2s ease, color 0.2s ease, transform 0.14s var(--ease);
}
.mono-auth-icon-btn:hover {
  color: var(--ink);
  border-color: var(--line-strong);
}
.mono-auth-icon-btn:active {
  transform: scale(0.92);
}

.mono-auth-grid {
  position: relative;
  z-index: 3;
  box-sizing: border-box;
  min-width: 0;
  min-height: 100vh;
  display: grid;
  grid-template-columns: minmax(0, 1.08fr) minmax(420px, 0.92fr);
}

.mono-auth-stage {
  min-height: 100vh;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto auto;
  gap: clamp(28px, 5vh, 54px);
  padding: clamp(34px, 5vw, 62px);
  border-right: 1px solid var(--line);
}
.mono-auth-brandmark {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  width: fit-content;
  color: inherit;
  text-decoration: none;
}
.mono-auth-logo {
  width: 34px;
  height: 34px;
  object-fit: contain;
  filter: grayscale(1) contrast(1.08);
}
.mono-auth-brandmark span {
  font-family: var(--display);
  font-size: 20px;
  font-weight: 800;
  letter-spacing: -0.03em;
}
.mono-auth-stage-copy {
  align-self: center;
  max-width: 720px;
}
.mono-auth-eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: var(--ink-muted);
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
}
.mono-auth-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--accent);
  animation: mono-auth-pulse 2.6s ease-in-out infinite;
}
@keyframes mono-auth-pulse {
  50% { opacity: 0.35; }
}
.mono-auth-stage-copy h1 {
  display: grid;
  margin: 24px 0 24px;
  font-family: var(--display);
  font-size: clamp(4rem, 8vw, 9rem);
  font-weight: 800;
  letter-spacing: -0.075em;
  line-height: 0.82;
}
.mono-auth-stage-copy h1 span:last-child {
  color: var(--ink-soft);
}
.mono-auth-stage-copy p {
  max-width: 34rem;
  margin: 0;
  padding-top: 24px;
  border-top: 1px solid var(--line);
  color: var(--ink-muted);
  font-size: 15px;
  line-height: 1.9;
}
.mono-auth-plate {
  max-width: 420px;
  margin: 0;
  padding: 12px;
  border: 1px solid var(--line);
  border-radius: 26px;
  background: color-mix(in oklab, var(--surface) 70%, transparent);
  transform: rotate(-1.2deg);
}
.mono-auth-plate img {
  display: block;
  width: 100%;
  max-height: 360px;
  object-fit: cover;
  border-radius: 18px;
  filter: grayscale(1) contrast(1.04);
}
.mono-auth-plate figcaption {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  padding: 12px 4px 2px;
  color: var(--ink-soft);
  font-family: var(--mono);
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}
.mono-auth-proof {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  border-top: 1px solid var(--line);
}
.mono-auth-proof div {
  display: grid;
  gap: 6px;
  padding-top: 20px;
}
.mono-auth-proof span {
  color: var(--ink-soft);
  font-family: var(--mono);
  font-size: 10px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}
.mono-auth-proof strong {
  font-family: var(--display);
  font-size: clamp(1.5rem, 2.4vw, 2.4rem);
  letter-spacing: -0.05em;
}

.mono-auth-panel {
  box-sizing: border-box;
  min-width: 0;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 20px;
  padding: 78px clamp(22px, 5vw, 72px) 104px;
}
.mono-auth-brandmark--mobile {
  display: none;
}
.mono-auth-card {
  box-sizing: border-box;
  width: min(100%, 470px);
  padding: clamp(28px, 4vw, 42px);
  border: 1px solid var(--line);
  border-radius: 28px;
  background: color-mix(in oklab, var(--surface) 88%, transparent);
  box-shadow: 0 24px 70px -48px oklch(0.16 0.004 95 / 0.58);
}
html.dark .mono-auth-card {
  box-shadow: 0 26px 78px -48px oklch(0 0 0 / 0.78);
}
.mono-auth-card-kicker {
  margin: 0 0 20px;
  color: var(--ink-soft);
  font-family: var(--mono);
  font-size: 10.5px;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}
.mono-auth-card :deep(*) {
  box-sizing: border-box;
  min-width: 0;
}
.mono-auth-card :deep(form),
.mono-auth-card :deep(.space-y-6),
.mono-auth-card :deep(.space-y-5),
.mono-auth-card :deep(.relative),
.mono-auth-card :deep(.btn),
.mono-auth-card :deep(.input),
.mono-auth-card :deep(input) {
  width: 100%;
  max-width: 100%;
}
.mono-auth-footer {
  width: min(100%, 470px);
  color: var(--ink-muted);
  text-align: center;
  font-size: 14px;
}
.mono-auth-copyright {
  margin: 0;
  color: var(--ink-soft);
  font-family: var(--mono);
  font-size: 10.5px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.mono-auth-bottom {
  position: fixed;
  inset: auto clamp(14px, 2vw, 28px) 16px;
  z-index: 22;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  overflow: hidden;
  border: 1px solid var(--line-strong);
  border-radius: 18px;
  background: color-mix(in oklab, var(--paper) 82%, transparent);
  backdrop-filter: blur(18px);
}
.mono-auth-bottom a {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 48px;
  border-right: 1px solid var(--line);
  color: var(--ink);
  font-family: var(--display);
  font-size: clamp(16px, 2vw, 22px);
  font-weight: 700;
  letter-spacing: -0.04em;
  text-decoration: none;
  transition: background-color 0.2s ease;
}
.mono-auth-bottom a:last-child {
  border-right: 0;
}
.mono-auth-bottom a:hover {
  background: color-mix(in oklab, var(--ink) 8%, transparent);
}

/* Deep restyle for LoginView / RegisterView internals. */
.mono-auth-card :deep(h2) {
  margin: 0;
  color: var(--ink);
  font-family: var(--display);
  font-size: clamp(2.1rem, 4vw, 3.45rem);
  font-weight: 800;
  letter-spacing: -0.055em;
  line-height: 0.96;
}
.mono-auth-card :deep(p) {
  color: var(--ink-muted);
}
.mono-auth-card :deep(.input-label) {
  color: var(--ink-muted);
  font-family: var(--mono);
  font-size: 10.5px;
  font-weight: 500;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}
.mono-auth-card :deep(.input),
.mono-auth-card :deep(input[type='email']),
.mono-auth-card :deep(input[type='password']),
.mono-auth-card :deep(input[type='text']) {
  border: 1px solid var(--line);
  border-radius: 14px;
  background: var(--paper);
  color: var(--ink);
  box-shadow: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
.mono-auth-card :deep(.input::placeholder) {
  color: var(--ink-soft);
}
.mono-auth-card :deep(.input:focus) {
  border-color: var(--ink);
  box-shadow: 0 0 0 3px oklch(0.16 0.004 95 / 0.08);
  outline: none;
}
html.dark .mono-auth-card :deep(.input:focus) {
  box-shadow: 0 0 0 3px oklch(0.91 0.012 85 / 0.1);
}
.mono-auth-card :deep(.input-error) {
  border-color: var(--accent);
}
.mono-auth-card :deep(.text-gray-400),
.mono-auth-card :deep(.text-gray-500),
.mono-auth-card :deep(.dark\:text-dark-400),
.mono-auth-card :deep(.dark\:text-dark-500) {
  color: var(--ink-soft);
}
.mono-auth-card :deep(.btn-primary) {
  min-height: 46px;
  border: 1px solid var(--ink);
  border-radius: 999px;
  background: var(--ink);
  color: var(--paper);
  font-family: var(--mono);
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.09em;
  text-transform: uppercase;
  box-shadow: none;
  transition: background-color 0.22s ease, color 0.22s ease, transform 0.14s var(--ease);
}
.mono-auth-card :deep(.btn-primary:hover:not(:disabled)) {
  background: transparent;
  color: var(--ink);
}
.mono-auth-card :deep(.btn-primary:active:not(:disabled)) {
  transform: scale(0.98);
}
.mono-auth-card :deep(.btn-primary:disabled) {
  opacity: 0.52;
}
.mono-auth-card :deep(.btn-secondary),
.mono-auth-card :deep(.btn-ghost) {
  border: 1px solid var(--line);
  border-radius: 999px;
  background: transparent;
  color: var(--ink);
  box-shadow: none;
}
.mono-auth-card :deep(.btn-secondary:hover),
.mono-auth-card :deep(.btn-ghost:hover) {
  border-color: var(--line-strong);
  background: color-mix(in oklab, var(--ink) 5%, transparent);
}
.mono-auth-card :deep(a),
.mono-auth-footer :deep(a) {
  color: var(--ink);
  font-weight: 600;
  text-decoration-thickness: 1px;
  text-underline-offset: 3px;
  transition: color 0.18s ease;
}
.mono-auth-card :deep(a:hover),
.mono-auth-footer :deep(a:hover) {
  color: var(--accent);
}
.mono-auth-card :deep(.h-px) {
  background: var(--line);
}
.mono-auth-card :deep(.rounded-lg.bg-green-50),
.mono-auth-card :deep(.dark\:bg-green-900\/20) {
  border: 1px solid var(--line);
  background: color-mix(in oklab, var(--surface) 88%, transparent);
}

@media (max-width: 1040px) {
  .mono-auth-grid {
    grid-template-columns: 1fr;
  }
  .mono-auth-stage {
    display: none;
  }
  .mono-auth-brandmark--mobile {
    display: inline-flex;
  }
  .mono-auth-panel {
    padding-top: 92px;
  }
}

@media (max-width: 640px) {
  .mono-auth-controls {
    top: 16px;
    right: 16px;
  }
  .mono-auth-panel {
    align-items: flex-start;
    padding-inline: 16px;
    padding-bottom: 164px;
  }
  .mono-auth-card,
  .mono-auth-footer {
    width: calc(100vw - 44px);
    max-width: calc(100vw - 44px);
  }
  .mono-auth-bottom {
    grid-template-columns: 1fr 1fr;
  }
  .mono-auth-bottom a {
    justify-content: space-between;
    padding-inline: 14px;
    border-bottom: 1px solid var(--line);
  }
  .mono-auth-bottom a:nth-last-child(-n + 2) {
    border-bottom: 0;
  }
  .mono-auth-bottom a:nth-child(2n) {
    border-right: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .mono-auth-grain,
  .mono-auth-dot {
    animation: none;
  }
  * {
    transition-duration: 1ms !important;
    scroll-behavior: auto !important;
  }
}
</style>
