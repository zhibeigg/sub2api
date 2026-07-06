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

  <!-- Default Home Page: monochrome editorial, ink & paper -->
  <div v-else class="pk-page">
    <div class="pk-noise" aria-hidden="true"></div>

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
            <Icon v-if="isDark" name="sun" size="sm" :stroke-width="1.6" />
            <Icon v-else name="moon" size="sm" :stroke-width="1.6" />
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
          <p class="pk-hero-kicker pk-enter" style="--d: 0ms">
            <span class="pk-dot" aria-hidden="true"></span>
            {{ t('home.hero.badge') }}
          </p>

          <h1 class="pk-hero-title">
            <span class="pk-hero-line pk-enter" style="--d: 80ms">{{ t('home.hero.titleLine1') }}</span>
            <span class="pk-hero-line pk-hero-line--em pk-enter" style="--d: 170ms">{{
              t('home.hero.titleLine2')
            }}</span>
          </h1>

          <div class="pk-hero-below">
            <p class="pk-hero-desc pk-enter" style="--d: 280ms">
              {{ t('home.hero.description') }}
            </p>

            <div class="pk-hero-side pk-enter" style="--d: 340ms">
              <div class="pk-hero-ctas">
                <router-link :to="isAuthenticated ? dashboardPath : '/register'" class="pk-btn pk-btn--lg">
                  {{ isAuthenticated ? t('home.goToDashboard') : t('home.hero.ctaPrimary') }}
                  <Icon name="arrowRight" size="sm" :stroke-width="1.8" />
                </router-link>
                <a
                  v-if="docUrl"
                  :href="docUrl"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="pk-linkline"
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
                    <Icon :name="copiedKey === ep.key ? 'check' : 'copy'" size="xs" :stroke-width="1.8" />
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
          <div class="pk-section-head" data-reveal>
            <span class="pk-index">01</span>
            <span class="pk-kicker">{{ t('home.value.kicker') }}</span>
            <h2 class="pk-section-title">{{ t('home.value.title') }}</h2>
          </div>

          <div class="pk-value-list">
            <article
              v-for="(item, i) in valueItems"
              :key="item.key"
              class="pk-value-row"
              data-reveal
              :style="{ '--rd': i * 70 + 'ms' }"
            >
              <span class="pk-value-num">{{ String(i + 1).padStart(2, '0') }}</span>
              <h3>{{ t(`home.value.items.${item.key}.title`) }}</h3>
              <p>{{ t(`home.value.items.${item.key}.desc`) }}</p>
            </article>
          </div>
        </div>
      </section>

      <!-- ============ WORKFLOW ============ -->
      <section id="workflow" class="pk-section">
        <div class="pk-container">
          <div class="pk-section-head" data-reveal>
            <span class="pk-index">02</span>
            <span class="pk-kicker">{{ t('home.workflow.kicker') }}</span>
            <h2 class="pk-section-title">{{ t('home.workflow.title') }}</h2>
            <p class="pk-section-sub">{{ t('home.workflow.subtitle') }}</p>
          </div>

          <div class="pk-steps">
            <article
              v-for="(step, i) in workflowSteps"
              :key="step.key"
              class="pk-step"
              data-reveal
              :style="{ '--rd': i * 90 + 'ms' }"
            >
              <span class="pk-step-num">{{ String(i + 1).padStart(2, '0') }}</span>
              <h3>{{ t(`home.workflow.steps.${step.key}.title`) }}</h3>
              <p>{{ t(`home.workflow.steps.${step.key}.desc`) }}</p>
              <code v-if="step.code" class="pk-step-code">{{ step.code }}</code>
            </article>
          </div>
        </div>
      </section>

      <!-- ============ ECOSYSTEM ============ -->
      <section id="ecosystem" class="pk-section">
        <div class="pk-container">
          <div class="pk-section-head" data-reveal>
            <span class="pk-index">03</span>
            <span class="pk-kicker">{{ t('home.ecosystem.kicker') }}</span>
            <h2 class="pk-section-title">{{ t('home.ecosystem.title') }}</h2>
          </div>

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
          <div class="pk-section-head" data-reveal>
            <span class="pk-index">04</span>
            <span class="pk-kicker">{{ t('home.pricing.kicker') }}</span>
            <h2 class="pk-section-title">{{ t('home.pricing.title') }}</h2>
            <p class="pk-section-sub">{{ t('home.pricing.subtitle') }}</p>
          </div>

          <div class="pk-rate" data-reveal>
            <div class="pk-rate-main">
              <span class="pk-rate-label">{{ t('home.pricing.rateLabel') }} · {{ t('home.pricing.badge') }}</span>
              <div class="pk-rate-value">{{ t('home.pricing.rateValue') }}</div>
              <div class="pk-rate-ref">
                {{ t('home.pricing.officialLabel') }} <s>{{ t('home.pricing.officialValue') }}</s>
              </div>
            </div>
            <div class="pk-rate-aside">
              <p class="pk-rate-note">{{ t('home.pricing.note') }}</p>
              <router-link :to="isAuthenticated ? dashboardPath : '/register'" class="pk-btn pk-btn--lg">
                {{ t('home.pricing.cta') }}
                <Icon name="arrowRight" size="sm" :stroke-width="1.8" />
              </router-link>
            </div>
          </div>
        </div>
      </section>

      <!-- ============ CTA ============ -->
      <section class="pk-section pk-section--cta">
        <div class="pk-container" data-reveal>
          <h2 class="pk-cta-title">{{ t('home.cta.title') }}</h2>
          <p class="pk-cta-desc">{{ t('home.cta.description') }}</p>
          <router-link :to="isAuthenticated ? dashboardPath : '/register'" class="pk-btn pk-btn--lg">
            {{ t('home.cta.button') }}
            <Icon name="arrowRight" size="sm" :stroke-width="1.8" />
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
   Poke API home — monochrome editorial (ink & paper).
   Light: warm paper + near-black ink.
   Dark:  near-black ink + off-white paper. (html.dark)
   Restraint: hairlines, mono labels, one red dot.
   ===================================================== */
