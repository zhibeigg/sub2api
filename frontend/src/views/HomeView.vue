<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page: monochrome studio / bymonolog-inspired -->
  <div
    v-else
    class="mono-page"
    :style="pageStyle"
    @pointermove="handlePointerMove"
  >
    <div class="mono-grain" aria-hidden="true"></div>

    <div class="mono-cursor" :class="{ 'mono-cursor--active': cursorLabel }" :style="cursorStyle" aria-hidden="true">
      <span>{{ cursorLabel || 'VIEW' }}</span>
    </div>

    <header class="mono-topbar" :class="{ 'mono-topbar--scrolled': isScrolled }">
      <router-link
        to="/home"
        class="mono-brand"
        @pointerenter="setCursor(t('home.cursor.home'))"
        @pointerleave="clearCursor"
      >
        <img :src="siteLogo || '/logo.png'" alt="Logo" class="mono-brand-logo" />
        <span class="mono-brand-text">{{ siteName }}</span>
      </router-link>

      <nav class="mono-top-links" :aria-label="t('home.aria.primaryNav')">
        <a href="#work" @click.prevent="scrollTo('work')">{{ t('home.nav.features') }}</a>
        <a href="#process" @click.prevent="scrollTo('process')">{{ t('home.nav.workflow') }}</a>
        <a href="#services" @click.prevent="scrollTo('services')">{{ t('home.nav.models') }}</a>
        <a href="#pricing" @click.prevent="scrollTo('pricing')">{{ t('home.nav.pricing') }}</a>
      </nav>

      <div class="mono-top-actions">
        <LocaleSwitcher />
        <button
          type="button"
          class="mono-icon-btn"
          :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          @click="toggleTheme"
          @pointerenter="setCursor(isDark ? t('home.cursor.light') : t('home.cursor.dark'))"
          @pointerleave="clearCursor"
        >
          <Icon v-if="isDark" name="sun" size="sm" :stroke-width="1.5" />
          <Icon v-else name="moon" size="sm" :stroke-width="1.5" />
        </button>
        <router-link
          v-if="isAuthenticated"
          :to="dashboardPath"
          class="mono-pill mono-pill--filled"
          @pointerenter="setCursor(t('home.cursor.enter'))"
          @pointerleave="clearCursor"
        >
          {{ t('home.dashboard') }}
        </router-link>
        <template v-else>
          <router-link
            to="/login"
            class="mono-text-link"
            @pointerenter="setCursor(t('home.cursor.login'))"
            @pointerleave="clearCursor"
          >
            {{ t('home.login') }}
          </router-link>
          <router-link
            to="/register"
            class="mono-pill mono-pill--filled"
            @pointerenter="setCursor(t('home.cursor.start'))"
            @pointerleave="clearCursor"
          >
            {{ t('home.getStarted') }}
          </router-link>
        </template>
      </div>
    </header>

    <main id="top">
      <section class="mono-hero" aria-labelledby="home-title">
        <div class="mono-container">
          <div class="mono-hero-grid">
            <div class="mono-hero-copy">
              <p class="mono-eyebrow mono-enter" style="--d: 0ms">
                <span class="mono-dot" aria-hidden="true"></span>
                <span>{{ t('home.hero.badge') }}</span>
              </p>

              <h1 id="home-title" class="mono-display">
                <span class="mono-display-line mono-enter" style="--d: 90ms">{{ t('home.hero.titleLine1') }}</span>
                <span class="mono-display-line mono-display-line--mute mono-enter" style="--d: 170ms">
                  {{ t('home.hero.titleLine2') }}
                </span>
              </h1>

              <p class="mono-hero-lead mono-enter" style="--d: 260ms">
                {{ t('home.hero.description') }}
              </p>

              <div class="mono-hero-actions mono-enter" style="--d: 350ms">
                <router-link
                  :to="isAuthenticated ? dashboardPath : '/register'"
                  class="mono-pill mono-pill--large mono-pill--filled"
                  @pointerenter="setCursor(t('home.cursor.start'))"
                  @pointerleave="clearCursor"
                >
                  {{ isAuthenticated ? t('home.goToDashboard') : t('home.hero.ctaPrimary') }}
                  <Icon name="arrowRight" size="sm" :stroke-width="1.5" />
                </router-link>
                <button
                  type="button"
                  class="mono-pill mono-pill--large"
                  @click="openAbout"
                  @pointerenter="setCursor(t('home.cursor.about'))"
                  @pointerleave="clearCursor"
                >
                  {{ t('home.about.open') }}
                </button>
              </div>
            </div>

            <aside class="mono-hero-plate mono-enter" style="--d: 420ms" aria-label="API gateway visual">
              <img :src="gatewayPlateUrl" alt="" class="mono-plate-img" />
              <div class="mono-plate-caption">
                <span>{{ t('home.visual.gatewayLabel') }}</span>
                <span>{{ t('home.visual.gatewayMeta') }}</span>
              </div>
            </aside>
          </div>

          <div class="mono-endpoints mono-enter" style="--d: 520ms" :aria-label="t('home.aria.endpoints')">
            <div v-for="ep in endpoints" :key="ep.key" class="mono-endpoint">
              <span class="mono-endpoint-label">{{ t(ep.labelKey) }}</span>
              <code class="mono-endpoint-url">{{ ep.url }}</code>
              <button
                type="button"
                class="mono-copy"
                :class="{ 'mono-copy--done': copiedKey === ep.key }"
                :aria-label="t('home.hero.copy')"
                @click="copyEndpoint(ep)"
                @pointerenter="setCursor(t('home.cursor.copy'))"
                @pointerleave="clearCursor"
              >
                <Icon :name="copiedKey === ep.key ? 'check' : 'copy'" size="xs" :stroke-width="1.5" />
                <span>{{ copiedKey === ep.key ? t('home.hero.copied') : t('home.hero.copy') }}</span>
              </button>
            </div>
          </div>
        </div>
      </section>

      <section id="work" class="mono-section mono-section--work">
        <div class="mono-container">
          <div class="mono-section-kicker" data-reveal>
            <span>{{ t('home.work.kicker') }}</span>
            <span>{{ t('home.work.index') }}</span>
          </div>

          <div class="mono-work-list">
            <article
              v-for="(item, i) in workItems"
              :key="item.key"
              class="mono-work-row"
              data-reveal
              :style="{ '--rd': i * 70 + 'ms' }"
              @pointerenter="setCursor(t('home.cursor.read'))"
              @pointerleave="clearCursor"
            >
              <span class="mono-row-num">{{ String(i + 1).padStart(2, '0') }}</span>
              <h2>{{ t(`home.value.items.${item.key}.title`) }}</h2>
              <p>{{ t(`home.value.items.${item.key}.desc`) }}</p>
            </article>
          </div>
        </div>
      </section>

      <section id="process" class="mono-section">
        <div class="mono-container mono-process-grid">
          <header class="mono-sticky-heading" data-reveal>
            <span class="mono-eyebrow mono-eyebrow--plain">{{ t('home.workflow.kicker') }}</span>
            <h2>{{ t('home.workflow.title') }}</h2>
            <p>{{ t('home.workflow.subtitle') }}</p>
          </header>

          <div class="mono-process-list">
            <article
              v-for="(step, i) in workflowSteps"
              :key="step.key"
              class="mono-process-card"
              data-reveal
              :style="{ '--rd': i * 90 + 'ms' }"
            >
              <span>{{ String(i + 1).padStart(2, '0') }}</span>
              <h3>{{ t(`home.workflow.steps.${step.key}.title`) }}</h3>
              <p>{{ t(`home.workflow.steps.${step.key}.desc`) }}</p>
              <code v-if="step.code">{{ step.code }}</code>
            </article>
          </div>
        </div>
      </section>

      <section id="services" class="mono-section mono-section--models">
        <div class="mono-container">
          <header class="mono-wide-heading" data-reveal>
            <span class="mono-eyebrow mono-eyebrow--plain">{{ t('home.ecosystem.kicker') }}</span>
            <h2>{{ t('home.ecosystem.title') }}</h2>
          </header>

          <div class="mono-model-marquee" data-reveal>
            <div class="mono-model-track">
              <div v-for="m in doubledModels" :key="m.id" class="mono-model-chip">
                <ModelIcon :model="m.icon" size="18px" />
                <span>{{ m.label }}</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section id="pricing" class="mono-section mono-section--pricing">
        <div class="mono-container mono-price-grid">
          <div data-reveal>
            <span class="mono-eyebrow mono-eyebrow--plain">{{ t('home.pricing.kicker') }}</span>
            <h2 class="mono-price-value">{{ t('home.pricing.rateValue') }}</h2>
            <p class="mono-price-note">
              {{ t('home.pricing.officialLabel') }} <s>{{ t('home.pricing.officialValue') }}</s>
              · {{ t('home.pricing.note') }}
            </p>
          </div>
          <div class="mono-price-aside" data-reveal style="--rd: 100ms">
            <p>{{ t('home.pricing.subtitle') }}</p>
            <router-link
              :to="isAuthenticated ? dashboardPath : '/register'"
              class="mono-pill mono-pill--large mono-pill--filled"
              @pointerenter="setCursor(t('home.cursor.start'))"
              @pointerleave="clearCursor"
            >
              {{ t('home.pricing.cta') }}
              <Icon name="arrowRight" size="sm" :stroke-width="1.5" />
            </router-link>
          </div>
        </div>
      </section>

      <section id="contact" class="mono-section mono-section--cta">
        <div class="mono-container" data-reveal>
          <p class="mono-eyebrow mono-eyebrow--plain">{{ t('home.cta.kicker') }}</p>
          <h2>{{ t('home.cta.title') }}</h2>
          <p>{{ t('home.cta.description') }}</p>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/register'"
            class="mono-pill mono-pill--large mono-pill--filled"
            @pointerenter="setCursor(t('home.cursor.enter'))"
            @pointerleave="clearCursor"
          >
            {{ t('home.cta.button') }}
            <Icon name="arrowRight" size="sm" :stroke-width="1.5" />
          </router-link>
        </div>
      </section>
    </main>

    <footer class="mono-footer">
      <div class="mono-container mono-footer-inner">
        <span>&copy; {{ currentYear }} {{ siteName }}</span>
        <span>{{ t('home.footer.allRightsReserved') }}</span>
      </div>
    </footer>

    <nav class="mono-bottom-nav" :aria-label="t('home.aria.bottomNav')">
      <button type="button" @click="openAbout">{{ t('home.bottomNav.about') }}<span>→</span></button>
      <button type="button" @click="scrollTo('work')">{{ t('home.bottomNav.work') }}<span>→</span></button>
      <button type="button" @click="scrollTo('process')">{{ t('home.bottomNav.process') }}<span>→</span></button>
      <button type="button" @click="scrollTo('services')">{{ t('home.bottomNav.services') }}<span>→</span></button>
      <router-link :to="isAuthenticated ? dashboardPath : '/register'">
        {{ t('home.bottomNav.contact') }}<span>→</span>
      </router-link>
    </nav>

    <transition name="mono-overlay">
      <section v-if="aboutOpen" class="mono-about" role="dialog" aria-modal="true" :aria-label="t('home.about.title')">
        <div class="mono-grain" aria-hidden="true"></div>
        <button
          type="button"
          class="mono-about-close"
          @click="closeAbout"
          @pointerenter="setCursor(t('home.cursor.close'))"
          @pointerleave="clearCursor"
        >
          <span>{{ t('home.about.close') }}</span>
          <kbd>esc</kbd>
        </button>

        <div class="mono-about-body">
          <p class="mono-eyebrow">
            <span class="mono-dot" aria-hidden="true"></span>
            <span>{{ t('home.about.eyebrow') }}</span>
          </p>
          <h2>{{ t('home.about.title') }}</h2>
          <p class="mono-about-lead">{{ t('home.about.body') }}</p>

          <div class="mono-about-meta">
            <span>{{ t('home.about.est') }}</span>
            <span>{{ t('home.about.based') }}</span>
          </div>

          <div class="mono-about-principles">
            <article v-for="item in aboutPrinciples" :key="item.key">
              <h3>{{ t(`home.about.principles.${item.key}.title`) }}</h3>
              <p>{{ t(`home.about.principles.${item.key}.desc`) }}</p>
            </article>
          </div>
        </div>
      </section>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import grainUrl from '@/assets/monolog/grain.svg'
