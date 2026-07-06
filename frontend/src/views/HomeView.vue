<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page: monochrome editorial (bymonolog-inspired) -->
  <div v-else class="pk-page">
    <!-- ============ NAV ============ -->
    <header class="pk-nav" :class="{ 'pk-nav--scrolled': isScrolled }">
      <nav class="pk-nav-inner">
        <a class="pk-brand" href="#top" @click.prevent="scrollToTop">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="pk-brand-logo" />
          <span class="pk-brand-name">{{ siteName }}</span>
        </a>

        <div class="pk-nav-links">
          <a href="#value" @click.prevent="scrollTo('value')">{{ t('home.nav.features') }}</a>
          <a href="#workflow" @click.prevent="scrollTo('workflow')">{{ t('home.nav.workflow') }}</a>
          <a href="#ecosystem" @click.prevent="scrollTo('ecosystem')">{{ t('home.nav.models') }}</a>
          <a href="#pricing" @click.prevent="scrollTo('pricing')">{{ t('home.nav.pricing') }}</a>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a>
        </div>

        <div class="pk-nav-actions">
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
          <router-link v-if="isAuthenticated" :to="dashboardPath" class="pk-btn pk-btn--sm">
            {{ t('home.dashboard') }}
          </router-link>
          <template v-else>
            <router-link to="/login" class="pk-nav-login">{{ t('home.login') }}</router-link>
            <router-link to="/register" class="pk-btn pk-btn--sm">
              {{ t('home.getStarted') }}
            </router-link>
          </template>
        </div>
      </nav>
    </header>

    <main id="top">
      <!-- ============ HERO ============ -->
      <section class="pk-hero">
        <div class="pk-container">
          <p class="pk-eyebrow pk-enter" style="--d: 0ms">
            <span class="pk-dot" aria-hidden="true"></span>
            <span>{{ t('home.hero.badge') }}</span>
          </p>

          <h1 class="pk-display">
            <span class="pk-display-line pk-enter" style="--d: 90ms">{{ t('home.hero.titleLine1') }}</span>
            <span class="pk-display-line pk-display-line--muted pk-enter" style="--d: 180ms">{{
              t('home.hero.titleLine2')
            }}</span>
          </h1>

          <div class="pk-hero-body">
            <p class="pk-hero-lead pk-enter" style="--d: 280ms">
              {{ t('home.hero.description') }}
            </p>

            <div class="pk-hero-actions pk-enter" style="--d: 360ms">
              <div class="pk-cta-row">
                <router-link :to="isAuthenticated ? dashboardPath : '/register'" class="pk-btn pk-btn--lg">
                  {{ isAuthenticated ? t('home.goToDashboard') : t('home.hero.ctaPrimary') }}
                  <Icon name="arrowRight" size="sm" :stroke-width="1.5" />
                </router-link>
                <a
                  v-if="docUrl"
                  :href="docUrl"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="pk-textlink"
                >
                  {{ t('home.hero.ctaDocs') }}
                </a>
              </div>

              <div class="pk-endpoints">
                <div v-for="ep in endpoints" :key="ep.key" class="pk-endpoint">
                  <span class="pk-endpoint-label">{{ t(ep.labelKey) }}</span>
                  <code class="pk-endpoint-url">{{ ep.url }}</code>
                  <button
                    type="button"
                    class="pk-copy"
                    :class="{ 'pk-copy--done': copiedKey === ep.key }"
                    :aria-label="t('home.hero.copy')"
                    @click="copyEndpoint(ep)"
                  >
                    <Icon :name="copiedKey === ep.key ? 'check' : 'copy'" size="xs" :stroke-width="1.5" />
                    <span>{{ copiedKey === ep.key ? t('home.hero.copied') : t('home.hero.copy') }}</span>
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- ============ VALUE ============ -->
      <section id="value" class="pk-section">
        <div class="pk-container">
          <header class="pk-section-head" data-reveal>
            <span class="pk-eyebrow pk-eyebrow--plain">({{ t('home.value.kicker') }})</span>
            <h2 class="pk-heading">{{ t('home.value.title') }}</h2>
          </header>

          <div class="pk-ledger">
            <article
              v-for="(item, i) in valueItems"
              :key="item.key"
              class="pk-ledger-row"
              data-reveal
              :style="{ '--rd': i * 70 + 'ms' }"
            >
              <span class="pk-ledger-num">{{ String(i + 1).padStart(2, '0') }}</span>
              <h3 class="pk-ledger-title">{{ t(`home.value.items.${item.key}.title`) }}</h3>
              <p class="pk-ledger-desc">{{ t(`home.value.items.${item.key}.desc`) }}</p>
            </article>
          </div>
        </div>
      </section>

      <!-- ============ WORKFLOW ============ -->
      <section id="workflow" class="pk-section">
        <div class="pk-container">
          <header class="pk-section-head" data-reveal>
            <span class="pk-eyebrow pk-eyebrow--plain">({{ t('home.workflow.kicker') }})</span>
            <h2 class="pk-heading">{{ t('home.workflow.title') }}</h2>
            <p class="pk-section-lead">{{ t('home.workflow.subtitle') }}</p>
          </header>

          <div class="pk-steps">
            <article
              v-for="(step, i) in workflowSteps"
              :key="step.key"
              class="pk-step"
              data-reveal
              :style="{ '--rd': i * 90 + 'ms' }"
            >
              <span class="pk-step-num">{{ String(i + 1).padStart(2, '0') }}</span>
              <h3 class="pk-step-title">{{ t(`home.workflow.steps.${step.key}.title`) }}</h3>
              <p class="pk-step-desc">{{ t(`home.workflow.steps.${step.key}.desc`) }}</p>
              <code v-if="step.code" class="pk-step-code">{{ step.code }}</code>
            </article>
          </div>
        </div>
      </section>

      <!-- ============ ECOSYSTEM ============ -->
      <section id="ecosystem" class="pk-section">
        <div class="pk-container">
          <header class="pk-section-head" data-reveal>
            <span class="pk-eyebrow pk-eyebrow--plain">({{ t('home.ecosystem.kicker') }})</span>
            <h2 class="pk-heading">{{ t('home.ecosystem.title') }}</h2>
          </header>

          <div class="pk-models" data-reveal>
            <div v-for="m in models" :key="m.label" class="pk-model">
              <ModelIcon :model="m.icon" size="18px" class="pk-model-icon" />
              <span>{{ m.label }}</span>
            </div>
            <div class="pk-model pk-model--more">
              <span>{{ t('home.ecosystem.more') }}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- ============ PRICING ============ -->
      <section id="pricing" class="pk-section">
        <div class="pk-container">
          <header class="pk-section-head" data-reveal>
            <span class="pk-eyebrow pk-eyebrow--plain">({{ t('home.pricing.kicker') }})</span>
            <h2 class="pk-heading">{{ t('home.pricing.title') }}</h2>
            <p class="pk-section-lead">{{ t('home.pricing.subtitle') }}</p>
          </header>

          <div class="pk-rate" data-reveal>
            <div class="pk-rate-main">
              <span class="pk-eyebrow pk-eyebrow--plain">{{ t('home.pricing.rateLabel') }} · {{ t('home.pricing.badge') }}</span>
              <div class="pk-rate-value">{{ t('home.pricing.rateValue') }}</div>
              <div class="pk-rate-ref">
                {{ t('home.pricing.officialLabel') }} <s>{{ t('home.pricing.officialValue') }}</s>
              </div>
            </div>
            <div class="pk-rate-aside">
              <p class="pk-rate-note">{{ t('home.pricing.note') }}</p>
              <router-link :to="isAuthenticated ? dashboardPath : '/register'" class="pk-btn pk-btn--lg">
                {{ t('home.pricing.cta') }}
                <Icon name="arrowRight" size="sm" :stroke-width="1.5" />
              </router-link>
            </div>
          </div>
        </div>
      </section>

      <!-- ============ CTA ============ -->
      <section class="pk-section pk-section--cta">
        <div class="pk-container" data-reveal>
          <h2 class="pk-cta-display">{{ t('home.cta.title') }}</h2>
          <p class="pk-cta-lead">{{ t('home.cta.description') }}</p>
          <router-link :to="isAuthenticated ? dashboardPath : '/register'" class="pk-btn pk-btn--lg">
            {{ t('home.cta.button') }}
            <Icon name="arrowRight" size="sm" :stroke-width="1.5" />
          </router-link>
        </div>
      </section>
    </main>

    <!-- ============ FOOTER ============ -->
    <footer class="pk-footer">
      <div class="pk-container pk-footer-inner">
        <p>&copy; {{ currentYear }} {{ siteName }} · {{ t('home.footer.allRightsReserved') }}</p>
        <div class="pk-footer-links">
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a>
          <router-link to="/login">{{ t('home.footer.console') }}</router-link>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Poke API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))

