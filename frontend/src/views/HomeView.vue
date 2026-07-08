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
    ref="pageRef"
    class="mono-page monolog-scope"
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
        <button type="button" @click="openAbout">{{ t('home.nav.about') }}</button>
        <a href="#work" @click.prevent="scrollTo('work')">{{ t('home.nav.features') }}</a>
        <a href="#services" @click.prevent="scrollTo('services')">{{ t('home.nav.models') }}</a>
        <a href="#process" @click.prevent="scrollTo('process')">{{ t('home.nav.workflow') }}</a>
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
      <section class="mono-hero mono-hero--poster" aria-labelledby="home-title">
        <canvas ref="auroraCanvas" class="mono-aurora-canvas" aria-hidden="true"></canvas>
        <div class="mono-aurora-fallback" aria-hidden="true"></div>
        <div class="mono-hero-cover" aria-hidden="true">
          <div class="mono-hero-scanline"></div>
          <div class="mono-hero-vignette"></div>
        </div>

        <div class="mono-hero-contain">
          <svg class="mono-hero-globe mono-enter" style="--d: 60ms" viewBox="0 0 32 32" fill="none" aria-hidden="true">
            <circle cx="16" cy="16" r="12" stroke="currentColor" stroke-width="1" />
            <ellipse cx="16" cy="16" rx="5" ry="12" stroke="currentColor" stroke-width="1" />
            <ellipse cx="16" cy="16" rx="12" ry="5" stroke="currentColor" stroke-width="1" />
            <line x1="4" y1="16" x2="28" y2="16" stroke="currentColor" stroke-width="1" />
          </svg>
          <p class="mono-hero-kicker mono-enter" style="--d: 120ms">
            {{ t('home.hero.badge') }}
          </p>
          <h1 id="home-title" class="mono-hero-statement">
            <SplitReveal :text="t('home.hero.posterStatement')" by="word" :on-scroll="false" />
          </h1>
          <p class="mono-hero-substatement mono-enter" style="--d: 520ms">
            {{ t('home.hero.posterSubstatement') }}
          </p>
        </div>

        <div class="mono-hero-meta mono-enter" style="--d: 350ms" aria-label="gateway highlights">
          <span>{{ t('home.hero.metaLatency') }}</span>
          <span>{{ t('home.hero.metaModels') }}</span>
          <span>{{ t('home.hero.metaControl') }}</span>
        </div>

        <div class="mono-mega-word" aria-hidden="true">
          <span>{{ posterWord }}</span>
        </div>

        <button
          type="button"
          class="mono-hero-scroll mono-enter"
          style="--d: 460ms"
          @click="scrollTo('work')"
          @pointerenter="setCursor(t('home.cursor.read'))"
          @pointerleave="clearCursor"
        >
          {{ t('home.hero.scrollCue') }} <span>↓</span>
        </button>
      </section>

      <section class="mono-section mono-section--quickstart" aria-label="API gateway quickstart">
        <div class="mono-container mono-quickstart-grid">
          <div class="mono-quickstart-copy" data-reveal>
            <p class="mono-eyebrow mono-eyebrow--plain">
              <span class="mono-dot" aria-hidden="true"></span>
              <span>{{ t('home.hero.quickstartKicker') }}</span>
            </p>
            <h2 class="mono-quickstart-title">
              <span>{{ t('home.hero.titleLine1') }}</span>
              <span>{{ t('home.hero.titleLine2') }}</span>
            </h2>
            <p class="mono-hero-lead">
              {{ t('home.hero.description') }}
            </p>
            <div class="mono-hero-actions">
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

          <aside class="mono-hero-plate" data-reveal style="--rd: 100ms" aria-label="API gateway visual">
            <img :src="gatewayPlateUrl" alt="" class="mono-plate-img" />
            <div class="mono-plate-caption">
              <span>{{ t('home.visual.gatewayLabel') }}</span>
              <span>{{ t('home.visual.gatewayMeta') }}</span>
            </div>
          </aside>
        </div>

        <div class="mono-container">
          <div class="mono-endpoints" data-reveal style="--rd: 160ms" :aria-label="t('home.aria.endpoints')">
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

    <nav class="mono-bottom-nav" :class="{ 'mono-bottom-nav--visible': isScrolled }" :aria-label="t('home.aria.bottomNav')">
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
import SplitReveal from '@/components/monolog/SplitReveal.vue'
import { useReveal } from '@/composables/useReveal'
import { useSmoothScroll } from '@/composables/useSmoothScroll'
import grainUrl from '@/assets/monolog/grain.svg'
import gatewayPlateUrl from '@/assets/monolog/gateway-plate.svg'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

