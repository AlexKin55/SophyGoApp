import { env } from '../config/env'
import { logger } from '../config/logger'
import type { Place } from './places'

export interface MapBounds {
  south: number
  west: number
  north: number
  east: number
}

interface GeoPage {
  pageid: number
  title: string
  length: number
  lat: number
  lon: number
  type?: string
}

interface PageImage {
  image?: string
}

interface NominatimPlace {
  osm_type?: string
  osm_id?: number
  lat: string
  lon: string
  category?: string
  type?: string
  name?: string
}

interface OverpassElement {
  type: string
  id: number
  lat?: number
  lon?: number
  center?: { lat: number; lon: number }
  tags?: Record<string, string>
}

interface CategoryRule {
  category: string
  keywords: string[]
}

const WIKIPEDIA_API = env.places.wikipediaApiUrl

const MAX_RESULTS = env.places.maxResults
const GEOSEARCH_LIMIT = env.places.geosearchLimit
const CATEGORY_CANDIDATES = env.places.categoryCandidates
const TITLES_PER_REQUEST = env.places.titlesPerRequest
const IMAGES_BATCH = env.places.imagesBatch
const REQUEST_TIMEOUT_MS = env.places.requestTimeoutMs
const CACHE_TTL_MS = env.places.cacheTtlMs
const CACHE_LIMIT = env.places.cacheLimit
const MAX_CONCURRENT_REQUESTS = env.places.maxConcurrentRequests
const RETRY_DELAY_MS = env.places.retryDelayMs

const NOMINATIM_URL = env.places.nominatimUrl
const NOMINATIM_TIMEOUT_MS = env.places.nominatimTimeoutMs
const NOMINATIM_DELAY_MS = env.places.nominatimDelayMs
const NOMINATIM_QUERIES = env.places.nominatimQueries

/** Резервный источник объектов: OSM Overpass (не зависит от Wikimedia). */
const OVERPASS_URLS = env.places.overpassUrls
const OVERPASS_TIMEOUT_MS = env.places.overpassTimeoutMs

/** Сколько ждём Wikimedia, прежде чем переключиться на OSM. */
const WIKI_FALLBACK_WAIT_MS = 6_000

let lastNominatimRequestAt = 0

/** Endpoint, который уже ответил успешно: в следующий раз пробуем его первым. */
let preferredOverpassUrl: string | null = null

const STORAGE_KEY = 'sophygo:places:v1'

/** Последний удачный список объектов — на случай недоступности всех API. */
function readStoredPlaces(bounds: MapBounds): Place[] {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as { places?: Place[] }
    const places = Array.isArray(parsed.places) ? parsed.places : []
    return places.filter(
      (place) =>
        place.latitude >= bounds.south &&
        place.latitude <= bounds.north &&
        place.longitude >= bounds.west &&
        place.longitude <= bounds.east
    )
  } catch {
    return []
  }
}

function writeStoredPlaces(places: Place[]): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ at: Date.now(), places }))
  } catch {
    // Хранилище недоступно (приватный режим) — работаем без него.
  }
}

interface CacheEntry {
  at: number
  places: Place[]
}

const cache = new Map<string, CacheEntry>()

function cacheKey(bounds: MapBounds): string {
  return [bounds.south, bounds.west, bounds.north, bounds.east]
    .map((value) => value.toFixed(3))
    .join(',')
}

function readCache(bounds: MapBounds): CacheEntry | undefined {
  const entry = cache.get(cacheKey(bounds))
  if (!entry) return undefined
  return Date.now() - entry.at <= CACHE_TTL_MS ? entry : undefined
}

function peekCache(bounds: MapBounds): CacheEntry | undefined {
  return cache.get(cacheKey(bounds))
}

function writeCache(bounds: MapBounds, places: Place[]): void {
  const key = cacheKey(bounds)

  if (!cache.has(key) && cache.size >= CACHE_LIMIT) {
    const oldest = [...cache.entries()].sort((a, b) => a[1].at - b[1].at)[0]
    if (oldest) cache.delete(oldest[0])
  }

  cache.set(key, { at: Date.now(), places })
}

