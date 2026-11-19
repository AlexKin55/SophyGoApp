// Package config загружает настройки сервиса из переменных окружения.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sophygo/backend/internal/envfile"
)

// Config — все настройки сервиса.
type Config struct {
	// Addr — адрес HTTP-сервера, например ":8080".
	Addr string
	// EventsFile — путь к JSONL-файлу, куда дописываются события.
	EventsFile string
	// CORSOrigins — список разрешённых Origin для браузерных запросов.
	CORSOrigins []string
	// MaxRecentEvents — сколько последних событий хранить в памяти.
	MaxRecentEvents int
	// ShutdownTimeout — сколько ждать завершения активных запросов.
	ShutdownTimeout time.Duration

	// LoggingEnabled — мастер-флаг логирования. По умолчанию false.
	LoggingEnabled bool
	// LogFile — путь к файлу логов backend. Пустая строка — только stdout.
	LogFile string
	// LogLevel — debug, info, warn или error.
	LogLevel string
	// LogFormat — json или text.
	LogFormat string
	// ClientLogFile — файл для логов frontend, присланных браузером.
	ClientLogFile string
	// ClientLogMaxBodyBytes — максимальный размер пакета клиентских логов.
	ClientLogMaxBodyBytes int64
	// ClientLogRatePerMinute — лимит пакетов логов с одного IP в минуту.
	ClientLogRatePerMinute int

	// HTTP-сервер: лимиты и таймауты.
	HTTPMaxBodyBytes      int64
	HTTPRequestTimeout    time.Duration
	HTTPReadHeaderTimeout time.Duration
	HTTPReadTimeout       time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration

	// API событий.
	EventsDefaultLimit int
	EventsMaxLimit     int

	// WebSocket-прокси браузер ↔ Yandex Realtime.
	WSMaxMessageBytes int64
	WSPingInterval    time.Duration
	WSPingTimeout     time.Duration
	WSWriteTimeout    time.Duration

	// Клиент Yandex Realtime.
	RealtimeMaxReadBytes int64
	RealtimeDialTimeout  time.Duration
	RealtimeWriteTimeout time.Duration

	// GuideCleanupInterval — как часто удалять истёкшие сессии.
	GuideCleanupInterval time.Duration

	// Валидация событий.
	EventMaxPlaceIDLen   int
	EventMaxNameLen      int
	EventMaxCategoryLen  int
	EventMaxSessionIDLen int
	EventMaxUserAgentLen int
	EventMaxLanguageLen  int
	EventMaxPlatformLen  int
	EventMaxRemoteIPLen  int
	EventMaxClockSkew    time.Duration
	EventMaxAge          time.Duration

	// TelegramBotToken — токен бота для проверки initData.
	TelegramBotToken string
	// TelegramInitDataMaxAge — максимальный возраст initData.
	TelegramInitDataMaxAge time.Duration
	// DevAuthUserID — только для локальной отладки: если задан, запросы
	// без initData считаются пользователем с этим Telegram id.
	DevAuthUserID int64

	// YandexAPIKey — API-ключ Yandex Cloud.
	YandexAPIKey string
	// YandexFolderID — folder id Yandex Cloud.
	YandexFolderID string
	// YandexRealtimeURL — WebSocket endpoint Yandex Realtime API.
	YandexRealtimeURL string
	// YandexRealtimeModel — modelUri голосовой модели.
	YandexRealtimeModel string
	// YandexInputRate — частота PCM на входе, Гц.
	YandexInputRate int
	// YandexOutputRate — частота PCM на выходе, Гц.
	YandexOutputRate int
	// YandexVoice — голос Гида.
	YandexVoice string
	// YandexVoiceRole — эмоциональная роль голоса.
	YandexVoiceRole string
	// YandexVADThreshold — чувствительность серверного VAD.
	YandexVADThreshold float64
	// YandexVADSilenceMS — тишина для завершения реплики, мс.
	YandexVADSilenceMS int

	// GuideInstructions — системный промпт Гида.
	GuideInstructions string
	// SessionTTL — время жизни сессии.
	SessionTTL time.Duration
	// RealtimeTokenTTL — время жизни одноразового токена WebSocket.
	RealtimeTokenTTL time.Duration
}

