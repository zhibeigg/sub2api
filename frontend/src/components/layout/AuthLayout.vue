<template>
  <div class="auth-page monolog-scope">
    <a class="public-skip-link" href="#auth-form-content">{{ t('auth.brand.skipToForm') }}</a>

    <header class="auth-header">
      <div class="public-shell auth-header-inner">
        <router-link to="/home" class="auth-brand" :aria-label="siteName">
          <span class="auth-logo-lockup" aria-hidden="true">
            <img :src="siteLogo || '/logo.svg'" alt="" class="auth-logo" />
          </span>
          <LetterSwapText :text="siteName" />
        </router-link>

        <div class="auth-header-actions">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="auth-docs-link"
          >
            <LetterSwapText :text="t('auth.brand.navDocs')" />
            <Icon name="externalLink" size="xs" :stroke-width="1.5" />
          </a>
          <LocaleSwitcher />
          <button
            type="button"
            class="auth-icon-button"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="sm" :stroke-width="1.5" />
            <Icon v-else name="moon" size="sm" :stroke-width="1.5" />
          </button>
        </div>
      </div>
    </header>

    <div class="public-shell auth-main">
      <aside class="auth-context">
        <SignalTrail :labels="trailLabels" bounded />
        <div class="auth-context-rails" aria-hidden="true"></div>

        <div class="auth-poster">
          <p class="public-kicker auth-eyebrow">
            <span class="auth-live-dot" aria-hidden="true"></span>
            {{ t('auth.brand.eyebrow') }}
            <b>{{ chapterCode }}</b>
          </p>

          <div class="auth-poster-stage">
            <div class="auth-poster-copy">
              <p class="auth-chapter-label">{{ chapterLabel }}</p>
              <h1 :key="authMode">
                <span>{{ titleLine1 }}</span>
                <span>{{ titleLine2 }}</span>
              </h1>
              <p class="auth-subtitle">{{ subtitle }}</p>
            </div>

            <ProtocolOrbit
              class="auth-protocol-orbit"
              :scene="sceneIndex"
              :label="t('auth.brand.orbitLabel')"
              :status="t('auth.brand.orbitStatus')"
              core-label="AUTH"
              compact
            />
          </div>
        </div>

        <dl class="auth-facts">
          <div>
            <dt>01</dt>
            <dd>{{ t('auth.brand.factStable') }}</dd>
          </div>
          <div>
            <dt>02</dt>
            <dd>{{ t('auth.brand.factTransparent') }}</dd>
          </div>
        </dl>

        <section class="auth-privacy" aria-labelledby="auth-privacy-title">
          <div class="auth-privacy-heading">
            <Icon name="lock" size="sm" :stroke-width="1.5" />
            <h2 id="auth-privacy-title">{{ t('auth.brand.privacyTitle') }}</h2>
          </div>
          <p>{{ t('auth.brand.privacyNote') }}</p>
        </section>
      </aside>

      <main class="auth-form-column">
        <section id="auth-form-content" class="auth-form" aria-live="polite">
          <p class="public-kicker auth-form-kicker">{{ t('auth.brand.formKicker') }}</p>
          <slot />
        </section>

        <div class="auth-form-footer">
          <slot name="footer" />
        </div>

        <div class="auth-meta">
          <router-link to="/home"><LetterSwapText :text="t('auth.brand.navHome')" /></router-link>
          <span>&copy; {{ currentYear }} {{ siteName }}</span>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import LetterSwapText from '@/components/public/LetterSwapText.vue'