/**
 * Ограничение области запроса: на очень мелком масштабе bbox может покрывать
 * полмира. В этом случае берём центральную часть видимой области.
 */
const MAX_LAT_SPAN = env.places.maxLatSpan
const MAX_LON_SPAN = env.places.maxLonSpan

/** На сколько ячеек делим bbox: GeoSearch отдаёт не больше 500 ближайших. */
const GRID_SIZE = env.places.gridSize

const EXCLUDED_GEO_TYPES = new Set([
  'city',
  'adm1st',
  'adm2nd',
  'adm3rd',
  'country',
  'continent',
  'waterbody',
  'event',
  'airport',
  'mountain',
  'river',
])

const EXCLUDED_CATEGORY_KEYWORDS = [
  'станции метро',
  'метрополитен',
  'железнодорожные станции',
  'вокзалы',
  'аэропорты',
  'кладбища',
  'парковки',
  'парковочные',
  'административные',
  'организации',
  'министерства',
  'компании',
  'предприятия',
  'вузы',
  'университеты',
  'школы',
  'больницы',
  'политические партии',
]

/** Порядок важен: более специфичные категории проверяются раньше. */
const CATEGORY_RULES: CategoryRule[] = [
  {
    category: 'military',
    keywords: [
      'военные музеи',
      'вооружённые силы',
      'вооруженные силы',
      'военная история',
      'военная техника',
      'военные',
      'войны',
      'война',
      'сражения',
      'битвы',
      'боевые действия',
      'оборонительные сооружения',
      'обороны',
      'оборона',
      'армия',
      'армии',
      'флот',
      'авиация',
      'казармы',
      'штабы',
      'бункеры',
      'воинские захоронения',
      'братские могилы',
      'вечный огонь',
    ],
  },
  { category: 'planetarium', keywords: ['планетарии', 'планетарий'] },
  {
    category: 'arts_centre',
    keywords: ['культурные центры', 'дома культуры', 'творческие центры'],
  },
  { category: 'zoo', keywords: ['зоопарки', 'зоопарк', 'зоосады', 'зоологические парки'] },
  {
    category: 'aquarium',
    keywords: ['аквариумы', 'аквариум', 'океанариумы', 'океанариум', 'дельфинарии'],
  },
  {
    category: 'theme_park',
    keywords: ['парки развлечений', 'парки аттракционов', 'аттракционы', 'аквапарки'],
  },
  {
    category: 'nature_reserve',
    keywords: ['заповедники', 'национальные парки', 'природные заповедники', 'оопт', 'памятники природы'],
  },
  { category: 'museum', keywords: ['музеи', 'музей'] },
  {
    category: 'gallery',
    keywords: ['галереи', 'галерея', 'выставочные залы', 'выставочные центры', 'экспоцентры'],
  },
  {
    category: 'theatre',
    keywords: ['театры', 'театр', 'оперные театры', 'концертные залы', 'филармонии', 'цирки'],
  },
  { category: 'library', keywords: ['библиотеки', 'библиотека'] },
  { category: 'alley', keywords: ['аллеи', 'переулки', 'бульвары'] },
  {
    category: 'park',
    keywords: [
      'парки',
      'парк',
      'лесопарки',
      'скверы',
      'природные парки',
      'парковые зоны',
      'зоны отдыха',
      'лугопарки',
      'парки культуры',
      'парки отдыха',
    ],
  },
  {
    category: 'garden',
    keywords: ['сады', 'ботанические сады', 'дендрарии', 'дендрарий', 'оранжереи'],
  },
  { category: 'monastery', keywords: ['монастыри', 'монастырь'] },
  { category: 'cathedral', keywords: ['соборы', 'собор'] },
  { category: 'mosque', keywords: ['мечети', 'мечеть'] },
  { category: 'synagogue', keywords: ['синагоги', 'синагога'] },
  {
    category: 'church',
    keywords: ['храмы', 'храм', 'церкви', 'церковь', 'часовни', 'костёлы', 'костелы'],
  },
  { category: 'memorial', keywords: ['мемориалы', 'мемориал'] },
  { category: 'artwork', keywords: ['арт-объекты', 'арт-объект', 'муралы', 'мурал'] },
  { category: 'ruins', keywords: ['руины'] },
  {
    category: 'archaeological_site',
    keywords: ['археологические памятники', 'археологический памятник'],
  },
  { category: 'viewpoint', keywords: ['смотровые площадки', 'смотровая площадка'] },
  { category: 'castle', keywords: ['замки', 'замок'] },
  { category: 'monument', keywords: ['памятники', 'монументы', 'скульптуры'] },
  {
    category: 'fort',
    keywords: ['крепости', 'кремли', 'башни'],
  },
  { category: 'city_gate', keywords: ['городские ворота'] },
  { category: 'palace', keywords: ['дворцы', 'усадьбы', 'резиденции'] },
  {
    category: 'historic_building',
    keywords: [
      'памятники архитектуры',
      'архитектурные памятники',
      'объекты культурного наследия',
      'памятники истории и культуры',
      'исторические здания',
      'исторические дома',
      'исторические особняки',
      'особняки',
      'доходные дома',
      'здания и сооружения',
    ],
  },
  {
    category: 'attraction',
    keywords: [
      'достопримечательности',
      'площади',
      'мосты',
      'набережные',
      'всемирное наследие',
      'пешеходные улицы',
    ],
  },
]

