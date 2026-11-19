// Package guide управляет сессиями Гида и связывает их с Realtime-соединениями.
package guide

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrSessionNotFound — сессия не найдена или истекла.
	ErrSessionNotFound = errors.New("guide: сессия не найдена")
	// ErrNoRealtime — к сессии не подключено Realtime-соединение.
	ErrNoRealtime = errors.New("guide: Realtime-соединение не установлено")
	// ErrInvalidToken — одноразовый токен недействителен.
	ErrInvalidToken = errors.New("guide: недействительный токен")
)

// RealtimeSender — минимальный интерфейс Realtime-соединения.
type RealtimeSender interface {
	SendUserText(ctx context.Context, text string) error
	CancelResponse(ctx context.Context) error
	Close() error
}

// SessionInfo — публичные данные сессии.
type SessionInfo struct {
	ID             string
	TelegramUserID int64
	CreatedAt      time.Time
	ExpiresAt      time.Time
	Realtime       bool
}

type session struct {
	info       SessionInfo
	sender     RealtimeSender
	generation uint64
}

type tokenEntry struct {
	sessionID string
	expiresAt time.Time
}

// Manager хранит сессии в памяти и очищает истёкшие.
type Manager struct {
	mu              sync.Mutex
	sessions        map[string]*session
	byUser          map[int64]string
	tokens          map[string]tokenEntry
	ttl             time.Duration
	tokenTTL        time.Duration
	cleanupInterval time.Duration
	now             func() time.Time
	stop            chan struct{}
	done            chan struct{}
	closeOnce       sync.Once
}

// NewManager создаёт менеджер сессий и запускает фоновую очистку.
func NewManager(ttl, tokenTTL time.Duration) *Manager {
	return NewManagerWithCleanup(ttl, tokenTTL, 0)
}

// NewManagerWithCleanup создаёт менеджер с явно заданным интервалом очистки.
// Нулевой интервал означает автоматический расчёт от TTL сессии.
func NewManagerWithCleanup(ttl, tokenTTL, cleanupInterval time.Duration) *Manager {
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	if tokenTTL <= 0 {
		tokenTTL = time.Minute
	}
	if cleanupInterval <= 0 {
		cleanupInterval = ttl / 2
		if cleanupInterval > time.Minute {
			cleanupInterval = time.Minute
		}
		if cleanupInterval < time.Second {
			cleanupInterval = time.Second
		}
	}

	manager := &Manager{
		sessions:        make(map[string]*session),
		byUser:          make(map[int64]string),
		tokens:          make(map[string]tokenEntry),
		ttl:             ttl,
		tokenTTL:        tokenTTL,
		cleanupInterval: cleanupInterval,
		now:             time.Now,
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
	}

	go manager.cleanupLoop()
	return manager
}

// Ensure возвращает существующую сессию пользователя или создаёт новую.
func (m *Manager) Ensure(telegramUserID int64) SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()

	if id, ok := m.byUser[telegramUserID]; ok {
		if existing, exists := m.sessions[id]; exists && now.Before(existing.info.ExpiresAt) {
			existing.info.ExpiresAt = now.Add(m.ttl)
			return existing.info
		}
		delete(m.sessions, id)
		delete(m.byUser, telegramUserID)
	}

	info := SessionInfo{
		ID:             "sess_" + rand.Text(),
		TelegramUserID: telegramUserID,
		CreatedAt:      now,
		ExpiresAt:      now.Add(m.ttl),
	}
	m.sessions[info.ID] = &session{info: info}
	m.byUser[telegramUserID] = info.ID
	return info
}

// Get возвращает сессию по id и продлевает её жизнь.
func (m *Manager) Get(sessionID string) (SessionInfo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.sessions[sessionID]
	if !ok {
		return SessionInfo{}, false
	}

	now := m.now()
	if !now.Before(existing.info.ExpiresAt) {
		delete(m.sessions, sessionID)
		if m.byUser[existing.info.TelegramUserID] == sessionID {
			delete(m.byUser, existing.info.TelegramUserID)
		}
		return SessionInfo{}, false
	}

	existing.info.ExpiresAt = now.Add(m.ttl)
	return existing.info, true
}

