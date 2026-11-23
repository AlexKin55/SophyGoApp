package max

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/sophygo/backend/internal/messenger"
)

const testBotToken = "max-test-token"

const testLaunchParams = "auth_date=1700000000\n" +
	"chat={\"id\":12345,\"type\":\"DIALOG\"}\n" +
	"ip=192.168.0.1\n" +
	"query_id=abc\n" +
	"user={\"id\":67890,\"first_name\":\"Max\",\"last_name\":\"User\",\"username\":\"maxuser\",\"language_code\":\"ru\"}"

// Эталон посчитан независимо по документации MAX:
// hex(HMAC_SHA256(HMAC_SHA256("WebAppData", botToken), launchParams)).
const testHash = "42d55aa8e2b097b3e56b6e47088e9071e395009faa5ff7c2294c9f14ea0d572e"

func TestSignMatchesMaxAlgorithm(t *testing.T) {
	if got := sign(testBotToken, testLaunchParams); got != testHash {
		t.Fatalf("sign() = %q, ожидали %q", got, testHash)
	}
}

func validInitData(t *testing.T) string {
	t.Helper()

	values := url.Values{}
	values.Set("chat", `{"id":12345,"type":"DIALOG"}`)
	values.Set("ip", "192.168.0.1")
	values.Set(
		"user",
		`{"id":67890,"first_name":"Max","last_name":"User","username":"maxuser","language_code":"ru"}`,
	)
	values.Set("query_id", "abc")
	values.Set("auth_date", "1700000000")
	values.Set("hash", testHash)

	return values.Encode()
}

func TestVerifyValidInitData(t *testing.T) {
	verifier := NewVerifier(testBotToken, time.Hour)
	verifier.now = func() time.Time { return time.Unix(1700000000+60, 0).UTC() }

	data, err := verifier.Verify(validInitData(t))
	if err != nil {
		t.Fatalf("Verify() вернул ошибку: %v", err)
	}

	if data.User.ID != 67890 {
		t.Errorf("User.ID = %d, ожидали 67890", data.User.ID)
	}
	if data.User.FirstName != "Max" {
		t.Errorf("User.FirstName = %q, ожидали Max", data.User.FirstName)
	}
	if data.User.Username != "maxuser" {
		t.Errorf("User.Username = %q, ожидали maxuser", data.User.Username)
	}
	if data.QueryID != "abc" {
		t.Errorf("QueryID = %q, ожидали abc", data.QueryID)
	}
	if want := time.Unix(1700000000, 0).UTC(); !data.AuthDate.Equal(want) {
		t.Errorf("AuthDate = %v, ожидали %v", data.AuthDate, want)
	}
}

func TestVerifyRejectsInvalidSignature(t *testing.T) {
	verifier := NewVerifier(testBotToken, time.Hour)
	verifier.now = func() time.Time { return time.Unix(1700000000+60, 0).UTC() }

	raw := validInitData(t)
	raw = raw[:len(raw)-1] + "0"

	if _, err := verifier.Verify(raw); !errors.Is(err, messenger.ErrInvalidSignature) {
		t.Fatalf("Verify() = %v, ожидали ErrInvalidSignature", err)
	}
}

func TestVerifyRejectsExpiredInitData(t *testing.T) {
	verifier := NewVerifier(testBotToken, time.Hour)
	verifier.now = func() time.Time { return time.Unix(1700000000+2*3600, 0).UTC() }

	if _, err := verifier.Verify(validInitData(t)); !errors.Is(err, messenger.ErrExpired) {
		t.Fatalf("Verify() = %v, ожидали ErrExpired", err)
	}
}

func TestVerifyRejectsDuplicateHash(t *testing.T) {
	verifier := NewVerifier(testBotToken, time.Hour)
	verifier.now = func() time.Time { return time.Unix(1700000000+60, 0).UTC() }

	raw := validInitData(t) + "&hash=" + testHash
	if _, err := verifier.Verify(raw); !errors.Is(err, messenger.ErrMalformed) {
		t.Fatalf("Verify() = %v, ожидали ErrMalformed", err)
	}
}

func TestVerifyRequiresConfiguration(t *testing.T) {
	verifier := NewVerifier("", time.Hour)
	if verifier.Configured() {
		t.Fatal("Configured() = true для пустого токена")
	}
	if _, err := verifier.Verify(validInitData(t)); !errors.Is(err, messenger.ErrNotConfigured) {
		t.Fatalf("Verify() = %v, ожидали ErrNotConfigured", err)
	}
}