const pageRef = ref<HTMLElement | null>(null)
useReveal(pageRef)
const { scrollTo: smoothScrollTo } = useSmoothScroll()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const posterWord = computed(() => siteName.value.replace(/[^a-z0-9]/gi, '').toUpperCase() || 'SUB2API')
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
const auroraCanvas = ref<HTMLCanvasElement | null>(null)
let copyTimer: ReturnType<typeof setTimeout> | null = null
let auroraRaf = 0
let auroraResize: (() => void) | null = null

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
  const el = document.getElementById(id)
  if (el) smoothScrollTo(el, { offset: -12 })
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

function setupAurora() {
  const canvas = auroraCanvas.value
  if (!canvas) return

  const prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const ctx = canvas.getContext('2d', { alpha: true })
  if (!ctx) return

  const dpr = Math.min(window.devicePixelRatio || 1, 1.5)
  const resize = () => {
    const rect = canvas.getBoundingClientRect()
    canvas.width = Math.max(1, Math.floor(rect.width * dpr))
    canvas.height = Math.max(1, Math.floor(rect.height * dpr))
  }

  const draw = (time = 0) => {
    const width = canvas.width / dpr
    const height = canvas.height / dpr
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, width, height)

    const t = time * 0.00018
    ctx.globalCompositeOperation = 'source-over'
    const base = ctx.createLinearGradient(0, 0, 0, height)
    base.addColorStop(0, 'rgba(10, 10, 9, 0.12)')
    base.addColorStop(0.42, 'rgba(255, 255, 246, 0.035)')
    base.addColorStop(1, 'rgba(7, 7, 6, 0.28)')
    ctx.fillStyle = base
    ctx.fillRect(0, 0, width, height)

    ctx.globalCompositeOperation = 'screen'
    ctx.filter = `blur(${Math.max(34, width * 0.04)}px)`

    const bands = [
      { y: 0.2, amp: 0.075, thick: 180, alpha: 0.28, speed: 1.0 },
      { y: 0.36, amp: 0.09, thick: 220, alpha: 0.2, speed: -0.74 },
      { y: 0.58, amp: 0.06, thick: 170, alpha: 0.15, speed: 0.58 },
      { y: 0.72, amp: 0.045, thick: 120, alpha: 0.1, speed: -0.48 }
    ]

    for (const band of bands) {
      const gradient = ctx.createLinearGradient(0, height * (band.y - 0.16), width, height * (band.y + 0.16))
      gradient.addColorStop(0, `rgba(255,255,244,0)`)
      gradient.addColorStop(0.32, `rgba(235,235,224,${band.alpha * 0.65})`)
      gradient.addColorStop(0.5, `rgba(255,255,249,${band.alpha})`)
      gradient.addColorStop(0.72, `rgba(176,176,166,${band.alpha * 0.54})`)
      gradient.addColorStop(1, `rgba(255,255,244,0)`)

      ctx.beginPath()
      ctx.moveTo(-width * 0.08, height * band.y)
      for (let i = 0; i <= 6; i += 1) {
        const x = (width * i) / 5 - width * 0.08
        const wave = Math.sin(t * band.speed + i * 1.7) * height * band.amp
        const drift = Math.cos(t * 0.72 + i * 1.15) * width * 0.025
        ctx.lineTo(x + drift, height * band.y + wave)
      }
      ctx.strokeStyle = gradient
      ctx.lineWidth = band.thick
      ctx.lineCap = 'round'
      ctx.stroke()
    }

    ctx.filter = 'blur(14px)'
    for (let i = 0; i < 11; i += 1) {
      const x = width * (0.04 + ((i * 0.125 + t * 0.075) % 1.05))
      const y = height * (0.16 + 0.58 * Math.abs(Math.sin(t * 0.55 + i * 0.78)))
      const radius = width * (0.055 + 0.035 * Math.sin(t + i * 0.6))
      const glow = ctx.createRadialGradient(x, y, 0, x, y, radius)
      glow.addColorStop(0, 'rgba(255,255,245,0.16)')
      glow.addColorStop(0.48, 'rgba(210,210,196,0.045)')
      glow.addColorStop(1, 'rgba(255,255,245,0)')
      ctx.fillStyle = glow
      ctx.fillRect(x - radius, y - radius, radius * 2, radius * 2)
    }

    ctx.filter = `blur(${Math.max(40, width * 0.055)}px)`
    ctx.globalCompositeOperation = 'multiply'
    for (let i = 0; i < 4; i += 1) {
      const x = width * (0.18 + i * 0.24 + Math.sin(t + i) * 0.04)
      const y = height * (0.36 + Math.cos(t * 0.8 + i) * 0.16)
      const radius = width * (0.16 + i * 0.018)
      const shadow = ctx.createRadialGradient(x, y, 0, x, y, radius)
      shadow.addColorStop(0, 'rgba(0,0,0,0.62)')
      shadow.addColorStop(1, 'rgba(0,0,0,0)')
      ctx.fillStyle = shadow
      ctx.fillRect(x - radius, y - radius, radius * 2, radius * 2)
    }

    ctx.filter = 'none'
    ctx.globalCompositeOperation = 'source-over'
    ctx.fillStyle = 'rgba(0,0,0,0.08)'
    ctx.fillRect(0, 0, width, height)

    if (!prefersReduced) {
      auroraRaf = window.requestAnimationFrame(draw)
    }
  }

  resize()
  auroraResize = resize
  window.addEventListener('resize', resize, { passive: true })
  draw(0)
}