const CATEGORY_WEIGHTS: Record<string, number> = {
  attraction: 30,
  military: 30,
  historic_building: 28,
  museum: 32,
  gallery: 30,
  arts_centre: 28,
  planetarium: 26,
  alley: 24,
  park: 30,
  garden: 24,
  nature_reserve: 26,
  theatre: 30,
  library: 22,
  monument: 26,
  memorial: 24,
  castle: 32,
  fort: 28,
  palace: 32,
  church: 26,
  cathedral: 30,
  monastery: 30,
  mosque: 26,
  synagogue: 26,
  zoo: 28,
  aquarium: 26,
  theme_park: 28,
  artwork: 20,
  viewpoint: 24,
  ruins: 20,
  archaeological_site: 24,
  city_gate: 22,
}

function clampBounds(bounds: MapBounds): MapBounds {
  const centerLat = (bounds.south + bounds.north) / 2
  const centerLon = (bounds.west + bounds.east) / 2
  const latSpan = Math.min(bounds.north - bounds.south, MAX_LAT_SPAN)
  const lonSpan = Math.min(bounds.east - bounds.west, MAX_LON_SPAN)

  return {
    south: centerLat - latSpan / 2,
    north: centerLat + latSpan / 2,
    west: centerLon - lonSpan / 2,
    east: centerLon + lonSpan / 2,
  }
}

function splitBounds(bounds: MapBounds): MapBounds[] {
  const midLat = (bounds.south + bounds.north) / 2
  const midLon = (bounds.west + bounds.east) / 2
  const cells: MapBounds[] = []

  for (let row = 0; row < GRID_SIZE; row += 1) {
    for (let col = 0; col < GRID_SIZE; col += 1) {
      cells.push({
        south: row === 0 ? bounds.south : midLat,
        north: row === 0 ? midLat : bounds.north,
        west: col === 0 ? bounds.west : midLon,
        east: col === 0 ? midLon : bounds.east,
      })
    }
  }

  return cells
}

let activeRequests = 0
const pendingRequests: Array<() => void> = []

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

/** Ограничивает ожидание источника: по таймауту переключаемся на резерв. */
function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
  return new Promise((resolve, reject) => {
    const timer = window.setTimeout(() => reject(new Error('timeout')), ms)
    promise.then(
      (value) => {
        window.clearTimeout(timer)
        resolve(value)
      },
      (err) => {
        window.clearTimeout(timer)
        reject(err)
      }
    )
  })
}

