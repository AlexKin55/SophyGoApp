// Package max проверяет данные мини-приложения MAX (WebAppData/initData).
//
// Алгоритм из документации MAX: значения initData декодируются,
// сортируются по ключу и подписываются так же, как в Telegram:
// secret = HMAC_SHA256(key="WebAppData", message=botToken),
// hash = hex(HMAC_SHA256(secret, launchParams)).
package max

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sophygo/backend/internal/messenger"
)

// Verifier проверяет подпись initData MAX.
type Verifier struct {
	botToken string
	maxAge   time.Duration
	now      func() time.Time
}

// NewVerifier создаёт верификатор. Пустой botToken означает, что проверка недоступна.
func NewVerifier(botToken string, maxAge time.Duration) *Verifier {
	if maxAge <= 0 {
		maxAge = time.Hour
	}
	return &Verifier{
		botToken: strings.TrimSpace(botToken),
		maxAge:   maxAge,
		now:      time.Now,
	}
}

// Name возвращает имя мессенджера.
func (v *Verifier) Name() string {
	return "max"
}

// Configured сообщает, можно ли проверять initData.
func (v *Verifier) Configured() bool {
	return v.botToken != ""
}

type paramPair struct {
	key   string
	value string
}

// Verify проверяет подпись и возвращает данные пользователя.
func (v *Verifier) Verify(raw string) (*messenger.InitData, error) {
	if !v.Configured() {
		return nil, messenger.ErrNotConfigured
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, messenger.ErrMissing
	}

	params, hash, userRaw, err := parseParams(raw)
	if err != nil {
		return nil, err
	}

	launchParams := buildLaunchParams(params)
	expected := sign(v.botToken, launchParams)
	if !hmac.Equal([]byte(expected), []byte(hash)) {
		return nil, messenger.ErrInvalidSignature
	}

	authDateRaw := findValue(params, "auth_date")
	if authDateRaw == "" {
		return nil, fmt.Errorf("%w: отсутствует auth_date", messenger.ErrMalformed)
	}

	authUnix, err := strconv.ParseInt(authDateRaw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: auth_date не число", messenger.ErrMalformed)
	}
	authDate := time.Unix(authUnix, 0).UTC()
	now := v.now().UTC()

	if authDate.After(now.Add(5 * time.Minute)) {
		return nil, fmt.Errorf("%w: auth_date в будущем", messenger.ErrMalformed)
	}
	if now.Sub(authDate) > v.maxAge {
		return nil, messenger.ErrExpired
	}

	if userRaw == "" {
		return nil, fmt.Errorf("%w: отсутствует user", messenger.ErrMalformed)
	}

	var user struct {
		ID           int64  `json:"id"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		Username     string `json:"username"`
		LanguageCode string `json:"language_code"`
	}
	if err := json.Unmarshal([]byte(userRaw), &user); err != nil {
		return nil, fmt.Errorf("%w: user не JSON", messenger.ErrMalformed)
	}
	if user.ID <= 0 {
		return nil, fmt.Errorf("%w: некорректный user.id", messenger.ErrMalformed)
	}

	return &messenger.InitData{
		User: messenger.User{
			ID:           user.ID,
			FirstName:    user.FirstName,
			LastName:     user.LastName,
			Username:     user.Username,
			LanguageCode: user.LanguageCode,
		},
		AuthDate: authDate,
		QueryID:  findValue(params, "query_id"),
	}, nil
}

// parseParams разбирает initData: декодирует значения, проверяет
// уникальность hash и запоминает JSON пользователя.
func parseParams(raw string) ([]paramPair, string, string, error) {
	pairs := strings.Split(raw, "&")
	params := make([]paramPair, 0, len(pairs))
	hash := ""
	userRaw := ""

	for _, pair := range pairs {
		if pair == "" {
			continue
		}

		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, "", "", fmt.Errorf("%w: параметр без значения", messenger.ErrMalformed)
		}

		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return nil, "", "", fmt.Errorf("%w: не удалось декодировать значение", messenger.ErrMalformed)
		}

		if key == "hash" {
			if hash != "" {
				return nil, "", "", fmt.Errorf("%w: hash встречается несколько раз", messenger.ErrMalformed)
			}
			hash = strings.ToLower(decoded)
			continue
		}

		if key == "user" {
			userRaw = decoded
		}

		params = append(params, paramPair{key: key, value: decoded})
	}

	if hash == "" {
		return nil, "", "", fmt.Errorf("%w: отсутствует hash", messenger.ErrMalformed)
	}

	return params, hash, userRaw, nil
}

func buildLaunchParams(params []paramPair) string {
	sorted := make([]paramPair, len(params))
	copy(sorted, params)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].key < sorted[j].key
	})

	var builder strings.Builder
	for i, pair := range sorted {
		if i > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(pair.key)
		builder.WriteByte('=')
		builder.WriteString(pair.value)
	}
	return builder.String()
}

func findValue(params []paramPair, key string) string {
	for _, pair := range params {
		if pair.key == key {
			return pair.value
		}
	}
	return ""
}

// sign считает подпись по алгоритму MAX (совпадает с Telegram):
// secret = HMAC_SHA256(key="WebAppData", message=botToken),
// hash = HMAC_SHA256(key=secret, message=launchParams).
func sign(botToken, launchParams string) string {
	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	secretMAC.Write([]byte(botToken))
	secretKey := secretMAC.Sum(nil)

	dataMAC := hmac.New(sha256.New, secretKey)
	dataMAC.Write([]byte(launchParams))
	return hex.EncodeToString(dataMAC.Sum(nil))
}
