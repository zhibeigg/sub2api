<template>
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <div v-else class="public-home monolog-scope">
    <a class="public-skip-link" href="#home-main">{{ t('home.aria.skipToContent') }}</a>

    <header class="public-header">
      <div class="public-shell public-header-inner">
        <router-link to="/home" class="public-brand" :aria-label="siteName">
          <img :src="siteLogo || '/logo.svg'" :alt="siteName" class="public-brand-logo" />
          <span>{{ siteName }}</span>
        </router-link>

        <nav class="public-nav" :aria-label="t('home.aria.primaryNav')">
          <a href="#principles"><LetterSwapText :text="t('home.nav.features')" /></a>
          <a href="#access"><LetterSwapText :text="t('home.nav.workflow')" /></a>
          <a href="#models"><LetterSwapText :text="t('home.nav.models')" /></a>
          <a href="#pricing"><LetterSwapText :text="t('home.nav.pricing')" /></a>
          <a
            data-testid="home-docs-link"
            :href="docUrl || '#docs'"
            :target="docUrl ? '_blank' : undefined"
            :rel="docUrl ? 'noopener noreferrer' : undefined"
          >
            <LetterSwapText :text="t('home.docs')" />
          </a>
        </nav>

        <div class="public-header-actions">
          <LocaleSwitcher />
          <button
            type="button"
            class="public-icon-button"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="sm" :stroke-width="1.5" />
            <Icon v-else name="moon" size="sm" :stroke-width="1.5" />
          </button>
          <router-link v-if="!isAuthenticated" to="/login" class="public-login-link">
            <LetterSwapText :text="t('home.login')" />
          </router-link>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/register'"
            class="public-header-cta"
            :aria-label="isAuthenticated ? t('home.dashboard') : t('home.getStarted')"
          >
            <span class="public-header-cta-label">
              <LetterSwapText :text="isAuthenticated ? t('home.dashboard') : t('home.getStarted')" />
            </span>
            <Icon name="externalLink" size="xs" :stroke-width="1.5" />
          </router-link>
        </div>
      </div>
    </header>

    <main id="home-main">
      <section ref="storyRoot" class="home-story" aria-labelledby="home-title">
        <SignalTrail :labels="signalLabels" bounded />

        <div class="home-story-stage">
          <div class="home-story-grid" aria-hidden="true"></div>
          <div class="home-story-index" aria-hidden="true">
            {{ String(activeStory + 1).padStart(2, '0') }}<span>/03</span>
          </div>

          <div class="public-shell home-story-shell">
            <div class="home-story-controls">
              <ChapterDial
                :active="activeStory"
                :total="storyScenes.length"
                :active-label="storySceneText(storyScenes[activeStory].key, 'label')"
                :previous-label="storyText('previous')"
                :next-label="storyText('next')"
                @previous="goToStory(activeStory - 1)"
                @next="goToStory(activeStory + 1)"
              />
            </div>

            <article
              v-for="(scene, index) in storyScenes"
              :key="scene.key"
              ref="storySceneRefs"
              class="home-story-scene"
              :class="{ 'is-active': activeStory === index }"
              :aria-hidden="!isMobileStory && activeStory !== index"
              :data-story-scene="scene.key"
            >
              <div class="home-story-copy">
                <p class="public-kicker home-status-line">
                  <span class="home-status-dot" aria-hidden="true"></span>
                  {{ storyText('status') }}
                  <span class="home-story-separator" aria-hidden="true">/</span>
                  {{ storySceneText(scene.key, 'label') }}
                </p>

                <component
                  :is="index === 0 ? 'h1' : 'h2'"
                  :id="index === 0 ? 'home-title' : undefined"
                  class="home-story-title"
                >
                  <span>{{ storySceneText(scene.key, 'titleLine1') }}</span>
                  <span>{{ storySceneText(scene.key, 'titleLine2') }}</span>
                </component>
                <p class="home-hero-subtitle">{{ storySceneText(scene.key, 'subtitle') }}</p>
                <p class="home-hero-description">{{ storySceneText(scene.key, 'description') }}</p>
                <p v-if="index === 0" class="home-commitment">
                  <span aria-hidden="true"></span>
                  {{ t('home.hero.commitment') }}
                </p>

                <div v-if="index === 0" class="home-hero-actions">
                  <router-link
                    :to="isAuthenticated ? dashboardPath : '/register'"
                    class="public-button public-button--primary"
                  >
                    <LetterSwapText :text="isAuthenticated ? t('home.goToDashboard') : t('home.hero.ctaPrimary')" />
                    <Icon name="arrowRight" size="sm" :stroke-width="1.5" />
                  </router-link>
                  <a
                    v-if="docUrl"
                    :href="docUrl"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="public-button"
                  >
                    <LetterSwapText :text="t('home.hero.ctaDocs')" />
                    <Icon name="externalLink" size="sm" :stroke-width="1.5" />
                  </a>
                  <a v-else href="#docs" class="public-button">
                    <LetterSwapText :text="t('home.hero.ctaDocs')" />
                    <Icon name="arrowDown" size="sm" :stroke-width="1.5" />
                  </a>
                </div>

                <router-link
                  v-else-if="index === 2"
                  :to="isAuthenticated ? dashboardPath : '/register'"
                  class="public-button public-button--primary home-story-console-cta"
                >
                  <LetterSwapText :text="isAuthenticated ? t('home.goToDashboard') : t('home.getStarted')" />
                  <Icon name="arrowRight" size="sm" :stroke-width="1.5" />
                </router-link>
              </div>

              <div class="home-story-evidence">
                <div v-if="index === 0" class="home-signal-board" aria-hidden="true">
                  <span v-for="(model, modelIndex) in models.slice(0, 4)" :key="model">
                    <i>{{ String(modelIndex + 1).padStart(2, '0') }}</i>
                    <strong>{{ model }}</strong>
                    <b>200</b>
                  </span>
                </div>

                <aside v-else-if="index === 1" class="home-access-panel" :aria-label="t('home.aria.endpoints')">
                  <div class="home-access-heading">
                    <span class="public-kicker">API ACCESS</span>
                    <span class="home-live-mark">LIVE</span>
                  </div>

                  <div class="home-endpoint-list">
                    <div v-for="endpoint in endpoints" :key="endpoint.key" class="home-endpoint-row">
                      <div>
                        <span>{{ t(endpoint.labelKey) }}</span>
                        <code>{{ endpoint.url }}</code>
                      </div>
                      <button
                        type="button"
                        class="home-copy-button"
                        :class="{ 'is-copied': copiedKey === endpoint.key }"
                        :title="copiedKey === endpoint.key ? t('home.hero.copied') : t('home.hero.copy')"
                        :aria-label="copiedKey === endpoint.key ? t('home.hero.copied') : t('home.hero.copy')"
                        @click="copyEndpoint(endpoint)"
                      >
                        <Icon :name="copiedKey === endpoint.key ? 'check' : 'copy'" size="sm" :stroke-width="1.5" />
                      </button>
                    </div>
                  </div>
                </aside>

                <div v-else class="home-story-proofs">
                  <span><i>01</i>{{ t('home.hero.metaModels') }}</span>
                  <span><i>02</i>{{ t('home.hero.metaControl') }}</span>
                  <span><i>03</i>{{ t('home.privacy.minimum') }}</span>
                </div>
              </div>
            </article>

            <div class="home-story-orbit">
              <ProtocolOrbit
                :scene="activeStory"
                :label="storyText('orbitLabel')"
                :status="storyText('orbitStatus')"
              />
            </div>

            <button type="button" class="home-story-scroll" @click="advanceStory">
              <span>{{ storyText('scroll') }}</span>
              <i aria-hidden="true"></i>
            </button>
          </div>
        </div>
      </section>

      <section id="principles" class="home-section">
        <div class="public-shell home-section-grid">
          <header class="home-section-heading">
            <p class="public-kicker">{{ t('home.value.kicker') }}</p>
            <h2>{{ t('home.value.title') }}</h2>
            <p>{{ t('home.value.subtitle') }}</p>
          </header>

          <div class="home-principle-list">
            <article v-for="(item, index) in valueItems" :key="item" class="home-principle-row">
              <span>{{ String(index + 1).padStart(2, '0') }}</span>
              <h3>{{ t(`home.value.items.${item}.title`) }}</h3>
              <p>{{ t(`home.value.items.${item}.desc`) }}</p>
            </article>
          </div>
        </div>
      </section>

      <section id="access" class="home-section home-section--surface">
        <div class="public-shell home-section-grid">
          <header class="home-section-heading">
            <p class="public-kicker">{{ t('home.workflow.kicker') }}</p>
            <h2>{{ t('home.workflow.title') }}</h2>
            <p>{{ t('home.workflow.subtitle') }}</p>
          </header>

          <ol class="home-step-list">
            <li v-for="(step, index) in workflowSteps" :key="step.key">
              <span>{{ String(index + 1).padStart(2, '0') }}</span>
              <div>
                <h3>{{ t(`home.workflow.steps.${step.key}.title`) }}</h3>
                <p>{{ t(`home.workflow.steps.${step.key}.desc`) }}</p>
                <code v-if="step.code">{{ step.code }}</code>
              </div>
            </li>
          </ol>
        </div>
      </section>

      <section id="models" class="home-section">
        <div class="public-shell">
          <header class="home-wide-heading">
            <p class="public-kicker">{{ t('home.ecosystem.kicker') }}</p>
            <h2>{{ t('home.ecosystem.title') }}</h2>
            <p>{{ t('home.ecosystem.subtitle') }}</p>
          </header>

          <div class="home-model-grid" aria-label="Supported model providers">
            <div v-for="(model, index) in models" :key="model" class="home-model-name">
              <span>{{ String(index + 1).padStart(2, '0') }}</span>
              <strong>{{ model }}</strong>
            </div>
            <router-link
              :to="isAuthenticated ? dashboardPath : '/register'"
              class="home-model-name home-model-name--more"
            >
              <span>+</span>
              <strong>{{ t('home.ecosystem.more') }}</strong>
            </router-link>
          </div>
        </div>
      </section>

      <section id="pricing" class="home-section home-section--surface">
        <div class="public-shell home-pricing-grid">
          <div>
            <p class="public-kicker">{{ t('home.pricing.kicker') }}</p>
            <h2>{{ t('home.pricing.title') }}</h2>
          </div>
          <div class="home-price-statement">
            <strong>{{ t('home.pricing.rateValue') }}</strong>
            <p>{{ t('home.pricing.note') }}</p>
            <span>{{ t('home.pricing.subtitle') }}</span>
          </div>
          <router-link :to="isAuthenticated ? dashboardPath : '/register'" class="public-button public-button--primary">
            {{ t('home.pricing.cta') }}
            <Icon name="arrowRight" size="sm" :stroke-width="1.5" />
          </router-link>
        </div>
      </section>

      <section id="docs" data-testid="home-docs-section" class="home-section home-docs-section">
        <div class="public-shell home-docs-grid">
          <div>
            <p class="public-kicker">{{ t('home.docsPanel.kicker') }}</p>
            <h2>{{ t('home.docsPanel.title') }}</h2>
          </div>
          <div class="home-docs-copy">
            <p>{{ t('home.docsPanel.description') }}</p>
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="public-button public-button--light"
            >
              {{ t('home.docsPanel.button') }}
              <Icon name="externalLink" size="sm" :stroke-width="1.5" />
            </a>
            <span v-else class="home-docs-unavailable">{{ t('home.docsPanel.unavailable') }}</span>
          </div>
        </div>
      </section>

      <section class="home-privacy-section">
        <div class="public-shell home-privacy-grid">
          <div>
            <p class="public-kicker">{{ t('home.privacy.kicker') }}</p>
            <h2>{{ t('home.privacy.title') }}</h2>
            <p>{{ t('home.privacy.description') }}</p>
          </div>
          <ul>
            <li>{{ t('home.privacy.minimum') }}</li>
            <li>{{ t('home.privacy.noContent') }}</li>
            <li>{{ t('home.privacy.noSale') }}</li>
            <li>{{ t('home.privacy.noTraining') }}</li>
          </ul>
        </div>
      </section>

    </main>

    <footer class="public-footer">
      <div class="public-shell public-footer-inner">
        <div class="public-footer-copy">
          <span>&copy; {{ currentYear }} {{ siteName }}</span>
          <span>{{ t('home.footer.allRightsReserved') }}</span>
        </div>
        <nav class="public-footer-nav" :aria-label="t('home.aria.footerNav')">
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">
            {{ t('home.docs') }}
          </a>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'">
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
          <a href="#home-title">{{ t('home.footer.backToTop') }}</a>
        </nav>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import ChapterDial from '@/components/public/ChapterDial.vue'