import ProtocolOrbit from '@/components/public/ProtocolOrbit.vue'
import SignalTrail from '@/components/public/SignalTrail.vue'
import { useAppStore } from '@/stores'
import { PUBLIC_DOCS_URL, sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const appStore = useAppStore()
const route = useRoute()

const authMode = computed(() => {
  const path = route.path.toLowerCase()
  return path.includes('register') || path.includes('email-verify') ? 'register' : 'login'
})
const sceneIndex = computed(() => (authMode.value === 'register' ? 1 : 0))
const chapterCode = computed(() => (authMode.value === 'register' ? '02 / CREATE' : '01 / RETURN'))
const chapterLabel = computed(() => t(`auth.brand.${authMode.value}Chapter`))
const titleLine1 = computed(() => t(`auth.brand.${authMode.value}TitleLine1`))
const titleLine2 = computed(() => t(`auth.brand.${authMode.value}TitleLine2`))
const subtitle = computed(() => t(`auth.brand.${authMode.value}Subtitle`))
const trailLabels = ['AUTH', 'KEY', '/v1', 'SECURE', 'ROUTE', '200 OK']

const siteName = computed(
  () => appStore.siteName || appStore.cachedPublicSettings?.site_name || 'Sub2API'
)
const siteLogo = computed(() =>
  sanitizeUrl(appStore.siteLogo || appStore.cachedPublicSettings?.site_logo || '', {
    allowRelative: true,
    allowDataUrl: true
  })
)
const docUrl = PUBLIC_DOCS_URL
const currentYear = new Date().getFullYear()
const isDark = ref(document.documentElement.classList.contains('dark'))

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

onMounted(() => {
  isDark.value = document.documentElement.classList.contains('dark')
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.auth-page {
  min-height: 100vh;
  overflow-x: clip;
}

.auth-header {
  border-bottom: 1px solid var(--public-line);
}

.auth-header-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 72px;
}

.auth-brand,
.auth-header-actions,
.auth-docs-link,
.auth-privacy-heading,
.auth-meta {
  display: flex;
  align-items: center;
}

.auth-brand {
  gap: 10px;
  color: var(--public-ink);
  font-size: 18px;
  text-decoration: none;
}

.auth-logo-lockup {
  display: grid;
  place-items: center;
  width: 2rem;
  aspect-ratio: 1;
  border: 1px solid var(--public-line);
  border-radius: 50%;
  transition: transform 420ms var(--public-ease), border-color 180ms var(--public-ease);
}

.auth-logo {
  width: 1.65rem;
  height: 1.65rem;
  object-fit: contain;
  transition: transform 420ms var(--public-ease);
}

.auth-brand:hover .auth-logo-lockup {
  border-color: var(--public-accent);
  transform: rotate(18deg);
}

.auth-brand:hover .auth-logo {
  transform: rotate(-18deg) scale(0.88);
}

.auth-header-actions {
  gap: 8px;
}

.auth-docs-link {
  justify-content: center;
  gap: 7px;
  min-height: 36px;
  padding: 0 10px;
  color: var(--public-muted);
  font-family: var(--public-font-mono);
  font-size: 11px;
  text-decoration: none;
  text-transform: uppercase;
}

.auth-docs-link:hover {
  color: var(--public-ink);
}

.auth-icon-button {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border: 1px solid var(--public-line);
  border-radius: 4px;
  background: transparent;
  color: var(--public-ink);
  cursor: pointer;
  transition: border-color 160ms var(--public-ease), background-color 160ms var(--public-ease);
}

.auth-icon-button:hover {
  border-color: var(--public-line-strong);
  background: var(--public-surface-strong);
}

.auth-main {
  display: grid;
  grid-template-columns: minmax(0, 1.22fr) minmax(460px, 0.78fr);
  min-height: calc(100svh - 73px);
  padding-inline: 0;
  border-left: 1px solid var(--public-line);
  border-right: 1px solid var(--public-line);
}

.auth-context {
  position: relative;
  display: grid;
  grid-template-rows: minmax(0, 1fr) auto auto;
  gap: 44px;
  min-width: 0;
  overflow: hidden;
  padding: clamp(50px, 7vh, 88px) clamp(34px, 5vw, 78px) 36px;
  isolation: isolate;
}

.auth-context-rails {
  position: absolute;
  inset: 0;
  z-index: -2;
  background:
    linear-gradient(90deg, transparent 0 31%, var(--public-line) 31% calc(31% + 1px), transparent calc(31% + 1px) 68%, var(--public-line) 68% calc(68% + 1px), transparent calc(68% + 1px)),
    linear-gradient(180deg, transparent 0 57%, var(--public-line) 57% calc(57% + 1px), transparent calc(57% + 1px));
  opacity: 0.42;
  pointer-events: none;
}

.auth-context-rails::after {
  content: '';
  position: absolute;
  width: min(38vw, 34rem);
  aspect-ratio: 1;
  right: -17%;
  bottom: -24%;
  border: 1px solid var(--public-line);
  border-radius: 50%;
  box-shadow: 0 0 0 3.25rem var(--public-bg), 0 0 0 calc(3.25rem + 1px) var(--public-line);
  opacity: 0.55;
}

.auth-poster {
  min-height: 0;
}

.auth-eyebrow {
  display: flex;
  align-items: center;
  gap: 10px;
}

.auth-eyebrow b {
  margin-left: auto;
  color: var(--public-soft);
  font-weight: 400;
  font-variant-numeric: tabular-nums;
}

.auth-live-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--public-accent);
  box-shadow: 0 0 0 0.35rem var(--public-accent-soft);
}