import gatewayPlateUrl from '@/assets/monolog/gateway-plate.svg'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
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

const endpoints = [
  { key: 'openai', labelKey: 'home.hero.baseUrlOpenai', url: `${window.location.origin}/v1` },
  { key: 'anthropic', labelKey: 'home.hero.baseUrlAnthropic', url: window.location.origin }
]

const workItems = [
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

const doubledModels = computed(() =>
  [...models, ...models].map((model, index) => ({ ...model, id: `${model.label}-${index}` }))
)

const aboutPrinciples = [
  { key: 'outcomes' },
  { key: 'signal' },
  { key: 'human' },
  { key: 'pace' }
]

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

const isScrolled = ref(false)
function onScroll() {
  isScrolled.value = window.scrollY > 16
}

function scrollTo(id: string) {
  closeAbout()
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

const aboutOpen = ref(false)
function openAbout() {
  aboutOpen.value = true
  document.documentElement.classList.add('mono-lock-scroll')
}
function closeAbout() {
  aboutOpen.value = false
  document.documentElement.classList.remove('mono-lock-scroll')
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && aboutOpen.value) {
    closeAbout()
  }
}

const cursorX = ref(0)
const cursorY = ref(0)
const cursorLabel = ref('')
const cursorStyle = computed(() => ({
  transform: `translate3d(${cursorX.value + 18}px, ${cursorY.value + 18}px, 0)`
}))

function handlePointerMove(event: PointerEvent) {
  cursorX.value = event.clientX
  cursorY.value = event.clientY
}
function setCursor(label: string) {
  cursorLabel.value = label
}
function clearCursor() {
  cursorLabel.value = ''
}

let observer: IntersectionObserver | null = null

function setupReveal() {
  const prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const targets = document.querySelectorAll<HTMLElement>('[data-reveal]')
  if (prefersReduced || !('IntersectionObserver' in window)) {
    targets.forEach((el) => el.classList.add('mono-revealed'))
    return
  }
  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          entry.target.classList.add('mono-revealed')
          observer?.unobserve(entry.target)
        }
      }
    },
    { threshold: 0.12, rootMargin: '0px 0px -44px 0px' }
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
  window.addEventListener('keydown', onKeydown)
  onScroll()
  requestAnimationFrame(() => setupReveal())
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
  window.removeEventListener('keydown', onKeydown)
  observer?.disconnect()
  if (copyTimer) clearTimeout(copyTimer)
  document.documentElement.classList.remove('mono-lock-scroll')
})
</script>

