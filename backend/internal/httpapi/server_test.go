package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/sophygo/backend/internal/events"
	"github.com/sophygo/backend/internal/guide"
	"github.com/sophygo/backend/internal/realtime"
	"github.com/sophygo/backend/internal/telegram"
)

const (
	testBotToken  = "123456:test-token"
	allowedOrigin = "http://localhost:5173"
)

type testEnv struct {
	handler http.Handler
	store   *events.Store
	guide   *guide.Manager
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	return newTestEnvWithConfig(t, realtime.Config{}, 0)
}

func newTestEnvWithRealtime(t *testing.T, realtimeConfig realtime.Config) *testEnv {
	t.Helper()
	return newTestEnvWithConfig(t, realtimeConfig, 0)
}

func newTestEnvWithDevAuth(t *testing.T, devAuthUserID int64) *testEnv {
	t.Helper()
	return newTestEnvWithConfig(t, realtime.Config{}, devAuthUserID)
}

func newTestEnvWithConfig(t *testing.T, realtimeConfig realtime.Config, devAuthUserID int64) *testEnv {
	t.Helper()

	store, err := events.Open(filepath.Join(t.TempDir(), "events.jsonl"), 100)
	if err != nil {
		t.Fatalf("не удалось открыть хранилище: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	manager := guide.NewManager(time.Hour, time.Minute)
	t.Cleanup(manager.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(Deps{
		Store:         store,
		Guide:         manager,
		Messenger:     telegram.NewVerifier(testBotToken, 24*time.Hour),
		Realtime:      realtimeConfig,
		Logger:        logger,
		CORSOrigins:   []string{allowedOrigin},
		DevAuthUserID: devAuthUserID,
	})

	return &testEnv{
		handler: server.Routes(),
		store:   store,
		guide:   manager,
	}
}

func testUser(id int64) telegram.User {
	return telegram.User{
		ID:           id,
		FirstName:    "Владислав",
		Username:     "vlad",
		LanguageCode: "ru",
	}
}

func makeInitData(t *testing.T, botToken string, user telegram.User) string {
	t.Helper()

	rawUser, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("не удалось сериализовать user: %v", err)
	}

	values := url.Values{}
	values.Set("query_id", "AAHdF6IQAAAAAN0XohDhrOrc")
	values.Set("user", string(rawUser))
	values.Set("auth_date", strconv.FormatInt(time.Now().Unix(), 10))
	values.Set("hash", signInitData(botToken, dataCheckString(values)))

	return values.Encode()
}

func dataCheckString(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "hash" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for i, key := range keys {
		if i > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(values.Get(key))
	}
	return builder.String()
}

func signInitData(botToken, dataCheckString string) string {
	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	secretMAC.Write([]byte(botToken))
	secretKey := secretMAC.Sum(nil)

	dataMAC := hmac.New(sha256.New, secretKey)
	dataMAC.Write([]byte(dataCheckString))
	return hex.EncodeToString(dataMAC.Sum(nil))
}

func doJSON(t *testing.T, handler http.Handler, method, path, body, initData string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if initData != "" {
		request.Header.Set("X-Telegram-Init-Data", initData)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func validBody(sessionID string) string {
	payload := map[string]any{
		"place": map[string]any{
			"id":        "way/123",
			"name":      "Красная площадь",
			"category":  "historic_building",
			"latitude":  55.7539,
			"longitude": 37.6208,
		},
		"client": map[string]any{
			"sessionId": "client-session",
			"platform":  "browser",
			"userAgent": "Mozilla/5.0",
			"language":  "ru-RU",
		},
	}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}

	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestCreateSessionRequiresTelegram(t *testing.T) {
	env := newTestEnv(t)

	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/session", "", "")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидали %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestCreateSessionReturnsRealtimeToken(t *testing.T) {
	env := newTestEnv(t)
	initData := makeInitData(t, testBotToken, testUser(42))

	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/session", "", initData)
	if recorder.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидали %d; тело: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response createSessionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("некорректный JSON ответа: %v", err)
	}
	if response.SessionID == "" || response.RealtimeToken == "" {
		t.Fatalf("sessionId и realtimeToken должны быть заполнены: %+v", response)
	}
	if response.RealtimeAvailable {
		t.Error("realtimeAvailable должен быть false без кредов Yandex")
	}
	if response.User.ID != 42 {
		t.Errorf("user.id = %d, ожидали 42", response.User.ID)
	}
	if response.RealtimePath != "/api/v1/realtime" {
		t.Errorf("realtimePath = %q", response.RealtimePath)
	}

	second := doJSON(t, env.handler, http.MethodPost, "/api/v1/session", "", initData)
	var secondResponse createSessionResponse
	if err := json.Unmarshal(second.Body.Bytes(), &secondResponse); err != nil {
		t.Fatalf("некорректный JSON повторного ответа: %v", err)
	}
	if secondResponse.SessionID != response.SessionID {
		t.Errorf("повторный вызов создал новую сессию: %q != %q", secondResponse.SessionID, response.SessionID)
	}
}

func TestCreateSessionWithDevAuth(t *testing.T) {
	env := newTestEnvWithDevAuth(t, 77)

	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/session", "", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидали %d; тело: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response createSessionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("некорректный JSON ответа: %v", err)
	}
	if response.User.ID != 77 {
		t.Errorf("user.id = %d, ожидали 77", response.User.ID)
	}
	if response.SessionID == "" || response.RealtimeToken == "" {
		t.Fatalf("sessionId и realtimeToken должны быть заполнены: %+v", response)
	}

	// Некорректный initData не должен подменяться dev-пользователем.
	invalid := makeInitData(t, "999:other-token", testUser(42))
	rejected := doJSON(t, env.handler, http.MethodPost, "/api/v1/session", "", invalid)
	if rejected.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидали %d", rejected.Code, http.StatusUnauthorized)
	}
}

func TestCreateSessionRejectsInvalidSignature(t *testing.T) {
	env := newTestEnv(t)
	initData := makeInitData(t, "999:other-token", testUser(42))

	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/session", "", initData)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидали %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestPlaceOpenAnonymousStoresEvent(t *testing.T) {
	env := newTestEnv(t)

	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/events/place-open", validBody(""), "")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("статус = %d, ожидали %d; тело: %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}

	var response placeOpenResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("некорректный JSON ответа: %v", err)
	}
	if response.Delivered {
		t.Error("delivered должен быть false без Realtime-сессии")
	}

	recent := env.store.Recent(10)
	if len(recent) != 1 {
		t.Fatalf("сохранено %d событий, ожидали 1", len(recent))
	}
	if recent[0].Telegram != nil {
		t.Error("анонимное событие не должно содержать Telegram-пользователя")
	}
}