const currentYear = computed(() => new Date().getFullYear())

// ---- Theme (follows sub2api html.dark convention) ----
const isDark = ref(document.documentElement.classList.contains('dark'))

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

// ---- Display fonts (loaded only on the home page) ----
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

// ---- Data ----
const endpoints = [
  { key: 'openai', labelKey: 'home.hero.baseUrlOpenai', url: `${window.location.origin}/v1` },
  { key: 'anthropic', labelKey: 'home.hero.baseUrlAnthropic', url: window.location.origin }
]

const valueItems = [
  { key: 'unified' },
  { key: 'observability' },
  { key: 'elastic' },
  { key: 'developer' }
]

const workflowSteps = [
  { key: 'register', code: '' },
  { key: 'configure', code: `export ANTHROPIC_BASE_URL=${window.location.origin}` },
  { key: 'observe', code: '' }
]

const models = [
  { label: 'Claude', icon: 'claude-3' },
  { label: 'OpenAI', icon: 'gpt-4o' },
  { label: 'Gemini', icon: 'gemini-pro' },
  { label: 'Grok', icon: 'grok-2' },
  { label: 'DeepSeek', icon: 'deepseek-chat' },
  { label: 'Qwen', icon: 'qwen-max' }
]

// ---- Copy interaction ----
const copiedKey = ref('')
let copyTimer: ReturnType<typeof setTimeout> | null = null