function cleanupAurora() {
  if (auroraRaf) {
    window.cancelAnimationFrame(auroraRaf)
    auroraRaf = 0
  }
  if (auroraResize) {
    window.removeEventListener('resize', auroraResize)
    auroraResize = null
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  window.addEventListener('scroll', onScroll, { passive: true })
  window.addEventListener('keydown', onKeydown)
  onScroll()
  requestAnimationFrame(() => {
    setupAurora()
  })
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
  window.removeEventListener('keydown', onKeydown)
  cleanupAurora()
  if (copyTimer) clearTimeout(copyTimer)
  document.documentElement.classList.remove('mono-lock-scroll')
})
</script>

<style scoped>
:global(html.mono-lock-scroll) {
  overflow: hidden;
}

.mono-page {
  /* bymonolog 暖米色单色系（恒深色） */
  --ink: #e8e8e3;
  --ink-muted: #bfbfb1;
  --ink-soft: #938f8a;
  --paper: #080807;
  --paper-deep: #050504;
  --line: rgba(232, 232, 227, 0.12);
  --line-strong: rgba(232, 232, 227, 0.32);
  --surface: #181715;
  --accent: #8c8c73;
  --ease: cubic-bezier(0.2, 1, 0.36, 1);
  --display: 'Khteka', 'PingFang SC', 'Microsoft YaHei', Arial, sans-serif;
  --mono: 'Suisse Mono', ui-monospace, 'Cascadia Code', Menlo, Consolas, monospace;
  --body: 'Khteka', 'PingFang SC', 'Microsoft YaHei', Arial, sans-serif;

  position: relative;
  min-height: 100vh;
  overflow-x: clip;
  background:
    radial-gradient(circle at 50% -18%, rgba(191, 191, 177, 0.1), transparent 40rem),
    linear-gradient(180deg, var(--paper), var(--paper-deep));
  color: var(--ink);
  font-family: var(--body);
  font-weight: 500;
  letter-spacing: -0.01em;
  -webkit-font-smoothing: antialiased;
}

