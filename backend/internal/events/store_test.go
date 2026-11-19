package events

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testEvent(id string) Event {
	now := time.Now().UTC()
	return Event{
		ID:         id,
		Type:       TypePlaceOpen,
		Place:      Place{ID: "way/1", Name: "Кремль", Latitude: 55.75, Longitude: 37.61},
		Client:     Client{SessionID: "s1", Platform: "browser"},
		OccurredAt: now,
		ReceivedAt: now,
	}
}

func TestStoreAppendsAndReturnsRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "events.jsonl")
	store, err := Open(path, 10)
	if err != nil {
		t.Fatalf("не удалось открыть хранилище: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	for _, id := range []string{"evt_1", "evt_2", "evt_3"} {
		if err := store.Append(ctx, testEvent(id)); err != nil {
			t.Fatalf("не удалось сохранить событие %q: %v", id, err)
		}
	}

	recent := store.Recent(10)
	if len(recent) != 3 {
		t.Fatalf("получили %d событий, ожидали 3", len(recent))
	}
	if recent[0].ID != "evt_3" || recent[2].ID != "evt_1" {
		t.Errorf("порядок событий неверный: %v", []string{recent[0].ID, recent[1].ID, recent[2].ID})
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("не удалось открыть файл событий: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		lines++
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("строка %d не является JSON: %v", lines, err)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("ошибка чтения файла: %v", err)
	}
	if lines != 3 {
		t.Errorf("в файле %d строк, ожидали 3", lines)
	}
}

func TestStoreLimitsRecentEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	store, err := Open(path, 2)
	if err != nil {
		t.Fatalf("не удалось открыть хранилище: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	for _, id := range []string{"evt_1", "evt_2", "evt_3"} {
		if err := store.Append(ctx, testEvent(id)); err != nil {
			t.Fatalf("не удалось сохранить событие %q: %v", id, err)
		}
	}

	recent := store.Recent(10)
	if len(recent) != 2 {
		t.Fatalf("получили %d событий, ожидали 2", len(recent))
	}
	if recent[0].ID != "evt_3" || recent[1].ID != "evt_2" {
		t.Errorf("неверные последние события: %q, %q", recent[0].ID, recent[1].ID)
	}
}

func TestStoreRecentRespectsLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	store, err := Open(path, 10)
	if err != nil {
		t.Fatalf("не удалось открыть хранилище: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	for _, id := range []string{"evt_1", "evt_2", "evt_3"} {
		if err := store.Append(ctx, testEvent(id)); err != nil {
			t.Fatalf("не удалось сохранить событие %q: %v", id, err)
		}
	}

	recent := store.Recent(1)
	if len(recent) != 1 || recent[0].ID != "evt_3" {
		t.Fatalf("ожидали только evt_3, получили %+v", recent)
	}
}

func TestStoreRejectsInvalidCapacity(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "events.jsonl"), 0); err == nil {
		t.Fatal("ожидали ошибку при нулевой ёмкости буфера")
	}
}

func TestStoreAppendHonorsCancelledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	store, err := Open(path, 10)
	if err != nil {
		t.Fatalf("не удалось открыть хранилище: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.Append(ctx, testEvent("evt_1")); err == nil {
		t.Fatal("ожидали ошибку при отменённом контексте")
	}
}
