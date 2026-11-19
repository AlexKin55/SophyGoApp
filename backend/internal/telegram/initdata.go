// Package telegram проверяет данные Telegram Mini App (initData).
package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrNotConfigured — сервер не знает токен бота и не может проверить подпись.
	ErrNotConfigured = errors.New("telegram: TELEGRAM_BOT_TOKEN не задан")
	// ErrMissing — initData не передан.
	ErrMissing = errors.New("telegram: initData не передан")
	// ErrMalformed — initData имеет неверный формат.
	ErrMalformed = errors.New("telegram: некорректный initData")
	// ErrInvalidSignature — подпись не совпадает.
	ErrInvalidSignature = errors.New("telegram: неверная подпись initData")
	// ErrExpired — initData устарел.
	ErrExpired = errors.New("telegram: initData устарел")
)

// User — данные пользователя из initData.
type User struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Username     string `json:"username"`
	LanguageCode string `json:"language_code"`
	IsPremium    bool   `json:"is_premium"`
}

// InitData — проверенные данные Telegram Mini App.
type InitData struct {
	User     User
	AuthDate time.Time
	QueryID  string
}

// Verifier проверяет подпись initData.
type Verifier struct {
	botToken string
	maxAge   time.Duration
	now      func() time.Time
}

// NewVerifier создаёт верификатор. Пустой botToken означает, что проверка недоступна.
func NewVerifier(botToken string, maxAge time.Duration) *Verifier {
	if maxAge <= 0 {
		maxAge = 24 * time.Hour
	}
	return &Verifier{
		botToken: strings.TrimSpace(botToken),
		maxAge:   maxAge,
		now:      time.Now,
	}
}

// Configured сообщает, можно ли проверять initData.
func (v *Verifier) Configured() bool {
	return v.botToken != ""
}

// Verify проверяет подпись и возвращает данные пользователя.
func (v *Verifier) Verify(raw string) (*InitData, error) {
	if !v.Configured() {
		return nil, ErrNotConfigured
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrMissing
	}

	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}

	hash := values.Get("hash")
	if hash == "" {
		return nil, fmt.Errorf("%w: отсутствует hash", ErrMalformed)
	}

	dataCheckString := buildDataCheckString(values)
	expected := sign(v.botToken, dataCheckString)

	if !hmac.Equal([]byte(expected), []byte(strings.ToLower(hash))) {
		return nil, ErrInvalidSignature
	}

	authDateRaw := values.Get("auth_date")
	if authDateRaw == "" {
		return nil, fmt.Errorf("%w: отсутствует auth_date", ErrMalformed)
	}

	authUnix, err := strconv.ParseInt(authDateRaw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: auth_date не число", ErrMalformed)
	}
	authDate := time.Unix(authUnix, 0).UTC()
	now := v.now().UTC()

	if authDate.After(now.Add(5 * time.Minute)) {
		return nil, fmt.Errorf("%w: auth_date в будущем", ErrMalformed)
	}
	if now.Sub(authDate) > v.maxAge {
		return nil, ErrExpired
	}

	userRaw := values.Get("user")
	if userRaw == "" {
		return nil, fmt.Errorf("%w: отсутствует user", ErrMalformed)
	}

	var user User
	if err := json.Unmarshal([]byte(userRaw), &user); err != nil {
		return nil, fmt.Errorf("%w: user не JSON", ErrMalformed)
	}
	if user.ID <= 0 {
		return nil, fmt.Errorf("%w: некорректный user.id", ErrMalformed)
	}

	return &InitData{
		User:     user,
		AuthDate: authDate,
		QueryID:  values.Get("query_id"),
	}, nil
}

func buildDataCheckString(values url.Values) string {
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

// sign считает подпись по алгоритму Telegram:
// secret = HMAC_SHA256(key=botToken, message="WebAppData"),
// hash = HMAC_SHA256(key=secret, message=dataCheckString).
func sign(botToken, dataCheckString string) string {
	secretMAC := hmac.New(sha256.New, []byte(botToken))
	secretMAC.Write([]byte("WebAppData"))
	secretKey := secretMAC.Sum(nil)

	dataMAC := hmac.New(sha256.New, secretKey)
	dataMAC.Write([]byte(dataCheckString))
	return hex.EncodeToString(dataMAC.Sum(nil))
}