<style scoped>
:global(html.mono-lock-scroll) {
  overflow: hidden;
}

.mono-page {
  --ink: oklch(0.16 0.004 95);
  --ink-muted: oklch(0.43 0.006 95);
  --ink-soft: oklch(0.63 0.006 95);
  --paper: oklch(0.955 0.012 85);
  --paper-deep: oklch(0.9 0.018 82);
  --line: oklch(0.16 0.004 95 / 0.16);
  --line-strong: oklch(0.16 0.004 95 / 0.38);
  --surface: oklch(0.985 0.007 85);
  --accent: oklch(0.55 0.2 28);
  --ease: cubic-bezier(0.16, 1, 0.3, 1);
  --display: 'Bricolage Grotesque', 'PingFang SC', 'Microsoft YaHei', system-ui, sans-serif;
  --mono: 'Geist Mono', ui-monospace, 'Cascadia Code', Menlo, Consolas, monospace;
  --body: 'PingFang SC', 'Microsoft YaHei', 'Bricolage Grotesque', system-ui, sans-serif;

  position: relative;
  min-height: 100vh;
  overflow-x: clip;
  background:
    radial-gradient(circle at 10% 12%, oklch(0.82 0.03 55 / 0.24), transparent 26rem),
    linear-gradient(180deg, var(--paper), var(--paper-deep));
  color: var(--ink);
  font-family: var(--body);
}
html.dark .mono-page {
  --ink: oklch(0.91 0.012 85);
  --ink-muted: oklch(0.69 0.012 85);
  --ink-soft: oklch(0.47 0.012 85);
  --paper: oklch(0.135 0.005 95);
  --paper-deep: oklch(0.09 0.004 95);
  --line: oklch(0.91 0.012 85 / 0.15);
  --line-strong: oklch(0.91 0.012 85 / 0.38);
  --surface: oklch(0.18 0.006 95);
  --accent: oklch(0.62 0.2 28);
}