func TestPlaceOpenWithTelegramStoresUserAndSession(t *testing.T) {
	env := newTestEnv(t)
	user := testUser(42)
	info := env.guide.Ensure(user.ID, user.FirstName)
	initData := makeInitData(t, testBotToken, user)

	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/events/place-open", validBody(info.ID), initData)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("статус = %d, ожидали %d; тело: %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}

	recent := env.store.Recent(10)
	if len(recent) != 1 {
		t.Fatalf("сохранено %d событий, ожидали 1", len(recent))
	}
	event := recent[0]
	if event.Telegram == nil || event.Telegram.ID != user.ID {
		t.Fatalf("telegram user не сохранён: %+v", event.Telegram)
	}
	if event.SessionID != info.ID {
		t.Errorf("sessionId = %q, ожидали %q", event.SessionID, info.ID)
	}
}

func TestPlaceOpenDeliversPromptToRealtime(t *testing.T) {
	env := newTestEnv(t)
	user := testUser(42)
	info := env.guide.Ensure(user.ID, user.FirstName)
	sender := &fakeSender{}
	if _, err := env.guide.Attach(info.ID, sender); err != nil {
		t.Fatalf("Attach вернул ошибку: %v", err)
	}
	initData := makeInitData(t, testBotToken, user)

	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/events/place-open", validBody(info.ID), initData)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("статус = %d, ожидали %d; тело: %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}

	var response placeOpenResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("некорректный JSON ответа: %v", err)
	}
	if !response.Delivered {
		t.Error("delivered должен быть true при активной Realtime-сессии")
	}

	texts, _, _ := sender.snapshot()
	if len(texts) != 1 {
		t.Fatalf("отправлено %d промптов, ожидали 1", len(texts))
	}
	if !strings.Contains(texts[0], "Красная площадь") {
		t.Errorf("промпт не содержит название: %q", texts[0])
	}
	if !strings.Contains(texts[0], "55.75390") {
		t.Errorf("промпт не содержит координаты: %q", texts[0])
	}
}

