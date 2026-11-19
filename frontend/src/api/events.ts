import type { Place } from '../places/places'
import { getTelegramInitData } from '../platform/telegram'
import { env } from '../config/env'
import { logger } from '../config/logger'

/**
 * Базовый URL сервиса событий.
 *
 * По умолчанию используется относительный путь: в dev-режиме его проксирует
 * Vite, на VPS — nginx. Абсолютный адрес можно задать через VITE_API_BASE_URL.
 */
const API_BASE_URL = env.apiBaseUrl

const PLACE_OPEN_PATH = '/api/v1/events/place-open'
const SESSION_STORAGE_KEY = 'sophygo.sessionId'

export interface PlaceOpenResult {
  id: string
  receivedAt: string
  sessionId?: string
  delivered: boolean
}

function createSessionId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `sess_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`
}

/**
 * Идентификатор вкладки. Позволяет связывать события одного визита
 * без авторизации пользователя.
 */
export function getSessionId(): string {
  try {
    const stored = window.sessionStorage.getItem(SESSION_STORAGE_KEY)
    if (stored) return stored

    const created = createSessionId()
    window.sessionStorage.setItem(SESSION_STORAGE_KEY, created)
    return created
  } catch {
    return 'anonymous'
  }
}

/**
 * Отправляет на backend событие «пользователь открыл карточку объекта».
 * Если приложение запущено в Telegram, добавляет подписанный initData,
 * чтобы сервер мог связать событие с сессией Гида.
 */
export async function sendPlaceOpen(place: Place, sessionId?: string): Promise<PlaceOpenResult | null> {
  const initData = getTelegramInitData()

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (initData) {
    headers['X-Telegram-Init-Data'] = initData
  }

  const payload = {
    place: {
      id: place.id,
      name: place.name,
      category: place.category,
      latitude: place.latitude,
      longitude: place.longitude,
    },
    client: {
      sessionId: getSessionId(),
      platform: initData ? 'telegram' : 'browser',
      userAgent: navigator.userAgent,
      language: navigator.language,
    },
    sessionId,
    occurredAt: new Date().toISOString(),
  }

  try {
    const response = await fetch(`${API_BASE_URL}${PLACE_OPEN_PATH}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(payload),
      keepalive: true,
      signal: AbortSignal.timeout(env.apiRequestTimeoutMs),
    })

    if (!response.ok) {
      logger.warn('не удалось отправить place_open', { status: response.status })
      return null
    }

    return (await response.json()) as PlaceOpenResult
  } catch (error) {
    logger.warn('ошибка отправки place_open', { error: String(error) })
    return null
  }
}