async function copyEndpoint(ep: { key: string; url: string }) {
  try {
    await navigator.clipboard.writeText(ep.url)
  } catch {
    const ta = document.createElement('textarea')
    ta.value = ep.url
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
  }
  copiedKey.value = ep.key
  if (copyTimer) clearTimeout(copyTimer)
  copyTimer = setTimeout(() => {
    copiedKey.value = ''
  }, 1800)
}

// ---- Scroll state (nav) ----
const isScrolled = ref(false)
function onScroll() {
  isScrolled.value = window.scrollY > 12
}

function scrollTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
function scrollToTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

// ---- Scroll reveal ----
let observer: IntersectionObserver | null = null

function setupReveal() {
  const prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const targets = document.querySelectorAll<HTMLElement>('[data-reveal]')
  if (prefersReduced || !('IntersectionObserver' in window)) {
    targets.forEach((el) => el.classList.add('pk-revealed'))
    return
  }
  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          entry.target.classList.add('pk-revealed')
          observer?.unobserve(entry.target)
        }
      }
    },
    { threshold: 0.12, rootMargin: '0px 0px -40px 0px' }
  )
  targets.forEach((el) => observer?.observe(el))
}

onMounted(() => {
  initTheme()
  loadDisplayFonts()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  window.addEventListener('scroll', onScroll, { passive: true })
  onScroll()
  requestAnimationFrame(() => setupReveal())
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
  observer?.disconnect()
  if (copyTimer) clearTimeout(copyTimer)
})
</script>

