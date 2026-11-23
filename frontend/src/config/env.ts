/**
 * Централизованное чтение параметров из Vite-окружения.
 *
 * Все таймауты, лимиты и адреса внешних сервисов задаются в .env
 * (VITE_*). Здесь только разбор значений и значения по умолчанию,
 * чтобы параметры не были захардкожены по файлам.
 */
function readString(key: keyof ImportMetaEnv, fallback: string): string {
  const value = import.meta.env[key]
  return typeof value === 'string' && value !== '' ? value : fallback
}

function readNumber(key: keyof ImportMetaEnv, fallback: number): number {
  const raw = readString(key, '')
  if (raw === '') return fallback
  const value = Number(raw)
  return Number.isFinite(value) && value > 0 ? value : fallback
}

function readBool(key: keyof ImportMetaEnv, fallback: boolean): boolean {
  const raw = readString(key, '')
  if (raw === '') return fallback
  return raw === 'true' || raw === '1'
}

function readList(key: keyof ImportMetaEnv, fallback: string[]): string[] {
  const raw = readString(key, '')
  if (raw === '') return fallback
  const items = raw
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
  return items.length > 0 ? items : fallback
}

const mapCenterLat = readNumber('VITE_MAP_CENTER_LAT', 55.751244)
const mapCenterLon = readNumber('VITE_MAP_CENTER_LON', 37.618423)

const defaultLogLevel = import.meta.env.DEV ? 'debug' : 'info'