.pk-page {
  /* light (paper) */
  --ink: oklch(0.2 0.006 80);
  --ink-mute: oklch(0.44 0.008 80);
  --ink-faint: oklch(0.6 0.008 80);
  --paper: oklch(0.955 0.006 90);
  --paper-raise: oklch(0.93 0.007 90);
  --line: oklch(0.2 0.006 80 / 0.16);
  --line-strong: oklch(0.2 0.006 80 / 0.4);
  --red: oklch(0.58 0.2 27);

  --ease: cubic-bezier(0.22, 1, 0.36, 1);
  --mono: ui-monospace, 'Cascadia Code', 'JetBrains Mono', Menlo, Consolas, monospace;

  min-height: 100vh;
  background: var(--paper);
  color: var(--ink);
  overflow-x: clip;
  transition:
    background-color 0.3s ease,
    color 0.3s ease;
}
html.dark .pk-page {
  --ink: oklch(0.93 0.004 90);
  --ink-mute: oklch(0.68 0.005 90);
  --ink-faint: oklch(0.52 0.006 90);
  --paper: oklch(0.165 0.004 80);
  --paper-raise: oklch(0.2 0.005 80);
  --line: oklch(0.93 0.004 90 / 0.14);
  --line-strong: oklch(0.93 0.004 90 / 0.38);
  --red: oklch(0.62 0.2 27);
}

/* film grain, both themes */
.pk-noise {
  position: fixed;
  inset: 0;
  z-index: 1;
  pointer-events: none;
  opacity: 0.05;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='160'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='2'/%3E%3C/filter%3E%3Crect width='160' height='160' filter='url(%23n)'/%3E%3C/svg%3E");
}

.pk-container {
  max-width: 1180px;
  margin-inline: auto;
  padding-inline: 28px;
}

/* ---------- motion ---------- */
.pk-enter {
  opacity: 0;
  transform: translateY(16px);
  animation: pk-rise 0.8s var(--ease) forwards;
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
  transform: translateY(22px);
  transition:
    opacity 0.7s var(--ease),
    transform 0.7s var(--ease);
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
  background: color-mix(in oklab, var(--paper) 88%, transparent);
  backdrop-filter: blur(12px);
  border-bottom-color: var(--line);
}
.pk-nav-inner {
  max-width: 1180px;
  margin-inline: auto;
  padding: 16px 28px;
  display: flex;
  align-items: center;
  gap: 26px;
}
.pk-brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
  color: inherit;
}
.pk-brand-logo {
  width: 26px;
  height: 26px;
  object-fit: contain;
  filter: grayscale(1) contrast(1.1);
}
.pk-brand-name {
  font-weight: 700;
  font-size: 15px;
  letter-spacing: 0.02em;
}
.pk-nav-links {
  display: none;
  gap: 26px;
  margin-inline: auto;
}
@media (min-width: 880px) {
  .pk-nav-links {
    display: flex;
  }
}
.pk-nav-links a {
  position: relative;
  font-family: var(--mono);
  font-size: 12px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--ink-mute);
  text-decoration: none;
  padding: 4px 0;
  transition: color 0.2s ease;
}
.pk-nav-links a::after {
  content: '';
  position: absolute;
  left: 0;
  bottom: 0;
  width: 100%;
  height: 1px;
  background: var(--ink);
  transform: scaleX(0);
  transform-origin: right;
  transition: transform 0.3s var(--ease);
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
  letter-spacing: 0.1em;
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
  width: 32px;
  height: 32px;
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
  transform: scale(0.92);
}
.pk-nav-actions :deep(button) {
  color: var(--ink-mute);
}
.pk-nav-actions :deep(button:hover) {
  color: var(--ink);
  background: color-mix(in oklab, var(--ink) 7%, transparent);
}