import LetterSwapText from '@/components/public/LetterSwapText.vue'
import ProtocolOrbit from '@/components/public/ProtocolOrbit.vue'
import SignalTrail from '@/components/public/SignalTrail.vue'
import { resolveStoryIndex } from '@/components/public/publicMotion'
import { useAppStore, useAuthStore } from '@/stores'
import { PUBLIC_DOCS_URL, sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const siteName = 'PokeAPI'
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true
  })
)
const docUrl = PUBLIC_DOCS_URL
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))
const currentYear = new Date().getFullYear()

const storyScenes = [
  { key: 'real' },
  { key: 'protocol' },
  { key: 'billing' }
] as const

type StorySceneKey = (typeof storyScenes)[number]['key']
type StorySceneField = 'label' | 'titleLine1' | 'titleLine2' | 'subtitle' | 'description'
type StoryTextKey = 'status' | 'scroll' | 'previous' | 'next' | 'orbitLabel' | 'orbitStatus'

function storyText(key: StoryTextKey): string {
  return t(`home.story.${key}`)
}

function storySceneText(scene: StorySceneKey, field: StorySceneField): string {
  return t(`home.story.scenes.${scene}.${field}`)
}

const signalLabels = ['API', '/v1', 'CLAUDE', 'OPENAI', 'GEMINI', '200 OK']
const storyRoot = ref<HTMLElement | null>(null)
const storySceneRefs = ref<HTMLElement[]>([])
const activeStory = ref(0)
const isMobileStory = ref(false)
let storyFrame = 0
let storyMedia: MediaQueryList | null = null

