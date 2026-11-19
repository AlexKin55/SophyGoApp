package realtime_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sophygo/backend/internal/envfile"
	"github.com/sophygo/backend/internal/realtime"
)

// defaultIntegrationModel — актуальная мультимодальная модель Realtime API.
const defaultIntegrationModel = "speech-realtime-deepseek-v4-flash"

// TestYandexRealtimeSession проверяет реальное открытие сессии в Yandex
// Realtime API и доступность модели.
//
// Тест интеграционный: он ходит в сеть. Настройки и ключи берутся из
// backend/.env (или из окружения, которое имеет приоритет). Если ключи
// не заданы, тест пропускается.
func TestYandexRealtimeSession(t *testing.T) {
	// Тесты запускаются из каталога пакета, поэтому .env ищется выше.
	_, _ = envfile.LoadNearest(".env")

	if os.Getenv("SOPHYGO_SKIP_INTEGRATION") == "1" {
		t.Skip("SOPHYGO_SKIP_INTEGRATION=1 — интеграционный тест пропущен")
	}

	apiKey := strings.TrimSpace(os.Getenv("YANDEX_API_KEY"))
	folderID := strings.TrimSpace(os.Getenv("YANDEX_FOLDER_ID"))
	if apiKey == "" || folderID == "" {
		t.Skip("YANDEX_API_KEY/YANDEX_FOLDER_ID не заданы — интеграционный тест пропущен")
	}

	modelURI := strings.TrimSpace(os.Getenv("YANDEX_REALTIME_MODEL"))
	if modelURI == "" {
		modelURI = "gpt://" + folderID + "/" + defaultIntegrationModel
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	conn, err := realtime.Dial(ctx, realtime.Config{
		URL:          envOr("YANDEX_REALTIME_URL", "wss://ai.api.cloud.yandex.net/v1/realtime"),
		ModelURI:     modelURI,
		APIKey:       apiKey,
		Instructions: "Ты тестовый гид. Ответь коротко.",
		InputRate:    16000,
		OutputRate:   16000,
		Voice:        envOr("YANDEX_VOICE", "alena"),
		VoiceRole:    envOr("YANDEX_VOICE_ROLE", "good"),
		VADThreshold: 0.5,
		VADSilenceMS: 400,
	})
	if err != nil {
		t.Fatalf("не удалось открыть сессию Yandex Realtime (модель %s): %v", modelURI, err)
	}
	defer func() { _ = conn.CloseNow() }()

	seen := make([]string, 0, 4)
	for {
		_, data, readErr := conn.Read(ctx)
		if readErr != nil {
			t.Fatalf("не удалось прочитать событие Realtime: %v; полученные события: %v", readErr, seen)
		}

		var event struct {
			Type  string `json:"type"`
			Error *struct {
				Type    string `json:"type"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatalf("некорректное событие Realtime: %v (%s)", err, string(data))
		}
		seen = append(seen, event.Type)

		if event.Type == "error" {
			code, message := "", ""
			if event.Error != nil {
				code, message = event.Error.Code, event.Error.Message
			}
			t.Fatalf("Yandex Realtime вернул ошибку: code=%s message=%s; события: %v", code, message, seen)
		}

		// session.created подтверждает доступ к модели, session.updated —
		// что наш session.update (промпт, голос, 16 кГц) принят.
		if event.Type == "session.updated" {
			return
		}
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