// IssueToken выдаёт одноразовый токен для подключения WebSocket.
func (m *Manager) IssueToken(sessionID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[sessionID]; !ok {
		return "", ErrSessionNotFound
	}

	token := "rt_" + rand.Text()
	m.tokens[token] = tokenEntry{
		sessionID: sessionID,
		expiresAt: m.now().Add(m.tokenTTL),
	}
	return token, nil
}

// ConsumeToken проверяет и погашает одноразовый токен.
func (m *Manager) ConsumeToken(token string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.tokens[token]
	if !ok {
		return "", ErrInvalidToken
	}
	delete(m.tokens, token)

	if !m.now().Before(entry.expiresAt) {
		return "", ErrInvalidToken
	}
	if _, exists := m.sessions[entry.sessionID]; !exists {
		return "", ErrSessionNotFound
	}
	return entry.sessionID, nil
}

// Attach подключает Realtime-соединение к сессии и возвращает поколение.
func (m *Manager) Attach(sessionID string, sender RealtimeSender) (uint64, error) {
	m.mu.Lock()
	existing, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return 0, ErrSessionNotFound
	}
	previous := existing.sender
	existing.generation++
	generation := existing.generation
	existing.sender = sender
	existing.info.Realtime = true
	m.mu.Unlock()

	if previous != nil {
		_ = previous.Close()
	}
	return generation, nil
}

// Detach отключает Realtime-соединение, если оно всё ещё актуально.
func (m *Manager) Detach(sessionID string, generation uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.sessions[sessionID]
	if !ok || existing.generation != generation {
		return
	}
	existing.sender = nil
	existing.info.Realtime = false
}

// SendPlacePrompt просит Гида рассказать об объекте.
//
// Перед новым промптом текущий ответ отменяется: при выборе другого объекта
// Гид должен прервать старый рассказ и сразу начать новый.
func (m *Manager) SendPlacePrompt(ctx context.Context, sessionID, placeName string, latitude, longitude float64) error {
	sender, err := m.sender(sessionID)
	if err != nil {
		return err
	}

	// Активного ответа может не быть — ошибку отмены игнорируем.
	_ = sender.CancelResponse(ctx)

	prompt := fmt.Sprintf(
		"Расскажи об этом историческом объекте: %s. Координаты: %.5f, %.5f.",
		placeName, latitude, longitude,
	)
	return sender.SendUserText(ctx, prompt)
}

// Stop прерывает текущий ответ Гида.
func (m *Manager) Stop(ctx context.Context, sessionID string) error {
	sender, err := m.sender(sessionID)
	if err != nil {
		return err
	}
	return sender.CancelResponse(ctx)
}

// Close останавливает очистку и закрывает все Realtime-соединения.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		close(m.stop)
		<-m.done

		m.mu.Lock()
		senders := make([]RealtimeSender, 0, len(m.sessions))
		for _, existing := range m.sessions {
			if existing.sender != nil {
				senders = append(senders, existing.sender)
				existing.sender = nil
			}
		}
		m.sessions = make(map[string]*session)
		m.byUser = make(map[int64]string)
		m.tokens = make(map[string]tokenEntry)
		m.mu.Unlock()

		for _, sender := range senders {
			_ = sender.Close()
		}
	})
}

func (m *Manager) sender(sessionID string) (RealtimeSender, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if existing.sender == nil {
		return nil, ErrNoRealtime
	}
	return existing.sender, nil
}

func (m *Manager) cleanupLoop() {
	defer close(m.done)

	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

func (m *Manager) cleanup() {
	now := m.now()

	m.mu.Lock()
	var expiredSenders []RealtimeSender
	for id, existing := range m.sessions {
		if now.Before(existing.info.ExpiresAt) {
			continue
		}
		delete(m.sessions, id)
		if m.byUser[existing.info.TelegramUserID] == id {
			delete(m.byUser, existing.info.TelegramUserID)
		}
		if existing.sender != nil {
			expiredSenders = append(expiredSenders, existing.sender)
			existing.sender = nil
		}
	}
	for token, entry := range m.tokens {
		if !now.Before(entry.expiresAt) {
			delete(m.tokens, token)
		}
	}
	m.mu.Unlock()

	for _, sender := range expiredSenders {
		_ = sender.Close()
	}
}