const apiBaseUrl = computed(() => {
  const configured = sanitizeUrl(
    appStore.cachedPublicSettings?.api_base_url || appStore.apiBaseUrl || ''
  )
  return (configured || window.location.origin).replace(/\/+$/, '')
})

const endpoints = computed(() => {
  const root = apiBaseUrl.value
  const openAi = root.endsWith('/v1') ? root : `${root}/v1`
  return [
    { key: 'openai', labelKey: 'home.hero.baseUrlOpenai', url: openAi },
    { key: 'anthropic', labelKey: 'home.hero.baseUrlAnthropic', url: root.replace(/\/v1$/, '') },
    { key: 'website', labelKey: 'home.hero.websiteNode', url: 'https://www.poke2api.com' }
  ]
})

const valueItems = ['unified', 'observability', 'elastic', 'developer']
const workflowSteps = computed(() => [
  { key: 'register', code: '' },
  { key: 'configure', code: `export ANTHROPIC_BASE_URL=${apiBaseUrl.value.replace(/\/v1$/, '')}` },
  { key: 'observe', code: '' }
])
const models = ['Claude', 'OpenAI', 'Gemini', 'Grok', 'DeepSeek', 'Qwen']

const isDark = ref(document.documentElement.classList.contains('dark'))
const copiedKey = ref('')
let copyTimer: ReturnType<typeof setTimeout> | null = null

function updateStoryIndex(): void {
  storyFrame = 0
  const root = storyRoot.value
  if (!root) return

  const rect = root.getBoundingClientRect()
  const headerHeight = document.querySelector<HTMLElement>('.public-header')?.offsetHeight ?? 72
  let progress = 0

  if (isMobileStory.value) {
    const readingLine = Math.min(window.innerHeight * 0.45, window.innerHeight - headerHeight)
    progress = (readingLine - rect.top) / Math.max(rect.height, 1)
  } else {
    const stageHeight = Math.max(1, window.innerHeight - headerHeight)
    const travel = Math.max(1, rect.height - stageHeight)
    progress = (headerHeight - rect.top) / travel
  }

  activeStory.value = resolveStoryIndex(progress, storyScenes.length)
}

function scheduleStoryUpdate(): void {
  if (storyFrame) return
  storyFrame = window.requestAnimationFrame(updateStoryIndex)
}

function handleStoryViewportChange(): void {
  isMobileStory.value = storyMedia?.matches ?? window.innerWidth <= 900
  scheduleStoryUpdate()
}

function goToStory(requestedIndex: number): void {
  const targetIndex = Math.min(storyScenes.length - 1, Math.max(0, requestedIndex))
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  activeStory.value = targetIndex

  if (isMobileStory.value) {
    storySceneRefs.value[targetIndex]?.scrollIntoView({
      behavior: reducedMotion ? 'auto' : 'smooth',
      block: 'start'
    })
    return
  }

  const root = storyRoot.value
  if (!root) return
  const rect = root.getBoundingClientRect()
  const headerHeight = document.querySelector<HTMLElement>('.public-header')?.offsetHeight ?? 72
  const stageHeight = Math.max(1, window.innerHeight - headerHeight)
  const travel = Math.max(1, rect.height - stageHeight)
  const progress = targetIndex / Math.max(storyScenes.length - 1, 1)
  const sectionTop = window.scrollY + rect.top

  window.scrollTo({
    top: sectionTop - headerHeight + travel * progress,
    behavior: reducedMotion ? 'auto' : 'smooth'
  })
}

