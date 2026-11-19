package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/sophygo/backend/internal/clientlog"
	"github.com/sophygo/backend/internal/events"
	"github.com/sophygo/backend/internal/guide"
	"github.com/sophygo/backend/internal/realtime"
	"github.com/sophygo/backend/internal/telegram"
)

const (
	defaultEventsLimit = 50
	maxEventsLimit     = 500

	maxWSMessageBytes = 1 << 20
	wsPingInterval    = 20 * time.Second
	wsPingTimeout     = 10 * time.Second
	wsWriteTimeout    = 10 * time.Second

	maxClientLogEntries = 100
	maxClientLogWindows = 1000
)

type healthResponse struct {
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

type createSessionResponse struct {
	SessionID         string              `json:"sessionId"`
	RealtimeToken     string              `json:"realtimeToken"`
	RealtimePath      string              `json:"realtimePath"`
	RealtimeAvailable bool                `json:"realtimeAvailable"`
	InputRate         int                 `json:"inputRate"`
	OutputRate        int                 `json:"outputRate"`
	ExpiresAt         time.Time           `json:"expiresAt"`
	User              events.TelegramUser `json:"user"`
}

type placeOpenResponse struct {
	ID         string    `json:"id"`
	ReceivedAt time.Time `json:"receivedAt"`
	SessionID  string    `json:"sessionId,omitempty"`
	Delivered  bool      `json:"delivered"`
}

type recentEventsResponse struct {
	Events []events.Event `json:"events"`
	Count  int            `json:"count"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status: "ok",
		Time:   time.Now().UTC(),
	})
}

// handleCreateSession проверяет initData и создаёт сессию Гида.
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	initData, ok := s.requireTelegram(w, r)
	if !ok {
		return
	}

	info := s.guide.Ensure(initData.User.ID)
	token, err := s.guide.IssueToken(info.ID)
	if err != nil {
		s.logger.Error("failed to issue realtime token", "error", err, "session_id", info.ID)
		writeError(w, http.StatusInternalServerError, "не удалось создать сессию")
		return
	}

	s.logger.Info("guide session created",
		"session_id", info.ID,
		"telegram_user_id", initData.User.ID,
		"realtime_available", s.realtimeConfigured,
		"input_rate", s.realtime.InputRate,
		"output_rate", s.realtime.OutputRate,
		"expires_at", info.ExpiresAt,
	)

	writeJSON(w, http.StatusOK, createSessionResponse{
		SessionID:         info.ID,
		RealtimeToken:     token,
		RealtimePath:      "/api/v1/realtime",
		RealtimeAvailable: s.realtimeConfigured,
		InputRate:         s.realtime.InputRate,
		OutputRate:        s.realtime.OutputRate,
		ExpiresAt:         info.ExpiresAt,
		User:              telegramUser(initData.User),
	})
}

// handlePlaceOpen сохраняет событие и просит Гида рассказать об объекте.
func (s *Server) handlePlaceOpen(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBodyBytes)

	var request events.PlaceOpenRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "тело запроса слишком большое")
			return
		}
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		writeError(w, http.StatusBadRequest, "в теле запроса должен быть один JSON-объект")
		return
	}

	now := time.Now().UTC()
	if err := request.ValidateWithLimits(now, s.eventLimits); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	initData, err := s.authenticate(r)
	if err != nil {
		s.writeAuthError(w, err)
		return
	}

	sessionID := request.SessionID
	if initData != nil {
		sessionID, err = s.resolveSession(sessionID, initData.User.ID)
		if err != nil {
			writeError(w, http.StatusForbidden, "сессия принадлежит другому пользователю")
			return
		}
	}

	event := events.NewEventWithLimits(request, now, clientIP(r), s.eventLimits)
	event.SessionID = sessionID
	if initData != nil {
		user := telegramUser(initData.User)
		event.Telegram = &user
	}

	if err := s.store.Append(r.Context(), event); err != nil {
		s.logger.Error("failed to store place_open event",
			"error", err,
			"place_id", event.Place.ID,
		)
		writeError(w, http.StatusInternalServerError, "не удалось сохранить событие")
		return
	}

	delivered := false
	if initData != nil && sessionID != "" {
		err := s.guide.SendPlacePrompt(r.Context(), sessionID, event.Place.Name, event.Place.Latitude, event.Place.Longitude)
		switch {
		case err == nil:
			delivered = true
		case errors.Is(err, guide.ErrNoRealtime):
			s.logger.Info("place prompt skipped: realtime is not connected", "session_id", sessionID)
		case errors.Is(err, guide.ErrSessionNotFound):
			s.logger.Info("place prompt skipped: session is gone", "session_id", sessionID)
		default:
			s.logger.Error("failed to send place prompt", "error", err, "session_id", sessionID)
		}
	}

	s.logger.Info("place opened",
		"event_id", event.ID,
		"place_id", event.Place.ID,
		"place_name", event.Place.Name,
		"place_category", event.Place.Category,
		"session_id", sessionID,
		"telegram_user_id", telegramUserID(event.Telegram),
		"delivered", delivered,
	)

	writeJSON(w, http.StatusAccepted, placeOpenResponse{
		ID:         event.ID,
		ReceivedAt: event.ReceivedAt,
		SessionID:  sessionID,
		Delivered:  delivered,
	})
}

// handleStopSession прерывает текущий ответ Гида.
func (s *Server) handleStopSession(w http.ResponseWriter, r *http.Request) {
	initData, ok := s.requireTelegram(w, r)
	if !ok {
		return
	}

	sessionID := chi.URLParam(r, "sessionID")
	info, exists := s.guide.Get(sessionID)
	if !exists {
		writeError(w, http.StatusNotFound, "сессия не найдена")
		return
	}
	if info.TelegramUserID != initData.User.ID {
		writeError(w, http.StatusForbidden, "сессия принадлежит другому пользователю")
		return
	}

	err := s.guide.Stop(r.Context(), sessionID)
	switch {
	case err == nil:
		s.logger.Info("guide response stopped",
			"session_id", sessionID,
			"telegram_user_id", initData.User.ID,
		)
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, guide.ErrNoRealtime):
		writeError(w, http.StatusConflict, "Realtime-соединение не установлено")
	default:
		s.logger.Error("failed to stop guide response", "error", err, "session_id", sessionID)
		writeError(w, http.StatusBadGateway, "не удалось остановить ответ Гида")
	}
}

// handleRealtime проксирует WebSocket между браузером и Yandex Realtime API.
func (s *Server) handleRealtime(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		writeError(w, http.StatusBadRequest, "token обязателен")
		return
	}

	if origin := r.Header.Get("Origin"); origin != "" && !s.isOriginAllowed(origin) {
		writeError(w, http.StatusForbidden, "origin не разрешён")
		return
	}

	if !s.realtimeConfigured {
		writeError(w, http.StatusServiceUnavailable, "Yandex Realtime API не настроен")
		return
	}

	sessionID, err := s.guide.ConsumeToken(token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "недействительный токен")
		return
	}

	info, exists := s.guide.Get(sessionID)
	if !exists {
		writeError(w, http.StatusUnauthorized, "сессия не найдена")
		return
	}

	upstream, err := realtime.Dial(r.Context(), s.realtime)
	if err != nil {
		s.logger.Error("failed to connect to Yandex Realtime API", "error", err)
		writeError(w, http.StatusBadGateway, "не удалось подключиться к Yandex Realtime API")
		return
	}
	defer func() { _ = upstream.CloseNow() }()

	browser, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionDisabled,
	})
	if err != nil {
		s.logger.Warn("websocket accept failed", "error", err)
		return
	}
	browser.SetReadLimit(s.wsMaxMessageBytes)

	generation, err := s.guide.Attach(sessionID, upstream)
	if err != nil {
		_ = browser.Close(websocket.StatusPolicyViolation, "session unavailable")
		return
	}
	defer s.guide.Detach(sessionID, generation)

	s.logger.Info("realtime connected",
		"session_id", sessionID,
		"telegram_user_id", info.TelegramUserID,
		"request_id", middleware.GetReqID(r.Context()),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 2)
	go func() { errCh <- pipeBrowserToRealtime(ctx, browser, upstream, s.logger) }()
	go func() { errCh <- pipeRealtimeToBrowser(ctx, upstream, browser, s.wsWriteTimeout, s.logger) }()
	go pingLoop(ctx, cancel, browser, upstream, s.wsPingInterval, s.wsPingTimeout)

	<-errCh
	cancel()

	_ = browser.CloseNow()
	_ = upstream.CloseNow()

	s.logger.Info("realtime disconnected", "session_id", sessionID)
}

func pipeBrowserToRealtime(ctx context.Context, browser *websocket.Conn, upstream *realtime.Conn, logger *slog.Logger) error {
	for {
		messageType, data, err := browser.Read(ctx)
		if err != nil {
			return err
		}
		if messageType != websocket.MessageText {
			continue
		}

		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			continue
		}
		logger.Debug("browser event", "type", event.Type)

		// Разрешаем только аудио и отмену ответа: браузер не должен
		// менять системный промпт или конфигурацию сессии.
		switch event.Type {
		case "input_audio_buffer.append", "input_audio_buffer.commit", "response.cancel":
		default:
			continue
		}

		if err := upstream.SendRaw(ctx, data); err != nil {
			return err
		}
	}
}

func pipeRealtimeToBrowser(ctx context.Context, upstream *realtime.Conn, browser *websocket.Conn, writeTimeout time.Duration, logger *slog.Logger) error {
	for {
		messageType, data, err := upstream.Read(ctx)
		if err != nil {
			return err
		}

		if logger.Enabled(ctx, slog.LevelDebug) {
			var event struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(data, &event) == nil {
				logger.Debug("realtime event", "type", event.Type)
			}
		}

		writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
		err = browser.Write(writeCtx, messageType, data)
		cancel()
		if err != nil {
			return err
		}
	}
}

func pingLoop(ctx context.Context, cancel context.CancelFunc, browser *websocket.Conn, upstream *realtime.Conn, interval, timeout time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, timeout)
			browserErr := browser.Ping(pingCtx)
			upstreamErr := upstream.Ping(pingCtx)
			pingCancel()

			if browserErr != nil || upstreamErr != nil {
				cancel()
				return
			}
		}
	}
}

type clientLogEntry struct {
	Timestamp string          `json:"timestamp,omitempty"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Context   json.RawMessage `json:"context,omitempty"`
	URL       string          `json:"url,omitempty"`
}

type clientLogRequest struct {
	Entries []clientLogEntry `json:"entries"`
}

type clientLogWindow struct {
	start time.Time
	count int
}

// handleClientLogs принимает пачки логов frontend и пишет их в файл.
// В Telegram WebView браузер не может писать файлы, поэтому логи
// отправляются на backend отдельными HTTP-пакетами.
func (s *Server) handleClientLogs(w http.ResponseWriter, r *http.Request) {
	if s.clientLog == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.clientLogMaxBodyBytes)

	var request clientLogRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "пакет логов слишком большой")
			return
		}
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}

	if len(request.Entries) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(request.Entries) > maxClientLogEntries {
		request.Entries = request.Entries[:maxClientLogEntries]
	}

	ip := clientIP(r)
	if !s.allowClientLog(ip) {
		writeError(w, http.StatusTooManyRequests, "слишком много логов")
		return
	}

	entries := make([]clientlog.Entry, 0, len(request.Entries))
	for _, entry := range request.Entries {
		entries = append(entries, clientlog.Entry{
			Timestamp: entry.Timestamp,
			Level:     entry.Level,
			Message:   entry.Message,
			Context:   entry.Context,
			URL:       entry.URL,
		})
	}

	if err := s.clientLog.Write(ip, entries); err != nil {
		s.logger.Error("failed to write client logs", "error", err)
		writeError(w, http.StatusInternalServerError, "не удалось сохранить логи")
		return
	}

	s.logger.Debug("client logs received", "count", len(entries), "remote_ip", ip)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) allowClientLog(ip string) bool {
	if s.clientLogRatePerMinute <= 0 {
		return true
	}

	now := time.Now()
	s.clientLogMu.Lock()
	defer s.clientLogMu.Unlock()

	window := s.clientLogWindows[ip]
	if window == nil || now.Sub(window.start) >= time.Minute {
		window = &clientLogWindow{start: now}
		s.clientLogWindows[ip] = window
	}
	window.count++

	if len(s.clientLogWindows) > maxClientLogWindows {
		for key, value := range s.clientLogWindows {
			if now.Sub(value.start) >= time.Minute {
				delete(s.clientLogWindows, key)
			}
		}
	}

	return window.count <= s.clientLogRatePerMinute
}