<style scoped>
/* =====================================================
   Poke API home — cold monochrome editorial.
   Inspired by bymonolog: near-black ink, off-white,
   oversized display type, mono eyebrows, hairlines.
   Light/dark via html.dark. One tiny red accent.
   ===================================================== */
.pk-page {
  /* light */
  --ink: oklch(0.2 0.004 250);
  --ink-mute: oklch(0.46 0.005 250);
  --ink-faint: oklch(0.62 0.005 250);
  --paper: oklch(0.96 0.002 250);
  --line: oklch(0.2 0.004 250 / 0.14);
  --line-strong: oklch(0.2 0.004 250 / 0.36);
  --hover: oklch(0.2 0.004 250 / 0.04);
  --red: oklch(0.58 0.2 27);

  --ease: cubic-bezier(0.22, 1, 0.36, 1);
  --display: 'Archivo', 'PingFang SC', 'Microsoft YaHei', system-ui, sans-serif;
  --mono: 'Geist Mono', ui-monospace, 'Cascadia Code', Menlo, Consolas, monospace;
  --body: 'PingFang SC', 'Microsoft YaHei', 'Archivo', system-ui, -apple-system, sans-serif;

  min-height: 100vh;
  background: var(--paper);
  color: var(--ink);
  font-family: var(--body);
  overflow-x: clip;
  transition:
    background-color 0.35s ease,
    color 0.35s ease;
}
html.dark .pk-page {
  /* dark — cold near-black à la bymonolog rgb(8,8,7) */
  --ink: oklch(0.93 0.004 250);
  --ink-mute: oklch(0.66 0.006 250);
  --ink-faint: oklch(0.5 0.006 250);
  --paper: oklch(0.155 0.003 250);
  --line: oklch(0.93 0.004 250 / 0.12);
  --line-strong: oklch(0.93 0.004 250 / 0.32);
  --hover: oklch(0.93 0.004 250 / 0.045);
  --red: oklch(0.62 0.2 27);
}

.pk-container {
  max-width: 1240px;
  margin-inline: auto;
  padding-inline: 32px;
}
@media (max-width: 640px) {
  .pk-container {
    padding-inline: 22px;
  }
}