function advanceStory(): void {
  if (activeStory.value < storyScenes.length - 1) {
    goToStory(activeStory.value + 1)
    return
  }

  document.querySelector<HTMLElement>('#principles')?.scrollIntoView({
    behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
    block: 'start'
  })
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

async function copyEndpoint(endpoint: { key: string; url: string }) {
  try {
    await navigator.clipboard.writeText(endpoint.url)
  } catch {
    const textarea = document.createElement('textarea')
    textarea.value = endpoint.url
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
  }

  copiedKey.value = endpoint.key
  if (copyTimer) clearTimeout(copyTimer)
  copyTimer = setTimeout(() => {
    copiedKey.value = ''
  }, 1600)
}

onMounted(() => {
  isDark.value = document.documentElement.classList.contains('dark')
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }

  storyMedia = window.matchMedia('(max-width: 900px)')
  isMobileStory.value = storyMedia.matches
  storyMedia.addEventListener('change', handleStoryViewportChange)
  window.addEventListener('scroll', scheduleStoryUpdate, { passive: true })
  window.addEventListener('resize', scheduleStoryUpdate, { passive: true })
  scheduleStoryUpdate()
})

onBeforeUnmount(() => {
  if (copyTimer) clearTimeout(copyTimer)
  if (storyFrame) window.cancelAnimationFrame(storyFrame)
  storyMedia?.removeEventListener('change', handleStoryViewportChange)
  window.removeEventListener('scroll', scheduleStoryUpdate)
  window.removeEventListener('resize', scheduleStoryUpdate)
})
</script>

<style scoped>
.public-home {
  min-height: 100vh;
  overflow-x: clip;
}

.public-header {
  position: sticky;
  top: 0;
  z-index: 40;
  border-bottom: 1px solid var(--public-line);
  background: var(--public-bg);
}

.public-header-inner {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) auto minmax(300px, 1fr);
  align-items: center;
  min-height: 72px;
}

.public-brand,
.public-nav,
.public-header-actions,
.public-button,
.public-header-cta,
.home-status-line,
.home-access-heading,
.home-live-mark,
.home-copy-button,
.public-footer-inner {
  display: flex;
  align-items: center;
}

.public-brand {
  gap: 10px;
  width: fit-content;
  color: var(--public-ink);
  font-size: 18px;
  text-decoration: none;
}

.public-brand-logo {
  width: 30px;
  height: 30px;
  object-fit: contain;
  transition: transform 420ms var(--public-ease), filter 180ms var(--public-ease);
}

.public-brand:hover .public-brand-logo {
  filter: saturate(0.75) contrast(1.12);
  transform: rotate(18deg) scale(0.9);
}

.public-nav {
  align-self: stretch;
}

.public-nav a {
  display: grid;
  place-items: center;
  min-width: 76px;
  padding-inline: 14px;
  border-left: 1px solid var(--public-line);
  color: var(--public-muted);
  font-family: var(--public-font-mono);
  font-size: 11px;
  text-decoration: none;
  text-transform: uppercase;
  transition: color 160ms var(--public-ease), background-color 160ms var(--public-ease);
}

.public-nav a:last-child {
  border-right: 1px solid var(--public-line);
}

.public-nav a:hover {
  color: var(--public-ink);
  background: var(--public-surface-strong);
}

.public-header-actions {
  justify-content: flex-end;
  gap: 8px;
  min-width: 0;
}

.public-icon-button,
.home-copy-button {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  width: 36px;
  height: 36px;
  border: 1px solid var(--public-line);
  border-radius: 4px;
  background: transparent;
  color: var(--public-ink);
  cursor: pointer;
  transition: border-color 160ms var(--public-ease), background-color 160ms var(--public-ease);
}

.public-icon-button:hover,
.home-copy-button:hover {
  border-color: var(--public-line-strong);
  background: var(--public-surface-strong);
}

.home-copy-button.is-copied {
  border-color: var(--public-accent);
  background: var(--public-accent);
  color: var(--public-inverse-bg);
  animation: home-copy-confirm 420ms var(--public-ease);
}

@keyframes home-copy-confirm {
  0% { transform: scale(0.88); }
  55% { transform: scale(1.06); }
  100% { transform: scale(1); }
}

.public-login-link {
  padding: 8px 10px;
  color: var(--public-muted);
  font-size: 13px;
  text-decoration: none;
}

.public-login-link:hover {
  color: var(--public-ink);
}

.public-header-cta {
  justify-content: center;
  gap: 8px;
  min-height: 38px;
  padding: 0 14px;
  border: 1px solid var(--public-ink);
  border-radius: 4px;
  background: var(--public-ink);
  color: var(--public-bg);
  font-size: 13px;
  text-decoration: none;
  transition: color 160ms var(--public-ease), background-color 160ms var(--public-ease);
}

.public-header-cta:hover {
  background: transparent;
  color: var(--public-ink);
}

.home-story {
  position: relative;
  height: 300svh;
  border-bottom: 1px solid var(--public-line);
  background: var(--public-bg);
}

.home-story-stage {
  position: sticky;
  top: 72px;
  height: calc(100svh - 72px);
  min-height: 38rem;
  overflow: hidden;
  isolation: isolate;
}

