// Сервис SophyGo: принимает события от frontend-приложения и ведёт
// голосовую сессию Гида через Yandex Realtime API.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sophygo/backend/internal/clientlog"
	"github.com/sophygo/backend/internal/config"
	"github.com/sophygo/backend/internal/events"
	"github.com/sophygo/backend/internal/guide"
	"github.com/sophygo/backend/internal/httpapi"
	"github.com/sophygo/backend/internal/realtime"
	"github.com/sophygo/backend/internal/telegram"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "не удалось загрузить конфигурацию:", err)
		os.Exit(1)
	}

	logger, closeLog := newLogger(cfg)
	defer closeLog()
	slog.SetDefault(logger)

	if err := run(logger, cfg); err != nil {
		logger.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

// newLogger собирает логгер: файл из LOG_FILE плюс stdout.
// При LOGGING_ENABLED=false все логи отбрасываются.
func newLogger(cfg config.Config) (*slog.Logger, func()) {
	if !cfg.LoggingEnabled {
		return slog.New(slog.NewTextHandler(io.Discard, nil)), func() {}
	}

	writer := io.Writer(os.Stdout)
	closeFn := func() {}

	if path := strings.TrimSpace(cfg.LogFile); path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "не удалось создать каталог логов %q: %v\n", filepath.Dir(path), err)
		} else if file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "не удалось открыть лог-файл %q: %v\n", path, err)
		} else {
			writer = io.MultiWriter(os.Stdout, file)
			closeFn = func() { _ = file.Close() }
		}
	}

	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	options := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.LogFormat == "text" {
		handler = slog.NewTextHandler(writer, options)
	} else {
		handler = slog.NewJSONHandler(writer, options)
	}
	return slog.New(handler), closeFn
}

func run(logger *slog.Logger, cfg config.Config) error {
	store, err := events.Open(cfg.EventsFile, cfg.MaxRecentEvents)
	if err != nil {
		return err
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("failed to close event store", "error", err)
		}
	}()

	guideManager := guide.NewManagerWithCleanup(cfg.SessionTTL, cfg.RealtimeTokenTTL, cfg.GuideCleanupInterval)
	defer guideManager.Close()

	var clientLogWriter *clientlog.Writer
	if cfg.LoggingEnabled {
		clientLogWriter, err = clientlog.Open(cfg.ClientLogFile)
		if err != nil {
			logger.Warn("не удалось открыть файл клиентских логов", "error", err)
		}
	}
	defer func() {
		if err := clientLogWriter.Close(); err != nil {
			logger.Warn("не удалось закрыть файл клиентских логов", "error", err)
		}
	}()

	eventLimits := events.Limits{
		MaxPlaceIDLen:   cfg.EventMaxPlaceIDLen,
		MaxNameLen:      cfg.EventMaxNameLen,
		MaxCategoryLen:  cfg.EventMaxCategoryLen,
		MaxSessionIDLen: cfg.EventMaxSessionIDLen,
		MaxUserAgentLen: cfg.EventMaxUserAgentLen,
		MaxLanguageLen:  cfg.EventMaxLanguageLen,
		MaxPlatformLen:  cfg.EventMaxPlatformLen,
		MaxRemoteIPLen:  cfg.EventMaxRemoteIPLen,
		MaxClockSkew:    cfg.EventMaxClockSkew,
		MaxAge:          cfg.EventMaxAge,
	}

	verifier := telegram.NewVerifier(cfg.TelegramBotToken, cfg.TelegramInitDataMaxAge)
	if !verifier.Configured() {
		logger.Warn("TELEGRAM_BOT_TOKEN не задан: запросы с initData будут отклонены")
	}

	realtimeConfig := realtime.Config{
		URL:          cfg.YandexRealtimeURL,
		ModelURI:     cfg.YandexRealtimeModel,
		APIKey:       cfg.YandexAPIKey,
		Instructions: cfg.GuideInstructions,
		InputRate:    cfg.YandexInputRate,
		OutputRate:   cfg.YandexOutputRate,
		Voice:        cfg.YandexVoice,
		VoiceRole:    cfg.YandexVoiceRole,
		VADThreshold: cfg.YandexVADThreshold,
		VADSilenceMS: cfg.YandexVADSilenceMS,
		DialTimeout:  cfg.RealtimeDialTimeout,
		MaxReadBytes: cfg.RealtimeMaxReadBytes,
		WriteTimeout: cfg.RealtimeWriteTimeout,
	}
	if !cfg.RealtimeConfigured() {
		logger.Warn("YANDEX_API_KEY или YANDEX_FOLDER_ID не заданы: голосовой Гид недоступен")
	}
	if cfg.DevAuthUserID > 0 {
		logger.Warn("DEV_AUTH_USER_ID задан: авторизация Telegram отключена. Не используйте в production",
			"dev_user_id", cfg.DevAuthUserID,
		)
	}

	server := &http.Server{
		Addr: cfg.Addr,
		Handler: httpapi.New(httpapi.Deps{
			Store:                  store,
			Guide:                  guideManager,
			Telegram:               verifier,
			Realtime:               realtimeConfig,
			Logger:                 logger,
			CORSOrigins:            cfg.CORSOrigins,
			DevAuthUserID:          cfg.DevAuthUserID,
			EventLimits:            eventLimits,
			MaxBodyBytes:           cfg.HTTPMaxBodyBytes,
			RequestTimeout:         cfg.HTTPRequestTimeout,
			EventsDefaultLimit:     cfg.EventsDefaultLimit,
			EventsMaxLimit:         cfg.EventsMaxLimit,
			WSMaxMessageBytes:      cfg.WSMaxMessageBytes,
			WSPingInterval:         cfg.WSPingInterval,
			WSPingTimeout:          cfg.WSPingTimeout,
			WSWriteTimeout:         cfg.WSWriteTimeout,
			ClientLog:              clientLogWriter,
			ClientLogMaxBodyBytes:  cfg.ClientLogMaxBodyBytes,
			ClientLogRatePerMinute: cfg.ClientLogRatePerMinute,
		}).Routes(),
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("http server started",
			"addr", cfg.Addr,
			"events_file", cfg.EventsFile,
			"cors_origins", cfg.CORSOrigins,
			"realtime_model", cfg.YandexRealtimeModel,
			"realtime_configured", cfg.RealtimeConfigured(),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	select {
	case err := <-serverErrors:
		return err
	case sig := <-stop:
		logger.Info("shutdown signal received", "signal", sig.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	logger.Info("http server stopped gracefully")
	return nil
}
