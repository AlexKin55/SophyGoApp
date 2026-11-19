package events

import (
	"errors"
	"testing"
	"time"
)

func validRequest() PlaceOpenRequest {
	return PlaceOpenRequest{
		Place: Place{
			ID:        "way/123",
			Name:      "Московский Кремль",
			Category:  "historic_building",
			Latitude:  55.752023,
			Longitude: 37.617499,
		},
		Client: Client{
			SessionID: "session-1",
			Platform:  "browser",
			UserAgent: "Mozilla/5.0",
			Language:  "ru-RU",
		},
	}
}

func TestValidateAcceptsValidRequest(t *testing.T) {
	now := time.Now().UTC()
	request := validRequest()

	if err := request.Validate(now); err != nil {
		t.Fatalf("ожидали отсутствие ошибки, получили %v", err)
	}
}

func TestValidateTrimsStrings(t *testing.T) {
	now := time.Now().UTC()
	request := validRequest()
	request.Place.ID = "  way/123  "
	request.Place.Name = "  Кремль  "
	request.Client.SessionID = "  session-1  "

	if err := request.Validate(now); err != nil {
		t.Fatalf("ожидали отсутствие ошибки, получили %v", err)
	}

	if request.Place.ID != "way/123" {
		t.Errorf("place.id = %q, ожидали %q", request.Place.ID, "way/123")
	}
	if request.Place.Name != "Кремль" {
		t.Errorf("place.name = %q, ожидали %q", request.Place.Name, "Кремль")
	}
	if request.Client.SessionID != "session-1" {
		t.Errorf("client.sessionId = %q, ожидали %q", request.Client.SessionID, "session-1")
	}
}

func TestValidateRejectsInvalidRequests(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name   string
		mutate func(*PlaceOpenRequest)
		field  string
	}{
		{
			name:   "пустой id",
			mutate: func(r *PlaceOpenRequest) { r.Place.ID = "   " },
			field:  "place.id",
		},
		{
			name:   "пустое имя",
			mutate: func(r *PlaceOpenRequest) { r.Place.Name = "" },
			field:  "place.name",
		},
		{
			name:   "широта вне диапазона",
			mutate: func(r *PlaceOpenRequest) { r.Place.Latitude = 91 },
			field:  "place.latitude",
		},
		{
			name:   "долгота вне диапазона",
			mutate: func(r *PlaceOpenRequest) { r.Place.Longitude = -181 },
			field:  "place.longitude",
		},
		{
			name:   "слишком длинное имя",
			mutate: func(r *PlaceOpenRequest) { r.Place.Name = string(make([]byte, maxNameLen+1)) },
			field:  "place.name",
		},
		{
			name: "время события в будущем",
			mutate: func(r *PlaceOpenRequest) {
				future := now.Add(48 * time.Hour)
				r.OccurredAt = &future
			},
			field: "occurredAt",
		},
		{
			name: "время события слишком давно",
			mutate: func(r *PlaceOpenRequest) {
				past := now.Add(-31 * 24 * time.Hour)
				r.OccurredAt = &past
			},
			field: "occurredAt",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest()
			test.mutate(&request)

			err := request.Validate(now)
			if err == nil {
				t.Fatal("ожидали ошибку валидации, получили nil")
			}

			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("ожидали *ValidationError, получили %T", err)
			}
			if validationErr.Field != test.field {
				t.Errorf("поле ошибки = %q, ожидали %q", validationErr.Field, test.field)
			}
		})
	}
}

func TestNewEventUsesServerTimeByDefault(t *testing.T) {
	now := time.Now().UTC()
	request := validRequest()

	event := NewEvent(request, now, "203.0.113.10")

	if event.ID == "" {
		t.Error("id события не должен быть пустым")
	}
	if event.Type != TypePlaceOpen {
		t.Errorf("type = %q, ожидали %q", event.Type, TypePlaceOpen)
	}
	if !event.OccurredAt.Equal(now) {
		t.Errorf("occurredAt = %v, ожидали %v", event.OccurredAt, now)
	}
	if !event.ReceivedAt.Equal(now) {
		t.Errorf("receivedAt = %v, ожидали %v", event.ReceivedAt, now)
	}
	if event.RemoteIP != "203.0.113.10" {
		t.Errorf("remoteIp = %q, ожидали %q", event.RemoteIP, "203.0.113.10")
	}
}

func TestNewEventKeepsClientOccurredAt(t *testing.T) {
	now := time.Now().UTC()
	occurredAt := now.Add(-time.Minute)
	request := validRequest()
	request.OccurredAt = &occurredAt

	event := NewEvent(request, now, "")

	if !event.OccurredAt.Equal(occurredAt) {
		t.Errorf("occurredAt = %v, ожидали %v", event.OccurredAt, occurredAt)
	}
}

func TestNewIDIsUnique(t *testing.T) {
	first := NewID()
	second := NewID()

	if first == second {
		t.Fatalf("идентификаторы событий совпали: %q", first)
	}
}
