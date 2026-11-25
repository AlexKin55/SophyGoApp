package guide

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

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

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	manager := NewManager(time.Hour, time.Minute)
	t.Cleanup(manager.Close)
	return manager
}

func TestEnsureReusesSessionForSameUser(t *testing.T) {
	manager := newTestManager(t)

	first := manager.Ensure(42, "Влад")
	second := manager.Ensure(42, "Влад")

	if first.ID != second.ID {
		t.Errorf("id сессий различаются: %q и %q", first.ID, second.ID)
	}

	other := manager.Ensure(43, "Иван")
	if other.ID == first.ID {
		t.Error("разные пользователи получили одну сессию")
	}
}

func TestIssueAndConsumeTokenIsOneTime(t *testing.T) {
	manager := newTestManager(t)
	info := manager.Ensure(42, "Влад")

	token, err := manager.IssueToken(info.ID)
	if err != nil {
		t.Fatalf("IssueToken вернул ошибку: %v", err)
	}

	sessionID, err := manager.ConsumeToken(token)
	if err != nil {
		t.Fatalf("ConsumeToken вернул ошибку: %v", err)
	}
	if sessionID != info.ID {
		t.Errorf("sessionID = %q, ожидали %q", sessionID, info.ID)
	}

	if _, err := manager.ConsumeToken(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("повторное использование токена: ожидали ErrInvalidToken, получили %v", err)
	}
}

func TestConsumeTokenRejectsExpiredToken(t *testing.T) {
	manager := newTestManager(t)
	base := time.Now()
	manager.now = func() time.Time { return base }

	info := manager.Ensure(42, "Влад")
	token, err := manager.IssueToken(info.ID)
	if err != nil {
		t.Fatalf("IssueToken вернул ошибку: %v", err)
	}

	manager.now = func() time.Time { return base.Add(2 * time.Minute) }

	if _, err := manager.ConsumeToken(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ожидали ErrInvalidToken, получили %v", err)
	}
}

func TestSendPlacePromptAndStop(t *testing.T) {
	manager := newTestManager(t)
	info := manager.Ensure(42, "Влад")
	sender := &fakeSender{}

	generation, err := manager.Attach(info.ID, sender)
	if err != nil {
		t.Fatalf("Attach вернул ошибку: %v", err)
	}

	err = manager.SendPlacePrompt(context.Background(), info.ID, "Красная площадь", 55.7539, 37.6208)
	if err != nil {
		t.Fatalf("SendPlacePrompt вернул ошибку: %v", err)
	}

	texts, cancels, _ := sender.snapshot()
	if len(texts) != 1 {
		t.Fatalf("отправлено %d сообщений, ожидали 1", len(texts))
	}
	if !strings.Contains(texts[0], "Красная площадь") {
		t.Errorf("промпт не содержит название объекта: %q", texts[0])
	}
	if !strings.Contains(texts[0], "55.75390") || !strings.Contains(texts[0], "37.62080") {
		t.Errorf("промпт не содержит координаты: %q", texts[0])
	}
	if cancels != 1 {
		t.Errorf("перед новым промптом должен отменяться текущий ответ: cancel=%d", cancels)
	}

	if err := manager.Stop(context.Background(), info.ID); err != nil {
		t.Fatalf("Stop вернул ошибку: %v", err)
	}
	_, cancels, _ = sender.snapshot()
	if cancels != 2 {
		t.Errorf("cancel вызван %d раз, ожидали 2", cancels)
	}

	manager.Detach(info.ID, generation)
	if err := manager.SendPlacePrompt(context.Background(), info.ID, "Кремль", 55.75, 37.61); !errors.Is(err, ErrNoRealtime) {
		t.Fatalf("после Detach ожидали ErrNoRealtime, получили %v", err)
	}
}

func TestAttachReplacesPreviousConnection(t *testing.T) {
	manager := newTestManager(t)
	info := manager.Ensure(42, "Влад")
	first := &fakeSender{}
	second := &fakeSender{}

	firstGeneration, err := manager.Attach(info.ID, first)
	if err != nil {
		t.Fatalf("Attach вернул ошибку: %v", err)
	}
	secondGeneration, err := manager.Attach(info.ID, second)
	if err != nil {
		t.Fatalf("повторный Attach вернул ошибку: %v", err)
	}

	if _, _, closed := first.snapshot(); !closed {
		t.Error("предыдущее соединение должно быть закрыто")
	}

	manager.Detach(info.ID, firstGeneration)
	if err := manager.SendPlacePrompt(context.Background(), info.ID, "Кремль", 55.75, 37.61); err != nil {
		t.Fatalf("старое поколение не должно отключать новое соединение: %v", err)
	}

	manager.Detach(info.ID, secondGeneration)
	if err := manager.SendPlacePrompt(context.Background(), info.ID, "Кремль", 55.75, 37.61); !errors.Is(err, ErrNoRealtime) {
		t.Fatalf("ожидали ErrNoRealtime, получили %v", err)
	}
}

func TestCleanupRemovesExpiredSessions(t *testing.T) {
	manager := newTestManager(t)
	base := time.Now()
	manager.now = func() time.Time { return base }

	info := manager.Ensure(42, "Влад")
	sender := &fakeSender{}
	if _, err := manager.Attach(info.ID, sender); err != nil {
		t.Fatalf("Attach вернул ошибку: %v", err)
	}

	manager.now = func() time.Time { return base.Add(2*time.Hour + time.Minute) }
	manager.cleanup()

	if _, ok := manager.Get(info.ID); ok {
		t.Error("истёкшая сессия должна быть удалена")
	}
	if _, _, closed := sender.snapshot(); !closed {
		t.Error("соединение истёкшей сессии должно быть закрыто")
	}
}

func TestUnknownSessionReturnsErrors(t *testing.T) {
	manager := newTestManager(t)

	if err := manager.SendPlacePrompt(context.Background(), "missing", "Кремль", 55.75, 37.61); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("ожидали ErrSessionNotFound, получили %v", err)
	}
	if _, err := manager.IssueToken("missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("ожидали ErrSessionNotFound, получили %v", err)
	}
}

func TestSendGreetingOnce(t *testing.T) {
	manager := newTestManager(t)
	info := manager.Ensure(42, "Алексей")
	sender := &fakeSender{}

	if _, err := manager.Attach(info.ID, sender); err != nil {
		t.Fatalf("Attach вернул ошибку: %v", err)
	}

	sent, err := manager.SendGreeting(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("SendGreeting вернул ошибку: %v", err)
	}
	if !sent {
		t.Fatal("первое приветствие должно быть отправлено")
	}

	sent, err = manager.SendGreeting(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("повторный SendGreeting вернул ошибку: %v", err)
	}
	if sent {
		t.Fatal("повторное приветствие не должно отправляться")
	}

	texts, _, _ := sender.snapshot()
	if len(texts) != 1 {
		t.Fatalf("отправлено %d сообщений, ожидали 1", len(texts))
	}
	if !strings.Contains(texts[0], "Алексей") {
		t.Fatalf("приветствие не содержит имя пользователя: %q", texts[0])
	}
}