/** Небольшой семафор: Wikipedia отвечает 429 на пачку параллельных запросов. */
async function withLimit<T>(task: () => Promise<T>): Promise<T> {
  if (activeRequests >= MAX_CONCURRENT_REQUESTS) {
    await new Promise<void>((resolve) => pendingRequests.push(resolve))
  }

  activeRequests += 1
  try {
    return await task()
  } finally {
    activeRequests -= 1
    pendingRequests.shift()?.()
  }
}

async function fetchJson<T>(
  url: string,
  accept: string,
  signal?: AbortSignal,
  body?: URLSearchParams,
  timeoutMs = REQUEST_TIMEOUT_MS
): Promise<T> {
  const controller = new AbortController()
  const abortFromCaller = () => controller.abort()
  signal?.addEventListener('abort', abortFromCaller, { once: true })
  const timer = window.setTimeout(() => controller.abort(), timeoutMs)

  const request = () =>
    fetch(url, {
      method: body ? 'POST' : 'GET',
      headers: {
        Accept: accept,
        ...(body ? { 'Content-Type': 'application/x-www-form-urlencoded' } : {}),
      },
      body,
      signal: controller.signal,
    })

  try {
    let response = await withLimit(request)

    if (response.status === 429 && !signal?.aborted) {
      await delay(RETRY_DELAY_MS)
      response = await withLimit(request)
    }

    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    return (await response.json()) as T
  } finally {
    window.clearTimeout(timer)
    signal?.removeEventListener('abort', abortFromCaller)
  }
}

/**
 * POST вместо GET: длинные списки названий статей не помещаются в URL,
 * а сервер в таком случае отвечает без CORS-заголовков.
 */
function fetchWikipedia<T>(
  params: Record<string, string>,
  signal?: AbortSignal
): Promise<T> {
  return fetchJson<T>(
    WIKIPEDIA_API,
    'application/json',
    signal,
    new URLSearchParams({ ...params, format: 'json', origin: '*' })
  )
}

function buildGeosearchUrl(bounds: MapBounds): string {
  const url = new URL(WIKIPEDIA_API)
  url.searchParams.set('action', 'query')
  url.searchParams.set('generator', 'geosearch')
  url.searchParams.set('ggsbbox', `${bounds.north}|${bounds.west}|${bounds.south}|${bounds.east}`)
  url.searchParams.set('ggslimit', String(GEOSEARCH_LIMIT))
  url.searchParams.set('ggsprop', 'type|name')
  url.searchParams.set('prop', 'info|coordinates')
  url.searchParams.set('coprop', 'type|name')
  url.searchParams.set('colimit', 'max')
  url.searchParams.set('format', 'json')
  url.searchParams.set('origin', '*')
  return url.toString()
}

async function fetchGeoPages(
  bounds: MapBounds,
  signal?: AbortSignal
): Promise<GeoPage[]> {
  const data = await fetchJson<{
    query?: {
      pages?: Record<
        string,
        {
          pageid: number
          title: string
          length?: number
          coordinates?: Array<{ lat: number; lon: number; type?: string }>
        }
      >
    }
  }>(buildGeosearchUrl(bounds), 'application/json', signal)

  const pages: GeoPage[] = []

  for (const page of Object.values(data.query?.pages ?? {})) {
    const coord = page.coordinates?.[0]
    if (!coord) continue

    pages.push({
      pageid: page.pageid,
      title: page.title,
      length: page.length ?? 0,
      lat: coord.lat,
      lon: coord.lon,
      type: coord.type,
    })
  }

  return pages
}

function chunk<T>(items: T[], size: number): T[][] {
  const result: T[][] = []
  for (let i = 0; i < items.length; i += size) {
    result.push(items.slice(i, i + size))
  }
  return result
}