.mono-grain {
  pointer-events: none;
  position: fixed;
  inset: -40px;
  z-index: 2;
  opacity: 0.42;
  background-image: var(--grain-url);
  background-size: 180px 180px;
  mix-blend-mode: multiply;
  animation: mono-grain 0.55s steps(6) infinite;
}
html.dark .mono-grain {
  mix-blend-mode: screen;
  opacity: 0.18;
}
@keyframes mono-grain {
  0%, 100% { transform: translate(0, 0); }
  20% { transform: translate(-2%, 1%); }
  40% { transform: translate(1%, -2%); }
  60% { transform: translate(2%, 2%); }
  80% { transform: translate(-1%, -1%); }
}

.mono-container {
  width: min(100% - 44px, 1320px);
  margin-inline: auto;
}

.mono-enter {
  opacity: 0;
  transform: translateY(24px);
  animation: mono-rise 0.9s var(--ease) forwards;
  animation-delay: var(--d, 0ms);
}
@keyframes mono-rise {
  to { opacity: 1; transform: translateY(0); }
}
[data-reveal] {
  opacity: 0;
  transform: translateY(28px);
  transition: opacity 0.78s var(--ease), transform 0.78s var(--ease);
  transition-delay: var(--rd, 0ms);
}
[data-reveal].mono-revealed {
  opacity: 1;
  transform: translateY(0);
}

