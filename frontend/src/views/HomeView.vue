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

  <!-- Default Home Page (dark-only, self-contained palette) -->
  <div v-else class="pk-page">
    <!-- Header -->
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
          <router-link v-if="isAuthenticated" :to="dashboardPath" class="pk-btn pk-btn--primary pk-btn--sm">
            {{ t('home.dashboard') }}
          </router-link>
          <template v-else>
            <router-link to="/login" class="pk-nav-login">{{ t('home.login') }}</router-link>
            <router-link to="/register" class="pk-btn pk-btn--primary pk-btn--sm">
              {{ t('home.getStarted') }}
            </router-link>
          </template>
        </div>
      </nav>
    </header>

    <main id="top">
      <!-- ============ HERO ============ -->
      <section class="pk-hero">
        <div class="pk-hero-glow" aria-hidden="true"></div>
        <div class="pk-hero-grid" aria-hidden="true"></div>

        <div class="pk-container pk-hero-layout">
          <div class="pk-hero-copy">
            <div class="pk-badge pk-enter" style="--d: 0ms">
              <span class="pk-badge-dot"></span>
              {{ t('home.hero.badge') }}
            </div>

            <h1 class="pk-hero-title pk-enter" style="--d: 90ms">
              {{ t('home.hero.titleLine1') }}<br />
              <span class="pk-hero-title-em">{{ t('home.hero.titleLine2') }}</span>
            </h1>

            <p class="pk-hero-desc pk-enter" style="--d: 180ms">
              {{ t('home.hero.description') }}
            </p>

            <div class="pk-hero-ctas pk-enter" style="--d: 270ms">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/register'"
                class="pk-btn pk-btn--primary pk-btn--lg"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.hero.ctaPrimary') }}
                <Icon name="arrowRight" size="sm" :stroke-width="2" />
              </router-link>
              <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="pk-btn pk-btn--ghost pk-btn--lg">
                {{ t('home.hero.ctaDocs') }}
              </a>
            </div>

            <!-- Base URL boxes -->
            <div class="pk-endpoints pk-enter" style="--d: 360ms">
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
                  <Icon :name="copiedKey === ep.key ? 'check' : 'copy'" size="xs" :stroke-width="2" />
                  <span>{{ copiedKey === ep.key ? t('home.hero.copied') : t('home.hero.copy') }}</span>
                </button>
              </div>
            </div>
          </div>

          <!-- Right: stacked feature cards -->
          <div class="pk-hero-side">
            <div
              v-for="(card, i) in heroCards"
              :key="card.key"
              class="pk-side-card pk-enter"
              :style="{ '--d': 240 + i * 110 + 'ms' }"
            >
              <div class="pk-side-icon">
                <Icon :name="card.icon" size="md" :stroke-width="1.6" />
              </div>
              <div>
                <div class="pk-side-title">{{ t(`home.hero.cards.${card.key}.title`) }}</div>
                <div class="pk-side-desc">{{ t(`home.hero.cards.${card.key}.desc`) }}</div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- ============ VALUE ============ -->
      <section id="value" class="pk-section">
        <div class="pk-container">
          <div class="pk-section-head" data-reveal>
            <span class="pk-kicker">{{ t('home.value.kicker') }}</span>
            <h2 class="pk-section-title">{{ t('home.value.title') }}</h2>
            <p class="pk-section-sub">{{ t('home.value.subtitle') }}</p>
          </div>

          <div class="pk-value-grid">
            <article
              v-for="(item, i) in valueItems"
              :key="item.key"
              class="pk-value-card"
              data-reveal
              :style="{ '--rd': (i % 2) * 90 + 'ms' }"
            >
              <div class="pk-value-icon">
                <Icon :name="item.icon" size="lg" :stroke-width="1.5" />
              </div>
              <h3>{{ t(`home.value.items.${item.key}.title`) }}</h3>
              <p>{{ t(`home.value.items.${item.key}.desc`) }}</p>
            </article>
          </div>
        </div>
      </section>

      <!-- ============ WORKFLOW ============ -->
      <section id="workflow" class="pk-section pk-section--alt">
        <div class="pk-container">
          <div class="pk-section-head" data-reveal>
            <span class="pk-kicker">{{ t('home.workflow.kicker') }}</span>
            <h2 class="pk-section-title">{{ t('home.workflow.title') }}</h2>
            <p class="pk-section-sub">{{ t('home.workflow.subtitle') }}</p>
          </div>

          <div class="pk-steps">
            <article
              v-for="(step, i) in workflowSteps"
              :key="step.key"
              class="pk-step"
              :class="{ 'pk-step--focus': i === 1 }"
              data-reveal
              :style="{ '--rd': i * 110 + 'ms' }"
            >
              <div class="pk-step-num">{{ i + 1 }}</div>
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
            <span class="pk-kicker">{{ t('home.ecosystem.kicker') }}</span>
            <h2 class="pk-section-title">{{ t('home.ecosystem.title') }}</h2>
            <p class="pk-section-sub">{{ t('home.ecosystem.subtitle') }}</p>
          </div>

          <div class="pk-models" data-reveal>
            <div v-for="m in models" :key="m.label" class="pk-model">
              <ModelIcon :model="m.icon" size="20px" />
              <span>{{ m.label }}</span>
            </div>
            <div class="pk-model pk-model--more">
              <Icon name="plus" size="sm" :stroke-width="2" />
              <span>{{ t('home.ecosystem.more') }}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- ============ PRICING ============ -->
      <section id="pricing" class="pk-section pk-section--alt">
        <div class="pk-container">
          <div class="pk-section-head" data-reveal>
            <span class="pk-kicker">{{ t('home.pricing.kicker') }}</span>
            <h2 class="pk-section-title">{{ t('home.pricing.title') }}</h2>
            <p class="pk-section-sub">{{ t('home.pricing.subtitle') }}</p>
          </div>

          <div class="pk-pricing" data-reveal>
            <div class="pk-rate-card">
              <span class="pk-rate-badge">{{ t('home.pricing.badge') }}</span>
              <div class="pk-rate-label">{{ t('home.pricing.rateLabel') }}</div>
              <div class="pk-rate-value">{{ t('home.pricing.rateValue') }}</div>
              <div class="pk-rate-ref">
                {{ t('home.pricing.officialLabel') }}：<s>{{ t('home.pricing.officialValue') }}</s>
              </div>
              <p class="pk-rate-note">{{ t('home.pricing.note') }}</p>
              <router-link :to="isAuthenticated ? dashboardPath : '/register'" class="pk-btn pk-btn--primary pk-btn--lg pk-rate-cta">
                {{ t('home.pricing.cta') }}
                <Icon name="arrowRight" size="sm" :stroke-width="2" />
              </router-link>
            </div>
          </div>
        </div>
      </section>

      <!-- ============ CTA ============ -->
      <section class="pk-section">
        <div class="pk-container">
          <div class="pk-cta" data-reveal>
            <div class="pk-cta-glow" aria-hidden="true"></div>
            <h2>{{ t('home.cta.title') }}</h2>
            <p>{{ t('home.cta.description') }}</p>
            <router-link :to="isAuthenticated ? dashboardPath : '/register'" class="pk-btn pk-btn--primary pk-btn--lg">
              {{ t('home.cta.button') }}
              <Icon name="arrowRight" size="sm" :stroke-width="2" />
            </router-link>
          </div>
        </div>
      </section>
    </main>

    <!-- Footer -->
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

