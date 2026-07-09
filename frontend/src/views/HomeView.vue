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
          <img :src="siteLogo || '/logo.png'" :alt="siteName" class="public-brand-logo" />
          <span>{{ siteName }}</span>
        </router-link>

        <nav class="public-nav" :aria-label="t('home.aria.primaryNav')">
          <a href="#principles">{{ t('home.nav.features') }}</a>
          <a href="#access">{{ t('home.nav.workflow') }}</a>
          <a href="#models">{{ t('home.nav.models') }}</a>
          <a href="#pricing">{{ t('home.nav.pricing') }}</a>
          <a
            data-testid="home-docs-link"
            :href="docUrl || '#docs'"
            :target="docUrl ? '_blank' : undefined"
            :rel="docUrl ? 'noopener noreferrer' : undefined"
          >
            {{ t('home.docs') }}
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
            {{ t('home.login') }}
          </router-link>
          <router-link :to="isAuthenticated ? dashboardPath : '/register'" class="public-header-cta">
            {{ isAuthenticated ? t('home.dashboard') : t('home.getStarted') }}
            <Icon name="externalLink" size="xs" :stroke-width="1.5" />
          </router-link>
        </div>
      </div>
    </header>

    <main id="home-main">
      <section class="home-hero" aria-labelledby="home-title">
        <div class="public-shell home-hero-grid">
          <div class="home-hero-copy">
            <p class="public-kicker home-status-line">
              <span class="home-status-dot" aria-hidden="true"></span>
              {{ t('home.hero.badge') }}
            </p>
            <h1 id="home-title">{{ t('home.hero.posterStatement') }}</h1>
            <p class="home-hero-subtitle">{{ t('home.hero.posterSubstatement') }}</p>
            <p class="home-hero-description">{{ t('home.hero.description') }}</p>

            <div class="home-hero-actions">
              <router-link :to="isAuthenticated ? dashboardPath : '/register'" class="public-button public-button--primary">
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.hero.ctaPrimary') }}
                <Icon name="arrowRight" size="sm" :stroke-width="1.5" />
              </router-link>
              <a
                v-if="docUrl"
                :href="docUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="public-button"
              >
                {{ t('home.hero.ctaDocs') }}
                <Icon name="externalLink" size="sm" :stroke-width="1.5" />
              </a>
              <a v-else href="#docs" class="public-button">
                {{ t('home.hero.ctaDocs') }}
                <Icon name="arrowDown" size="sm" :stroke-width="1.5" />
              </a>
            </div>
          </div>

          <aside class="home-access-panel" :aria-label="t('home.aria.endpoints')">
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
                  :title="copiedKey === endpoint.key ? t('home.hero.copied') : t('home.hero.copy')"
                  :aria-label="copiedKey === endpoint.key ? t('home.hero.copied') : t('home.hero.copy')"
                  @click="copyEndpoint(endpoint)"
                >
                  <Icon :name="copiedKey === endpoint.key ? 'check' : 'copy'" size="sm" :stroke-width="1.5" />
                </button>
              </div>
            </div>

            <div class="home-access-meta">
              <span>{{ t('home.hero.metaModels') }}</span>
              <span>{{ t('home.hero.metaControl') }}</span>
              <span>{{ t('home.privacy.minimum') }}</span>
            </div>
          </aside>
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
})

onBeforeUnmount(() => {
  if (copyTimer) clearTimeout(copyTimer)
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

.home-hero {
  min-height: calc(100svh - 260px);
  border-bottom: 1px solid var(--public-line);
}

.home-hero-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.22fr) minmax(360px, 0.78fr);
  gap: clamp(42px, 7vw, 112px);
  align-items: center;
  min-height: calc(100svh - 260px);
  padding-top: clamp(52px, 8vh, 92px);
  padding-bottom: clamp(64px, 10vh, 110px);
}

.home-hero-copy {
  max-width: 850px;
}

.home-status-line {
  gap: 10px;
  margin-bottom: 32px;
}

.home-status-dot {
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 50%;
  background: var(--public-accent);
}

.home-hero h1 {
  max-width: 9.5ch;
  margin: 0;
  color: var(--public-ink);
  font-size: clamp(3.25rem, 7vw, 5.75rem);
  font-weight: 500;
  letter-spacing: -0.035em;
  line-height: 0.96;
  white-space: pre-line;
}

.home-hero-subtitle {
  max-width: 34rem;
  margin: 1.875rem 0 0;
  color: var(--public-ink);
  font-size: clamp(1.125rem, 1.6vw, 1.375rem);
  line-height: 1.45;
}

.home-hero-description {
  max-width: 42rem;
  margin: 1.25rem 0 0;
  color: var(--public-muted);
  font-size: 1rem;
  line-height: 1.75;
}

.home-hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 36px;
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

.home-hero + .home-section {
  padding-top: 56px;
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

@media (max-width: 1120px) {
  .public-header-inner {
    grid-template-columns: 1fr auto;
  }

  .public-nav {
    display: none;
  }

  .home-hero-grid,
  .home-section-grid,
  .home-docs-grid,
  .home-privacy-grid {
    grid-template-columns: 1fr;
  }

  .home-hero-grid {
    gap: 52px;
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

@media (min-width: 1121px) and (max-height: 820px) {
  .home-hero-grid {
    padding-top: 48px;
    padding-bottom: 56px;
  }

  .home-hero h1 {
    font-size: 4.5rem;
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

  .home-hero,
  .home-hero-grid {
    min-height: auto;
  }

  .home-hero-grid {
    padding-top: 58px;
    padding-bottom: 68px;
  }

  .home-hero h1 {
    max-width: 10.5ch;
    font-size: 3.25rem;
    line-height: 1.02;
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

  .home-hero + .home-section {
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

  .public-header-cta svg {
    display: none;
  }

  .home-hero h1 {
    font-size: 2.75rem;
  }

  .home-hero-actions,
  .home-hero-actions .public-button {
    width: 100%;
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
