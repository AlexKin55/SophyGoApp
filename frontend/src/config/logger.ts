/**
 * Логгер frontend.
 *
 * В Telegram WebView браузер не может писать файлы, поэтому записи
 * отправляются на backend отдельными HTTP-пакетами (POST /api/v1/client-logs),
 * а сервер дописывает их в файл из CLIENT_LOG_FILE.
 *
 * Мастер-флаг — VITE_LOGGING_ENABLED. По умолчанию логи выключены:
 * ни консоль, ни сеть не используются.
 */
import { env } from './env'

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

const PRIORITY: Record<LogLevel, number> = {
  debug: 10,
  info: 20,
  warn: 30,
  error: 40,
}

interface LogEntry {
  timestamp: string
  level: LogLevel
  message: string
  context?: unknown
  url: string
}

function normalizeLevel(value: string): LogLevel {
  if (value === 'debug' || value === 'warn' || value === 'error') return value
  return 'info'
}

const minPriority = PRIORITY[normalizeLevel(env.log.level)]
const enabled = env.log.enabled
const endpoint = env.log.clientEndpoint

let queue: LogEntry[] = []
let flushTimer: number | null = null

function flush(): void {
  if (flushTimer !== null) {
    window.clearTimeout(flushTimer)
    flushTimer = null
  }
  if (!enabled || !endpoint || queue.length === 0) return

  const entries = queue
  queue = []

  try {
    void fetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ entries }),
      keepalive: true,
    }).catch(() => {
      // Ошибку отправки логов не логируем, чтобы не зациклиться.
    })
  } catch {
    // fetch может быть недоступен при выгрузке страницы.
  }
}

function write(level: LogLevel, message: string, context?: unknown): void {
  if (!enabled || PRIORITY[level] < minPriority) return

  const entry: LogEntry = {
    timestamp: new Date().toISOString(),
    level,
    message,
    context,
    url: typeof window !== 'undefined' ? window.location.pathname : '',
  }

  if (level === 'error') {
    console.error(`[${level}] ${message}`, context ?? '')
  } else if (level === 'warn') {
    console.warn(`[${level}] ${message}`, context ?? '')
  } else {
    console.log(`[${level}] ${message}`, context ?? '')
  }

  if (!endpoint) return

  queue.push(entry)
  if (queue.length >= env.log.batchSize) {
    flush()
  } else if (flushTimer === null) {
    flushTimer = window.setTimeout(flush, env.log.flushMs)
  }
}

if (typeof window !== 'undefined') {
  window.addEventListener('pagehide', flush)
}

export const logger = {
  debug: (message: string, context?: unknown): void => write('debug', message, context),
  info: (message: string, context?: unknown): void => write('info', message, context),
  warn: (message: string, context?: unknown): void => write('warn', message, context),
  error: (message: string, context?: unknown): void => write('error', message, context),
  flush,
}