/* 浅色模式：整页暖白，hero 海报区仍保留深色（见下方 hero 令牌覆盖） */
:global(html:not(.dark) .mono-page) {
  --ink: #1a1a17;
  --ink-muted: #55504a;
  --ink-soft: #8a857d;
  --paper: #f4f2ec;
  --paper-deep: #eae7df;
  --line: rgba(20, 20, 15, 0.12);
  --line-strong: rgba(20, 20, 15, 0.3);
  --surface: #fbfaf6;
  --accent: #8c8c73;
  background:
    radial-gradient(circle at 50% -18%, rgba(140, 140, 115, 0.14), transparent 40rem),
    linear-gradient(180deg, var(--paper), var(--paper-deep));
}
/* 浅色下颗粒叠加改为正片叠底，避免屏幕混合导致发白 */
:global(html:not(.dark) .mono-page .mono-grain) {
  mix-blend-mode: multiply;
  opacity: 0.12;
}
/* hero 海报始终深色：在浅色模式下把 hero 作用域的令牌重置回深色，
   使其内部 kicker/statement/meta/mega-word 等继续用浅字压深底。 */
:global(html:not(.dark) .mono-hero) {
  --ink: #e8e8e3;
  --ink-muted: #bfbfb1;
  --ink-soft: #938f8a;
  --paper: #080807;
}