.home-story-grid {
  position: absolute;
  inset: 0;
  z-index: -2;
  opacity: 0.7;
  background-image:
    linear-gradient(to right, transparent calc(50% - 0.5px), var(--public-line) 50%, transparent calc(50% + 0.5px)),
    linear-gradient(to bottom, transparent calc(50% - 0.5px), var(--public-line) 50%, transparent calc(50% + 0.5px));
  background-size: min(24vw, 20rem) min(24vw, 20rem);
  mask-image: linear-gradient(to bottom, transparent, #000 16%, #000 84%, transparent);
}

.home-story-grid::before,
.home-story-grid::after {
  content: '';
  position: absolute;
  border: 1px solid var(--public-line);
  border-radius: 50%;
}

.home-story-grid::before {
  width: min(54vw, 52rem);
  aspect-ratio: 1;
  right: -18vw;
  bottom: -44%;
}

.home-story-grid::after {
  width: min(26vw, 24rem);
  aspect-ratio: 1;
  top: 8%;
  left: 42%;
}

.home-story-shell {
  position: relative;
  height: 100%;
}

.home-story-index {
  position: absolute;
  top: clamp(1.5rem, 4vh, 3rem);
  right: max(1.5rem, calc((100vw - var(--public-shell, 1440px)) / 2));
  z-index: 1;
  color: var(--public-line-strong);
  font-family: var(--public-font-mono);
  font-size: clamp(4rem, 11vw, 10rem);
  font-weight: 300;
  letter-spacing: -0.08em;
  line-height: 0.8;
  pointer-events: none;
}

.home-story-index span {
  margin-left: 0.1em;
  color: var(--public-soft);
  font-size: 0.18em;
  letter-spacing: 0;
}

.home-story-scene {
  position: absolute;
  inset-block: 0;
  inset-inline: var(--public-gutter);
  display: grid;
  grid-template-columns: minmax(0, 1.12fr) minmax(21rem, 0.88fr);
  gap: clamp(3rem, 7vw, 8rem);
  align-items: center;
  padding-block: clamp(4.5rem, 10vh, 8rem) clamp(8.5rem, 17vh, 11rem);
  opacity: 0;
  pointer-events: none;
  transform: translate3d(0, 3rem, 0);
  transition: opacity 420ms var(--public-ease), transform 560ms var(--public-ease);
}

.home-story-scene.is-active {
  z-index: 2;
  opacity: 1;
  pointer-events: auto;
  transform: translate3d(0, 0, 0);
}

.home-story-copy {
  position: relative;
  z-index: 2;
  max-width: 52rem;
}

.home-status-line {
  gap: 10px;
  margin-bottom: clamp(1.5rem, 4vh, 2.75rem);
}

.home-status-dot {
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 50%;
  background: var(--public-accent);
  box-shadow: 0 0 0 0.3rem var(--public-accent-soft);
}

.home-story-separator {
  color: var(--public-line-strong);
}

.home-story-title {
  max-width: 10ch;
  margin: 0;
  color: var(--public-ink);
  font-size: clamp(4rem, 7.2vw, 7.25rem);
  font-style: oblique 10deg;
  font-weight: 500;
  letter-spacing: -0.06em;
  line-height: 0.87;
}

.home-story-title span {
  display: block;
}

.home-story-title span:last-child {
  color: var(--public-accent);
  transform: translate3d(clamp(1rem, 4vw, 4rem), 0, 0);
}

.home-hero-subtitle {
  max-width: 36rem;
  margin: 2rem 0 0;
  color: var(--public-ink);
  font-size: clamp(1.125rem, 1.6vw, 1.375rem);
  line-height: 1.45;
}

.home-hero-description {
  max-width: 40rem;
  margin: 1.125rem 0 0;
  color: var(--public-muted);
  font-size: 1rem;
  line-height: 1.75;
}

.home-commitment {
  display: flex;
  gap: 0.75rem;
  align-items: center;
  width: fit-content;
  margin: 1.25rem 0 0;
  padding: 0.75rem 1rem;
  border: 1px solid var(--public-line-strong);
  background: var(--public-accent-soft);
  color: var(--public-ink);
  font-family: var(--public-font-mono);
  font-size: 0.8125rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  line-height: 1.4;
}

.home-commitment span {
  width: 0.5rem;
  height: 0.5rem;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--public-accent);
  box-shadow: 0 0 0 0.3rem var(--public-accent-soft);
}

.home-hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 2rem;
}

.home-story-console-cta {
  width: fit-content;
  margin-top: 2rem;
}

.home-story-evidence {
  position: relative;
  z-index: 2;
  justify-self: end;
  width: min(100%, 31rem);
  margin-bottom: 2rem;
}

.home-signal-board {
  display: grid;
  border-top: 1px solid var(--public-line-strong);
  border-left: 1px solid var(--public-line);
  background: color-mix(in oklch, var(--public-bg) 88%, transparent);
}

.home-signal-board > span {
  display: grid;
  grid-template-columns: 3rem minmax(0, 1fr) auto;
  gap: 1rem;
  align-items: center;
  min-height: 4.5rem;
  padding: 0 1.25rem;
  border-right: 1px solid var(--public-line);
  border-bottom: 1px solid var(--public-line);
  font-family: var(--public-font-mono);
}

.home-signal-board i,
.home-signal-board b {
  color: var(--public-soft);
  font-size: 0.625rem;
  font-style: normal;
  font-weight: 400;
}

.home-signal-board strong {
  color: var(--public-ink);
  font-size: 1rem;
  font-weight: 400;
  letter-spacing: 0.08em;
}

.home-signal-board b {
  color: var(--public-accent);
}

.home-story-proofs {
  display: grid;
  border-top: 1px solid var(--public-line-strong);
}

.home-story-proofs span {
  display: grid;
  grid-template-columns: 3rem minmax(0, 1fr);
  gap: 1rem;
  align-items: start;
  padding: 1.5rem 0;
  border-bottom: 1px solid var(--public-line);
  color: var(--public-ink);
  font-size: clamp(1rem, 1.35vw, 1.2rem);
  line-height: 1.5;
}

.home-story-proofs i {
  color: var(--public-accent);
  font-family: var(--public-font-mono);
  font-size: 0.625rem;
  font-style: normal;
}

.home-story-orbit {
  position: absolute;
  right: clamp(-9rem, -7vw, -3rem);
  bottom: clamp(-10rem, -12vh, -5rem);
  z-index: 0;
  opacity: 0.3;
  pointer-events: none;
  transform: rotate(-7deg) scale(1.14);
  transform-origin: bottom right;
  transition: opacity 400ms var(--public-ease), transform 600ms var(--public-ease);
}

