// Package httpapi реализует HTTP-интерфейс сервиса.
package httpapi

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/sophygo/backend/internal/clientlog"
	"github.com/sophygo/backend/internal/events"
	"github.com/sophygo/backend/internal/guide"
	"github.com/sophygo/backend/internal/realtime"
	"github.com/sophygo/backend/internal/telegram"
)

const (
	maxRequestBodyBytes = 64 << 10
	requestTimeout      = 15 * time.Second
)

// Deps — зависимости HTTP-слоя.
type Deps struct {
	Store         *events.Store
	Guide         *guide.Manager
	Telegram      *telegram.Verifier
	Realtime      realtime.Config
	Logger        *slog.Logger
	CORSOrigins   []string
	DevAuthUserID int64
	EventLimits   events.Limits

	MaxBodyBytes       int64
	RequestTimeout     time.Duration
	EventsDefaultLimit int
	EventsMaxLimit     int
	WSMaxMessageBytes  int64
	WSPingInterval     time.Duration
	WSPingTimeout      time.Duration
	WSWriteTimeout     time.Duration

	ClientLog              *clientlog.Writer
	ClientLogMaxBodyBytes  int64
	ClientLogRatePerMinute int
}

// Server связывает HTTP-обработчики с зависимостями.
type Server struct {
	store              *events.Store
	guide              *guide.Manager
	telegram           *telegram.Verifier
	realtime           realtime.Config
	logger             *slog.Logger
	allowedOrigins     map[string]struct{}
	allowAllOrigins    bool
	realtimeConfigured bool
	devAuthUserID      int64
	eventLimits        events.Limits

	maxBodyBytes       int64
	requestTimeout     time.Duration
	eventsDefaultLimit int
	eventsMaxLimit     int
	wsMaxMessageBytes  int64
	wsPingInterval     time.Duration
	wsPingTimeout      time.Duration
	wsWriteTimeout     time.Duration

	clientLog              *clientlog.Writer
	clientLogMaxBodyBytes  int64
	clientLogRatePerMinute int
	clientLogMu            sync.Mutex
	clientLogWindows       map[string]*clientLogWindow
}

// New создаёт HTTP-сервер.
func New(deps Deps) *Server {
	allowed := make(map[string]struct{}, len(deps.CORSOrigins))
	allowAll := false
	for _, origin := range deps.CORSOrigins {
		if origin == "*" {
			allowAll = true
			continue
		}
		allowed[origin] = struct{}{}
	}

	return &Server{
		store:              deps.Store,
		guide:              deps.Guide,
		telegram:           deps.Telegram,
		realtime:           deps.Realtime,
		logger:             deps.Logger,
		allowedOrigins:     allowed,
		allowAllOrigins:    allowAll,
		realtimeConfigured: deps.Realtime.APIKey != "" && deps.Realtime.ModelURI != "",
		devAuthUserID:      deps.DevAuthUserID,
		eventLimits:        deps.EventLimits.WithDefaults(),
		maxBodyBytes:       positiveInt64(deps.MaxBodyBytes, maxRequestBodyBytes),
		requestTimeout:     positiveDuration(deps.RequestTimeout, requestTimeout),
		eventsDefaultLimit: positiveInt(deps.EventsDefaultLimit, defaultEventsLimit),
		eventsMaxLimit:     positiveInt(deps.EventsMaxLimit, maxEventsLimit),
		wsMaxMessageBytes:  positiveInt64(deps.WSMaxMessageBytes, maxWSMessageBytes),
		wsPingInterval:     positiveDuration(deps.WSPingInterval, wsPingInterval),
		wsPingTimeout:      positiveDuration(deps.WSPingTimeout, wsPingTimeout),
		wsWriteTimeout:     positiveDuration(deps.WSWriteTimeout, wsWriteTimeout),

		clientLog:              deps.ClientLog,
		clientLogMaxBodyBytes:  positiveInt64(deps.ClientLogMaxBodyBytes, maxRequestBodyBytes),
		clientLogRatePerMinute: positiveInt(deps.ClientLogRatePerMinute, 120),
		clientLogWindows:       make(map[string]*clientLogWindow),
	}
}

func positiveInt64(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

// Routes собирает маршрутизатор со всеми middleware.
func (s *Server) Routes() http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(s.requestLogger)
	router.Use(s.cors)

	router.Get("/healthz", s.handleHealth)

	// WebSocket-соединение живёт дольше обычного запроса, поэтому таймаут
	// middleware.Timeout к нему не применяется.
	router.Get("/api/v1/realtime", s.handleRealtime)

	router.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(s.requestTimeout))

		r.Post("/api/v1/session", s.handleCreateSession)
		r.Post("/api/v1/session/{sessionID}/stop", s.handleStopSession)
		r.Post("/api/v1/events/place-open", s.handlePlaceOpen)
		r.Get("/api/v1/events", s.handleRecentEvents)
		r.Post("/api/v1/client-logs", s.handleClientLogs)
	})

	return router
}
