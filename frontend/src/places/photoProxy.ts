import { env } from '../config/env'
import { getMessengerKind } from '../platform/messenger'

/**
 * URL фотографии через backend-прокси.
 *
 * Возвращает null вне MAX: в Telegram и браузере прямой доступ к Wikimedia
 * работает, и поведение не должно меняться.
 */
export function photoProxyUrl(src: string): string | null {
  if (getMessengerKind() !== 'max') return null
  return `${env.apiBaseUrl}/api/v1/photos/proxy?url=${encodeURIComponent(src)}`
}