.home-story-controls {
  position: absolute;
  bottom: clamp(1.25rem, 4vh, 2.5rem);
  left: var(--public-gutter);
  z-index: 5;
}

.home-story-scroll {
  position: absolute;
  bottom: clamp(1.25rem, 4vh, 2.5rem);
  left: 50%;
  z-index: 5;
  display: grid;
  gap: 0.65rem;
  place-items: center;
  min-width: 8rem;
  min-height: 3.25rem;
  border: 0;
  background: transparent;
  color: var(--public-muted);
  font-family: var(--public-font-mono);
  font-size: 0.625rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  cursor: pointer;
  transform: translateX(-50%);
}

.home-story-scroll i {
  position: relative;
  display: block;
  width: 1px;
  height: 1.75rem;
  overflow: hidden;
  background: var(--public-line-strong);
}

.home-story-scroll i::after {
  content: '';
  position: absolute;
  inset: 0;
  background: var(--public-accent);
  transform: translateY(-100%);
  animation: home-scroll-signal 1.8s var(--public-ease) infinite;
}

@keyframes home-scroll-signal {
  50%, 100% { transform: translateY(100%); }
}

.public-button {
  justify-content: center;
  gap: 9px;
  min-height: 46px;
  padding: 0 18px;
  border: 1px solid var(--public-line-strong);
  border-radius: 4px;
  background: transparent;
  color: var(--public-ink);
  font-family: var(--public-font-mono);
  font-size: 12px;
  text-decoration: none;
  text-transform: uppercase;
  transition: border-color 180ms var(--public-ease), color 180ms var(--public-ease), background-color 180ms var(--public-ease);
}

.public-button:hover {
  border-color: var(--public-ink);
  background: var(--public-surface-strong);
}

.public-button--primary {
  border-color: var(--public-accent);
  background: var(--public-accent);
  color: var(--public-inverse-bg);
}

.public-button--primary:hover {
  border-color: var(--public-ink);
  background: var(--public-ink);
  color: var(--public-bg);
}

.home-access-panel {
  border: 1px solid var(--public-line-strong);
  border-radius: 4px;
  background: var(--public-surface);
}

.home-access-heading {
  justify-content: space-between;
  min-height: 52px;
  padding: 0 18px;
  border-bottom: 1px solid var(--public-line);
}

.home-live-mark {
  gap: 7px;
  color: var(--public-muted);
  font-family: var(--public-font-mono);
  font-size: 10px;
}

.home-live-mark::before {
  content: '';
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--public-accent);
}

.home-endpoint-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 18px;
  align-items: center;
  padding: 20px 18px;
  border-bottom: 1px solid var(--public-line);
}

.home-endpoint-row > div {
  min-width: 0;
}

.home-endpoint-row span {
  display: block;
  margin-bottom: 9px;
  color: var(--public-muted);
  font-size: 12px;
}

.home-endpoint-row code {
  display: block;
  overflow: hidden;
  color: var(--public-ink);
  font-family: var(--public-font-mono);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.home-access-meta {
  display: grid;
  gap: 10px;
  padding: 18px;
  color: var(--public-muted);
  font-size: 12px;
  line-height: 1.5;
}

.home-access-meta span {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.home-access-meta span::before {
  content: '—';
  color: var(--public-accent);
}

.home-section {
  padding: clamp(80px, 11vh, 132px) 0;
  border-bottom: 1px solid var(--public-line);
  scroll-margin-top: 72px;
}

.home-story + .home-section {
  padding-top: clamp(5rem, 11vh, 8.25rem);
}

.home-section--surface {
  background: var(--public-surface);
}

.home-section-grid {
  display: grid;
  grid-template-columns: minmax(250px, 0.42fr) minmax(0, 1fr);
  gap: clamp(56px, 9vw, 148px);
}

.home-section-heading {
  align-self: start;
  position: sticky;
  top: 108px;
}

.home-section-heading h2,
.home-wide-heading h2,
.home-pricing-grid h2,
.home-docs-grid h2,
.home-privacy-grid h2 {
  margin: 1.125rem 0 0;
  color: var(--public-ink);
  font-size: clamp(2.5rem, 4.8vw, 3.75rem);
  font-weight: 500;
  letter-spacing: -0.03em;
  line-height: 1.04;
  white-space: pre-line;
}

.home-section-heading > p:last-child,
.home-wide-heading > p:last-child {
  max-width: 30rem;
  margin: 1.375rem 0 0;
  color: var(--public-muted);
  font-size: 1rem;
  line-height: 1.75;
}

.home-principle-list {
  border-top: 1px solid var(--public-line-strong);
}

.home-principle-row {
  display: grid;
  grid-template-columns: 54px minmax(170px, 0.55fr) minmax(0, 1fr);
  gap: 28px;
  align-items: baseline;
  padding: 28px 0;
  border-bottom: 1px solid var(--public-line);
}

.home-principle-row > span,
.home-step-list > li > span,
.home-model-name > span {
  color: var(--public-soft);
  font-family: var(--public-font-mono);
  font-size: 11px;
}

.home-principle-row h3,
.home-step-list h3 {
  margin: 0;
  color: var(--public-ink);
  font-size: 1.5rem;
  font-weight: 500;
  line-height: 1.25;
}

.home-principle-row p,
.home-step-list p {
  margin: 0;
  color: var(--public-muted);
  font-size: 1rem;
  line-height: 1.7;
}

.home-step-list {
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--public-line-strong);
  list-style: none;
}

.home-step-list li {
  display: grid;
  grid-template-columns: 54px minmax(0, 1fr);
  gap: 28px;
  padding: 30px 0;
  border-bottom: 1px solid var(--public-line);
}

.home-step-list li > div {
  display: grid;
  gap: 12px;
}

.home-step-list code {
  width: fit-content;
  max-width: 100%;
  overflow-x: auto;
  padding: 9px 11px;
  border: 1px solid var(--public-line);
  border-radius: 2px;
  background: var(--public-bg);
  color: var(--public-muted);
  font-family: var(--public-font-mono);
  font-size: 11px;
  white-space: nowrap;
}

.home-wide-heading {
  max-width: 900px;
  margin-bottom: 54px;
}

.home-model-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  border-top: 1px solid var(--public-line-strong);
  border-left: 1px solid var(--public-line);
}