// ---- Data ----
const endpoints = [
  { key: 'openai', labelKey: 'home.hero.baseUrlOpenai', url: `${window.location.origin}/v1` },
  { key: 'anthropic', labelKey: 'home.hero.baseUrlAnthropic', url: window.location.origin }
]

const heroCards = [
  { key: 'routing', icon: 'bolt' as const },
  { key: 'observability', icon: 'chartBar' as const },
  { key: 'billing', icon: 'creditCard' as const }
]

const valueItems = [
  { key: 'unified', icon: 'key' as const },
  { key: 'observability', icon: 'chartBar' as const },
  { key: 'elastic', icon: 'bolt' as const },
  { key: 'developer', icon: 'terminal' as const }
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
    // Fallback for non-secure contexts
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
    { threshold: 0.15, rootMargin: '0px 0px -40px 0px' }
  )
  targets.forEach((el) => observer?.observe(el))
}

onMounted(() => {
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  window.addEventListener('scroll', onScroll, { passive: true })
  onScroll()
  // Wait one frame so v-if branches settle before observing
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
   Poke API home — dark-only, self-contained palette.
   Neutrals tilted toward the brand red hue (OKLCH h≈25).
   ===================================================== */
.pk-page {
  /* surfaces */
  --pk-bg: oklch(0.16 0.008 25);
  --pk-bg-raise: oklch(0.19 0.01 25);
  --pk-bg-card: oklch(0.21 0.012 25);
  --pk-border: oklch(0.3 0.012 25);
  --pk-border-strong: oklch(0.38 0.015 25);
  /* text */
  --pk-fg: oklch(0.95 0.005 25);
  --pk-fg-mute: oklch(0.72 0.01 25);
  --pk-fg-faint: oklch(0.56 0.012 25);
  /* brand */
  --pk-red: oklch(0.6 0.21 27);
  --pk-red-soft: oklch(0.6 0.21 27 / 0.14);
  --pk-red-line: oklch(0.6 0.21 27 / 0.35);
  --pk-green: oklch(0.75 0.15 160);
  /* motion */
  --ease-out-quint: cubic-bezier(0.22, 1, 0.36, 1);
  --ease-out-expo: cubic-bezier(0.16, 1, 0.3, 1);
  --pk-mono: ui-monospace, 'Cascadia Code', 'JetBrains Mono', Menlo, Consolas, monospace;

  min-height: 100vh;
  background: var(--pk-bg);
  color: var(--pk-fg);
  font-feature-settings: 'ss01';
  overflow-x: clip;
}

.pk-container {
  max-width: 1120px;
  margin-inline: auto;
  padding-inline: 24px;
}

/* ---------- entrance animation (hero) ---------- */
.pk-enter {
  opacity: 0;
  transform: translateY(18px);
  animation: pk-rise 0.7s var(--ease-out-quint) forwards;
  animation-delay: var(--d, 0ms);
}
@keyframes pk-rise {
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* ---------- scroll reveal ---------- */
[data-reveal] {
  opacity: 0;
  transform: translateY(24px);
  transition:
    opacity 0.65s var(--ease-out-quint),
    transform 0.65s var(--ease-out-quint);
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
    border-color 0.25s ease,
    backdrop-filter 0.25s ease;
}
.pk-nav--scrolled {
  background: oklch(0.16 0.008 25 / 0.82);
  backdrop-filter: blur(14px);
  border-bottom-color: var(--pk-border);
}
.pk-nav-inner {
  max-width: 1120px;
  margin-inline: auto;
  padding: 14px 24px;
  display: flex;
  align-items: center;
  gap: 28px;
}
.pk-brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
  color: inherit;
}
.pk-brand-logo {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  object-fit: contain;
}
.pk-brand-name {
  font-weight: 700;
  font-size: 16px;
  letter-spacing: 0.01em;
}
.pk-nav-links {
  display: none;
  gap: 4px;
  margin-inline: auto;
}
@media (min-width: 860px) {
  .pk-nav-links {
    display: flex;
  }
}
.pk-nav-links a {
  padding: 7px 13px;
  border-radius: 8px;
  font-size: 14px;
  color: var(--pk-fg-mute);
  text-decoration: none;
  transition:
    color 0.18s ease,
    background-color 0.18s ease;
}
.pk-nav-links a:hover {
  color: var(--pk-fg);
  background: oklch(1 0 0 / 0.06);
}
.pk-nav-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-left: auto;
}
.pk-nav-login {
  font-size: 14px;
  color: var(--pk-fg-mute);
  text-decoration: none;
  padding: 7px 10px;
  border-radius: 8px;
  transition: color 0.18s ease;
}
.pk-nav-login:hover {
  color: var(--pk-fg);
}
/* LocaleSwitcher was designed for light backgrounds; nudge it */
.pk-nav-actions :deep(button) {
  color: var(--pk-fg-mute);
}
.pk-nav-actions :deep(button:hover) {
  background: oklch(1 0 0 / 0.07);
  color: var(--pk-fg);
}

/* ---------- buttons ---------- */
.pk-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-weight: 600;
  border-radius: 10px;
  text-decoration: none;
  border: 1px solid transparent;
  cursor: pointer;
  transition:
    transform 0.15s var(--ease-out-quint),
    background-color 0.2s ease,
    border-color 0.2s ease,
    box-shadow 0.2s ease;
  will-change: transform;
}
.pk-btn:hover {
  transform: translateY(-1px);
}
.pk-btn:active {
  transform: translateY(0) scale(0.97);
}
.pk-btn--sm {
  font-size: 13px;
  padding: 7px 14px;
}
.pk-btn--lg {
  font-size: 15px;
  padding: 12px 24px;
  border-radius: 12px;
}
.pk-btn--primary {
  background: var(--pk-red);
  color: oklch(0.98 0.005 25);
  box-shadow: 0 8px 24px -10px var(--pk-red-line);
}
.pk-btn--primary:hover {
  box-shadow: 0 12px 28px -10px var(--pk-red-line);
  filter: brightness(1.07);
}
.pk-btn--ghost {
  background: transparent;
  border-color: var(--pk-border-strong);
  color: var(--pk-fg);
}
.pk-btn--ghost:hover {
  border-color: var(--pk-fg-faint);
  background: oklch(1 0 0 / 0.04);
}