/* ---------- motion ---------- */
.pk-enter {
  opacity: 0;
  transform: translateY(18px);
  animation: pk-rise 0.85s var(--ease) forwards;
  animation-delay: var(--d, 0ms);
}
@keyframes pk-rise {
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
[data-reveal] {
  opacity: 0;
  transform: translateY(26px);
  transition:
    opacity 0.75s var(--ease),
    transform 0.75s var(--ease);
  transition-delay: var(--rd, 0ms);
}
[data-reveal].pk-revealed {
  opacity: 1;
  transform: translateY(0);
}
@media (prefers-reduced-motion: reduce) {
  .pk-enter,
  [data-reveal] {
    animation: none;
    transition: none;
    opacity: 1;
    transform: none;
  }
}

/* ---------- eyebrow (mono kicker) ---------- */
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
.pk-eyebrow--plain {
  color: var(--ink-faint);
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

/* ---------- nav ---------- */
.pk-nav {
  position: sticky;
  top: 0;
  z-index: 50;
  border-bottom: 1px solid transparent;
  transition:
    background-color 0.25s ease,
    border-color 0.25s ease;
}
.pk-nav--scrolled {
  background: color-mix(in oklab, var(--paper) 82%, transparent);
  backdrop-filter: blur(14px);
  border-bottom-color: var(--line);
}
.pk-nav-inner {
  max-width: 1240px;
  margin-inline: auto;
  padding: 18px 32px;
  display: flex;
  align-items: center;
  gap: 28px;
}
@media (max-width: 640px) {
  .pk-nav-inner {
    padding: 16px 22px;
  }
}
.pk-brand {
  display: inline-flex;
  align-items: center;
  gap: 11px;
  text-decoration: none;
  color: inherit;
}
.pk-brand-logo {
  width: 24px;
  height: 24px;
  object-fit: contain;
  filter: grayscale(1) contrast(1.1);
}
.pk-brand-name {
  font-family: var(--display);
  font-weight: 700;
  font-size: 16px;
  letter-spacing: -0.01em;
}
.pk-nav-links {
  display: none;
  gap: 28px;
  margin-inline: auto;
}
@media (min-width: 900px) {
  .pk-nav-links {
    display: flex;
  }
}
.pk-nav-links a {
  position: relative;
  font-family: var(--mono);
  font-size: 12px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--ink-mute);
  text-decoration: none;
  padding: 3px 0;
  transition: color 0.2s ease;
}
.pk-nav-links a::after {
  content: '';
  position: absolute;
  left: 0;
  bottom: 0;
  width: 100%;
  height: 1px;
  background: currentColor;
  transform: scaleX(0);
  transform-origin: right;
  transition: transform 0.32s var(--ease);
}
.pk-nav-links a:hover {
  color: var(--ink);
}
.pk-nav-links a:hover::after {
  transform: scaleX(1);
  transform-origin: left;
}
.pk-nav-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-left: auto;
}
.pk-nav-login {
  font-family: var(--mono);
  font-size: 12px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-mute);
  text-decoration: none;
  transition: color 0.2s ease;
}
.pk-nav-login:hover {
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
.pk-nav-actions :deep(button) {
  color: var(--ink-mute);
}
.pk-nav-actions :deep(button:hover) {
  color: var(--ink);
  background: var(--hover);
}

/* ---------- buttons ---------- */
.pk-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  font-family: var(--mono);
  font-size: 12.5px;
  font-weight: 500;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  border: 1px solid var(--ink);
  background: transparent;
  color: var(--ink);
  text-decoration: none;
  border-radius: 999px;
  cursor: pointer;
  transition:
    background-color 0.24s ease,
    color 0.24s ease,
    transform 0.15s var(--ease);
}
.pk-btn:hover {
  background: var(--ink);
  color: var(--paper);
}
.pk-btn:active {
  transform: scale(0.97);
}
.pk-btn--sm {
  padding: 8px 18px;
}
.pk-btn--lg {
  font-size: 13px;
  padding: 15px 30px;
}
.pk-textlink {
  position: relative;
  font-family: var(--mono);
  font-size: 12.5px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--ink);
  text-decoration: none;
  padding-bottom: 3px;
  align-self: center;
}
.pk-textlink::after {
  content: '';
  position: absolute;
  left: 0;
  bottom: 0;
  width: 100%;
  height: 1px;
  background: currentColor;
  transform-origin: right;
  transition: transform 0.32s var(--ease);
}
.pk-textlink:hover::after {
  transform: scaleX(0.4);
  transform-origin: left;
}

/* ---------- hero ---------- */
.pk-hero {
  position: relative;
  padding: clamp(72px, 12vh, 132px) 0 clamp(64px, 10vh, 104px);
  border-bottom: 1px solid var(--line);
}
.pk-eyebrow.pk-enter {
  margin-bottom: clamp(28px, 5vh, 48px);
}
.pk-display {
  font-family: var(--display);
  font-size: clamp(3rem, 9vw, 8rem);
  font-weight: 700;
  line-height: 0.98;
  letter-spacing: -0.04em;
  margin: 0 0 clamp(40px, 6vh, 60px);
}
.pk-display-line {
  display: block;
}
.pk-display-line--muted {
  color: var(--ink-faint);
}
.pk-hero-body {
  display: grid;
  gap: 44px;
  align-items: start;
}
@media (min-width: 960px) {
  .pk-hero-body {
    grid-template-columns: 0.95fr 1.05fr;
    gap: 80px;
  }
}
.pk-hero-lead {
  font-size: clamp(1rem, 1.4vw, 1.15rem);
  line-height: 1.9;
  color: var(--ink-mute);
  max-width: 30em;
  margin: 0;
  padding-top: 28px;
  border-top: 1px solid var(--line);
}
.pk-hero-actions {
  display: grid;
  gap: 34px;
  padding-top: 28px;
  border-top: 1px solid var(--line);
}
.pk-cta-row {
  display: flex;
  flex-wrap: wrap;
  gap: 28px;
  align-items: center;
}