.mono-topbar {
  position: sticky;
  top: 0;
  z-index: 40;
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 28px;
  padding: 22px clamp(22px, 3vw, 42px);
  border-bottom: 1px solid transparent;
  transition: background-color 0.24s ease, border-color 0.24s ease;
}
.mono-topbar--scrolled {
  background: color-mix(in oklab, var(--paper) 82%, transparent);
  border-bottom-color: var(--line);
  backdrop-filter: blur(16px);
}
.mono-brand,
.mono-top-links,
.mono-top-actions,
.mono-bottom-nav,
.mono-footer,
.mono-about-close {
  position: relative;
  z-index: 4;
}
.mono-brand {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  color: inherit;
  text-decoration: none;
}
.mono-brand-logo {
  width: 30px;
  height: 30px;
  object-fit: contain;
  filter: grayscale(1) contrast(1.1);
}
.mono-brand-text {
  font-family: var(--display);
  font-size: 18px;
  font-weight: 800;
  letter-spacing: -0.02em;
}
.mono-top-links {
  display: flex;
  justify-content: center;
  gap: 30px;
}
.mono-top-links a,
.mono-text-link {
  font-family: var(--mono);
  font-size: 11.5px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--ink-muted);
  text-decoration: none;
  transition: color 0.18s ease;
}
.mono-top-links a:hover,
.mono-text-link:hover {
  color: var(--ink);
}
.mono-top-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.mono-top-actions :deep(button) {
  color: var(--ink-muted);
}
.mono-icon-btn {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: transparent;
  color: var(--ink-muted);
  cursor: pointer;
  transition: border-color 0.2s ease, color 0.2s ease, transform 0.14s var(--ease);
}
.mono-icon-btn:hover {
  color: var(--ink);
  border-color: var(--line-strong);
}
.mono-icon-btn:active {
  transform: scale(0.92);
}

.mono-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-height: 38px;
  padding: 9px 18px;
  border: 1px solid var(--ink);
  border-radius: 999px;
  background: transparent;
  color: var(--ink);
  font-family: var(--mono);
  font-size: 11.5px;
  font-weight: 500;
  letter-spacing: 0.09em;
  text-transform: uppercase;
  text-decoration: none;
  cursor: pointer;
  transition: background-color 0.24s ease, color 0.24s ease, transform 0.14s var(--ease);
}
.mono-pill--large {
  min-height: 50px;
  padding: 14px 28px;
  font-size: 12px;
}
.mono-pill--filled,
.mono-pill:hover {
  background: var(--ink);
  color: var(--paper);
}
.mono-pill--filled:hover {
  background: transparent;
  color: var(--ink);
}
.mono-pill:active {
  transform: scale(0.97);
}