/* ---------- hero ---------- */
.pk-hero {
  position: relative;
  padding: 84px 0 72px;
}
.pk-hero-glow {
  position: absolute;
  top: -180px;
  left: 50%;
  transform: translateX(-50%);
  width: 780px;
  height: 480px;
  background: radial-gradient(ellipse at center, var(--pk-red-soft), transparent 65%);
  pointer-events: none;
}
.pk-hero-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(oklch(1 0 0 / 0.028) 1px, transparent 1px),
    linear-gradient(90deg, oklch(1 0 0 / 0.028) 1px, transparent 1px);
  background-size: 56px 56px;
  -webkit-mask-image: radial-gradient(ellipse 75% 60% at 50% 0%, #000 35%, transparent 100%);
  mask-image: radial-gradient(ellipse 75% 60% at 50% 0%, #000 35%, transparent 100%);
  pointer-events: none;
}
.pk-hero-layout {
  position: relative;
  display: grid;
  grid-template-columns: 1fr;
  gap: 56px;
  align-items: center;
}
@media (min-width: 960px) {
  .pk-hero-layout {
    grid-template-columns: 1.15fr 0.85fr;
  }
}
.pk-badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: oklch(0.78 0.12 27);
  border: 1px solid var(--pk-red-line);
  background: var(--pk-red-soft);
  padding: 6px 14px;
  border-radius: 999px;
  margin-bottom: 26px;
}
.pk-badge-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--pk-red);
  animation: pk-pulse 2.4s ease-in-out infinite;
}
@keyframes pk-pulse {
  0%,
  100% {
    box-shadow: 0 0 0 0 var(--pk-red-line);
  }
  50% {
    box-shadow: 0 0 0 5px transparent;
  }
}
@media (prefers-reduced-motion: reduce) {
  .pk-badge-dot {
    animation: none;
  }
}
.pk-hero-title {
  font-size: clamp(2.4rem, 5.4vw, 3.9rem);
  font-weight: 800;
  line-height: 1.12;
  letter-spacing: -0.02em;
  margin: 0 0 22px;
}
.pk-hero-title-em {
  color: var(--pk-red);
}
.pk-hero-desc {
  font-size: 17px;
  line-height: 1.85;
  color: var(--pk-fg-mute);
  max-width: 34em;
  margin: 0 0 34px;
}
.pk-hero-ctas {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  margin-bottom: 40px;
}