async function fetchCategories(
  pages: GeoPage[],
  signal?: AbortSignal
): Promise<Map<number, string[]>> {
  const result = new Map<number, string[]>()

  await Promise.all(
    chunk(pages, TITLES_PER_REQUEST).map(async (batch) => {
      const data = await fetchWikipedia<{
        query?: {
          pages?: Record<
            string,
            { pageid: number; categories?: Array<{ title: string }> }
          >
        }
      }>(
        {
          action: 'query',
          prop: 'categories',
          titles: batch.map((page) => page.title).join('|'),
          cllimit: 'max',
          clshow: '!hidden',
          redirects: '1',
        },
        signal
      )

      for (const page of Object.values(data.query?.pages ?? {})) {
        result.set(
          page.pageid,
          (page.categories ?? []).map((category) =>
            category.title.replace(/^Категория:/, '')
          )
        )
      }
    })
  )

  return result
}

function classify(categories: string[]): string | null {
  if (categories.length === 0) return null
  const joined = categories.join(' | ').toLowerCase()

  if (EXCLUDED_CATEGORY_KEYWORDS.some((keyword) => joined.includes(keyword))) {
    return null
  }

  for (const rule of CATEGORY_RULES) {
    if (rule.keywords.some((keyword) => joined.includes(keyword))) {
      return rule.category
    }
  }

  return null
}

function scorePage(page: GeoPage, category: string): number {
  const weight = CATEGORY_WEIGHTS[category] ?? 12
  const size = Math.log10(Math.max(page.length, 1)) * 10
  const landmarkBonus = page.type === 'landmark' ? 15 : 0
  return weight + size + landmarkBonus
}

async function fetchImages(
  pages: GeoPage[],
  signal?: AbortSignal
): Promise<Map<number, PageImage>> {
  const result = new Map<number, PageImage>()

  await Promise.all(
    chunk(pages, IMAGES_BATCH).map(async (batch) => {
      const data = await fetchWikipedia<{
        query?: {
          pages?: Record<
            string,
            { pageid: number; thumbnail?: { source?: string } }
          >
        }
      }>(
        {
          action: 'query',
          prop: 'pageimages',
          piprop: 'thumbnail',
          pithumbsize: '640',
          titles: batch.map((page) => page.title).join('|'),
          redirects: '1',
        },
        signal
      )

      for (const page of Object.values(data.query?.pages ?? {})) {
        result.set(page.pageid, { image: page.thumbnail?.source })
      }
    })
  )

  return result
}

function normalizeName(name: string): string {
  return name.toLowerCase().replace(/\s+/g, ' ').trim()
}

function nominatimCategory(item: NominatimPlace): string | null {
  if (item.category === 'tourism') {
    switch (item.type) {
      case 'museum':
        return 'museum'
      case 'gallery':
        return 'gallery'
      case 'artwork':
        return 'artwork'
      case 'viewpoint':
        return 'viewpoint'
      case 'zoo':
        return 'zoo'
      case 'theme_park':
        return 'theme_park'
      case 'attraction':
        return 'attraction'
      default:
        return null
    }
  }

  if (item.category === 'historic') {
    switch (item.type) {
      case 'memorial':
        return 'memorial'
      case 'monument':
        return 'monument'
      case 'castle':
        return 'castle'
      case 'fort':
        return 'fort'
      case 'palace':
        return 'palace'
      case 'ruins':
        return 'ruins'
      case 'archaeological_site':
        return 'archaeological_site'
      default:
        return 'historic_building'
    }
  }

  if (item.category === 'amenity') {
    if (item.type === 'theatre') return 'theatre'
    if (item.type === 'arts_centre') return 'arts_centre'
  }

  if (item.category === 'leisure') {
    if (item.type === 'garden') return 'garden'
    if (item.type === 'nature_reserve') return 'nature_reserve'
    if (item.type === 'park') return 'park'
  }

  if (item.category === 'landuse' && item.type === 'forest') return 'park'
  if (item.category === 'place' && item.type === 'square') return 'attraction'
  return null
}