export const env = {
  apiBaseUrl: readString('VITE_API_BASE_URL', '').replace(/\/+$/, ''),

  log: {
    enabled: readBool('VITE_LOGGING_ENABLED', false),
    level: readString('VITE_LOG_LEVEL', defaultLogLevel),
    clientEndpoint: readString('VITE_CLIENT_LOG_ENDPOINT', '/api/v1/client-logs'),
    batchSize: readNumber('VITE_CLIENT_LOG_BATCH_SIZE', 20),
    flushMs: readNumber('VITE_CLIENT_LOG_FLUSH_MS', 1_000),
  },
  apiRequestTimeoutMs: readNumber('VITE_API_REQUEST_TIMEOUT_MS', 15_000),
  devAuth: readBool('VITE_DEV_AUTH', false),
  telegramSdkTimeoutMs: readNumber('VITE_TELEGRAM_SDK_TIMEOUT_MS', 4_000),

  vadAssetBaseUrl: readString('VITE_VAD_ASSET_BASE_URL', '/vad/'),
  ortAssetBaseUrl: readString('VITE_ORT_ASSET_BASE_URL', '/ort/'),

  audio: {
    defaultSampleRate: readNumber('VITE_AUDIO_SAMPLE_RATE', 16_000),
    captureBufferSize: readNumber('VITE_AUDIO_CAPTURE_BUFFER_SIZE', 4_096),
    base64ChunkSize: readNumber('VITE_AUDIO_BASE64_CHUNK_SIZE', 8_192),
  },

  vad: {
    speechThreshold: readNumber('VITE_VAD_SPEECH_THRESHOLD', 0.5),
    negativeThreshold: readNumber('VITE_VAD_NEGATIVE_THRESHOLD', 0.35),
    frameStaleMs: readNumber('VITE_VAD_FRAME_STALE_MS', 3_000),
    redemptionMs: readNumber('VITE_VAD_REDEMPTION_MS', 700),
    preSpeechPadMs: readNumber('VITE_VAD_PRE_SPEECH_PAD_MS', 300),
    minSpeechMs: readNumber('VITE_VAD_MIN_SPEECH_MS', 250),
    numThreads: readNumber('VITE_VAD_NUM_THREADS', 1),
    simd: readBool('VITE_VAD_SIMD', true),
  },

  guide: {
    speechRmsThreshold: readNumber('VITE_GUIDE_RMS_THRESHOLD', 0.02),
    bargeInFrames: readNumber('VITE_GUIDE_BARGE_IN_FRAMES', 2),
    idleTimeoutMs: readNumber('VITE_GUIDE_IDLE_TIMEOUT_MS', 45_000),
    idleCheckIntervalMs: readNumber('VITE_GUIDE_IDLE_CHECK_INTERVAL_MS', 5_000),
    connectTimeoutMs: readNumber('VITE_GUIDE_CONNECT_TIMEOUT_MS', 10_000),
  },

  places: {
    wikipediaApiUrl: readString('VITE_WIKIPEDIA_API_URL', 'https://ru.wikipedia.org/w/api.php'),
    nominatimUrl: readString('VITE_NOMINATIM_URL', 'https://nominatim.openstreetmap.org/search'),
    overpassUrls: readList('VITE_OVERPASS_URLS', [
      'https://overpass.openstreetmap.ru/api/interpreter',
      'https://overpass.kumi.systems/api/interpreter',
      'https://overpass-api.de/api/interpreter',
      'https://maps.mail.ru/osm/tools/overpass/api/interpreter',
    ]),
    overpassTimeoutMs: readNumber('VITE_OVERPASS_TIMEOUT_MS', 10_000),
    maxResults: readNumber('VITE_PLACES_MAX_RESULTS', 30),
    geosearchLimit: readNumber('VITE_PLACES_GEOSEARCH_LIMIT', 500),
    categoryCandidates: readNumber('VITE_PLACES_CATEGORY_CANDIDATES', 100),
    titlesPerRequest: readNumber('VITE_PLACES_TITLES_PER_REQUEST', 50),
    imagesBatch: readNumber('VITE_PLACES_IMAGES_BATCH', 20),
    requestTimeoutMs: readNumber('VITE_PLACES_REQUEST_TIMEOUT_MS', 30_000),
    cacheTtlMs: readNumber('VITE_PLACES_CACHE_TTL_MS', 120_000),
    cacheLimit: readNumber('VITE_PLACES_CACHE_LIMIT', 50),
    maxConcurrentRequests: readNumber('VITE_PLACES_MAX_CONCURRENT_REQUESTS', 2),
    retryDelayMs: readNumber('VITE_PLACES_RETRY_DELAY_MS', 1_000),
    nominatimTimeoutMs: readNumber('VITE_PLACES_NOMINATIM_TIMEOUT_MS', 4_000),
    nominatimDelayMs: readNumber('VITE_PLACES_NOMINATIM_DELAY_MS', 1_100),
    nominatimQueries: readList('VITE_PLACES_NOMINATIM_QUERIES', [
      'музей',
      'памятник',
      'достопримечательность',
      'парк',
      'театр',
      'усадьба',
    ]),
    maxLatSpan: readNumber('VITE_PLACES_MAX_LAT_SPAN', 3),
    maxLonSpan: readNumber('VITE_PLACES_MAX_LON_SPAN', 6),
    gridSize: readNumber('VITE_PLACES_GRID_SIZE', 2),
  },

  geolocation: {
    enableHighAccuracy: readBool('VITE_GEO_HIGH_ACCURACY', true),
    timeoutMs: readNumber('VITE_GEO_TIMEOUT_MS', 10_000),
    maximumAgeMs: readNumber('VITE_GEO_MAXIMUM_AGE_MS', 5_000),
    roundFactor: readNumber('VITE_GEO_ROUND_FACTOR', 1_000),
  },

  reverseGeocode: {
    debounceMs: readNumber('VITE_REVERSE_GEOCODE_DEBOUNCE_MS', 400),
    timeoutMs: readNumber('VITE_REVERSE_GEOCODE_TIMEOUT_MS', 4_000),
    zoom: readNumber('VITE_REVERSE_GEOCODE_ZOOM', 10),
    language: readString('VITE_REVERSE_GEOCODE_LANGUAGE', 'ru'),
  },

  map: {
    center: [mapCenterLat, mapCenterLon] as [number, number],
    zoom: readNumber('VITE_MAP_ZOOM', 13),
    moveDebounceMs: readNumber('VITE_MAP_MOVE_DEBOUNCE_MS', 500),
    tileUrl: readString('VITE_MAP_TILE_URL', 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png'),
    // Fallback; актуальное значение приходит с backend (/api/v1/config).
    markerScale: readNumber('VITE_MARKER_SCALE', 1.5),
  },
} as const