/* endpoints */
.pk-endpoints {
  display: grid;
  gap: 10px;
  max-width: 560px;
}
.pk-endpoint {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border: 1px solid var(--pk-border);
  border-radius: 12px;
  background: var(--pk-bg-raise);
  transition: border-color 0.2s ease;
}
.pk-endpoint:hover {
  border-color: var(--pk-border-strong);
}
.pk-endpoint-label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--pk-fg-faint);
  white-space: nowrap;
}
.pk-endpoint-url {
  font-family: var(--pk-mono);
  font-size: 13.5px;
  color: var(--pk-fg);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}
.pk-copy {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  font-weight: 600;
  color: var(--pk-fg-mute);
  background: oklch(1 0 0 / 0.05);
  border: 1px solid var(--pk-border);
  border-radius: 8px;
  padding: 5px 10px;
  cursor: pointer;
  white-space: nowrap;
  transition:
    color 0.18s ease,
    border-color 0.18s ease,
    background-color 0.18s ease,
    transform 0.12s var(--ease-out-quint);
}
.pk-copy:hover {
  color: var(--pk-fg);
  border-color: var(--pk-border-strong);
}
.pk-copy:active {
  transform: scale(0.94);
}
.pk-copy--done {
  color: var(--pk-green);
  border-color: oklch(0.75 0.15 160 / 0.4);
  background: oklch(0.75 0.15 160 / 0.1);
}

