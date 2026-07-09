<template>
  <div class="auth-page monolog-scope">
    <a class="public-skip-link" href="#auth-form-content">{{ t('auth.brand.skipToForm') }}</a>

    <header class="auth-header">
      <div class="public-shell auth-header-inner">
        <router-link to="/home" class="auth-brand" :aria-label="siteName">
          <img :src="siteLogo || '/logo.png'" :alt="siteName" class="auth-logo" />
          <span>{{ siteName }}</span>
        </router-link>

        <div class="auth-header-actions">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="auth-docs-link"
          >
            {{ t('auth.brand.navDocs') }}
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
        <div>
          <p class="public-kicker auth-eyebrow">
            <span aria-hidden="true"></span>
            {{ t('auth.brand.eyebrow') }}
          </p>
          <h1>
            <span>{{ t('auth.brand.titleLine1') }}</span>
            <span>{{ t('auth.brand.titleLine2') }}</span>
          </h1>
          <p class="auth-subtitle">{{ t('auth.brand.subtitle') }}</p>
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
          <router-link to="/home">{{ t('auth.brand.navHome') }}</router-link>
          <span>&copy; {{ currentYear }} {{ siteName }}</span>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { PUBLIC_DOCS_URL, sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const appStore = useAppStore()

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

.auth-logo {
  width: 30px;
  height: 30px;
  object-fit: contain;
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
  grid-template-columns: minmax(0, 1.15fr) minmax(460px, 0.85fr);
  min-height: calc(100svh - 73px);
  padding-inline: 0;
  border-left: 1px solid var(--public-line);
  border-right: 1px solid var(--public-line);
}

.auth-context {
  display: grid;
  grid-template-rows: 1fr auto auto;
  gap: 52px;
  min-width: 0;
  padding: clamp(64px, 9vh, 112px) clamp(34px, 6vw, 96px) 44px;
}

.auth-eyebrow {
  display: flex;
  align-items: center;
  gap: 10px;
}

.auth-eyebrow span {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--public-accent);
}

.auth-context h1 {
  display: grid;
  gap: 0.125rem;
  max-width: 9ch;
  margin: 1.875rem 0 0;
  color: var(--public-ink);
  font-size: clamp(3.5rem, 6.5vw, 5.25rem);
  font-weight: 500;
  letter-spacing: -0.035em;
  line-height: 0.96;
}

.auth-context h1 span:last-child {
  color: var(--public-muted);
}

.auth-subtitle {
  max-width: 36rem;
  margin: 1.875rem 0 0;
  color: var(--public-muted);
  font-size: 1rem;
  line-height: 1.75;
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

  .auth-context h1 {
    max-width: 12ch;
    font-size: 3.25rem;
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

  .auth-context h1 {
    margin-top: 22px;
    font-size: 2.625rem;
    line-height: 1.04;
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
