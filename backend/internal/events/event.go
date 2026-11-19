// Package events описывает события, которые присылает frontend, и их хранение.
package events

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// TypePlaceOpen — событие «пользователь открыл карточку исторического объекта».
const TypePlaceOpen = "place_open"

const (
	maxPlaceIDLen   = 128
	maxNameLen      = 300
	maxCategoryLen  = 64
	maxSessionIDLen = 128
	maxUserAgentLen = 512
	maxLanguageLen  = 32
	maxPlatformLen  = 32
	maxClockSkew    = 24 * time.Hour
	maxEventAge     = 30 * 24 * time.Hour
	maxRemoteIPLen  = 64
)

// Limits — ограничения валидации событий. Нулевые значения заменяются
// значениями по умолчанию, поэтому структуру можно заполнять частично.
type Limits struct {
	MaxPlaceIDLen   int
	MaxNameLen      int
	MaxCategoryLen  int
	MaxSessionIDLen int
	MaxUserAgentLen int
	MaxLanguageLen  int
	MaxPlatformLen  int
	MaxRemoteIPLen  int
	MaxClockSkew    time.Duration
	MaxAge          time.Duration
}

// DefaultLimits возвращает ограничения по умолчанию.
func DefaultLimits() Limits {
	return Limits{
		MaxPlaceIDLen:   maxPlaceIDLen,
		MaxNameLen:      maxNameLen,
		MaxCategoryLen:  maxCategoryLen,
		MaxSessionIDLen: maxSessionIDLen,
		MaxUserAgentLen: maxUserAgentLen,
		MaxLanguageLen:  maxLanguageLen,
		MaxPlatformLen:  maxPlatformLen,
		MaxRemoteIPLen:  maxRemoteIPLen,
		MaxClockSkew:    maxClockSkew,
		MaxAge:          maxEventAge,
	}
}

// WithDefaults возвращает копию ограничений, где нулевые поля заменены
// значениями по умолчанию.
func (l Limits) WithDefaults() Limits {
	defaults := DefaultLimits()
	if l.MaxPlaceIDLen <= 0 {
		l.MaxPlaceIDLen = defaults.MaxPlaceIDLen
	}
	if l.MaxNameLen <= 0 {
		l.MaxNameLen = defaults.MaxNameLen
	}
	if l.MaxCategoryLen <= 0 {
		l.MaxCategoryLen = defaults.MaxCategoryLen
	}
	if l.MaxSessionIDLen <= 0 {
		l.MaxSessionIDLen = defaults.MaxSessionIDLen
	}
	if l.MaxUserAgentLen <= 0 {
		l.MaxUserAgentLen = defaults.MaxUserAgentLen
	}
	if l.MaxLanguageLen <= 0 {
		l.MaxLanguageLen = defaults.MaxLanguageLen
	}
	if l.MaxPlatformLen <= 0 {
		l.MaxPlatformLen = defaults.MaxPlatformLen
	}
	if l.MaxRemoteIPLen <= 0 {
		l.MaxRemoteIPLen = defaults.MaxRemoteIPLen
	}
	if l.MaxClockSkew <= 0 {
		l.MaxClockSkew = defaults.MaxClockSkew
	}
	if l.MaxAge <= 0 {
		l.MaxAge = defaults.MaxAge
	}
	return l
}

// Place — краткая информация об объекте, на который нажал пользователь.
type Place struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Category  string  `json:"category"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Client — сведения о клиенте, отправившем событие.
type Client struct {
	SessionID string `json:"sessionId,omitempty"`
	Platform  string `json:"platform,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	Language  string `json:"language,omitempty"`
}

// TelegramUser — проверенные данные пользователя Telegram Mini App.
type TelegramUser struct {
	ID           int64  `json:"id"`
	Username     string `json:"username,omitempty"`
	FirstName    string `json:"firstName,omitempty"`
	LastName     string `json:"lastName,omitempty"`
	LanguageCode string `json:"languageCode,omitempty"`
}

// PlaceOpenRequest — тело запроса POST /api/v1/events/place-open.
type PlaceOpenRequest struct {
	Place      Place      `json:"place"`
	Client     Client     `json:"client"`
	SessionID  string     `json:"sessionId,omitempty"`
	OccurredAt *time.Time `json:"occurredAt,omitempty"`
}

// Event — сохранённое событие: данные запроса плюс серверные поля.
type Event struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Place      Place         `json:"place"`
	Client     Client        `json:"client"`
	SessionID  string        `json:"sessionId,omitempty"`
	Telegram   *TelegramUser `json:"telegram,omitempty"`
	OccurredAt time.Time     `json:"occurredAt"`
	ReceivedAt time.Time     `json:"receivedAt"`
	RemoteIP   string        `json:"remoteIp,omitempty"`
}

