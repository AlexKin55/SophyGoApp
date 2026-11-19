import { env } from '../config/env'

/**
 * Минимальный адаптер Telegram Mini App.
 *
 * SDK подключается в index.html. В обычном браузере window.Telegram отсутствует,
 * и приложение продолжает работать без голосового Гида.
 */
export interface TelegramWebApp {
  initData: string
  initDataUnsafe?: {
    user?: {
      id: number
      username?: string
      first_name?: string
    }
  }
  ready(): void
  expand(): void
  close(): void
  isExpanded: boolean
  setHeaderColor?(color: string): void
  setBackgroundColor?(color: string): void
}

declare global {
  interface Window {
    Telegram?: {
      WebApp?: TelegramWebApp
    }
  }
}

export function getTelegramWebApp(): TelegramWebApp | null {
  return window.Telegram?.WebApp ?? null
}

/**
 * Ждёт загрузку Telegram SDK, подключённого в index.html с async.
 *
 * Если SDK уже доступен — возвращает его сразу. Если скрипт не загрузился
 * (нет сети, telegram.org заблокирован, обычный браузер) — ждёт не дольше
 * таймаута и возвращает null, не блокируя запуск приложения.
 */
export function loadTelegramSdk(timeoutMs = env.telegramSdkTimeoutMs): Promise<TelegramWebApp | null> {
  const existing = getTelegramWebApp()
  if (existing) return Promise.resolve(existing)

  const script = document.querySelector<HTMLScriptElement>('script[data-telegram-sdk]')
  if (!script) return Promise.resolve(null)

  return new Promise((resolve) => {
    let settled = false
    const finish = () => {
      if (settled) return
      settled = true
      window.clearTimeout(timer)
      script.removeEventListener('load', finish)
      script.removeEventListener('error', finish)
      resolve(getTelegramWebApp())
    }
    const timer = window.setTimeout(finish, timeoutMs)
    script.addEventListener('load', finish)
    script.addEventListener('error', finish)
  })
}

/** Инициализирует Mini App: сообщает Telegram, что приложение готово. */
export function initTelegram(): TelegramWebApp | null {
  const app = getTelegramWebApp()
  if (!app) return null

  try {
    app.ready()
    app.expand()
    app.setHeaderColor?.('#ffffff')
    app.setBackgroundColor?.('#ffffff')
  } catch {
    // Telegram SDK не должен ломать запуск приложения.
  }

  return app
}

/** Подписанная строка Telegram, которую нужно передавать на backend. */
export function getTelegramInitData(): string {
  return getTelegramWebApp()?.initData ?? ''
}