/* ---------- buttons ---------- */
.pk-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  font-weight: 600;
  border: 1px solid var(--ink);
  background: var(--ink);
  color: var(--paper);
  text-decoration: none;
  border-radius: 999px;
  cursor: pointer;
  transition:
    background-color 0.22s ease,
    color 0.22s ease,
    transform 0.15s var(--ease);
}
.pk-btn:hover {
  background: transparent;
  color: var(--ink);
}
.pk-btn:active {
  transform: scale(0.97);
}
.pk-btn--sm {
  font-size: 12.5px;
  padding: 7px 16px;
}
.pk-btn--lg {
  font-size: 14.5px;
  padding: 13px 28px;
}
.pk-linkline {
  position: relative;
  font-size: 14.5px;
  font-weight: 600;
  color: var(--ink);
  text-decoration: none;
  padding-bottom: 3px;
  align-self: center;
}
.pk-linkline::after {
  content: '';
  position: absolute;
  left: 0;
  bottom: 0;
  width: 100%;
  height: 1px;
  background: var(--ink);
  transform-origin: right;
  transition: transform 0.3s var(--ease);
}
.pk-linkline:hover::after {
  transform: scaleX(0.35);
  transform-origin: left;
}

/* ---------- hero ---------- */
.pk-hero {
  position: relative;
  z-index: 2;
  padding: 104px 0 96px;
  border-bottom: 1px solid var(--line);
}
.pk-hero-kicker {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-family: var(--mono);
  font-size: 12px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-mute);
  margin: 0 0 34px;
}
.pk-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--red);
}
.pk-hero-title {
  font-size: clamp(2.9rem, 8vw, 6rem);
  font-weight: 800;
  line-height: 1.04;
  letter-spacing: -0.035em;
  margin: 0 0 56px;
}
.pk-hero-line {
  display: block;
}
.pk-hero-line--em {
  color: var(--ink-faint);
}
.pk-hero-below {
  display: grid;
  gap: 44px;
  align-items: start;
}
@media (min-width: 960px) {
  .pk-hero-below {
    grid-template-columns: 0.9fr 1.1fr;
    gap: 72px;
  }
}
.pk-hero-desc {
  font-size: 16.5px;
  line-height: 2;
  color: var(--ink-mute);
  max-width: 30em;
  margin: 0;
  border-top: 1px solid var(--line);
  padding-top: 26px;
}
.pk-hero-side {
  display: grid;
  gap: 30px;
  border-top: 1px solid var(--line);
  padding-top: 26px;
}
.pk-hero-ctas {
  display: flex;
  flex-wrap: wrap;
  gap: 26px;
}

/* endpoints */
.pk-endpoints {
  display: grid;
}
.pk-endpoint {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 13px 2px;
  border-bottom: 1px solid var(--line);
}
.pk-endpoint:first-child {
  border-top: 1px solid var(--line);
}
.pk-endpoint-label {
  font-family: var(--mono);
  font-size: 10.5px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--ink-faint);
  white-space: nowrap;
  min-width: 128px;
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
  font-size: 11px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-mute);
  background: transparent;
  border: 1px solid var(--line);
  border-radius: 999px;
  padding: 5px 12px;
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
  z-index: 2;
  padding: 96px 0;
  border-bottom: 1px solid var(--line);
}
.pk-section-head {
  display: grid;
  grid-template-columns: auto 1fr;
  column-gap: 18px;
  row-gap: 10px;
  align-items: baseline;
  margin-bottom: 60px;
}
.pk-index {
  font-family: var(--mono);
  font-size: 12px;
  letter-spacing: 0.1em;
  color: var(--ink-faint);
}
.pk-kicker {
  font-family: var(--mono);
  font-size: 12px;
  letter-spacing: 0.22em;
  text-transform: uppercase;
  color: var(--ink-mute);
}
.pk-section-title {
  grid-column: 2;
  font-size: clamp(1.8rem, 4vw, 2.8rem);
  font-weight: 800;
  letter-spacing: -0.025em;
  line-height: 1.15;
  margin: 0;
}
.pk-section-sub {
  grid-column: 2;
  font-size: 15px;
  color: var(--ink-mute);
  line-height: 1.9;
  margin: 6px 0 0;
  max-width: 40em;
}