/* hero side cards */
.pk-hero-side {
  display: grid;
  gap: 14px;
}
.pk-side-card {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  padding: 20px 22px;
  border: 1px solid var(--pk-border);
  border-radius: 14px;
  background: var(--pk-bg-card);
  transition:
    transform 0.25s var(--ease-out-quint),
    border-color 0.25s ease,
    box-shadow 0.25s ease;
}
.pk-side-card:hover {
  transform: translateY(-3px);
  border-color: var(--pk-border-strong);
  box-shadow: 0 16px 36px -20px oklch(0 0 0 / 0.7);
}
.pk-side-card:nth-child(2) {
  margin-left: 26px;
}
@media (max-width: 959px) {
  .pk-side-card:nth-child(2) {
    margin-left: 0;
  }
}
.pk-side-icon {
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  color: var(--pk-red);
  background: var(--pk-red-soft);
  border: 1px solid var(--pk-red-line);
}
.pk-side-title {
  font-weight: 700;
  font-size: 15px;
  margin-bottom: 4px;
}
.pk-side-desc {
  font-size: 13.5px;
  line-height: 1.7;
  color: var(--pk-fg-mute);
}

/* ---------- sections ---------- */
.pk-section {
  padding: 88px 0;
}
.pk-section--alt {
  background: var(--pk-bg-raise);
  border-block: 1px solid var(--pk-border);
}
.pk-section-head {
  max-width: 560px;
  margin-bottom: 52px;
}
.pk-kicker {
  display: inline-block;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.16em;
  color: var(--pk-red);
  margin-bottom: 14px;
}
.pk-section-title {
  font-size: clamp(1.7rem, 3.2vw, 2.3rem);
  font-weight: 800;
  letter-spacing: -0.015em;
  line-height: 1.25;
  margin: 0 0 12px;
}
.pk-section-sub {
  font-size: 15.5px;
  color: var(--pk-fg-mute);
  line-height: 1.8;
  margin: 0;
}

/* ---------- value grid ---------- */
.pk-value-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 18px;
}
.pk-value-card {
  padding: 30px 28px;
  border: 1px solid var(--pk-border);
  border-radius: 16px;
  background: var(--pk-bg-card);
  transition:
    transform 0.25s var(--ease-out-quint),
    border-color 0.25s ease,
    box-shadow 0.25s ease;
}
.pk-value-card:hover {
  transform: translateY(-4px);
  border-color: var(--pk-border-strong);
  box-shadow: 0 20px 44px -24px oklch(0 0 0 / 0.8);
}
.pk-value-icon {
  width: 44px;
  height: 44px;
  border-radius: 11px;
  display: grid;
  place-items: center;
  color: var(--pk-red);
  background: var(--pk-red-soft);
  border: 1px solid var(--pk-red-line);
  margin-bottom: 20px;
  transition: transform 0.25s var(--ease-out-quint);
}
.pk-value-card:hover .pk-value-icon {
  transform: scale(1.08);
}
.pk-value-card h3 {
  font-size: 17px;
  font-weight: 700;
  margin: 0 0 10px;
}
.pk-value-card p {
  font-size: 14px;
  line-height: 1.85;
  color: var(--pk-fg-mute);
  margin: 0;
}