func (s *Server) handleRecentEvents(w http.ResponseWriter, r *http.Request) {
	limit := s.eventsDefaultLimit

	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			writeError(w, http.StatusBadRequest, "limit должен быть положительным числом")
			return
		}
		if value > s.eventsMaxLimit {
			value = s.eventsMaxLimit
		}
		limit = value
	}

	recent := s.store.Recent(limit)
	writeJSON(w, http.StatusOK, recentEventsResponse{
		Events: recent,
		Count:  len(recent),
	})
}

func (s *Server) authenticate(r *http.Request) (*telegram.InitData, error) {
	raw := strings.TrimSpace(r.Header.Get("X-Telegram-Init-Data"))
	if raw == "" {
		// DEV_AUTH_USER_ID включает локальную отладку в обычном браузере,
		// где Telegram WebApp недоступен и initData отсутствует.
		if s.devAuthUserID > 0 {
			return &telegram.InitData{
				User: telegram.User{
					ID:        s.devAuthUserID,
					FirstName: "Dev",
					Username:  "dev",
				},
			}, nil
		}
		return nil, nil
	}
	if s.telegram == nil {
		return nil, telegram.ErrNotConfigured
	}
	return s.telegram.Verify(raw)
}

func (s *Server) requireTelegram(w http.ResponseWriter, r *http.Request) (*telegram.InitData, bool) {
	initData, err := s.authenticate(r)
	if err != nil {
		s.writeAuthError(w, err)
		return nil, false
	}
	if initData == nil {
		writeError(w, http.StatusUnauthorized, "требуется Telegram initData")
		return nil, false
	}
	return initData, true
}