/* ---------- value (hairline ledger) ---------- */
.pk-value-list {
  display: grid;
}
.pk-value-row {
  display: grid;
  grid-template-columns: 64px 260px 1fr;
  gap: 20px;
  align-items: baseline;
  padding: 30px 2px;
  border-top: 1px solid var(--line);
  transition: background-color 0.25s ease;
}
.pk-value-row:last-child {
  border-bottom: 1px solid var(--line);
}
.pk-value-row:hover {
  background: color-mix(in oklab, var(--ink) 4%, transparent);
}
@media (max-width: 820px) {
  .pk-value-row {
    grid-template-columns: 52px 1fr;
  }
  .pk-value-row p {
    grid-column: 2;
  }
}
.pk-value-num {
  font-family: var(--mono);
  font-size: 12px;
  color: var(--ink-faint);
}
.pk-value-row h3 {
  font-size: 18px;
  font-weight: 700;
  letter-spacing: -0.01em;
  margin: 0;
}
.pk-value-row p {
  font-size: 14px;
  line-height: 1.95;
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
  padding: 8px 28px 8px 0;
}
.pk-step + .pk-step {
  border-left: 1px solid var(--line);
  padding-left: 28px;
}
@media (max-width: 820px) {
  .pk-step + .pk-step {
    border-left: none;
    padding-left: 0;
    border-top: 1px solid var(--line);
    margin-top: 26px;
    padding-top: 26px;
  }
}
.pk-step-num {
  display: block;
  font-family: var(--mono);
  font-size: 12px;
  color: var(--ink-faint);
  margin-bottom: 20px;
}
.pk-step h3 {
  font-size: 17px;
  font-weight: 700;
  margin: 0 0 12px;
}
.pk-step p {
  font-size: 14px;
  line-height: 1.95;
  color: var(--ink-mute);
  margin: 0;
}
.pk-step-code {
  display: block;
  margin-top: 18px;
  padding: 10px 0;
  border-top: 1px solid var(--line);
  font-family: var(--mono);
  font-size: 12px;
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
  gap: 10px;
  padding: 14px 26px 14px 0;
  margin-right: 26px;
  font-family: var(--mono);
  font-size: 13px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--ink);
  border-bottom: 1px solid var(--line);
  transition: border-color 0.25s ease;
}
.pk-model:hover {
  border-bottom-color: var(--ink);
}
.pk-model-icon {
  filter: grayscale(1) contrast(1.05);
  opacity: 0.85;
}
.pk-model--more {
  color: var(--ink-faint);
}

/* ---------- pricing ---------- */
.pk-rate {
  display: grid;
  gap: 40px;
  align-items: end;
}
@media (min-width: 880px) {
  .pk-rate {
    grid-template-columns: 1.2fr 0.8fr;
    gap: 72px;
  }
}
.pk-rate-label {
  display: block;
  font-family: var(--mono);
  font-size: 12px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-mute);
  margin-bottom: 18px;
}
.pk-rate-value {
  font-size: clamp(3rem, 9vw, 6.4rem);
  font-weight: 800;
  letter-spacing: -0.04em;
  line-height: 1;
  margin-bottom: 16px;
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
  padding-top: 24px;
}
.pk-rate-note {
  font-size: 14px;
  line-height: 2;
  color: var(--ink-mute);
  margin: 0 0 26px;
}

/* ---------- CTA ---------- */
.pk-section--cta {
  text-align: center;
  padding: 130px 0;
}
.pk-cta-title {
  font-size: clamp(2.2rem, 6vw, 4.2rem);
  font-weight: 800;
  letter-spacing: -0.03em;
  line-height: 1.08;
  margin: 0 0 18px;
}
.pk-cta-desc {
  font-size: 15.5px;
  color: var(--ink-mute);
  line-height: 1.9;
  margin: 0 0 40px;
}

/* ---------- footer ---------- */
.pk-footer {
  position: relative;
  z-index: 2;
  padding: 30px 0;
}
.pk-footer-inner {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  font-family: var(--mono);
  font-size: 11.5px;
  letter-spacing: 0.08em;
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