.auth-poster-stage {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(15rem, 0.55fr);
  gap: clamp(24px, 4vw, 68px);
  align-items: center;
  min-height: clamp(24rem, 58vh, 38rem);
}

.auth-poster-copy {
  position: relative;
  z-index: 2;
}

.auth-chapter-label {
  margin: 0 0 1.125rem;
  color: var(--public-soft);
  font-family: var(--public-font-mono);
  font-size: 0.625rem;
  text-transform: uppercase;
}

.auth-context h1 {
  display: grid;
  gap: 0.125rem;
  max-width: 10ch;
  margin: 0;
  color: var(--public-ink);
  font-size: clamp(3.75rem, 7vw, 6.75rem);
  font-style: oblique 10deg;
  font-weight: 500;
  letter-spacing: -0.055em;
  line-height: 0.84;
  transform: skewX(-3deg);
  animation: auth-title-enter 680ms var(--public-ease) both;
}

.auth-context h1 span {
  white-space: nowrap;
}

.auth-context h1 span:last-child {
  margin-left: clamp(1.75rem, 6vw, 6rem);
  color: var(--public-muted);
}

.auth-subtitle {
  max-width: 32rem;
  margin: 2rem 0 0;
  color: var(--public-muted);
  font-size: 1rem;
  line-height: 1.75;
}

.auth-protocol-orbit {
  justify-self: end;
  margin-right: -34%;
  opacity: 0.82;
}

@keyframes auth-title-enter {
  from {
    opacity: 0;
    transform: translate3d(0, 2rem, 0) skewX(-3deg);
  }
  to {
    opacity: 1;
    transform: translate3d(0, 0, 0) skewX(-3deg);
  }
}

.auth-facts {
  display: grid;
  grid-template-columns: 1fr 1fr;
  margin: 0;
  border-top: 1px solid var(--public-line-strong);
}

.auth-facts div {
  display: grid;
  gap: 18px;
  min-height: 112px;
  padding: 20px 22px 18px 0;
  border-right: 1px solid var(--public-line);
}

.auth-facts div:last-child {
  padding-left: 22px;
  border-right: 0;
}

.auth-facts dt {
  color: var(--public-soft);
  font-family: var(--public-font-mono);
  font-size: 10px;
}

.auth-facts dd {
  align-self: end;
  margin: 0;
  color: var(--public-ink);
  font-size: 1.375rem;
}

.auth-privacy {
  padding-top: 20px;
  border-top: 1px solid var(--public-line);
}

.auth-privacy-heading {
  gap: 8px;
}

