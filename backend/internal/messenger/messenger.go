// Package messenger описывает единый интерфейс мессенджеров,
// в которых запускается мини-приложение SophyGo.
//
// Сейчас поддерживаются Telegram и MAX. Оба передают подписанные
// стартовые параметры (initData) и проверяются по одной схеме
// HMAC-SHA256, поэтому HTTP-слою достаточно интерфейса Verifier.
package messenger

import (
	"errors"
	"time"
)

var (
	// ErrNotConfigured — сервер не знает токен бота и не может проверить подпись.
	ErrNotConfigured = errors.New("messenger: токен бота не задан")
	// ErrMissing — initData не передан.
	ErrMissing = errors.New("messenger: initData не передан")
	// ErrMalformed — initData имеет неверный формат.
	ErrMalformed = errors.New("messenger: некорректный initData")
	// ErrInvalidSignature — подпись не совпадает.
	ErrInvalidSignature = errors.New("messenger: неверная подпись initData")
	// ErrExpired — initData устарел.
	ErrExpired = errors.New("messenger: initData устарел")
)

// User — данные пользователя из initData.
// Теги нужны, чтобы Telegram-провайдер разбирал JSON без отдельной структуры.
type User struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Username     string `json:"username"`
	LanguageCode string `json:"language_code"`
	IsPremium    bool   `json:"is_premium"`
}

// InitData — проверенные стартовые параметры мини-приложения.
type InitData struct {
	User     User
	AuthDate time.Time
	QueryID  string
}

// Verifier проверяет подпись initData конкретного мессенджера.
type Verifier interface {
	// Name возвращает имя мессенджера: telegram или max.
	Name() string
	// Configured сообщает, задан ли токен бота.
	Configured() bool
	// Verify проверяет подпись и возвращает данные пользователя.
	Verify(raw string) (*InitData, error)
}
