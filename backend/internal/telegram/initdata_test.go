package telegram

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"testing"
	"time"
)

const testBotToken = "123456:test-token"

func buildTestInitData(t *testing.T, botToken string, authDate time.Time, user User) string {
	t.Helper()

	rawUser, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("не удалось сериализовать user: %v", err)
	}

	values := url.Values{}
	values.Set("query_id", "AAHdF6IQAAAAAN0XohDhrOrc")
	values.Set("user", string(rawUser))
	values.Set("auth_date", strconv.FormatInt(authDate.Unix(), 10))
	values.Set("hash", sign(botToken, buildDataCheckString(values)))

	return values.Encode()
}

func testUser() User {
	return User{
		ID:           279058397,
		FirstName:    "Владислав",
		Username:     "vlad",
		LanguageCode: "ru",
	}
}

func TestSignMatchesTelegramAlgorithm(t *testing.T) {
	const token = "123456:test-token"
	const dataCheckString = "auth_date=1700000000\nquery_id=AAHdF6IQAAAAAN0XohDhrOrc\nuser={\"id\":279058397,\"first_name\":\"Vlad\",\"username\":\"vlad\",\"language_code\":\"ru\"}"
	const expected = "364101eb82d4dbe65aa4cbeee792c21ac0abee895dfddc6cbfc1d6ea70daaa4d"

	if got := sign(token, dataCheckString); got != expected {
		t.Fatalf("sign() = %q, ожидали %q", got, expected)
	}
}

func TestVerifyValidInitDataWithSignatureField(t *testing.T) {
	now := time.Now().UTC()
	verifier := NewVerifier(testBotToken, 24*time.Hour)
	verifier.now = func() time.Time { return now }

	rawUser, err := json.Marshal(testUser())
	if err != nil {
		t.Fatalf("не удалось сериализовать user: %v", err)
	}

	values := url.Values{}
	values.Set("query_id", "AAHdF6IQAAAAAN0XohDhrOrc")
	values.Set("user", string(rawUser))
	values.Set("auth_date", strconv.FormatInt(now.Add(-time.Minute).Unix(), 10))
	values.Set("signature", "base64url-signature")
	// Telegram вычисляет hash до добавления поля signature.
	values.Set("hash", sign(testBotToken, buildDataCheckString(values, true)))

	initData, err := verifier.Verify(values.Encode())
	if err != nil {
		t.Fatalf("ожидали успешную проверку с полем signature, получили %v", err)
	}
	if initData.User.ID != 279058397 {
		t.Errorf("user.id = %d, ожидали 279058397", initData.User.ID)
	}
}

func TestVerifyValidInitData(t *testing.T) {
	now := time.Now().UTC()
	verifier := NewVerifier(testBotToken, 24*time.Hour)
	verifier.now = func() time.Time { return now }

	raw := buildTestInitData(t, testBotToken, now.Add(-time.Minute), testUser())

	initData, err := verifier.Verify(raw)
	if err != nil {
		t.Fatalf("ожидали успешную проверку, получили %v", err)
	}
	if initData.User.ID != 279058397 {
		t.Errorf("user.id = %d, ожидали 279058397", initData.User.ID)
	}
	if initData.User.Username != "vlad" {
		t.Errorf("user.username = %q, ожидали %q", initData.User.Username, "vlad")
	}
	if initData.QueryID == "" {
		t.Error("query_id не должен быть пустым")
	}
}

func TestVerifyRejectsInvalidSignature(t *testing.T) {
	now := time.Now().UTC()
	verifier := NewVerifier(testBotToken, 24*time.Hour)
	verifier.now = func() time.Time { return now }

	raw := buildTestInitData(t, "999:other-token", now, testUser())

	if _, err := verifier.Verify(raw); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("ожидали ErrInvalidSignature, получили %v", err)
	}
}

func TestVerifyRejectsTamperedData(t *testing.T) {
	now := time.Now().UTC()
	verifier := NewVerifier(testBotToken, 24*time.Hour)
	verifier.now = func() time.Time { return now }

	raw := buildTestInitData(t, testBotToken, now, testUser())

	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("не удалось разобрать initData: %v", err)
	}
	values.Set("user", `{"id":1,"first_name":"Подмена"}`)

	if _, err := verifier.Verify(values.Encode()); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("ожидали ErrInvalidSignature, получили %v", err)
	}
}

func TestVerifyRejectsExpiredData(t *testing.T) {
	now := time.Now().UTC()
	verifier := NewVerifier(testBotToken, 24*time.Hour)
	verifier.now = func() time.Time { return now }

	raw := buildTestInitData(t, testBotToken, now.Add(-25*time.Hour), testUser())

	if _, err := verifier.Verify(raw); !errors.Is(err, ErrExpired) {
		t.Fatalf("ожидали ErrExpired, получили %v", err)
	}
}

func TestVerifyRejectsFutureAuthDate(t *testing.T) {
	now := time.Now().UTC()
	verifier := NewVerifier(testBotToken, 24*time.Hour)
	verifier.now = func() time.Time { return now }

	raw := buildTestInitData(t, testBotToken, now.Add(10*time.Minute), testUser())

	if _, err := verifier.Verify(raw); !errors.Is(err, ErrMalformed) {
		t.Fatalf("ожидали ErrMalformed, получили %v", err)
	}
}

func TestVerifyRejectsMalformedData(t *testing.T) {
	verifier := NewVerifier(testBotToken, 24*time.Hour)

	if _, err := verifier.Verify("auth_date=1"); !errors.Is(err, ErrMalformed) {
		t.Fatalf("ожидали ErrMalformed, получили %v", err)
	}
	if _, err := verifier.Verify(""); !errors.Is(err, ErrMissing) {
		t.Fatalf("ожидали ErrMissing, получили %v", err)
	}
}

func TestVerifyRequiresConfiguration(t *testing.T) {
	verifier := NewVerifier("", 24*time.Hour)

	if _, err := verifier.Verify("hash=abc"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ожидали ErrNotConfigured, получили %v", err)
	}
}
