import { env } from '../config/env'
import { logger } from '../config/logger'
import type { Place } from './places'

const WIKIPEDIA_API = env.places.wikipediaApiUrl
const PHOTO_WIDTH = 640
/** Показываем все фото, которые отдала Wikipedia, но не больше шести. */
const MAX_PHOTOS = 6
/** Сколько файлов запрашиваем у статьи, чтобы после фильтра осталось достаточно. */
const MAX_FILES = 20
const REQUEST_TIMEOUT_MS = 8_000

/** Служебные картинки статьи: логотипы, иконки, марки, схемы, гербы и т.п. */
const SERVICE_FILE_PATTERN =
  /(commons-logo|wiki|icon|logo|flag|coat[ _]of[ _]arms|symbol|question[ _]book|edit-clear|ambox|portal|disambig|nuvola|locator|map[ _]of|stamp|postage|banknote|coin|марк[аи]|монет|герб|флаг|эмблем|diagram|scheme|карта)/i

/** Минимальный размер: мелкие картинки — это значки и миниатюры, а не фото. */
const MIN_PHOTO_WIDTH = 400
const MIN_PHOTO_HEIGHT = 280

/** Оставляем только растровые фото: SVG обычно оказываются схемами и значками. */
const PHOTO_FILE_PATTERN = /\.(jpe?g|png|webp)(\?|$)/i

interface WikipediaPagesResponse {
  query?: {
    pages?: Record<
      string,
      {
        pageid?: number
        title?: string
        images?: Array<{ title: string }>
        imageinfo?: Array<{
          thumburl?: string
          url?: string
          width?: number
          height?: number
          thumbwidth?: number
          thumbheight?: number
        }>
      }
    >
  }
}

/** Уже загруженные фото: повторный выбор объекта не дёргает Wikipedia. */
const photoCache = new Map<string, string[]>()

function normalizeImageUrl(url: string): string {
  return url.replace('https://thumb.wikimedia.org/', 'https://upload.wikimedia.org/')
}

/** Паузы перед повтором при 429: Wikipedia ограничивает частоту запросов. */
const RETRY_DELAYS_MS = [1_000, 2_500]

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

async function fetchWikipedia<T>(
  params: Record<string, string>,
  signal?: AbortSignal
): Promise<T> {
  for (let attempt = 0; ; attempt += 1) {
    const controller = new AbortController()
    const abortFromCaller = () => controller.abort()
    signal?.addEventListener('abort', abortFromCaller, { once: true })
    const timer = window.setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS)

    try {
      // POST: длинные списки названий файлов не помещаются в URL.
      const response = await fetch(WIKIPEDIA_API, {
        method: 'POST',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: new URLSearchParams({ ...params, format: 'json', origin: '*' }),
        signal: controller.signal,
      })

      if (response.status === 429 && attempt < RETRY_DELAYS_MS.length) {
        await delay(RETRY_DELAYS_MS[attempt])
        continue
      }

      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      return (await response.json()) as T
    } finally {
      window.clearTimeout(timer)
      signal?.removeEventListener('abort', abortFromCaller)
    }
  }
}

/** Ключ для дедупликации: одно и то же фото в разных размерах. */
function photoKey(url: string): string {
  return url.split('/').pop() ?? url
}

/** Главное фото объекта остаётся первым, дополнительные идут следом. */
function mergePhotos(existing: string[], fetched: string[]): string[] {
  const seen = new Set<string>()
  const result: string[] = []

  for (const url of [...existing, ...fetched]) {
    if (!url) continue
    const key = photoKey(url)
    if (seen.has(key)) continue
    seen.add(key)
    result.push(url)
    if (result.length >= MAX_PHOTOS) break
  }

  return result
}

async function loadPhotos(place: Place, signal?: AbortSignal): Promise<string[]> {
  const pageMatch = /^page\/(\d+)$/.exec(place.id)
  const selector: Record<string, string> = pageMatch
    ? { pageids: pageMatch[1] }
    : { titles: place.name }

  const filesResponse = await fetchWikipedia<WikipediaPagesResponse>(
    {
      action: 'query',
      prop: 'images',
      imlimit: String(MAX_FILES),
      redirects: '1',
      ...selector,
    },
    signal
  )

  const page = Object.values(filesResponse.query?.pages ?? {})[0]
  const files = (page?.images ?? [])
    .map((image) => image.title)
    .filter((title) => PHOTO_FILE_PATTERN.test(title) && !SERVICE_FILE_PATTERN.test(title))
    .slice(0, MAX_FILES)

  if (!files.length) return []

  const infoResponse = await fetchWikipedia<WikipediaPagesResponse>(
    {
      action: 'query',
      prop: 'imageinfo',
      iiprop: 'url|size',
      iiurlwidth: String(PHOTO_WIDTH),
      titles: files.join('|'),
    },
    signal
  )

  const infoByTitle = new Map<
    string,
    { url: string; width: number; height: number }
  >()

  for (const info of Object.values(infoResponse.query?.pages ?? {})) {
    const details = info.imageinfo?.[0]
    const source = details?.thumburl ?? details?.url
    if (!info.title || !source) continue

    infoByTitle.set(info.title, {
      url: normalizeImageUrl(source),
      width: details?.thumbwidth ?? details?.width ?? 0,
      height: details?.thumbheight ?? details?.height ?? 0,
    })
  }

  const candidates = files
    .map((file, order) => {
      const info = infoByTitle.get(file)
      if (!info) return null
      if (info.width && info.width < MIN_PHOTO_WIDTH) return null
      if (info.height && info.height < MIN_PHOTO_HEIGHT) return null
      return { ...info, order }
    })
    .filter(
      (
        item
      ): item is { url: string; width: number; height: number; order: number } =>
        item !== null
    )

  // Сначала горизонтальные кадры — они лучше смотрятся в панели.
  candidates.sort((a, b) => {
    const aLandscape = a.width >= a.height * 1.15 ? 0 : 1
    const bLandscape = b.width >= b.height * 1.15 ? 0 : 1
    return aLandscape - bLandscape || a.order - b.order
  })

  return candidates.slice(0, MAX_PHOTOS).map((item) => item.url)
}

/**
 * Догружает фотографии объекта при его выборе (не больше шести).
 * Основное фото из списка сохраняется первым, поэтому панель
 * показывает привычную картинку сразу, а остальные появляются после загрузки.
 */
export async function fetchPlacePhotos(
  place: Place,
  signal?: AbortSignal
): Promise<string[]> {
  const existing = place.images ?? []
  const cached = photoCache.get(place.id)
  if (cached) return mergePhotos(existing, cached)

  const fetched = await loadPhotos(place, signal).catch((err: unknown) => {
    logger.warn('не удалось получить фото из Wikipedia', {
      place: place.name,
      error: String(err),
    })
    return []
  })
  if (!fetched.length) return existing

  photoCache.set(place.id, fetched)
  return mergePhotos(existing, fetched)
}