function overpassCategory(tags: Record<string, string>): string | null {
  switch (tags.tourism) {
    case 'museum':
      return 'museum'
    case 'gallery':
      return 'gallery'
    case 'artwork':
      return 'artwork'
    case 'viewpoint':
      return 'viewpoint'
    case 'zoo':
      return 'zoo'
    case 'theme_park':
      return 'theme_park'
    case 'attraction':
      return 'attraction'
    default:
      break
  }

  switch (tags.historic) {
    case 'memorial':
      return 'memorial'
    case 'monument':
      return 'monument'
    case 'castle':
      return 'castle'
    case 'fort':
      return 'fort'
    case 'palace':
      return 'palace'
    case 'ruins':
      return 'ruins'
    case 'archaeological_site':
      return 'archaeological_site'
    default:
      break
  }

  if (tags.leisure === 'park') return 'park'
  if (tags.leisure === 'garden') return 'garden'
  if (tags.amenity === 'theatre') return 'theatre'
  if (tags.amenity === 'arts_centre') return 'arts_centre'
  return null
}

function overpassName(tags: Record<string, string>): string {
  return (tags['name:ru'] || tags.name || tags['name:en'] || '').trim()
}

function buildOverpassQuery(bounds: MapBounds): string {
  const bbox = [bounds.south, bounds.west, bounds.north, bounds.east].join(',')
  return `[out:json][timeout:20];(
nwr["tourism"~"^(attraction|museum|gallery|artwork|viewpoint|zoo|theme_park)$"](${bbox});
nwr["historic"~"^(memorial|monument|castle|fort|palace|ruins|archaeological_site)$"](${bbox});
nwr["leisure"~"^(park|garden)$"](${bbox});
nwr["amenity"~"^(theatre|arts_centre)$"](${bbox});
);out center ${MAX_RESULTS * 4};`
}

function overpassPlaces(elements: OverpassElement[]): Place[] {
  const byName = new Map<string, { place: Place; score: number }>()

  for (const element of elements) {
    const tags = element.tags ?? {}
    const name = overpassName(tags)
    const category = overpassCategory(tags)
    const latitude = element.lat ?? element.center?.lat
    const longitude = element.lon ?? element.center?.lon

    if (
      !name ||
      !category ||
      !Number.isFinite(latitude) ||
      !Number.isFinite(longitude)
    ) {
      continue
    }

    const image = tags.image
    const score =
      (CATEGORY_WEIGHTS[category] ?? 12) +
      (tags.wikipedia ? 8 : 0) +
      (image ? 2 : 0)
    const key = normalizeName(name)
    const existing = byName.get(key)

    if (!existing || score > existing.score) {
      byName.set(key, {
        place: {
          id: `osm/${element.type}/${element.id}`,
          name,
          latitude: latitude as number,
          longitude: longitude as number,
          category,
          images: image && /^https?:\/\//.test(image) ? [image] : [],
        },
        score,
      })
    }
  }

  return [...byName.values()]
    .sort((a, b) => b.score - a.score)
    .slice(0, MAX_RESULTS)
    .map((entry) => entry.place)
}

/**
 * Загружает достопримечательности из OSM Overpass. Это основной источник:
 * Wikimedia API периодически недоступен из России, а карта и Overpass работают.
 * Endpoint'ы перебираются по порядку, пока один не ответит.
 */
async function fetchOsmAttractions(
  bounds: MapBounds,
  signal?: AbortSignal
): Promise<Place[]> {
  const query = buildOverpassQuery(bounds)
  const urls = preferredOverpassUrl
    ? [preferredOverpassUrl, ...OVERPASS_URLS.filter((url) => url !== preferredOverpassUrl)]
    : OVERPASS_URLS
  let lastError: unknown = null

  for (const url of urls) {
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')

    try {
      const data = await fetchJson<{ elements?: OverpassElement[] }>(
        url,
        'application/json',
        signal,
        new URLSearchParams({ data: query }),
        OVERPASS_TIMEOUT_MS
      )
      const places = overpassPlaces(data.elements ?? [])
      if (places.length) {
        preferredOverpassUrl = url
        return places
      }
    } catch (err) {
      if (signal?.aborted) throw err
      lastError = err
      logger.warn('Overpass endpoint unavailable', { url, error: String(err) })
    }
  }

  if (lastError) {
    logger.warn('Overpass attractions unavailable', { error: String(lastError) })
  }
  return []
}