/* ---------- workflow ---------- */
.pk-steps {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 18px;
  align-items: stretch;
}
.pk-step {
  position: relative;
  padding: 30px 28px;
  border: 1px solid var(--pk-border);
  border-radius: 16px;
  background: var(--pk-bg-card);
  transition:
    transform 0.25s var(--ease-out-quint),
    border-color 0.25s ease;
}
.pk-step:hover {
  transform: translateY(-4px);
  border-color: var(--pk-border-strong);
}
.pk-step--focus {
  border-color: var(--pk-red-line);
  box-shadow: 0 0 44px -18px var(--pk-red-line);
}
.pk-step-num {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  display: grid;
  place-items: center;
  font-family: var(--pk-mono);
  font-weight: 700;
  font-size: 15px;
  color: var(--pk-red);
  background: var(--pk-red-soft);
  border: 1px solid var(--pk-red-line);
  margin-bottom: 18px;
}
.pk-step h3 {
  font-size: 16.5px;
  font-weight: 700;
  margin: 0 0 10px;
}
.pk-step p {
  font-size: 14px;
  line-height: 1.85;
  color: var(--pk-fg-mute);
  margin: 0;
}
.pk-step-code {
  display: block;
  margin-top: 16px;
  padding: 10px 12px;
  border-radius: 9px;
  background: var(--pk-bg);
  border: 1px solid var(--pk-border);
  font-family: var(--pk-mono);
  font-size: 12px;
  color: var(--pk-fg-mute);
  overflow-x: auto;
  white-space: nowrap;
}

/* ---------- ecosystem ---------- */
.pk-models {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.pk-model {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 12px 20px;
  border: 1px solid var(--pk-border);
  border-radius: 12px;
  background: var(--pk-bg-card);
  font-size: 14.5px;
  font-weight: 600;
  transition:
    transform 0.2s var(--ease-out-quint),
    border-color 0.2s ease;
}
.pk-model:hover {
  transform: translateY(-2px);
  border-color: var(--pk-border-strong);
}
.pk-model--more {
  color: var(--pk-fg-faint);
  border-style: dashed;
  background: transparent;
}

/* ---------- pricing ---------- */
.pk-pricing {
  display: flex;
  justify-content: center;
}
.pk-rate-card {
  position: relative;
  width: min(460px, 100%);
  text-align: center;
  padding: 44px 36px 40px;
  border: 1px solid var(--pk-red-line);
  border-radius: 20px;
  background: var(--pk-bg-card);
  box-shadow: 0 0 70px -30px var(--pk-red-line);
}
.pk-rate-badge {
  position: absolute;
  top: -13px;
  left: 50%;
  transform: translateX(-50%);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.06em;
  color: oklch(0.98 0.005 25);
  background: var(--pk-red);
  padding: 5px 16px;
  border-radius: 999px;
  white-space: nowrap;
}
.pk-rate-label {
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--pk-fg-faint);
  margin-bottom: 10px;
}
.pk-rate-value {
  font-size: clamp(2rem, 4.6vw, 2.9rem);
  font-weight: 800;
  letter-spacing: -0.02em;
  color: var(--pk-fg);
  margin-bottom: 10px;
}
.pk-rate-ref {
  font-size: 14px;
  color: var(--pk-fg-faint);
  margin-bottom: 18px;
}
.pk-rate-ref s {
  color: var(--pk-fg-faint);
}
.pk-rate-note {
  font-size: 13.5px;
  line-height: 1.8;
  color: var(--pk-fg-mute);
  margin: 0 0 28px;
}
.pk-rate-cta {
  width: 100%;
}

/* ---------- CTA ---------- */
.pk-cta {
  position: relative;
  text-align: center;
  padding: 72px 32px;
  border: 1px solid var(--pk-border);
  border-radius: 24px;
  background: var(--pk-bg-raise);
  overflow: hidden;
}
.pk-cta-glow {
  position: absolute;
  inset: auto -20% -70% -20%;
  height: 250px;
  background: radial-gradient(ellipse at center, var(--pk-red-soft), transparent 70%);
  pointer-events: none;
}
.pk-cta h2 {
  position: relative;
  font-size: clamp(1.6rem, 3vw, 2.2rem);
  font-weight: 800;
  letter-spacing: -0.015em;
  margin: 0 0 14px;
}
.pk-cta p {
  position: relative;
  font-size: 15.5px;
  color: var(--pk-fg-mute);
  margin: 0 0 30px;
}
.pk-cta .pk-btn {
  position: relative;
}

/* ---------- footer ---------- */
.pk-footer {
  border-top: 1px solid var(--pk-border);
  padding: 30px 0;
}
.pk-footer-inner {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  font-size: 13.5px;
  color: var(--pk-fg-faint);
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
  gap: 20px;
}
.pk-footer-links a {
  color: var(--pk-fg-faint);
  text-decoration: none;
  transition: color 0.18s ease;
}
.pk-footer-links a:hover {
  color: var(--pk-fg);
}
</style>