.auth-privacy-heading h2 {
  margin: 0;
  color: var(--public-ink);
  font-family: var(--public-font-mono);
  font-size: 11px;
  font-weight: 400;
  text-transform: uppercase;
}

.auth-privacy p {
  max-width: 38rem;
  margin: 12px 0 0;
  color: var(--public-muted);
  font-size: 13px;
  line-height: 1.7;
}

.auth-form-column {
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: clamp(58px, 8vh, 96px) clamp(28px, 5vw, 76px) 32px;
  border-left: 1px solid var(--public-line);
  background: var(--public-surface);
  animation: auth-form-enter 620ms 100ms var(--public-ease) both;
}

@keyframes auth-form-enter {
  from {
    opacity: 0;
    transform: translate3d(1.25rem, 0, 0);
  }
  to {
    opacity: 1;
    transform: translate3d(0, 0, 0);
  }
}

.auth-form {
  width: min(100%, 540px);
  margin-inline: auto;
}

.auth-form-kicker {
  margin-bottom: 22px;
}

.auth-form-footer {
  width: min(100%, 540px);
  margin: 24px auto 0;
  color: var(--public-muted);
  font-size: 13px;
  text-align: left;
}

.auth-meta {
  justify-content: space-between;
  gap: 18px;
  width: min(100%, 540px);
  margin: auto auto 0;
  padding-top: 42px;
  color: var(--public-soft);
  font-family: var(--public-font-mono);
  font-size: 10px;
  text-transform: uppercase;
}

.auth-meta a {
  color: inherit;
  text-decoration: none;
}

.auth-meta a:hover {
  color: var(--public-ink);
}

.auth-form :deep(*) {
  min-width: 0;
}

.auth-form :deep(h2) {
  margin: 0;
  color: var(--public-ink);
  font-size: 2.5rem;
  font-weight: 500;
  letter-spacing: -0.025em;
  line-height: 1.08;
}

.auth-form :deep(h2 + p) {
  margin-top: 0.625rem;
  color: var(--public-muted);
  font-size: 1rem;
  line-height: 1.6;
}

.auth-form :deep(.input-label) {
  margin-bottom: 8px;
  color: var(--public-muted);
  font-family: var(--public-font-mono);
  font-size: 10px;
  font-weight: 400;
  text-transform: uppercase;
}

.auth-form :deep(.input) {
  min-height: 48px;
  border: 1px solid var(--public-line);
  border-radius: 2px;
  background: var(--public-bg);
  color: var(--public-ink);
  box-shadow: none;
}

.auth-form :deep(.input::placeholder) {
  color: var(--public-soft);
}

.auth-form :deep(.input:focus),
.auth-form :deep(.input:focus-visible) {
  border-color: var(--public-ink);
  outline: 3px solid color-mix(in oklch, var(--public-accent) 48%, transparent);
  outline-offset: 1px;
  box-shadow: none;
}

.auth-form :deep(.input-error) {
  border-color: var(--public-danger);
  animation: auth-field-error 260ms var(--public-ease);
}

@keyframes auth-field-error {
  0%, 100% { transform: translateX(0); }
  35% { transform: translateX(-0.25rem); }
  70% { transform: translateX(0.2rem); }
}

.auth-form :deep(.input-hint),
.auth-form :deep(.text-gray-400),
.auth-form :deep(.text-gray-500),
.auth-form :deep(.dark\:text-dark-400),
.auth-form :deep(.dark\:text-dark-500) {
  color: var(--public-soft);
}

.auth-form :deep(.btn) {
  min-height: 48px;
  border-radius: 2px;
  box-shadow: none;
}

.auth-form :deep(.btn-primary) {
  border: 1px solid var(--public-accent);
  background: var(--public-accent);
  color: var(--public-inverse-bg);
  font-family: var(--public-font-mono);
  font-size: 0.75rem;
  text-transform: uppercase;
}