.mono-grain {
  pointer-events: none;
  position: fixed;
  inset: -40px;
  z-index: 2;
  opacity: 0.22;
  background-image: var(--grain-url);
  background-size: 180px 180px;
  mix-blend-mode: screen;
  animation: mono-grain 0.55s steps(6) infinite;
}
html.dark .mono-grain {
  mix-blend-mode: screen;
  opacity: 0.22;
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
/* GSAP (useReveal) drives [data-reveal] fade-up; JS sets the from-state on mount.
   Default to visible so content is never stuck hidden if JS/GSAP is unavailable. */
[data-reveal] {
  opacity: 1;
}

.mono-topbar {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 50;
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 28px;
  padding: 14px clamp(14px, 2vw, 28px);
  border-bottom: 1px solid transparent;
  color: oklch(0.92 0.01 85);
  mix-blend-mode: difference;
  transition: background-color 0.24s ease, border-color 0.24s ease, mix-blend-mode 0.24s ease;
}
.mono-topbar--scrolled {
  background: color-mix(in oklab, var(--paper) 72%, transparent);
  border-bottom-color: var(--line);
  backdrop-filter: blur(18px);
  mix-blend-mode: normal;
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
}
.mono-brand-text {
  font-family: var(--display);
  font-size: 18px;
  font-weight: 500;
  letter-spacing: -0.02em;
}
.mono-top-links {
  display: flex;
  justify-content: center;
  gap: 30px;
}
.mono-top-links a,
.mono-top-links button,
.mono-text-link {
  border: 0;
  background: transparent;
  padding: 0;
  font-family: var(--display);
  font-size: 13px;
  font-weight: 500;
  letter-spacing: -0.03em;
  text-transform: none;
  color: color-mix(in oklab, var(--ink) 76%, transparent);
  text-decoration: none;
  cursor: pointer;
  transition: color 0.18s ease, opacity 0.18s ease;
}
.mono-top-links a:hover,
.mono-top-links button:hover,
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
  isolation: isolate;
  min-height: 100svh;
  overflow: hidden;
  border-bottom: 1px solid var(--line);
  background: oklch(0.055 0.003 95);
}
.mono-aurora-canvas,
.mono-aurora-fallback,
.mono-hero-cover,
.mono-hero-scanline,
.mono-hero-vignette {
  pointer-events: none;
  position: absolute;
  inset: 0;
}
.mono-aurora-canvas {
  z-index: 0;
  width: 100%;
  height: 100%;
  opacity: 0.96;
  mix-blend-mode: screen;
  filter: grayscale(1) contrast(1.08) saturate(0.2);
}
.mono-aurora-fallback {
  z-index: -1;
  background:
    radial-gradient(ellipse at 50% 22%, oklch(0.76 0.012 85 / 0.28), transparent 24%),
    radial-gradient(ellipse at 18% 54%, oklch(0.72 0.01 85 / 0.16), transparent 28%),
    radial-gradient(ellipse at 82% 48%, oklch(0.62 0.008 85 / 0.12), transparent 30%),
    linear-gradient(180deg, oklch(0.08 0.004 95), oklch(0.035 0.003 95));
  animation: mono-aurora-drift 15s ease-in-out infinite alternate;
}
@keyframes mono-aurora-drift {
  from { transform: scale(1.04) translate3d(-1.5%, -1%, 0); filter: blur(0); }
  to { transform: scale(1.1) translate3d(1.5%, 1%, 0); filter: blur(3px); }
}
.mono-hero-cover {
  z-index: 1;
}
.mono-hero-scanline {
  opacity: 0.42;
  background:
    repeating-linear-gradient(0deg, oklch(1 0 0 / 0.04) 0 1px, transparent 1px 3px),
    radial-gradient(circle at center, transparent 0 42%, oklch(0 0 0 / 0.36) 82%);
  mix-blend-mode: overlay;
}
.mono-hero-vignette {
  background:
    linear-gradient(180deg, oklch(0 0 0 / 0.42), transparent 20%, transparent 66%, oklch(0 0 0 / 0.55)),
    radial-gradient(ellipse at center, transparent 28%, oklch(0 0 0 / 0.58) 100%);
}
.mono-hero-contain {
  position: relative;
  z-index: 4;
  display: grid;
  justify-items: center;
  align-content: start;
  min-height: 100svh;
  width: min(100% - 36px, 620px);
  margin-inline: auto;
  padding-top: clamp(128px, 20vh, 194px);
  text-align: center;
}
.mono-hero-globe {
  width: 32px;
  height: 32px;
  margin-bottom: 26px;
  color: color-mix(in oklab, var(--ink) 80%, transparent);
  opacity: 0.85;
}
.mono-hero-kicker,
.mono-hero-substatement,
.mono-hero-meta,
.mono-hero-scroll {
  color: color-mix(in oklab, var(--ink) 76%, transparent);
  text-shadow: 0 1px 18px oklch(0 0 0 / 0.5);
}
.mono-hero-kicker {
  max-width: 24rem;
  margin: 0 0 18px;
  font-family: var(--mono);
  font-size: 10.5px;
  letter-spacing: 0.13em;
  line-height: 1.55;
  text-transform: uppercase;
}
.mono-hero-statement {
  max-width: 32rem;
  margin: 0;
  color: var(--ink);
  font-family: var(--display);
  font-size: clamp(1.25rem, 1.9vw, 1.9rem);
  font-weight: 500;
  letter-spacing: -0.01em;
  line-height: 1.35;
  overflow-wrap: anywhere;
  text-wrap: balance;
}
.mono-hero-substatement {
  max-width: 28rem;
  margin: 22px 0 0;
  font-family: var(--display);
  font-size: clamp(0.95rem, 1.1vw, 1.1rem);
  font-weight: 500;
  letter-spacing: -0.01em;
  line-height: 1.5;
  overflow-wrap: anywhere;
  text-wrap: balance;
}
.mono-hero-meta {
  position: absolute;
  z-index: 4;
  left: clamp(18px, 3vw, 44px);
  right: clamp(18px, 3vw, 44px);
  bottom: clamp(156px, 18vw, 220px);
  display: flex;
  justify-content: space-between;
  gap: 18px;
  font-family: var(--mono);
  font-size: 10px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.mono-mega-word {
  position: absolute;
  z-index: 3;
  left: 50%;
  bottom: clamp(-24px, -2.4vw, -10px);
  width: 124vw;
  transform: translateX(-50%) scaleX(1.08);
  color: color-mix(in oklab, var(--ink) 68%, transparent);
  font-family: var(--display);
  font-size: clamp(7.6rem, 29vw, 28rem);
  font-weight: 500;
  letter-spacing: -0.12em;
  line-height: 0.72;
  text-align: center;
  white-space: nowrap;
  opacity: 0.5;
  filter: blur(0.18px);
}
.mono-mega-word span {
  display: inline-block;
}
.mono-hero-scroll {
  position: absolute;
  z-index: 5;
  left: 50%;
  bottom: clamp(88px, 10vw, 118px);
  transform: translateX(-50%);
  border: 1px solid color-mix(in oklab, var(--ink) 16%, transparent);
  border-radius: 999px;
  background: oklch(0.08 0.004 95 / 0.24);
  padding: 7px 11px;
  font-family: var(--mono);
  font-size: 9.5px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  cursor: pointer;
  opacity: 0;
  pointer-events: none;
  backdrop-filter: blur(12px);
}
.mono-hero-scroll span {
  display: inline-block;
  margin-left: 6px;
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
.mono-section--quickstart {
  background:
    radial-gradient(circle at 75% 18%, oklch(0.7 0.01 85 / 0.08), transparent 28rem),
    linear-gradient(180deg, oklch(0.085 0.004 95), var(--paper-deep));
}
.mono-quickstart-grid {
  display: grid;
  grid-template-columns: minmax(0, 0.95fr) minmax(300px, 0.72fr);
  gap: clamp(38px, 7vw, 96px);
  align-items: end;
}
.mono-quickstart-copy {
  display: grid;
  gap: 22px;
}
.mono-quickstart-title {
  display: grid;
  gap: 0.08em;
  margin: 0;
  color: var(--ink);
  font-family: var(--display);
  font-size: clamp(3rem, 7vw, 6.5rem);
  font-weight: 500;
  letter-spacing: -0.01em;
  line-height: 1.08;
}
.mono-quickstart-title span + span {
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
  margin-top: clamp(44px, 8vh, 82px);
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
  letter-spacing: -0.01em;
}
.mono-work-row h2 {
  font-size: clamp(1.45rem, 3vw, 2.6rem);
  line-height: 1.1;
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
  font-size: clamp(2.2rem, 5vw, 4.5rem);
  line-height: 1.1;
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
  font-weight: 500;
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
  max-width: 16ch;
  font-size: clamp(2.8rem, 7vw, 6rem);
  line-height: 1.12;
  letter-spacing: -0.01em;
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
  overflow: hidden;
  border: 1px solid color-mix(in oklab, var(--ink) 24%, transparent);
  border-radius: 16px;
  background: oklch(0.07 0.004 95 / 0.72);
  box-shadow: inset 0 1px 0 oklch(1 0 0 / 0.06), 0 18px 70px oklch(0 0 0 / 0.38);
  opacity: 0;
  transform: translateY(110%);
  pointer-events: none;
  backdrop-filter: blur(20px);
  transition: opacity 0.42s var(--ease), transform 0.42s var(--ease);
}
.mono-bottom-nav--visible {
  opacity: 1;
  transform: translateY(0);
  pointer-events: auto;
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
  font-weight: 500;
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
  max-width: 20ch;
  margin: 24px 0 34px;
  font-family: var(--display);
  font-size: clamp(2.6rem, 6.5vw, 6rem);
  font-weight: 500;
  letter-spacing: -0.01em;
  line-height: 1.1;
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
  .mono-quickstart-grid,
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
  .mono-hero-contain {
    width: min(100% - 40px, 360px);
    padding-top: 116px;
  }
  .mono-hero-statement {
    max-width: 20.5rem;
    font-size: clamp(1.15rem, 5.4vw, 1.42rem);
    letter-spacing: -0.01em;
    line-height: 1.35;
  }
  .mono-hero-substatement {
    max-width: 20rem;
    font-size: clamp(0.92rem, 4.2vw, 1.04rem);
    letter-spacing: -0.01em;
    line-height: 1.5;
  }
  .mono-hero-meta {
    bottom: 150px;
    display: grid;
    grid-template-columns: 1fr;
    gap: 6px;
    font-size: 9px;
    letter-spacing: 0.04em;
    text-align: center;
  }
  .mono-mega-word {
    bottom: 20px;
    width: 176vw;
    font-size: clamp(7rem, 38vw, 10.4rem);
  }
  .mono-hero-scroll {
    display: none;
  }
  .mono-quickstart-title {
    font-size: clamp(2.6rem, 15vw, 4.4rem);
    letter-spacing: -0.01em;
    line-height: 1.1;
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
  .mono-aurora-fallback,
  .mono-model-track {
    animation: none;
    transition: none;
    opacity: 1;
    transform: none;
  }
}
</style>