const (
	defaultAddr            = ":8080"
	defaultEventsFile      = "data/events.jsonl"
	defaultCORSOrigins     = "http://localhost:5173,http://127.0.0.1:5173"
	defaultMaxRecentEvents = 1000
	defaultShutdownTimeout = 10 * time.Second

	// Логирование по умолчанию выключено; включается LOGGING_ENABLED=true.
	defaultLoggingEnabled = false

	defaultLogFile   = "/tmp/sophygoapp/backend.log"
	defaultLogLevel  = "info"
	defaultLogFormat = "json"

	defaultClientLogFile          = "/tmp/sophygoapp/frontend-client.log"
	defaultClientLogMaxBodyBytes  = 64 << 10
	defaultClientLogRatePerMinute = 120

	defaultHTTPMaxBodyBytes      = 64 << 10
	defaultHTTPRequestTimeout    = 15 * time.Second
	defaultHTTPReadHeaderTimeout = 5 * time.Second
	defaultHTTPReadTimeout       = 15 * time.Second
	defaultHTTPWriteTimeout      = 20 * time.Second
	defaultHTTPIdleTimeout       = 60 * time.Second

	defaultEventsLimit    = 50
	defaultEventsMaxLimit = 500

	defaultWSMaxMessageBytes = 1 << 20
	defaultWSPingInterval    = 20 * time.Second
	defaultWSPingTimeout     = 10 * time.Second
	defaultWSWriteTimeout    = 10 * time.Second

	defaultRealtimeMaxReadBytes = 16 << 20
	defaultRealtimeDialTimeout  = 15 * time.Second
	defaultRealtimeWriteTimeout = 10 * time.Second

	defaultGuideCleanupInterval = 30 * time.Second

	defaultEventMaxPlaceIDLen   = 128
	defaultEventMaxNameLen      = 300
	defaultEventMaxCategoryLen  = 64
	defaultEventMaxSessionIDLen = 128
	defaultEventMaxUserAgentLen = 512
	defaultEventMaxLanguageLen  = 32
	defaultEventMaxPlatformLen  = 32
	defaultEventMaxRemoteIPLen  = 64
	defaultEventMaxClockSkew    = 24 * time.Hour
	defaultEventMaxAge          = 30 * 24 * time.Hour

	defaultTelegramInitDataAge = 24 * time.Hour

	defaultYandexRealtimeURL   = "wss://ai.api.cloud.yandex.net/v1/realtime"
	defaultYandexRealtimeModel = "speech-realtime-deepseek-v4-flash"
	// 16 кГц — wideband-речь: разборчиво для STT и в 2,75 раза легче 44,1 кГц.
	defaultYandexInputRate    = 16000
	defaultYandexOutputRate   = 16000
	defaultYandexVoice        = "alena"
	defaultYandexVoiceRole    = "good"
	defaultYandexVADThreshold = 0.5
	defaultYandexVADSilenceMS = 400

	defaultGuideInstructions = "Ты умный, общительный гид по историческим достопримечательностям. " +
		"Ты рассказываешь только факты, шутишь. Пользователь тебя может перебить и что-то уточнить. " +
		"Отвечай живо и кратко, голосом, без списков и markdown."

	defaultSessionTTL       = 2 * time.Hour
	defaultRealtimeTokenTTL = 60 * time.Second
)