/* endpoints */
.pk-endpoints {
  display: grid;
}
.pk-endpoint {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 14px 2px;
  border-bottom: 1px solid var(--line);
}
.pk-endpoint:first-child {
  border-top: 1px solid var(--line);
}
.pk-endpoint-label {
  font-family: var(--mono);
  font-size: 10.5px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--ink-faint);
  white-space: nowrap;
  min-width: 132px;
}
.pk-endpoint-url {
  font-family: var(--mono);
  font-size: 13px;
  color: var(--ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}
.pk-copy {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-family: var(--mono);
  font-size: 10.5px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-mute);
  background: transparent;
  border: 1px solid var(--line);
  border-radius: 999px;
  padding: 6px 13px;
  cursor: pointer;
  white-space: nowrap;
  transition:
    color 0.18s ease,
    border-color 0.18s ease,
    transform 0.12s var(--ease);
}
.pk-copy:hover {
  color: var(--ink);
  border-color: var(--line-strong);
}
.pk-copy:active {
  transform: scale(0.94);
}
.pk-copy--done {
  color: var(--ink);
  border-color: var(--ink);
}

/* ---------- sections ---------- */
.pk-section {
  position: relative;
  padding: clamp(72px, 11vh, 120px) 0;
  border-bottom: 1px solid var(--line);
}
.pk-section-head {
  display: grid;
  gap: 16px;
  margin-bottom: clamp(44px, 7vh, 72px);
  max-width: 44em;
}
.pk-heading {
  font-family: var(--display);
  font-size: clamp(1.9rem, 4.4vw, 3.4rem);
  font-weight: 600;
  letter-spacing: -0.03em;
  line-height: 1.06;
  margin: 0;
}
.pk-section-lead {
  font-size: 15px;
  color: var(--ink-mute);
  line-height: 1.85;
  margin: 4px 0 0;
  max-width: 40em;
}

/* ---------- value ledger ---------- */
.pk-ledger {
  display: grid;
}
.pk-ledger-row {
  display: grid;
  grid-template-columns: 68px 300px 1fr;
  gap: 24px;
  align-items: baseline;
  padding: clamp(24px, 3vw, 34px) 2px;
  border-top: 1px solid var(--line);
  transition: background-color 0.28s ease;
}
.pk-ledger-row:last-child {
  border-bottom: 1px solid var(--line);
}
.pk-ledger-row:hover {
  background: var(--hover);
}
@media (max-width: 860px) {
  .pk-ledger-row {
    grid-template-columns: 54px 1fr;
  }
  .pk-ledger-desc {
    grid-column: 2;
  }
}
.pk-ledger-num {
  font-family: var(--mono);
  font-size: 12px;
  color: var(--ink-faint);
}
.pk-ledger-title {
  font-family: var(--display);
  font-size: clamp(1.15rem, 1.8vw, 1.5rem);
  font-weight: 600;
  letter-spacing: -0.015em;
  margin: 0;
}
.pk-ledger-desc {
  font-size: 14px;
  line-height: 1.9;
  color: var(--ink-mute);
  margin: 0;
  max-width: 42em;
}