func (s *Server) writeAuthError(w http.ResponseWriter, err error) {
	status := http.StatusUnauthorized
	if errors.Is(err, telegram.ErrNotConfigured) {
		status = http.StatusServiceUnavailable
	}
	s.logger.Warn("telegram auth failed", "error", err)
	writeError(w, status, "не удалось проверить Telegram initData")
}

// resolveSession возвращает существующую сессию пользователя или создаёт новую.
func (s *Server) resolveSession(sessionID string, telegramUserID int64) (string, error) {
	if sessionID == "" {
		return s.guide.Ensure(telegramUserID).ID, nil
	}

	info, ok := s.guide.Get(sessionID)
	if !ok {
		return s.guide.Ensure(telegramUserID).ID, nil
	}
	if info.TelegramUserID != telegramUserID {
		return "", fmt.Errorf("session owner mismatch")
	}
	return sessionID, nil
}

func telegramUser(user telegram.User) events.TelegramUser {
	return events.TelegramUser{
		ID:           user.ID,
		Username:     user.Username,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		LanguageCode: user.LanguageCode,
	}
}

func telegramUserID(user *events.TelegramUser) int64 {
	if user == nil {
		return 0
	}
	return user.ID
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra json.RawMessage
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("лишние данные после JSON")
	}
	return err
}

func clientIP(r *http.Request) string {
	// middleware.RealIP уже подставляет IP из X-Forwarded-For / X-Real-IP.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