// Load читает конфигурацию из окружения и проверяет её корректность.
func Load() (Config, error) {
	// .env ищется в текущей директории и выше. Уже заданные переменные
	// окружения имеют приоритет, поэтому в production файл можно не создавать.
	_, _ = envfile.LoadNearest(".env")

	cfg := Config{
		Addr:                   getEnv("HTTP_ADDR", defaultAddr),
		EventsFile:             getEnv("EVENTS_FILE", defaultEventsFile),
		CORSOrigins:            splitList(getEnv("CORS_ORIGINS", defaultCORSOrigins)),
		MaxRecentEvents:        defaultMaxRecentEvents,
		ShutdownTimeout:        defaultShutdownTimeout,
		LoggingEnabled:         defaultLoggingEnabled,
		LogFile:                getEnv("LOG_FILE", defaultLogFile),
		LogLevel:               strings.ToLower(getEnv("LOG_LEVEL", defaultLogLevel)),
		LogFormat:              strings.ToLower(getEnv("LOG_FORMAT", defaultLogFormat)),
		ClientLogFile:          getEnv("CLIENT_LOG_FILE", defaultClientLogFile),
		ClientLogMaxBodyBytes:  defaultClientLogMaxBodyBytes,
		ClientLogRatePerMinute: defaultClientLogRatePerMinute,
		HTTPMaxBodyBytes:       defaultHTTPMaxBodyBytes,
		HTTPRequestTimeout:     defaultHTTPRequestTimeout,
		HTTPReadHeaderTimeout:  defaultHTTPReadHeaderTimeout,
		HTTPReadTimeout:        defaultHTTPReadTimeout,
		HTTPWriteTimeout:       defaultHTTPWriteTimeout,
		HTTPIdleTimeout:        defaultHTTPIdleTimeout,
		EventsDefaultLimit:     defaultEventsLimit,
		EventsMaxLimit:         defaultEventsMaxLimit,
		WSMaxMessageBytes:      defaultWSMaxMessageBytes,
		WSPingInterval:         defaultWSPingInterval,
		WSPingTimeout:          defaultWSPingTimeout,
		WSWriteTimeout:         defaultWSWriteTimeout,
		RealtimeMaxReadBytes:   defaultRealtimeMaxReadBytes,
		RealtimeDialTimeout:    defaultRealtimeDialTimeout,
		RealtimeWriteTimeout:   defaultRealtimeWriteTimeout,
		GuideCleanupInterval:   defaultGuideCleanupInterval,
		EventMaxPlaceIDLen:     defaultEventMaxPlaceIDLen,
		EventMaxNameLen:        defaultEventMaxNameLen,
		EventMaxCategoryLen:    defaultEventMaxCategoryLen,
		EventMaxSessionIDLen:   defaultEventMaxSessionIDLen,
		EventMaxUserAgentLen:   defaultEventMaxUserAgentLen,
		EventMaxLanguageLen:    defaultEventMaxLanguageLen,
		EventMaxPlatformLen:    defaultEventMaxPlatformLen,
		EventMaxRemoteIPLen:    defaultEventMaxRemoteIPLen,
		EventMaxClockSkew:      defaultEventMaxClockSkew,
		EventMaxAge:            defaultEventMaxAge,
		TelegramBotToken:       strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramInitDataMaxAge: defaultTelegramInitDataAge,
		YandexAPIKey:           strings.TrimSpace(os.Getenv("YANDEX_API_KEY")),
		YandexFolderID:         strings.TrimSpace(os.Getenv("YANDEX_FOLDER_ID")),
		YandexRealtimeURL:      getEnv("YANDEX_REALTIME_URL", defaultYandexRealtimeURL),
		YandexInputRate:        defaultYandexInputRate,
		YandexOutputRate:       defaultYandexOutputRate,
		YandexVoice:            getEnv("YANDEX_VOICE", defaultYandexVoice),
		YandexVoiceRole:        getEnv("YANDEX_VOICE_ROLE", defaultYandexVoiceRole),
		YandexVADThreshold:     defaultYandexVADThreshold,
		YandexVADSilenceMS:     defaultYandexVADSilenceMS,
		GuideInstructions:      getEnv("GUIDE_INSTRUCTIONS", defaultGuideInstructions),
		SessionTTL:             defaultSessionTTL,
		RealtimeTokenTTL:       defaultRealtimeTokenTTL,
	}

	if strings.TrimSpace(cfg.Addr) == "" {
		return Config{}, fmt.Errorf("HTTP_ADDR не может быть пустым")
	}
	if strings.TrimSpace(cfg.EventsFile) == "" {
		return Config{}, fmt.Errorf("EVENTS_FILE не может быть пустым")
	}
	if strings.TrimSpace(cfg.YandexRealtimeURL) == "" {
		return Config{}, fmt.Errorf("YANDEX_REALTIME_URL не может быть пустым")
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL должен быть debug, info, warn или error, получено %q", cfg.LogLevel)
	}
	switch cfg.LogFormat {
	case "json", "text":
	default:
		return Config{}, fmt.Errorf("LOG_FORMAT должен быть json или text, получено %q", cfg.LogFormat)
	}

	if raw := strings.TrimSpace(os.Getenv("DEV_AUTH_USER_ID")); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || value < 0 {
			return Config{}, fmt.Errorf("DEV_AUTH_USER_ID должен быть неотрицательным числом, получено %q", raw)
		}
		cfg.DevAuthUserID = value
	}

	var err error
	if cfg.MaxRecentEvents, err = intEnv("MAX_RECENT_EVENTS", cfg.MaxRecentEvents); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = durationEnv("SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTPMaxBodyBytes, err = int64Env("HTTP_MAX_BODY_BYTES", cfg.HTTPMaxBodyBytes); err != nil {
		return Config{}, err
	}
	if cfg.HTTPRequestTimeout, err = durationEnv("HTTP_REQUEST_TIMEOUT", cfg.HTTPRequestTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTPReadHeaderTimeout, err = durationEnv("HTTP_READ_HEADER_TIMEOUT", cfg.HTTPReadHeaderTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTPReadTimeout, err = durationEnv("HTTP_READ_TIMEOUT", cfg.HTTPReadTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTPWriteTimeout, err = durationEnv("HTTP_WRITE_TIMEOUT", cfg.HTTPWriteTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTPIdleTimeout, err = durationEnv("HTTP_IDLE_TIMEOUT", cfg.HTTPIdleTimeout); err != nil {
		return Config{}, err
	}
	if cfg.EventsDefaultLimit, err = intEnv("EVENTS_DEFAULT_LIMIT", cfg.EventsDefaultLimit); err != nil {
		return Config{}, err
	}
	if cfg.EventsMaxLimit, err = intEnv("EVENTS_MAX_LIMIT", cfg.EventsMaxLimit); err != nil {
		return Config{}, err
	}
	if cfg.WSMaxMessageBytes, err = int64Env("WS_MAX_MESSAGE_BYTES", cfg.WSMaxMessageBytes); err != nil {
		return Config{}, err
	}
	if cfg.WSPingInterval, err = durationEnv("WS_PING_INTERVAL", cfg.WSPingInterval); err != nil {
		return Config{}, err
	}
	if cfg.WSPingTimeout, err = durationEnv("WS_PING_TIMEOUT", cfg.WSPingTimeout); err != nil {
		return Config{}, err
	}
	if cfg.WSWriteTimeout, err = durationEnv("WS_WRITE_TIMEOUT", cfg.WSWriteTimeout); err != nil {
		return Config{}, err
	}
	if cfg.RealtimeMaxReadBytes, err = int64Env("REALTIME_MAX_READ_BYTES", cfg.RealtimeMaxReadBytes); err != nil {
		return Config{}, err
	}
	if cfg.RealtimeDialTimeout, err = durationEnv("REALTIME_DIAL_TIMEOUT", cfg.RealtimeDialTimeout); err != nil {
		return Config{}, err
	}
	if cfg.RealtimeWriteTimeout, err = durationEnv("REALTIME_WRITE_TIMEOUT", cfg.RealtimeWriteTimeout); err != nil {
		return Config{}, err
	}
	if cfg.GuideCleanupInterval, err = durationEnv("GUIDE_CLEANUP_INTERVAL", cfg.GuideCleanupInterval); err != nil {
		return Config{}, err
	}
	if cfg.EventMaxPlaceIDLen, err = intEnv("EVENT_MAX_PLACE_ID_LEN", cfg.EventMaxPlaceIDLen); err != nil {
		return Config{}, err
	}
	if cfg.EventMaxNameLen, err = intEnv("EVENT_MAX_NAME_LEN", cfg.EventMaxNameLen); err != nil {
		return Config{}, err
	}
	if cfg.EventMaxCategoryLen, err = intEnv("EVENT_MAX_CATEGORY_LEN", cfg.EventMaxCategoryLen); err != nil {
		return Config{}, err
	}
	if cfg.EventMaxSessionIDLen, err = intEnv("EVENT_MAX_SESSION_ID_LEN", cfg.EventMaxSessionIDLen); err != nil {
		return Config{}, err
	}
	if cfg.EventMaxUserAgentLen, err = intEnv("EVENT_MAX_USER_AGENT_LEN", cfg.EventMaxUserAgentLen); err != nil {
		return Config{}, err
	}
	if cfg.EventMaxLanguageLen, err = intEnv("EVENT_MAX_LANGUAGE_LEN", cfg.EventMaxLanguageLen); err != nil {
		return Config{}, err
	}
	if cfg.EventMaxPlatformLen, err = intEnv("EVENT_MAX_PLATFORM_LEN", cfg.EventMaxPlatformLen); err != nil {
		return Config{}, err
	}
	if cfg.EventMaxRemoteIPLen, err = intEnv("EVENT_MAX_REMOTE_IP_LEN", cfg.EventMaxRemoteIPLen); err != nil {
		return Config{}, err
	}
	if cfg.EventMaxClockSkew, err = durationEnv("EVENT_MAX_CLOCK_SKEW", cfg.EventMaxClockSkew); err != nil {
		return Config{}, err
	}
	if cfg.EventMaxAge, err = durationEnv("EVENT_MAX_AGE", cfg.EventMaxAge); err != nil {
		return Config{}, err
	}
	if cfg.TelegramInitDataMaxAge, err = durationEnv("TELEGRAM_INIT_DATA_MAX_AGE", cfg.TelegramInitDataMaxAge); err != nil {
		return Config{}, err
	}
	if cfg.SessionTTL, err = durationEnv("SESSION_TTL", cfg.SessionTTL); err != nil {
		return Config{}, err
	}
	if cfg.RealtimeTokenTTL, err = durationEnv("REALTIME_TOKEN_TTL", cfg.RealtimeTokenTTL); err != nil {
		return Config{}, err
	}
	if cfg.LoggingEnabled, err = boolEnv("LOGGING_ENABLED", cfg.LoggingEnabled); err != nil {
		return Config{}, err
	}
	if cfg.ClientLogMaxBodyBytes, err = int64Env("CLIENT_LOG_MAX_BODY_BYTES", cfg.ClientLogMaxBodyBytes); err != nil {
		return Config{}, err
	}
	if cfg.ClientLogRatePerMinute, err = intEnv("CLIENT_LOG_RATE_PER_MINUTE", cfg.ClientLogRatePerMinute); err != nil {
		return Config{}, err
	}
	if cfg.YandexInputRate, err = intEnv("YANDEX_INPUT_RATE", cfg.YandexInputRate); err != nil {
		return Config{}, err
	}
	if cfg.YandexOutputRate, err = intEnv("YANDEX_OUTPUT_RATE", cfg.YandexOutputRate); err != nil {
		return Config{}, err
	}
	if cfg.YandexVADSilenceMS, err = intEnv("YANDEX_VAD_SILENCE_MS", cfg.YandexVADSilenceMS); err != nil {
		return Config{}, err
	}
	if cfg.YandexVADThreshold, err = floatEnv("YANDEX_VAD_THRESHOLD", cfg.YandexVADThreshold); err != nil {
		return Config{}, err
	}

	if cfg.EventsDefaultLimit > cfg.EventsMaxLimit {
		cfg.EventsDefaultLimit = cfg.EventsMaxLimit
	}

	model := strings.TrimSpace(os.Getenv("YANDEX_REALTIME_MODEL"))
	switch {
	case model != "":
		cfg.YandexRealtimeModel = model
	case cfg.YandexFolderID != "":
		cfg.YandexRealtimeModel = fmt.Sprintf("gpt://%s/%s", cfg.YandexFolderID, defaultYandexRealtimeModel)
	default:
		cfg.YandexRealtimeModel = defaultYandexRealtimeModel
	}

	if strings.TrimSpace(cfg.GuideInstructions) == "" {
		cfg.GuideInstructions = defaultGuideInstructions
	}

	return cfg, nil
}

// RealtimeConfigured сообщает, заданы ли креды для Yandex Realtime API.
func (c Config) RealtimeConfigured() bool {
	return c.YandexAPIKey != "" && c.YandexFolderID != ""
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func intEnv(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s должен быть положительным числом, получено %q", key, raw)
	}
	return value, nil
}

func int64Env(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s должен быть положительным числом, получено %q", key, raw)
	}
	return value, nil
}

func boolEnv(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback, nil
	}

	switch raw {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s должен быть true или false, получено %q", key, raw)
	}
}

func floatEnv(key string, fallback float64) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || value >= 1 {
		return 0, fmt.Errorf("%s должен быть числом в диапазоне (0, 1), получено %q", key, raw)
	}
	return value, nil
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s должен быть положительной длительностью, получено %q", key, raw)
	}
	return value, nil
}
