import { getInitData } from '../platform/messenger'
import { env } from '../config/env'

const API_BASE_URL = env.apiBaseUrl

export interface GuideUser {
  id: number
  username?: string
  firstName?: string
  lastName?: string
  languageCode?: string
}

export interface GuideSession {
  sessionId: string
  realtimeToken: string
  realtimePath: string
  realtimeAvailable: boolean
  inputRate?: number
  outputRate?: number
  expiresAt: string
  user: GuideUser
}

function initDataHeaders(): Record<string, string> {
  const initData = getInitData()
  return initData ? { 'X-Init-Data': initData } : {}
}

/** Создаёт (или переиспользует) серверную сессию Гида и выдаёт токен WebSocket. */
export async function createGuideSession(): Promise<GuideSession> {
  const response = await fetch(`${API_BASE_URL}/api/v1/session`, {
    method: 'POST',
    headers: initDataHeaders(),
    signal: AbortSignal.timeout(env.apiRequestTimeoutMs),
  })

  if (!response.ok) {
    throw new Error(`Не удалось создать сессию Гида: HTTP ${response.status}`)
  }

  return (await response.json()) as GuideSession
}

/** Собирает WebSocket URL для текущей сессии. */
export function buildRealtimeUrl(session: GuideSession): string {
  const base = API_BASE_URL || window.location.origin
  const wsBase = base.replace(/^http/i, 'ws')
  return `${wsBase}${session.realtimePath}?token=${encodeURIComponent(session.realtimeToken)}`
}

/** Просит сервер прервать текущий ответ Гида. */
export async function stopGuide(sessionId: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/v1/session/${encodeURIComponent(sessionId)}/stop`, {
    method: 'POST',
    headers: initDataHeaders(),
    signal: AbortSignal.timeout(env.apiRequestTimeoutMs),
  })

  if (!response.ok && response.status !== 409) {
    throw new Error(`Не удалось остановить Гида: HTTP ${response.status}`)
  }
}