.home-model-name {
  display: grid;
  gap: 1.625rem;
  min-height: 8.875rem;
  padding: 1.375rem;
  border-right: 1px solid var(--public-line);
  border-bottom: 1px solid var(--public-line);
  color: inherit;
  text-decoration: none;
}

.home-model-name strong {
  align-self: end;
  color: var(--public-ink);
  font-size: 1.875rem;
  font-weight: 500;
}

.home-model-name--more {
  background: var(--public-accent-soft);
  transition: background-color 180ms var(--public-ease), color 180ms var(--public-ease);
}

.home-model-name--more:hover {
  background: var(--public-accent);
}

.home-pricing-grid {
  display: grid;
  grid-template-columns: minmax(260px, 0.48fr) minmax(320px, 1fr) auto;
  gap: clamp(34px, 7vw, 100px);
  align-items: end;
}

.home-price-statement {
  padding-top: 22px;
  border-top: 1px solid var(--public-line-strong);
}

.home-price-statement strong {
  display: block;
  color: var(--public-ink);
  font-size: clamp(2.75rem, 4.5vw, 3.5rem);
  font-weight: 500;
  letter-spacing: -0.03em;
  line-height: 1;
}

.home-price-statement p {
  max-width: 38rem;
  margin: 1.375rem 0 0;
  color: var(--public-muted);
  font-size: 1rem;
  line-height: 1.7;
}

.home-price-statement span {
  display: block;
  margin-top: 14px;
  color: var(--public-soft);
  font-family: var(--public-font-mono);
  font-size: 11px;
  text-transform: uppercase;
}

.home-docs-section {
  color: var(--public-inverse-ink);
  background: var(--public-inverse-bg);
}

.home-docs-grid {
  display: grid;
  grid-template-columns: minmax(280px, 0.7fr) minmax(0, 1fr);
  gap: clamp(3rem, 10vw, 9.75rem);
  align-items: end;
}

.home-docs-grid .public-kicker,
.home-docs-copy p {
  color: var(--public-inverse-muted);
}

.home-docs-grid h2 {
  color: var(--public-inverse-ink);
}

.home-docs-copy {
  display: grid;
  gap: 1.75rem;
  justify-items: start;
  padding-top: 1.5rem;
  border-top: 1px solid var(--public-inverse-line);
}

.home-docs-copy p {
  max-width: 38rem;
  margin: 0;
  font-size: 1rem;
  line-height: 1.75;
}

.public-button--light {
  border-color: var(--public-inverse-ink);
  color: var(--public-inverse-ink);
}

.public-button--light:hover {
  background: var(--public-inverse-ink);
  color: var(--public-inverse-bg);
}

.home-docs-unavailable {
  color: var(--public-inverse-muted);
  font-family: var(--public-font-mono);
  font-size: 0.75rem;
}

.home-privacy-section {
  padding: clamp(4.75rem, 10vh, 7.375rem) 0;
  border-bottom: 1px solid var(--public-line);
  background: var(--public-surface);
}

.home-privacy-grid {
  display: grid;
  grid-template-columns: minmax(280px, 0.7fr) minmax(0, 1fr);
  gap: clamp(3rem, 10vw, 9.75rem);
}

.home-privacy-grid p {
  max-width: 46rem;
  margin: 1.5rem 0 0;
  color: var(--public-muted);
  font-size: 1rem;
  line-height: 1.75;
}

.home-privacy-grid ul {
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--public-line-strong);
  list-style: none;
}

.home-privacy-grid li {
  display: grid;
  grid-template-columns: 0.75rem minmax(0, 1fr);
  gap: 0.875rem;
  align-items: start;
  padding: 1.125rem 0;
  border-bottom: 1px solid var(--public-line);
  color: var(--public-ink);
  font-size: 1rem;
  line-height: 1.55;
}

.home-privacy-grid li::before {
  content: '';
  width: 0.5rem;
  height: 0.5rem;
  margin-top: 0.48em;
  border-radius: 50%;
  background: var(--public-accent);
}

.public-footer {
  padding: 1.75rem 0 max(1.75rem, env(safe-area-inset-bottom));
}

.public-footer-inner,
.public-footer-copy,
.public-footer-nav {
  display: flex;
  align-items: center;
}

.public-footer-inner {
  justify-content: space-between;
  gap: 1.5rem;
  color: var(--public-soft);
  font-family: var(--public-font-mono);
  font-size: 0.6875rem;
  text-transform: uppercase;
}

.public-footer-copy,
.public-footer-nav {
  gap: 1.25rem;
}

.public-footer-nav a {
  min-height: 2.75rem;
  display: inline-flex;
  align-items: center;
  color: var(--public-muted);
  text-decoration: none;
}

.public-footer-nav a:hover {
  color: var(--public-ink);
}

.public-home :where(a, button):focus-visible {
  outline: 2px solid var(--public-accent);
  outline-offset: 3px;
}

@media (prefers-reduced-motion: reduce) {
  .home-story-scene,
  .home-story-orbit {
    transition: none;
  }

  .home-story-scroll i::after {
    animation: none;
    opacity: 0.7;
    transform: none;
  }
}

