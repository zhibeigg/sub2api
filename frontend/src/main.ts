import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import i18n, { initI18n } from './i18n'
import { useAppStore } from '@/stores/app'
import { updateFavicon } from '@/utils/branding'
import { isIOSDevice } from '@/utils/device'
import './style.css'
import './styles/monolog.css'

export function renderFatalApplicationError(root: HTMLElement | null = document.getElementById('app')) {
  if (!root || root.dataset.fatalError === 'true') return
  root.dataset.fatalError = 'true'
  root.replaceChildren()

  const language = document.documentElement.lang.toLowerCase()
  const english = language.startsWith('en')
  const japanese = language.startsWith('ja')
  const copy = english
    ? {
        title: 'The page could not be displayed',
        message: 'A client-side error occurred. No internal error details were exposed.',
        retry: 'Reload page',
        home: 'Return home'
      }
    : japanese
      ? {
          title: 'ページを表示できませんでした',
          message: 'クライアント側エラーが発生しました。内部エラーの詳細は表示されません。',
          retry: 'ページを再読み込み',
          home: 'ホームへ戻る'
        }
      : {
          title: '页面暂时无法显示',
          message: '客户端发生异常，内部错误详情不会暴露。',
          retry: '重新加载页面',
          home: '返回首页'
        }

  const panel = document.createElement('main')
  panel.className = 'fatal-error-fallback'
  panel.setAttribute('role', 'alert')
  const title = document.createElement('h1')
  title.textContent = copy.title
  const message = document.createElement('p')
  message.textContent = copy.message
  const actions = document.createElement('div')
  actions.className = 'fatal-error-actions'
  const retry = document.createElement('button')
  retry.type = 'button'
  retry.textContent = copy.retry
  retry.addEventListener('click', () => window.location.reload())
  const home = document.createElement('button')
  home.type = 'button'
  home.textContent = copy.home
  home.addEventListener('click', () => window.location.assign('/'))
  actions.append(retry, home)
  panel.append(title, message, actions)
  root.append(panel)
}

function initIOSViewportZoomFix() {
  // iOS Safari 在输入框字号小于 16px 时聚焦会自动放大页面，且失焦后不会恢复。
  // 限制 maximum-scale 可阻止该行为；iOS 10+ 用户仍可双指手动缩放，不影响可访问性。
  // 仅在 iOS 设备上注入，避免影响 Android Chrome 的手动缩放能力。
  if (!isIOSDevice()) return

  const viewport = document.querySelector('meta[name="viewport"]')
  if (!viewport) return

  const content = viewport.getAttribute('content') || ''
  if (/maximum-scale/i.test(content)) return
  viewport.setAttribute('content', `${content}, maximum-scale=1.0`)
}

function initThemeClass() {
  const savedTheme = localStorage.getItem('theme')
  const shouldUseDark =
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', shouldUseDark)
}

async function bootstrap() {
  // Apply theme class globally before app mount to keep all routes consistent.
  initThemeClass()
  initIOSViewportZoomFix()

  const app = createApp(App)
  app.config.errorHandler = (error, _instance, info) => {
    const errorName = error instanceof Error ? error.name : 'UnknownError'
    console.error('Unhandled Vue application error', { errorName, info })
    renderFatalApplicationError()
  }
  const pinia = createPinia()
  app.use(pinia)

  // Initialize settings from injected config BEFORE mounting (prevents flash)
  // This must happen after pinia is installed but before router and i18n
  const appStore = useAppStore()
  appStore.initFromInjectedConfig()

  // Apply favicon before mount; route SEO owns the document title.
  updateFavicon(appStore.siteLogo)

  await initI18n()

  app.use(router)
  app.use(i18n)

  // 等待路由器完成初始导航后再挂载，避免竞态条件导致的空白渲染
  await router.isReady()
  app.mount('#app')
}

if (!import.meta.env.VITEST) {
  bootstrap().catch((error) => {
    const errorName = error instanceof Error ? error.name : 'BootstrapError'
    console.error('Application bootstrap failed', { errorName })
    renderFatalApplicationError()
  })
}