// ValidationError описывает некорректное поле запроса.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate нормализует строковые поля и проверяет запрос с ограничениями по умолчанию.
func (r *PlaceOpenRequest) Validate(now time.Time) error {
	return r.ValidateWithLimits(now, DefaultLimits())
}

// ValidateWithLimits нормализует строковые поля и проверяет запрос.
func (r *PlaceOpenRequest) ValidateWithLimits(now time.Time, limits Limits) error {
	limits = limits.WithDefaults()

	r.Place.ID = strings.TrimSpace(r.Place.ID)
	r.Place.Name = strings.TrimSpace(r.Place.Name)
	r.Place.Category = strings.TrimSpace(r.Place.Category)
	r.Client.SessionID = strings.TrimSpace(r.Client.SessionID)
	r.Client.Platform = strings.TrimSpace(r.Client.Platform)
	r.Client.UserAgent = strings.TrimSpace(r.Client.UserAgent)
	r.Client.Language = strings.TrimSpace(r.Client.Language)
	r.SessionID = strings.TrimSpace(r.SessionID)

	switch {
	case r.Place.ID == "":
		return &ValidationError{Field: "place.id", Message: "обязательное поле"}
	case len(r.Place.ID) > limits.MaxPlaceIDLen:
		return &ValidationError{Field: "place.id", Message: fmt.Sprintf("длина не должна превышать %d символов", limits.MaxPlaceIDLen)}
	case r.Place.Name == "":
		return &ValidationError{Field: "place.name", Message: "обязательное поле"}
	case len(r.Place.Name) > limits.MaxNameLen:
		return &ValidationError{Field: "place.name", Message: fmt.Sprintf("длина не должна превышать %d символов", limits.MaxNameLen)}
	case len(r.Place.Category) > limits.MaxCategoryLen:
		return &ValidationError{Field: "place.category", Message: fmt.Sprintf("длина не должна превышать %d символов", limits.MaxCategoryLen)}
	case r.Place.Latitude < -90 || r.Place.Latitude > 90:
		return &ValidationError{Field: "place.latitude", Message: "должна быть в диапазоне [-90, 90]"}
	case r.Place.Longitude < -180 || r.Place.Longitude > 180:
		return &ValidationError{Field: "place.longitude", Message: "должна быть в диапазоне [-180, 180]"}
	case len(r.Client.SessionID) > limits.MaxSessionIDLen:
		return &ValidationError{Field: "client.sessionId", Message: fmt.Sprintf("длина не должна превышать %d символов", limits.MaxSessionIDLen)}
	case len(r.Client.Platform) > limits.MaxPlatformLen:
		return &ValidationError{Field: "client.platform", Message: fmt.Sprintf("длина не должна превышать %d символов", limits.MaxPlatformLen)}
	case len(r.Client.UserAgent) > limits.MaxUserAgentLen:
		return &ValidationError{Field: "client.userAgent", Message: fmt.Sprintf("длина не должна превышать %d символов", limits.MaxUserAgentLen)}
	case len(r.Client.Language) > limits.MaxLanguageLen:
		return &ValidationError{Field: "client.language", Message: fmt.Sprintf("длина не должна превышать %d символов", limits.MaxLanguageLen)}
	case len(r.SessionID) > limits.MaxPlaceIDLen:
		return &ValidationError{Field: "sessionId", Message: fmt.Sprintf("длина не должна превышать %d символов", limits.MaxPlaceIDLen)}
	}

	if r.OccurredAt != nil {
		occurredAt := r.OccurredAt.UTC()
		if occurredAt.After(now.Add(limits.MaxClockSkew)) {
			return &ValidationError{Field: "occurredAt", Message: "время события слишком далеко в будущем"}
		}
		if occurredAt.Before(now.Add(-limits.MaxAge)) {
			return &ValidationError{Field: "occurredAt", Message: "время события слишком давно"}
		}
	}

	return nil
}

// NewEvent создаёт событие на основе проверенного запроса с ограничениями по умолчанию.
func NewEvent(request PlaceOpenRequest, now time.Time, remoteIP string) Event {
	return NewEventWithLimits(request, now, remoteIP, DefaultLimits())
}

// NewEventWithLimits создаёт событие на основе проверенного запроса.
func NewEventWithLimits(request PlaceOpenRequest, now time.Time, remoteIP string, limits Limits) Event {
	limits = limits.WithDefaults()

	occurredAt := now
	if request.OccurredAt != nil {
		occurredAt = request.OccurredAt.UTC()
	}

	if len(remoteIP) > limits.MaxRemoteIPLen {
		remoteIP = remoteIP[:limits.MaxRemoteIPLen]
	}

	return Event{
		ID:         NewID(),
		Type:       TypePlaceOpen,
		Place:      request.Place,
		Client:     request.Client,
		OccurredAt: occurredAt,
		ReceivedAt: now,
		RemoteIP:   remoteIP,
	}
}

// NewID генерирует идентификатор события.
func NewID() string {
	return "evt_" + rand.Text()
}