@media (max-width: 1120px) {
  .public-header-inner {
    grid-template-columns: 1fr auto;
  }

  .public-nav {
    display: none;
  }

  .home-section-grid,
  .home-docs-grid,
  .home-privacy-grid {
    grid-template-columns: 1fr;
  }

  .home-story-scene {
    grid-template-columns: minmax(0, 1.05fr) minmax(19rem, 0.75fr);
    gap: 2.5rem;
  }

  .home-story-title {
    font-size: clamp(3.75rem, 7vw, 5.75rem);
  }

  .home-access-panel {
    max-width: 720px;
  }

  .home-section-heading {
    position: static;
    max-width: 760px;
  }

  .home-pricing-grid {
    grid-template-columns: 1fr 1fr;
  }

  .home-pricing-grid > .public-button {
    grid-column: 2;
    justify-self: start;
  }
}

@media (min-width: 901px) and (max-height: 820px) {
  .home-story-scene {
    padding-block: 3.25rem 7.5rem;
  }

  .home-story-title {
    font-size: clamp(3.5rem, 6.5vw, 5.25rem);
  }

  .home-hero-subtitle {
    margin-top: 1.25rem;
  }

  .home-hero-description {
    margin-top: 0.75rem;
    line-height: 1.55;
  }

  .home-hero-actions,
  .home-story-console-cta {
    margin-top: 1.25rem;
  }
}

@media (max-width: 900px) {
  .home-story {
    height: auto;
  }

  .home-story-stage {
    position: relative;
    top: auto;
    height: auto;
    min-height: 0;
    overflow: visible;
  }

  .home-story-grid {
    position: absolute;
    z-index: 0;
  }

  .home-story-index,
  .home-story-scroll {
    display: none;
  }

  .home-story-shell {
    height: auto;
    padding-inline: 0;
  }

  .home-story-controls {
    position: sticky;
    top: 5.25rem;
    bottom: auto;
    left: auto;
    z-index: 12;
    width: fit-content;
    margin-bottom: -4.5rem;
    margin-left: var(--public-gutter);
    padding: 0.4rem;
    border: 1px solid var(--public-line);
    background: var(--public-bg);
  }

  .home-story-scene {
    position: relative;
    inset: auto;
    grid-template-columns: 1fr;
    gap: 3rem;
    width: calc(100% - (2 * var(--public-gutter)));
    min-height: calc(100svh - 64px);
    margin-inline: var(--public-gutter);
    padding-block: clamp(5rem, 12vh, 7rem);
    border-bottom: 1px solid var(--public-line);
    opacity: 1;
    pointer-events: auto;
    scroll-margin-top: 64px;
    transform: none;
  }

  .home-story-scene:last-of-type {
    border-bottom: 0;
  }

  .home-story-copy {
    max-width: 44rem;
  }

  .home-story-title {
    max-width: 11ch;
    font-size: clamp(3.5rem, 11vw, 5.75rem);
    line-height: 0.92;
  }

  .home-story-evidence {
    justify-self: start;
    width: min(100%, 38rem);
    margin: 0;
  }

  .home-story-orbit {
    right: -8rem;
    bottom: 1rem;
    z-index: 0;
    opacity: 0.12;
    transform: rotate(-9deg) scale(0.9);
  }
}

@media (max-width: 760px) {
  .public-header-inner {
    min-height: 64px;
  }

  .public-brand span,
  .public-login-link {
    display: none;
  }

  .public-header-actions {
    gap: 6px;
  }

  .public-header-cta {
    min-height: 36px;
    padding-inline: 11px;
    font-size: 12px;
  }

  .home-story-scene {
    gap: 2.5rem;
    min-height: calc(100svh - 64px);
    padding-block: 4.5rem;
  }

  .home-story-title {
    font-size: clamp(3.25rem, 13vw, 4.75rem);
  }

  .home-story-title span:last-child {
    transform: translate3d(0.75rem, 0, 0);
  }

  .home-hero-subtitle {
    font-size: 1.1875rem;
  }

  .home-hero-description {
    font-size: 1rem;
  }

  .home-section {
    padding: 76px 0;
  }

  .home-story + .home-section {
    padding-top: 76px;
  }

  .home-section-heading h2,
  .home-wide-heading h2,
  .home-pricing-grid h2,
  .home-docs-grid h2,
  .home-privacy-grid h2 {
    font-size: 2.5rem;
  }

  .home-principle-row {
    grid-template-columns: 36px 1fr;
    gap: 12px 18px;
  }

  .home-principle-row p {
    grid-column: 2;
  }

  .home-model-grid {
    grid-template-columns: 1fr 1fr;
  }

  .home-model-name {
    min-height: 116px;
    padding: 18px;
  }

  .home-model-name strong {
    font-size: 1.5rem;
  }

  .home-pricing-grid {
    grid-template-columns: 1fr;
  }

  .home-pricing-grid > .public-button {
    grid-column: auto;
  }

  .home-price-statement strong {
    font-size: 2.75rem;
  }

  .home-privacy-grid li {
    font-size: 1rem;
  }

  .public-footer-inner,
  .public-footer-copy,
  .public-footer-nav {
    align-items: flex-start;
    flex-direction: column;
  }

  .public-footer-nav {
    gap: 0;
  }
}

@media (max-width: 430px) {
  .public-brand-logo {
    width: 28px;
    height: 28px;
  }

  .public-header-cta {
    width: 2.25rem;
    padding-inline: 0;
  }

  .public-header-cta-label {
    display: none;
  }

  .public-header-cta svg {
    display: block;
  }

  .home-story-title {
    font-size: 2.75rem;
  }

  .home-status-line {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .home-hero-actions,
  .home-hero-actions .public-button,
  .home-story-console-cta {
    width: 100%;
  }

  .home-endpoint-row {
    gap: 0.75rem;
    padding-inline: 0.875rem;
  }

  .home-principle-row {
    grid-template-columns: 1fr;
  }

  .home-principle-row p {
    grid-column: auto;
  }

  .home-step-list li {
    grid-template-columns: 1fr;
  }

  .home-model-grid {
    grid-template-columns: 1fr;
  }
}
</style>