/* ---------- workflow ---------- */
.pk-steps {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
}
.pk-step {
  padding: 4px 30px 4px 0;
}
.pk-step + .pk-step {
  border-left: 1px solid var(--line);
  padding-left: 32px;
}
@media (max-width: 860px) {
  .pk-step + .pk-step {
    border-left: none;
    padding-left: 0;
    border-top: 1px solid var(--line);
    margin-top: 28px;
    padding-top: 28px;
  }
}
.pk-step-num {
  display: block;
  font-family: var(--mono);
  font-size: 12px;
  color: var(--ink-faint);
  margin-bottom: 22px;
}
.pk-step-title {
  font-family: var(--display);
  font-size: 1.15rem;
  font-weight: 600;
  letter-spacing: -0.015em;
  margin: 0 0 12px;
}
.pk-step-desc {
  font-size: 14px;
  line-height: 1.9;
  color: var(--ink-mute);
  margin: 0;
}
.pk-step-code {
  display: block;
  margin-top: 18px;
  padding: 12px 0;
  border-top: 1px solid var(--line);
  font-family: var(--mono);
  font-size: 11.5px;
  color: var(--ink-mute);
  overflow-x: auto;
  white-space: nowrap;
}

/* ---------- ecosystem ---------- */
.pk-models {
  display: flex;
  flex-wrap: wrap;
}
.pk-model {
  display: inline-flex;
  align-items: center;
  gap: 11px;
  padding: 16px 30px 16px 0;
  margin-right: 30px;
  font-family: var(--mono);
  font-size: 13px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink);
  border-bottom: 1px solid var(--line);
  transition: border-color 0.28s ease;
}
.pk-model:hover {
  border-bottom-color: var(--ink);
}
.pk-model-icon {
  filter: grayscale(1) contrast(1.05);
  opacity: 0.82;
}
.pk-model--more {
  color: var(--ink-faint);
}

/* ---------- pricing ---------- */
.pk-rate {
  display: grid;
  gap: 44px;
  align-items: end;
}
@media (min-width: 900px) {
  .pk-rate {
    grid-template-columns: 1.25fr 0.75fr;
    gap: 80px;
  }
}
.pk-rate-value {
  font-family: var(--display);
  font-size: clamp(3.2rem, 10vw, 7rem);
  font-weight: 700;
  letter-spacing: -0.05em;
  line-height: 0.94;
  margin: 18px 0 16px;
}
.pk-rate-ref {
  font-family: var(--mono);
  font-size: 13px;
  color: var(--ink-faint);
}
.pk-rate-ref s {
  color: var(--ink-faint);
}
.pk-rate-aside {
  border-top: 1px solid var(--line);
  padding-top: 26px;
}
.pk-rate-note {
  font-size: 14px;
  line-height: 2;
  color: var(--ink-mute);
  margin: 0 0 28px;
}

/* ---------- CTA ---------- */
.pk-section--cta {
  text-align: center;
  padding: clamp(96px, 16vh, 180px) 0;
}
.pk-cta-display {
  font-family: var(--display);
  font-size: clamp(2.6rem, 8vw, 6.5rem);
  font-weight: 700;
  letter-spacing: -0.045em;
  line-height: 1;
  margin: 0 0 24px;
}
.pk-cta-lead {
  font-size: 15.5px;
  color: var(--ink-mute);
  line-height: 1.9;
  margin: 0 auto 40px;
  max-width: 32em;
}

/* ---------- footer ---------- */
.pk-footer {
  padding: 34px 0;
}
.pk-footer-inner {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--ink-faint);
}
@media (min-width: 640px) {
  .pk-footer-inner {
    flex-direction: row;
    justify-content: space-between;
  }
}
.pk-footer-inner p {
  margin: 0;
}
.pk-footer-links {
  display: flex;
  gap: 24px;
}
.pk-footer-links a {
  color: var(--ink-faint);
  text-decoration: none;
  transition: color 0.2s ease;
}
.pk-footer-links a:hover {
  color: var(--ink);
}
</style>