/**
 * Дополняет Wikipedia объектами из OSM через Nominatim: маленькие парки,
 * скверы и лесопарки часто не имеют статей, поэтому GeoSearch их не находит.
 * Nominatim разрешает не больше одного запроса в секунду — соблюдаем паузу.
 */
async function fetchOsmParks(
  bounds: MapBounds,
  signal?: AbortSignal
): Promise<Place[]> {
  const byName = new Map<string, { place: Place; score: number }>()
  let lastError: unknown = null

  for (const query of NOMINATIM_QUERIES) {
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')

    const wait = NOMINATIM_DELAY_MS - (Date.now() - lastNominatimRequestAt)
    if (wait > 0) await delay(wait)
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
    lastNominatimRequestAt = Date.now()

    const url = new URL(NOMINATIM_URL)
    url.searchParams.set('format', 'jsonv2')
    url.searchParams.set('q', query)
    url.searchParams.set(
      'viewbox',
      `${bounds.west},${bounds.north},${bounds.east},${bounds.south}`
    )
    url.searchParams.set('bounded', '1')
    url.searchParams.set('limit', '50')
    url.searchParams.set('accept-language', 'ru')

    try {
      const data = await fetchJson<NominatimPlace[]>(
        url.toString(),
        'application/json',
        signal,
        undefined,
        NOMINATIM_TIMEOUT_MS
      )

      for (const item of data) {
        const name = item.name?.trim()
        const category = nominatimCategory(item)
        const latitude = Number(item.lat)
        const longitude = Number(item.lon)

        if (
          !name ||
          !category ||
          !Number.isFinite(latitude) ||
          !Number.isFinite(longitude)
        ) {
          continue
        }

        const score =
          (CATEGORY_WEIGHTS[category] ?? 12) +
          (name.toLowerCase().includes('парк') ? 2 : 0)
        const key = normalizeName(name)
        const existing = byName.get(key)

        if (!existing || score > existing.score) {
          byName.set(key, {
            place: {
              id: `osm/${item.osm_type ?? 'x'}/${item.osm_id ?? key}`,
              name,
              latitude,
              longitude,
              category,
              images: [],
            },
            score,
          })
        }
      }
    } catch (err) {
      if (signal?.aborted) throw err
      lastError = err
    }
  }

  if (byName.size === 0 && lastError) {
    logger.warn('Nominatim parks unavailable', { error: String(lastError) })
  }

  return [...byName.values()]
    .sort((a, b) => b.score - a.score)
    .slice(0, MAX_RESULTS)
    .map((entry) => entry.place)
}

/**
 * Возвращает MAX_RESULTS самых значимых объектов в границах карты.
 *
 * Основной источник — Wikipedia GeoSearch. Параллельно запускаем Nominatim:
 * если Wikimedia не ответила за WIKI_FALLBACK_WAIT_MS, берём объекты из OSM,
 * чтобы карта не оставалась пустой при недоступности Википедии.
 */