.auth-form :deep(.btn-primary:hover:not(:disabled)) {
  border-color: var(--public-ink);
  background: var(--public-ink);
  color: var(--public-bg);
}

.auth-form :deep(.btn:active:not(:disabled)) {
  transform: translateY(0.125rem) scale(0.992);
  transition-duration: 90ms;
}

.auth-form :deep(.btn-primary svg) {
  color: currentColor;
}

.auth-form :deep(.btn-secondary),
.auth-form :deep(.btn-ghost) {
  border: 1px solid var(--public-line);
  background: transparent;
  color: var(--public-ink);
}

.auth-form :deep(.btn-secondary:hover:not(:disabled)),
.auth-form :deep(.btn-ghost:hover:not(:disabled)) {
  border-color: var(--public-line-strong);
  background: var(--public-surface-strong);
}

.auth-form :deep(a),
.auth-form-footer :deep(a) {
  color: var(--public-ink);
  text-decoration: underline;
  text-decoration-thickness: 1px;
  text-underline-offset: 3px;
}

.auth-form :deep(a:hover),
.auth-form-footer :deep(a:hover) {
  color: var(--public-muted);
}

.auth-form :deep(.h-px) {
  background: var(--public-line);
}

.auth-form :deep(.auth-status),
.auth-form :deep(.auth-validation-success) {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  border: 1px solid var(--public-line);
  border-radius: 2px;
  padding: 0.875rem 1rem;
}

.auth-form :deep(.auth-status--warning) {
  background: var(--public-surface-strong);
  color: var(--public-ink);
}

.auth-form :deep(.auth-validation-success) {
  margin-top: 0.5rem;
  background: var(--public-accent-soft);
  color: var(--public-ink);
}

.auth-form :deep(.auth-status p),
.auth-form :deep(.auth-validation-success span) {
  margin: 0;
  color: inherit;
  font-size: 0.875rem;
  line-height: 1.55;
}

@media (max-width: 980px) {
  .auth-main {
    grid-template-columns: 1fr;
    border-inline: 0;
  }

  .auth-context {
    grid-row: 2;
    grid-template-rows: auto auto;
    gap: 2.125rem;
    padding: 3rem var(--public-gutter) max(2.25rem, env(safe-area-inset-bottom));
    border-top: 1px solid var(--public-line);
  }

  .auth-poster-stage {
    grid-template-columns: minmax(0, 1fr) auto;
    min-height: auto;
    padding-top: 2.5rem;
  }

  .auth-context h1 {
    max-width: 12ch;
    font-size: clamp(3.25rem, 9vw, 5rem);
  }

  .auth-protocol-orbit {
    width: 13rem;
    margin-right: -30%;
  }

  .auth-facts {
    display: none;
  }

  .auth-privacy {
    max-width: 680px;
  }

  .auth-form-column {
    grid-row: 1;
    padding: 3.375rem var(--public-gutter) 1.875rem;
    border-left: 0;
  }
}

@media (max-width: 600px) {
  .auth-header-inner {
    min-height: 64px;
  }

  .auth-brand span,
  .auth-docs-link {
    display: none;
  }

  .auth-context {
    padding-top: 38px;
  }

  .auth-poster-stage {
    display: block;
    padding-top: 2rem;
  }

  .auth-context h1 {
    font-size: clamp(2.75rem, 16vw, 4rem);
    line-height: 0.9;
  }

  .auth-context h1 span:last-child {
    margin-left: 1.25rem;
  }

  .auth-protocol-orbit {
    position: absolute;
    right: -5rem;
    bottom: 5.5rem;
    width: 12rem;
    margin: 0;
    opacity: 0.28;
  }

  .auth-subtitle {
    margin-top: 20px;
    font-size: 1rem;
  }

  .auth-privacy {
    padding-top: 16px;
  }

  .auth-form-column {
    padding-top: 42px;
  }

  .auth-form :deep(h2) {
    font-size: 2.125rem;
  }

  .auth-meta {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
