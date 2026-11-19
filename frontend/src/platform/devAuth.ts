/**
 * Режим локальной отладки в обычном браузере, где Telegram WebApp
 * недоступен и initData отсутствует.
 *
 * Включается переменной окружения VITE_DEV_AUTH=true. На backend
 * при этом должен быть задан DEV_AUTH_USER_ID — иначе запросы
 * без initData получат 401.
 */
export function isDevAuthEnabled(): boolean {
  return import.meta.env.VITE_DEV_AUTH === 'true'
}
