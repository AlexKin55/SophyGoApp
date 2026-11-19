/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Абсолютный URL backend-сервиса. Пустое значение — относительные запросы. */
  readonly VITE_API_BASE_URL?: string
  /** Мастер-флаг логирования frontend. По умолчанию false. */
  readonly VITE_LOGGING_ENABLED?: string
  /** Уровень логов frontend: debug, info, warn, error. */
  readonly VITE_LOG_LEVEL?: string
  /** Endpoint приёма логов на backend. */
  readonly VITE_CLIENT_LOG_ENDPOINT?: string
  /** Сколько записей отправлять одним пакетом. */
  readonly VITE_CLIENT_LOG_BATCH_SIZE?: string
  /** Как часто отправлять накопленные записи, мс. */
  readonly VITE_CLIENT_LOG_FLUSH_MS?: string
  /** Таймаут HTTP-запросов к backend, мс. */
  readonly VITE_API_REQUEST_TIMEOUT_MS?: string
  /** "true" — разрешить голосовую сессию без Telegram initData (локальная отладка). */
  readonly VITE_DEV_AUTH?: string
  /** Сколько ждать загрузку Telegram SDK, мс. */
  readonly VITE_TELEGRAM_SDK_TIMEOUT_MS?: string

  /** Базовый путь к файлам Silero VAD (model + worklet). Должен заканчиваться на "/". */
  readonly VITE_VAD_ASSET_BASE_URL?: string
  /** Базовый путь к wasm-файлам ONNX Runtime. Должен заканчиваться на "/". */
  readonly VITE_ORT_ASSET_BASE_URL?: string
  /** Порог уверенности Silero VAD. */
  readonly VITE_VAD_SPEECH_THRESHOLD?: string
  /** Порог окончания речи Silero VAD. */
  readonly VITE_VAD_NEGATIVE_THRESHOLD?: string
  /** Через сколько считать кадры VAD устаревшими, мс. */
  readonly VITE_VAD_FRAME_STALE_MS?: string
  /** Сколько ждать окончания речи, мс. */
  readonly VITE_VAD_REDEMPTION_MS?: string
  /** Сколько аудио до начала речи добавить, мс. */
  readonly VITE_VAD_PRE_SPEECH_PAD_MS?: string
  /** Минимальная длина речи, мс. */
  readonly VITE_VAD_MIN_SPEECH_MS?: string
  /** Количество потоков ONNX Runtime. */
  readonly VITE_VAD_NUM_THREADS?: string
  /** Использовать SIMD в ONNX Runtime. */
  readonly VITE_VAD_SIMD?: string

  /** Частота PCM по умолчанию, Гц. */
  readonly VITE_AUDIO_SAMPLE_RATE?: string
  /** Размер буфера захвата микрофона. */
  readonly VITE_AUDIO_CAPTURE_BUFFER_SIZE?: string
  /** Размер порции при base64-кодировании аудио. */
  readonly VITE_AUDIO_BASE64_CHUNK_SIZE?: string

  /** Порог RMS для резервного VAD. */
  readonly VITE_GUIDE_RMS_THRESHOLD?: string
  /** Сколько кадров речи нужно для перебивания при RMS-fallback. */
  readonly VITE_GUIDE_BARGE_IN_FRAMES?: string
  /** Тишина до выключения микрофона, мс. */
  readonly VITE_GUIDE_IDLE_TIMEOUT_MS?: string
  /** Период проверки тишины, мс. */
  readonly VITE_GUIDE_IDLE_CHECK_INTERVAL_MS?: string
  /** Таймаут установки WebSocket-соединения, мс. */
  readonly VITE_GUIDE_CONNECT_TIMEOUT_MS?: string

  readonly VITE_WIKIPEDIA_API_URL?: string
  readonly VITE_NOMINATIM_URL?: string
  readonly VITE_PLACES_MAX_RESULTS?: string
  readonly VITE_PLACES_GEOSEARCH_LIMIT?: string
  readonly VITE_PLACES_CATEGORY_CANDIDATES?: string
  readonly VITE_PLACES_TITLES_PER_REQUEST?: string
  readonly VITE_PLACES_IMAGES_BATCH?: string
  readonly VITE_PLACES_REQUEST_TIMEOUT_MS?: string
  readonly VITE_PLACES_CACHE_TTL_MS?: string
  readonly VITE_PLACES_CACHE_LIMIT?: string
  readonly VITE_PLACES_MAX_CONCURRENT_REQUESTS?: string
  readonly VITE_PLACES_RETRY_DELAY_MS?: string
  readonly VITE_PLACES_OSM_RESERVED?: string
  readonly VITE_PLACES_OSM_WAIT_MS?: string
  readonly VITE_PLACES_NOMINATIM_TIMEOUT_MS?: string
  readonly VITE_PLACES_NOMINATIM_DELAY_MS?: string
  readonly VITE_PLACES_NOMINATIM_QUERIES?: string
  readonly VITE_PLACES_MAX_LAT_SPAN?: string
  readonly VITE_PLACES_MAX_LON_SPAN?: string
  readonly VITE_PLACES_GRID_SIZE?: string

  readonly VITE_GEO_HIGH_ACCURACY?: string
  readonly VITE_GEO_TIMEOUT_MS?: string
  readonly VITE_GEO_MAXIMUM_AGE_MS?: string
  readonly VITE_GEO_ROUND_FACTOR?: string
  readonly VITE_REVERSE_GEOCODE_DEBOUNCE_MS?: string
  readonly VITE_REVERSE_GEOCODE_TIMEOUT_MS?: string
  readonly VITE_REVERSE_GEOCODE_ZOOM?: string
  readonly VITE_REVERSE_GEOCODE_LANGUAGE?: string

  readonly VITE_MAP_CENTER_LAT?: string
  readonly VITE_MAP_CENTER_LON?: string
  readonly VITE_MAP_ZOOM?: string
  readonly VITE_MAP_MOVE_DEBOUNCE_MS?: string
  readonly VITE_MAP_TILE_URL?: string
}