export async function fetchTopPlaces(
  bounds: MapBounds,
  signal?: AbortSignal
): Promise<Place[]> {
  const clamped = clampBounds(bounds)
  const cached = readCache(clamped)
  if (cached) return cached.places

  const osmController = new AbortController()
  const wikiController = new AbortController()
  const abortOsm = () => osmController.abort()
  const abortWiki = () => wikiController.abort()
  signal?.addEventListener('abort', abortOsm, { once: true })
  signal?.addEventListener('abort', abortWiki, { once: true })

  const nominatimPromise = fetchOsmParks(clamped, osmController.signal).catch(() => [])

  try {
    const places = await withTimeout(
      loadTopPlaces(clamped, wikiController.signal),
      WIKI_FALLBACK_WAIT_MS
    )

    if (places.length) {
      osmController.abort()
      writeCache(clamped, places)
      writeStoredPlaces(places)
      return places
    }
  } catch (err) {
    wikiController.abort()
    if (signal?.aborted) throw err
  } finally {
    signal?.removeEventListener('abort', abortOsm)
    signal?.removeEventListener('abort', abortWiki)
  }

  if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')

  let osmPlaces = await nominatimPromise
  if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')

  // Nominatim не дал результатов — пробуем Overpass (если он доступен).
  if (!osmPlaces.length) {
    osmPlaces = await fetchOsmAttractions(clamped, osmController.signal).catch(() => [])
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
  }

  if (osmPlaces.length) {
    writeCache(clamped, osmPlaces)
    writeStoredPlaces(osmPlaces)
    return osmPlaces
  }

  const stale = peekCache(clamped)
  if (stale) return stale.places

  const stored = readStoredPlaces(clamped)
  if (stored.length) return stored

  throw new Error('Не удалось загрузить объекты')
}

async function loadTopPlaces(
  bounds: MapBounds,
  signal?: AbortSignal
): Promise<Place[]> {
  const cells = splitBounds(bounds)
  const results: Array<{ pages?: GeoPage[]; error?: unknown }> = []

  // Запросы идём последовательно: Wikipedia отвечает 429 на пачку запросов.
  for (const [index, cell] of cells.entries()) {
    if (signal?.aborted) break
    try {
      results.push({ pages: await fetchGeoPages(cell, signal) })
    } catch (err) {
      results.push({ error: err })
    }
    if (index < cells.length - 1) await delay(150)
  }

  if (signal?.aborted) {
    throw new DOMException('Aborted', 'AbortError')
  }

  const byId = new Map<number, GeoPage>()
  let firstError: unknown = null

  for (const result of results) {
    if (!result.pages) {
      firstError ??= result.error
      continue
    }

    for (const page of result.pages) {
      if (EXCLUDED_GEO_TYPES.has(page.type ?? '')) continue
      const existing = byId.get(page.pageid)
      if (!existing || page.length > existing.length) {
        byId.set(page.pageid, page)
      }
    }
  }

  const pages = [...byId.values()].sort((a, b) => b.length - a.length)
  if (pages.length === 0) {
    throw firstError ?? new Error('Wikipedia GeoSearch returned no places')
  }

  const candidates = pages.slice(0, CATEGORY_CANDIDATES)
  const categories = await fetchCategories(candidates, signal).catch(
    () => new Map<number, string[]>()
  )

  const classified: Array<{ page: GeoPage; category: string }> = []

  for (const page of candidates) {
    const category = classify(categories.get(page.pageid) ?? [])
    if (category) classified.push({ page, category })
  }

  const ranked = classified.map((entry) => ({
    ...entry,
    score: scorePage(entry.page, entry.category),
  }))

  ranked.sort((a, b) => b.score - a.score)

  let selected = ranked.slice(0, MAX_RESULTS)

  // Показываем только реально найденные объекты: если в области один музей —
  // будет один маркер. Резерв нужен лишь когда категории не загрузились.
  if (selected.length === 0) {
    selected = pages
      .filter((page) => page.type === 'landmark')
      .slice(0, MAX_RESULTS)
      .map((page) => ({
        page,
        category: 'attraction',
        score: scorePage(page, 'attraction'),
      }))
  }

  const images = await fetchImages(
    selected.map((entry) => entry.page),
    signal
  ).catch(() => new Map<number, PageImage>())

  const wikiPlaces = selected.map(({ page, category }) => {
    const info = images.get(page.pageid)
    return {
      id: `page/${page.pageid}`,
      name: page.title,
      latitude: page.lat,
      longitude: page.lon,
      category,
      images: info?.image ? [info.image] : [],
    }
  })

  return wikiPlaces
}
