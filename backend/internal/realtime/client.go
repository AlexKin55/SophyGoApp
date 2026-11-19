// Package realtime реализует клиент Yandex Realtime API (WebSocket).
//
// Протокол совместим с OpenAI Realtime: клиент отправляет session.update,
// input_audio_buffer.append, conversation.item.create и response.create,
// а сервер присылает события response.output_audio.delta,
// conversation.item.input_audio_transcription.completed и другие.
package realtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const (
	maxReadBytes  = 16 << 20
	dialTimeout   = 15 * time.Second
	writeTimeout  = 10 * time.Second
	audioPCMType  = "audio/pcm"
	defaultVoice  = "alena"
	defaultRole   = "good"
	serverVADType = "server_vad"
)

// Config — параметры подключения к Realtime API.
type Config struct {
	URL          string
	ModelURI     string
	APIKey       string
	Instructions string
	InputRate    int
	OutputRate   int
	Voice        string
	VoiceRole    string
	VADThreshold float64
	VADSilenceMS int
	DialTimeout  time.Duration
	MaxReadBytes int64
	WriteTimeout time.Duration
}

// Conn — активное WebSocket-соединение с Yandex Realtime API.
type Conn struct {
	ws           *websocket.Conn
	writeMu      sync.Mutex
	writeTimeout time.Duration

	// responseActive позволяет не отправлять response.cancel, когда
	// активного ответа нет: Yandex отвечает ошибкой "no active response".
	responseActive atomic.Bool
}

// Dial открывает соединение и отправляет начальную конфигурацию сессии.
func Dial(ctx context.Context, cfg Config) (*Conn, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("realtime: не задан YANDEX_API_KEY")
	}
	if cfg.ModelURI == "" {
		return nil, fmt.Errorf("realtime: не задан modelUri")
	}

	endpoint, err := buildURL(cfg.URL, cfg.ModelURI)
	if err != nil {
		return nil, err
	}

	timeout := cfg.DialTimeout
	if timeout <= 0 {
		timeout = dialTimeout
	}
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	headers := http.Header{}
	headers.Set("Authorization", "Api-Key "+cfg.APIKey)

	ws, response, err := websocket.Dial(dialCtx, endpoint, &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("realtime: подключение не удалось (HTTP %d): %w", response.StatusCode, err)
		}
		return nil, fmt.Errorf("realtime: подключение не удалось: %w", err)
	}

	readLimit := cfg.MaxReadBytes
	if readLimit <= 0 {
		readLimit = maxReadBytes
	}
	ws.SetReadLimit(readLimit)

	conn := &Conn{ws: ws, writeTimeout: cfg.WriteTimeout}
	if err := conn.SendJSON(ctx, buildSessionUpdate(cfg)); err != nil {
		_ = ws.CloseNow()
		return nil, err
	}

	return conn, nil
}

// SendJSON отправляет событие в Realtime API.
func (c *Conn) SendJSON(ctx context.Context, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("realtime: сериализация события: %w", err)
	}
	return c.write(ctx, data)
}

// SendRaw отправляет уже готовое JSON-событие без изменений.
func (c *Conn) SendRaw(ctx context.Context, data []byte) error {
	return c.write(ctx, data)
}

// SendAudio отправляет фрагмент PCM16 (mono) в base64.
func (c *Conn) SendAudio(ctx context.Context, pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}

	return c.SendJSON(ctx, map[string]string{
		"type":  "input_audio_buffer.append",
		"audio": base64.StdEncoding.EncodeToString(pcm),
	})
}

// SendUserText добавляет текстовую реплику пользователя и запускает ответ Гида.
func (c *Conn) SendUserText(ctx context.Context, text string) error {
	item := map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type": "message",
			"role": "user",
			"content": []map[string]string{
				{"type": "input_text", "text": text},
			},
		},
	}
	if err := c.SendJSON(ctx, item); err != nil {
		return err
	}

	return c.SendJSON(ctx, map[string]string{"type": "response.create"})
}

// CancelResponse прерывает текущий ответ Гида.
// Если активного ответа нет, ничего не отправляет.
func (c *Conn) CancelResponse(ctx context.Context) error {
	if !c.responseActive.Load() {
		return nil
	}
	return c.SendJSON(ctx, map[string]string{"type": "response.cancel"})
}

// Read читает следующее событие сервера и отслеживает состояние ответа.
func (c *Conn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	messageType, data, err := c.ws.Read(ctx)
	if err != nil {
		return messageType, data, err
	}
	if messageType != websocket.MessageText {
		return messageType, data, nil
	}

	// Тип события находится в начале JSON; ограничиваемся префиксом,
	// чтобы не разбирать большие base64-дельты аудио.
	prefix := data
	if len(prefix) > 96 {
		prefix = prefix[:96]
	}
	switch {
	case bytes.Contains(prefix, []byte(`"response.created"`)):
		c.responseActive.Store(true)
	case bytes.Contains(prefix, []byte(`"response.done"`)):
		c.responseActive.Store(false)
	}

	return messageType, data, nil
}

// Ping проверяет живость соединения.
func (c *Conn) Ping(ctx context.Context) error {
	return c.ws.Ping(ctx)
}

// Close закрывает соединение.
func (c *Conn) Close() error {
	return c.ws.Close(websocket.StatusNormalClosure, "session closed")
}

// CloseNow закрывает соединение без рукопожатия.
func (c *Conn) CloseNow() error {
	return c.ws.CloseNow()
}

func (c *Conn) write(ctx context.Context, data []byte) error {
	timeout := c.writeTimeout
	if timeout <= 0 {
		timeout = writeTimeout
	}

	writeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := c.ws.Write(writeCtx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("realtime: отправка события: %w", err)
	}
	return nil
}

func buildURL(base, modelURI string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("realtime: некорректный URL %q: %w", base, err)
	}

	query := parsed.Query()
	query.Set("model", modelURI)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func buildSessionUpdate(cfg Config) map[string]any {
	voice := cfg.Voice
	if voice == "" {
		voice = defaultVoice
	}
	role := cfg.VoiceRole
	if role == "" {
		role = defaultRole
	}
	silence := cfg.VADSilenceMS
	if silence <= 0 {
		silence = 400
	}
	threshold := cfg.VADThreshold
	if threshold <= 0 {
		threshold = 0.5
	}

	return map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"instructions":      cfg.Instructions,
			"output_modalities": []string{"audio"},
			"audio": map[string]any{
				"input": map[string]any{
					"format": map[string]any{
						"type": audioPCMType,
						"rate": cfg.InputRate,
					},
					"turn_detection": map[string]any{
						"type":                serverVADType,
						"threshold":           threshold,
						"silence_duration_ms": silence,
					},
					"languages": []string{"ru-RU"},
				},
				"output": map[string]any{
					"format": map[string]any{
						"type": audioPCMType,
						"rate": cfg.OutputRate,
					},
					"voice": voice,
					"role":  role,
				},
			},
		},
	}
}
