import { env } from '../config/env'
import { getTelegramInitData, initTelegram } from './telegram'

/**
 * Единый слой над Mini App мессенджеров.
 *
 * Telegram отдаёт данные через window.Telegram.WebApp, MAX — через
 * window.WebApp (библиотека MAX Bridge). Оба объекта содержат строку
 * initData, которую backend проверяет по подписи.
 */
export type MessengerKind = 'telegram' | 'max' | 'browser'

export interface MaxWebApp {
  initData: string
  initDataUnsafe?: {
    user?: {
      id: number
      first_name?: string
      last_name?: string
      username?: string
      language_code?: string
    }
  }
  platform?: string
  version?: string
  /** Сообщает MAX, что мини-приложение готово (событие WebAppReady). */
  ready?: () => void
  /** Разворачивает мини-приложение на доступную область экрана. */
  expand?: () => void
}

declare global {
  interface Window {
    WebApp?: MaxWebApp
  }
}

export function getMaxWebApp(): MaxWebApp | null {
  return window.WebApp ?? null
}

/**
 * Мессенджер определяется по непустому initData: SDK обоих мессенджеров
 * могут быть загружены одновременно, но данные заполняет только активный.
 */
export function getMessengerKind(): MessengerKind {
  if (getTelegramInitData()) return 'telegram'
  if (getMaxWebApp()?.initData) return 'max'
  return 'browser'
}

/** Подписанная строка активного мессенджера для заголовка X-Init-Data. */
export function getInitData(): string {
  return getTelegramInitData() || getMaxWebApp()?.initData || ''
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

/**
 * Ждёт, пока SDK мессенджера проставит initData.
 * В обычном браузере возвращает 'browser' по таймауту, не блокируя запуск.
 */
export async function loadMessengerSdk(
  timeoutMs = env.telegramSdkTimeoutMs
): Promise<MessengerKind> {
  const initial = getMessengerKind()
  if (initial !== 'browser') return initial

  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    await sleep(100)
    const kind = getMessengerKind()
    if (kind !== 'browser') return kind
  }

  return 'browser'
}

/** Инициализирует MAX Bridge: без WebAppReady клиент может показать ошибку. */
export function initMax(): MaxWebApp | null {
  const app = getMaxWebApp()
  if (!app) return null

  app.ready?.()
  app.expand?.()
  return app
}

/** Сообщает активному Mini App, что приложение готово. */
export function initMessenger(): MessengerKind {
  const kind = getMessengerKind()
  if (kind === 'telegram') {
    initTelegram()
  } else if (kind === 'max') {
    initMax()
  }
  return kind
}