func TestPlaceOpenRejectsForeignSession(t *testing.T) {
	env := newTestEnv(t)
	ownerSession := env.guide.Ensure(1, "User")
	initData := makeInitData(t, testBotToken, testUser(2))

	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/events/place-open", validBody(ownerSession.ID), initData)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидали %d", recorder.Code, http.StatusForbidden)
	}
	if len(env.store.Recent(10)) != 0 {
		t.Error("событие с чужой сессией не должно сохраняться")
	}
}

func TestPlaceOpenRejectsInvalidPayload(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "некорректный JSON", body: `{"place":`},
		{name: "нет объекта", body: `{"client": {"sessionId": "s1"}}`},
		{name: "широта вне диапазона", body: `{"place":{"id":"1","name":"Тест","latitude":120,"longitude":37}}`},
		{name: "лишние данные после JSON", body: validBody("") + ` {"extra":true}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := newTestEnv(t)

			recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/events/place-open", test.body, "")
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("статус = %d, ожидали %d; тело: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if len(env.store.Recent(10)) != 0 {
				t.Error("некорректное событие не должно сохраняться")
			}
		})
	}
}

func TestPlaceOpenRejectsOversizedBody(t *testing.T) {
	env := newTestEnv(t)

	body := `{"padding":"` + strings.Repeat("a", maxRequestBodyBytes+100) + `"}`
	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/events/place-open", body, "")

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("статус = %d, ожидали %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestStopSession(t *testing.T) {
	env := newTestEnv(t)
	user := testUser(42)
	info := env.guide.Ensure(user.ID, user.FirstName)
	sender := &fakeSender{}
	if _, err := env.guide.Attach(info.ID, sender); err != nil {
		t.Fatalf("Attach вернул ошибку: %v", err)
	}
	initData := makeInitData(t, testBotToken, user)

	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/session/"+info.ID+"/stop", "", initData)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("статус = %d, ожидали %d; тело: %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}

	_, cancels, _ := sender.snapshot()
	if cancels != 1 {
		t.Errorf("cancel вызван %d раз, ожидали 1", cancels)
	}
}

func TestStopSessionWithoutRealtime(t *testing.T) {
	env := newTestEnv(t)
	user := testUser(42)
	info := env.guide.Ensure(user.ID, user.FirstName)
	initData := makeInitData(t, testBotToken, user)

	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/session/"+info.ID+"/stop", "", initData)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("статус = %d, ожидали %d", recorder.Code, http.StatusConflict)
	}
}

func TestStopForeignSession(t *testing.T) {
	env := newTestEnv(t)
	info := env.guide.Ensure(1, "User")
	initData := makeInitData(t, testBotToken, testUser(2))

	recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/session/"+info.ID+"/stop", "", initData)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидали %d", recorder.Code, http.StatusForbidden)
	}
}

func TestHandleRecentEventsReturnsNewestFirst(t *testing.T) {
	env := newTestEnv(t)

	for _, id := range []string{"way/1", "way/2"} {
		body := strings.Replace(validBody(""), "way/123", id, 1)
		if recorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/events/place-open", body, ""); recorder.Code != http.StatusAccepted {
			t.Fatalf("не удалось отправить событие %q: статус %d", id, recorder.Code)
		}
	}

	recorder := doJSON(t, env.handler, http.MethodGet, "/api/v1/events?limit=1", "", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидали %d", recorder.Code, http.StatusOK)
	}

	var response recentEventsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("некорректный JSON ответа: %v", err)
	}
	if response.Count != 1 || len(response.Events) != 1 {
		t.Fatalf("count = %d, events = %d, ожидали по 1", response.Count, len(response.Events))
	}
	if response.Events[0].Place.ID != "way/2" {
		t.Errorf("вернулось событие %q, ожидали последнее way/2", response.Events[0].Place.ID)
	}
}

func TestHandleRecentEventsRejectsInvalidLimit(t *testing.T) {
	env := newTestEnv(t)

	recorder := doJSON(t, env.handler, http.MethodGet, "/api/v1/events?limit=abc", "", "")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("статус = %d, ожидали %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestHandleHealth(t *testing.T) {
	env := newTestEnv(t)

	recorder := doJSON(t, env.handler, http.MethodGet, "/healthz", "", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидали %d", recorder.Code, http.StatusOK)
	}

	var response healthResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("некорректный JSON ответа: %v", err)
	}
	if response.Status != "ok" {
		t.Errorf("status = %q, ожидали %q", response.Status, "ok")
	}
}

func TestCORSPreflightForAllowedOrigin(t *testing.T) {
	env := newTestEnv(t)

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/events/place-open", nil)
	request.Header.Set("Origin", allowedOrigin)
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	recorder := httptest.NewRecorder()
	env.handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("статус = %d, ожидали %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, ожидали %q", got, allowedOrigin)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Telegram-Init-Data") {
		t.Errorf("Access-Control-Allow-Headers = %q, ожидали X-Telegram-Init-Data", got)
	}
}

func TestCORSDoesNotAllowUnknownOrigin(t *testing.T) {
	env := newTestEnv(t)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/events/place-open", strings.NewReader(validBody("")))
	request.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()
	env.handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, ожидали пустое значение", got)
	}
}

func TestRealtimeRequiresConfiguration(t *testing.T) {
	env := newTestEnv(t)
	info := env.guide.Ensure(42, "User")
	token, err := env.guide.IssueToken(info.ID)
	if err != nil {
		t.Fatalf("IssueToken вернул ошибку: %v", err)
	}

	recorder := doJSON(t, env.handler, http.MethodGet, "/api/v1/realtime?token="+url.QueryEscape(token), "", "")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("статус = %d, ожидали %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestRealtimeProxy(t *testing.T) {
	upstreamMessages := make(chan string, 4)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, sessionUpdate, err := conn.Read(ctx)
		if err != nil {
			return
		}
		upstreamMessages <- string(sessionUpdate)

		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"response.output_text.delta","delta":"Привет"}`)); err != nil {
			return
		}

		_, audio, err := conn.Read(ctx)
		if err != nil {
			return
		}
		upstreamMessages <- string(audio)
	}))
	defer upstream.Close()

	env := newTestEnvWithRealtime(t, realtime.Config{
		URL:          "ws" + strings.TrimPrefix(upstream.URL, "http"),
		ModelURI:     "gpt://folder/speech-realtime-250923",
		APIKey:       "key",
		Instructions: "Ты гид",
		InputRate:    16000,
		OutputRate:   16000,
	})

	apiServer := httptest.NewServer(env.handler)
	defer apiServer.Close()

	initData := makeInitData(t, testBotToken, testUser(42))
	sessionRecorder := doJSON(t, env.handler, http.MethodPost, "/api/v1/session", "", initData)
	if sessionRecorder.Code != http.StatusOK {
		t.Fatalf("не удалось создать сессию: статус %d, тело %s", sessionRecorder.Code, sessionRecorder.Body.String())
	}

	var session createSessionResponse
	if err := json.Unmarshal(sessionRecorder.Body.Bytes(), &session); err != nil {
		t.Fatalf("некорректный JSON сессии: %v", err)
	}

	wsURL := "ws" + strings.TrimPrefix(apiServer.URL, "http") +
		"/api/v1/realtime?token=" + url.QueryEscape(session.RealtimeToken)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	browser, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("не удалось подключиться к WebSocket: %v", err)
	}
	defer browser.CloseNow()

	_, delta, err := browser.Read(ctx)
	if err != nil {
		t.Fatalf("не удалось прочитать событие от Гида: %v", err)
	}
	if !strings.Contains(string(delta), "response.output_text.delta") {
		t.Errorf("событие от Гида = %s", delta)
	}

	// Попытка изменить конфигурацию сессии должна игнорироваться прокси.
	if err := browser.Write(ctx, websocket.MessageText, []byte(`{"type":"session.update","session":{"instructions":"взлом"}}`)); err != nil {
		t.Fatalf("не удалось отправить session.update: %v", err)
	}

	appendEvent := `{"type":"input_audio_buffer.append","audio":"AQID"}`
	if err := browser.Write(ctx, websocket.MessageText, []byte(appendEvent)); err != nil {
		t.Fatalf("не удалось отправить аудио: %v", err)
	}

	first := <-upstreamMessages
	if !strings.Contains(first, "session.update") {
		t.Errorf("первое событие upstream = %s, ожидали session.update", first)
	}

	second := <-upstreamMessages
	if second != appendEvent {
		t.Errorf("upstream получил %s, ожидали %s", second, appendEvent)
	}
}

type fakeSender struct {
	mu      sync.Mutex
	texts   []string
	cancels int
	closed  bool
}

func (f *fakeSender) SendUserText(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.texts = append(f.texts, text)
	return nil
}

func (f *fakeSender) CancelResponse(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels++
	return nil
}

func (f *fakeSender) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeSender) snapshot() (texts []string, cancels int, closed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.texts...), f.cancels, f.closed
}
