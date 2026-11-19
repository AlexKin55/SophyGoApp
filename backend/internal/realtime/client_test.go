package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestDialSendsSessionUpdate(t *testing.T) {
	authHeader := make(chan string, 1)
	modelParam := make(chan string, 1)
	sessionUpdate := make(chan map[string]any, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader <- r.Header.Get("Authorization")
		modelParam <- r.URL.Query().Get("model")

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var message map[string]any
		if err := json.Unmarshal(data, &message); err != nil {
			return
		}
		sessionUpdate <- message
	}))
	defer server.Close()

	cfg := Config{
		URL:          "ws" + strings.TrimPrefix(server.URL, "http"),
		ModelURI:     "gpt://folder/speech-realtime-250923",
		APIKey:       "secret-key",
		Instructions: "Ты гид",
		InputRate:    16000,
		OutputRate:   16000,
		Voice:        "alena",
		VoiceRole:    "good",
		VADThreshold: 0.5,
		VADSilenceMS: 400,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, cfg)
	if err != nil {
		t.Fatalf("Dial вернул ошибку: %v", err)
	}
	defer conn.CloseNow()

	if got := <-authHeader; got != "Api-Key secret-key" {
		t.Errorf("Authorization = %q, ожидали %q", got, "Api-Key secret-key")
	}
	if got := <-modelParam; got != cfg.ModelURI {
		t.Errorf("model = %q, ожидали %q", got, cfg.ModelURI)
	}

	message := <-sessionUpdate
	if message["type"] != "session.update" {
		t.Fatalf("первое событие type = %v, ожидали session.update", message["type"])
	}

	session, ok := message["session"].(map[string]any)
	if !ok {
		t.Fatalf("session отсутствует в session.update: %v", message)
	}
	if session["instructions"] != cfg.Instructions {
		t.Errorf("instructions = %v, ожидали %q", session["instructions"], cfg.Instructions)
	}

	audio, ok := session["audio"].(map[string]any)
	if !ok {
		t.Fatalf("audio отсутствует в session.update: %v", session)
	}
	input := audio["input"].(map[string]any)
	format := input["format"].(map[string]any)
	if format["rate"] != float64(16000) {
		t.Errorf("input rate = %v, ожидали 16000", format["rate"])
	}
	turnDetection := input["turn_detection"].(map[string]any)
	if turnDetection["type"] != "server_vad" {
		t.Errorf("turn_detection.type = %v, ожидали server_vad", turnDetection["type"])
	}
	output := audio["output"].(map[string]any)
	if output["voice"] != "alena" {
		t.Errorf("voice = %v, ожидали alena", output["voice"])
	}
}

func TestSendAudioUserTextAndCancel(t *testing.T) {
	messages := make(chan map[string]any, 8)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Сообщаем клиенту, что ответ активен: только тогда CancelResponse
		// отправит response.cancel.
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"response.created"}`)); err != nil {
			return
		}

		for i := 0; i < 5; i++ {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var message map[string]any
			if err := json.Unmarshal(data, &message); err != nil {
				return
			}
			messages <- message
		}
	}))
	defer server.Close()

	cfg := Config{
		URL:          "ws" + strings.TrimPrefix(server.URL, "http"),
		ModelURI:     "gpt://folder/model",
		APIKey:       "key",
		Instructions: "prompt",
		InputRate:    16000,
		OutputRate:   16000,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, cfg)
	if err != nil {
		t.Fatalf("Dial вернул ошибку: %v", err)
	}
	defer conn.CloseNow()

	if _, _, err := conn.Read(ctx); err != nil {
		t.Fatalf("Read вернул ошибку: %v", err)
	}

	pcm := []byte{0, 1, 2, 3}
	if err := conn.SendAudio(ctx, pcm); err != nil {
		t.Fatalf("SendAudio вернул ошибку: %v", err)
	}
	if err := conn.SendUserText(ctx, "Расскажи о Кремле"); err != nil {
		t.Fatalf("SendUserText вернул ошибку: %v", err)
	}
	if err := conn.CancelResponse(ctx); err != nil {
		t.Fatalf("CancelResponse вернул ошибку: %v", err)
	}

	first := <-messages // session.update
	if first["type"] != "session.update" {
		t.Fatalf("первое событие = %v, ожидали session.update", first["type"])
	}

	audioEvent := <-messages
	if audioEvent["type"] != "input_audio_buffer.append" {
		t.Fatalf("событие = %v, ожидали input_audio_buffer.append", audioEvent["type"])
	}
	encoded, _ := audioEvent["audio"].(string)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("audio не base64: %v", err)
	}
	if string(decoded) != string(pcm) {
		t.Errorf("audio = %v, ожидали %v", decoded, pcm)
	}

	itemEvent := <-messages
	if itemEvent["type"] != "conversation.item.create" {
		t.Fatalf("событие = %v, ожидали conversation.item.create", itemEvent["type"])
	}
	item := itemEvent["item"].(map[string]any)
	content := item["content"].([]any)
	text := content[0].(map[string]any)["text"]
	if text != "Расскажи о Кремле" {
		t.Errorf("text = %v, ожидали %q", text, "Расскажи о Кремле")
	}

	responseEvent := <-messages
	if responseEvent["type"] != "response.create" {
		t.Fatalf("событие = %v, ожидали response.create", responseEvent["type"])
	}

	cancelEvent := <-messages
	if cancelEvent["type"] != "response.cancel" {
		t.Fatalf("событие = %v, ожидали response.cancel", cancelEvent["type"])
	}
}

func TestDialReportsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Dial(ctx, Config{
		URL:      "ws" + strings.TrimPrefix(server.URL, "http"),
		ModelURI: "gpt://folder/model",
		APIKey:   "key",
	})
	if err == nil {
		t.Fatal("ожидали ошибку подключения")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("ошибка = %v, ожидали упоминание HTTP 401", err)
	}
}