.mono-hero {
  position: relative;
  z-index: 3;
  min-height: calc(100vh - 84px);
  padding: clamp(30px, 5vh, 62px) 0 clamp(120px, 16vh, 172px);
  border-bottom: 1px solid var(--line);
}
.mono-hero-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.06fr) minmax(300px, 0.72fr);
  gap: clamp(38px, 7vw, 96px);
  align-items: end;
}
.mono-eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: var(--ink-muted);
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
}
.mono-eyebrow--plain {
  color: var(--ink-soft);
}
.mono-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--accent);
  animation: mono-pulse 2.6s ease-in-out infinite;
}
@keyframes mono-pulse {
  50% { opacity: 0.35; }
}
.mono-display {
  margin: clamp(20px, 3.5vh, 38px) 0 clamp(22px, 3.5vh, 36px);
  font-family: var(--display);
  font-size: clamp(4rem, 10.3vw, 10rem);
  font-weight: 800;
  letter-spacing: -0.075em;
  line-height: 0.84;
}
.mono-display-line {
  display: block;
}
.mono-display-line--mute {
  color: var(--ink-soft);
}
.mono-hero-lead {
  max-width: 44rem;
  margin: 0;
  padding-top: 22px;
  border-top: 1px solid var(--line);
  color: var(--ink-muted);
  font-size: clamp(1rem, 1.35vw, 1.14rem);
  line-height: 1.72;
}
.mono-hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 18px;
  margin-top: 24px;
}
.mono-hero-plate {
  position: relative;
  padding: 14px;
  border: 1px solid var(--line);
  border-radius: 28px;
  background: color-mix(in oklab, var(--surface) 80%, transparent);
  transform: rotate(-1.4deg);
}
.mono-plate-img {
  display: block;
  width: 100%;
  border-radius: 18px;
  filter: grayscale(1) contrast(1.02);
}
.mono-plate-caption {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  padding: 14px 4px 2px;
  color: var(--ink-soft);
  font-family: var(--mono);
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.mono-endpoints {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 clamp(18px, 4vw, 44px);
  margin-top: clamp(24px, 4vh, 46px);
  border-top: 1px solid var(--line);
}
.mono-endpoint {
  display: grid;
  grid-template-columns: 156px minmax(0, 1fr) auto;
  gap: 14px;
  align-items: center;
  min-height: 54px;
  border-bottom: 1px solid var(--line);
}
.mono-endpoint-label,
.mono-row-num,
.mono-section-kicker,
.mono-copy,
.mono-process-card span {
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--ink-soft);
}
.mono-endpoint-url {
  overflow: hidden;
  color: var(--ink);
  font-family: var(--mono);
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mono-copy {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: transparent;
  padding: 7px 13px;
  cursor: pointer;
  transition: border-color 0.2s ease, color 0.2s ease;
}
.mono-copy:hover,
.mono-copy--done {
  color: var(--ink);
  border-color: var(--line-strong);
}

.mono-section {
  position: relative;
  z-index: 3;
  padding: clamp(80px, 12vh, 140px) 0;
  border-bottom: 1px solid var(--line);
}
.mono-section-kicker {
  display: flex;
  justify-content: space-between;
  margin-bottom: clamp(34px, 6vh, 70px);
}
.mono-work-list {
  display: grid;
}
.mono-work-row {
  display: grid;
  grid-template-columns: 80px minmax(220px, 0.48fr) minmax(0, 1fr);
  gap: clamp(18px, 4vw, 58px);
  align-items: baseline;
  padding: clamp(26px, 3.8vw, 42px) 0;
  border-top: 1px solid var(--line);
  transition: background-color 0.28s ease;
}
.mono-work-row:last-child {
  border-bottom: 1px solid var(--line);
}
.mono-work-row:hover {
  background: color-mix(in oklab, var(--ink) 4%, transparent);
}
.mono-work-row h2,
.mono-sticky-heading h2,
.mono-wide-heading h2,
.mono-process-card h3,
.mono-section--cta h2 {
  margin: 0;
  color: var(--ink);
  font-family: var(--display);
  letter-spacing: -0.035em;
}
.mono-work-row h2 {
  font-size: clamp(1.45rem, 3vw, 2.6rem);
  line-height: 0.98;
}
.mono-work-row p,
.mono-sticky-heading p,
.mono-process-card p,
.mono-price-aside p,
.mono-section--cta p,
.mono-about-lead,
.mono-about-principles p {
  margin: 0;
  color: var(--ink-muted);
  font-size: 14.5px;
  line-height: 1.9;
}

.mono-process-grid,
.mono-price-grid {
  display: grid;
  grid-template-columns: minmax(260px, 0.58fr) minmax(0, 1fr);
  gap: clamp(42px, 8vw, 120px);
}
.mono-sticky-heading {
  position: sticky;
  top: 110px;
  align-self: start;
  display: grid;
  gap: 18px;
}
.mono-sticky-heading h2,
.mono-wide-heading h2 {
  font-size: clamp(2.4rem, 6vw, 5.5rem);
  line-height: 0.94;
}
.mono-process-list {
  display: grid;
  gap: 18px;
}
.mono-process-card {
  display: grid;
  gap: 18px;
  padding: clamp(24px, 4vw, 42px);
  border: 1px solid var(--line);
  border-radius: 28px;
  background: color-mix(in oklab, var(--surface) 68%, transparent);
}
.mono-process-card h3 {
  font-size: clamp(1.45rem, 2.2vw, 2.1rem);
}
.mono-process-card code {
  overflow-x: auto;
  padding-top: 18px;
  border-top: 1px solid var(--line);
  color: var(--ink-muted);
  font-family: var(--mono);
  font-size: 12px;
  white-space: nowrap;
}

.mono-wide-heading {
  display: grid;
  gap: 20px;
  max-width: 820px;
  margin-bottom: clamp(44px, 8vh, 76px);
}
.mono-model-marquee {
  overflow: hidden;
  border-top: 1px solid var(--line);
  border-bottom: 1px solid var(--line);
}
.mono-model-track {
  display: flex;
  width: max-content;
  animation: mono-marquee 28s linear infinite;
}
.mono-model-chip {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  min-width: 180px;
  padding: 24px 42px 24px 0;
  color: var(--ink);
  font-family: var(--mono);
  font-size: 12px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}
.mono-model-chip :deep(svg),
.mono-model-chip :deep(img) {
  filter: grayscale(1) contrast(1.08);
  opacity: 0.82;
}
@keyframes mono-marquee {
  to { transform: translateX(-50%); }
}

.mono-price-value {
  margin: 22px 0 18px;
  color: var(--ink);
  font-family: var(--display);
  font-size: clamp(4rem, 11vw, 10rem);
  font-weight: 800;
  letter-spacing: -0.08em;
  line-height: 0.82;
}
.mono-price-note {
  max-width: 46rem;
  color: var(--ink-muted);
  font-family: var(--mono);
  font-size: 12px;
  line-height: 1.8;
  text-transform: uppercase;
}
.mono-price-aside {
  display: grid;
  gap: 28px;
  align-content: end;
  padding-top: 28px;
  border-top: 1px solid var(--line);
}

.mono-section--cta {
  text-align: center;
}
.mono-section--cta .mono-container {
  display: grid;
  justify-items: center;
  gap: 26px;
}
.mono-section--cta h2 {
  max-width: 980px;
  font-size: clamp(3.8rem, 11vw, 11rem);
  line-height: 0.82;
}
.mono-section--cta p {
  max-width: 34rem;
}

.mono-footer {
  position: relative;
  z-index: 3;
  padding: 34px 0 92px;
}
.mono-footer-inner {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  color: var(--ink-soft);
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.mono-bottom-nav {
  position: fixed;
  inset: auto clamp(14px, 2vw, 28px) 16px;
  z-index: 42;
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  border: 1px solid var(--line-strong);
  border-radius: 18px;
  overflow: hidden;
  background: color-mix(in oklab, var(--paper) 82%, transparent);
  backdrop-filter: blur(18px);
}
.mono-bottom-nav button,
.mono-bottom-nav a {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 48px;
  border: 0;
  border-right: 1px solid var(--line);
  background: transparent;
  color: var(--ink);
  font-family: var(--display);
  font-size: clamp(16px, 2vw, 24px);
  font-weight: 700;
  letter-spacing: -0.04em;
  text-decoration: none;
  cursor: pointer;
  transition: background-color 0.2s ease;
}
.mono-bottom-nav > :last-child {
  border-right: 0;
}
.mono-bottom-nav button:hover,
.mono-bottom-nav a:hover {
  background: color-mix(in oklab, var(--ink) 8%, transparent);
}

.mono-about {
  position: fixed;
  inset: 0;
  z-index: 80;
  overflow-y: auto;
  background: var(--paper);
  color: var(--ink);
}
.mono-about-close {
  position: fixed;
  top: 24px;
  right: 28px;
  display: inline-flex;
  align-items: center;
  gap: 10px;
  border: 1px solid var(--line-strong);
  border-radius: 999px;
  background: var(--paper);
  color: var(--ink);
  padding: 9px 12px 9px 18px;
  font-family: var(--mono);
  font-size: 12px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  cursor: pointer;
}
.mono-about-close kbd {
  border-radius: 999px;
  background: var(--ink);
  color: var(--paper);
  padding: 4px 8px;
  font: inherit;
}
.mono-about-body {
  width: min(100% - 44px, 1180px);
  margin-inline: auto;
  padding: clamp(92px, 12vh, 140px) 0 clamp(74px, 10vh, 120px);
}
.mono-about-body h2 {
  max-width: 980px;
  margin: 24px 0 34px;
  font-family: var(--display);
  font-size: clamp(3.2rem, 9vw, 9.5rem);
  font-weight: 800;
  letter-spacing: -0.07em;
  line-height: 0.84;
}
.mono-about-lead {
  max-width: 62rem;
  font-size: clamp(1.1rem, 2.1vw, 1.8rem);
  line-height: 1.55;
}
.mono-about-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 18px;
  margin: clamp(38px, 7vh, 70px) 0;
  color: var(--ink-soft);
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}
.mono-about-principles {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  border-top: 1px solid var(--line);
}
.mono-about-principles article {
  padding: 28px 24px 0 0;
  border-right: 1px solid var(--line);
}
.mono-about-principles article:last-child {
  border-right: 0;
}
.mono-about-principles h3 {
  margin: 0 0 14px;
  font-family: var(--display);
  font-size: 1.3rem;
  letter-spacing: -0.03em;
}

.mono-overlay-enter-active,
.mono-overlay-leave-active {
  transition: opacity 0.32s var(--ease), transform 0.32s var(--ease);
}
.mono-overlay-enter-from,
.mono-overlay-leave-to {
  opacity: 0;
  transform: translateY(28px);
}

.mono-cursor {
  pointer-events: none;
  position: fixed;
  top: 0;
  left: 0;
  z-index: 100;
  display: none;
  padding: 8px 12px;
  border-radius: 9px;
  background: var(--ink);
  color: var(--paper);
  font-family: var(--mono);
  font-size: 10px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  opacity: 0;
  clip-path: inset(0 100% 0 0 round 9px);
  transition: opacity 0.18s ease, clip-path 0.34s var(--ease);
}
.mono-cursor--active {
  opacity: 1;
  clip-path: inset(0 0 0 0 round 9px);
}

@media (hover: hover) and (pointer: fine) {
  .mono-cursor {
    display: block;
  }
}

@media (max-width: 980px) {
  .mono-topbar {
    grid-template-columns: 1fr auto;
  }
  .mono-top-links {
    display: none;
  }
  .mono-hero-grid,
  .mono-process-grid,
  .mono-price-grid {
    grid-template-columns: 1fr;
  }
  .mono-hero-plate {
    transform: none;
  }
  .mono-sticky-heading {
    position: static;
  }
  .mono-work-row {
    grid-template-columns: 58px 1fr;
  }
  .mono-work-row p {
    grid-column: 2;
  }
  .mono-about-principles {
    grid-template-columns: 1fr 1fr;
  }
  .mono-about-principles article:nth-child(2) {
    border-right: 0;
  }
}

@media (max-width: 700px) {
  .mono-topbar {
    padding: 18px 18px;
  }
  .mono-top-actions :deep(.locale-switcher),
  .mono-top-actions > .mono-text-link {
    display: none;
  }
  .mono-container,
  .mono-about-body {
    width: min(100% - 32px, 1320px);
  }
  .mono-display {
    font-size: clamp(3.4rem, 18vw, 5.2rem);
    letter-spacing: -0.065em;
    line-height: 0.88;
  }
  .mono-endpoint {
    grid-template-columns: 1fr;
    gap: 8px;
    padding: 16px 0;
  }
  .mono-copy {
    width: fit-content;
  }
  .mono-work-row {
    grid-template-columns: 1fr;
    gap: 12px;
  }
  .mono-work-row p {
    grid-column: auto;
  }
  .mono-bottom-nav {
    grid-template-columns: 1fr 1fr;
  }
  .mono-bottom-nav button,
  .mono-bottom-nav a {
    justify-content: space-between;
    padding-inline: 14px;
    font-size: 18px;
    border-bottom: 1px solid var(--line);
  }
  .mono-bottom-nav > :nth-last-child(-n + 1) {
    grid-column: 1 / -1;
    border-bottom: 0;
  }
  .mono-footer {
    padding-bottom: 178px;
  }
  .mono-footer-inner {
    flex-direction: column;
  }
  .mono-about-principles {
    grid-template-columns: 1fr;
  }
  .mono-about-principles article {
    border-right: 0;
    border-bottom: 1px solid var(--line);
    padding-bottom: 24px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .mono-enter,
  [data-reveal],
  .mono-grain,
  .mono-dot,
  .mono-model-track {
    animation: none;
    transition: none;
    opacity: 1;
    transform: none;
  }
}
</style>
